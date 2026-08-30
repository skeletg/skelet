package cart

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A FILE option lets a customer put a file into the order, so the allow-list is the only thing
// standing between "upload your artwork" and "upload anything" - and it is authored as free text in
// the dashboard, so the parser has to be forgiving about how it is written and strict about what it
// then admits

func fileOption(exts string, maxBytes *int64) ProductOptionConfig {
	return ProductOptionConfig{ID: uuid.New(), InputType: "file", FileExtensions: exts, MaxFileBytes: maxBytes}
}

func bytes(n int64) *int64 { return &n }

func TestParseExtensionAllowListNormalisesHowItIsWritten(t *testing.T) {
	got := parseExtensionAllowList("jpg, PNG, .pdf ,,  .SVG")
	want := []string{"jpg", "png", "pdf", "svg"}
	if len(got) != len(want) {
		t.Fatalf("parsed %v, want %d entries", got, len(want))
	}
	for _, e := range want {
		if !got[e] {
			t.Errorf("%q missing from %v - case, dots and padding should all be normalised away", e, got)
		}
	}
}

func TestParseExtensionAllowListTreatsBlankAsNoConstraint(t *testing.T) {
	// nil is the "no constraint" signal validateFilePick checks with len() - an empty non-nil map
	// would read the same, but an entry-less list must never be mistaken for "allow nothing"
	for _, raw := range []string{"", "   ", ",", " , , "} {
		if got := parseExtensionAllowList(raw); len(got) != 0 {
			t.Errorf("parseExtensionAllowList(%q) = %v, want no constraint", raw, got)
		}
	}
}

func TestFileExtensionReadsTheLastSuffix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"art.PNG", "png"},
		{"my.file.name.pdf", "pdf"},
		{"no-extension", ""},
		{"trailing.", ""},
		{"", ""},
		{".hidden", "hidden"},
	}
	for _, c := range cases {
		if got := fileExtension(c.in); got != c.want {
			t.Errorf("fileExtension(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidateFilePickAcceptsAnAllowedType(t *testing.T) {
	if err := validateFilePick(fileOption("jpg,png", nil), FileAsset{Filename: "art.PNG", SizeBytes: 10}); err != nil {
		t.Errorf("an allowed extension in the wrong case was rejected: %v", err)
	}
}

func TestValidateFilePickRefusesATypeOutsideTheList(t *testing.T) {
	err := validateFilePick(fileOption("jpg,png", nil), FileAsset{Filename: "payload.svg", SizeBytes: 10})
	if !errors.Is(err, ErrProductOptionInvalid) {
		t.Errorf("err = %v, want ErrProductOptionInvalid for a type outside the list", err)
	}
}

func TestValidateFilePickRefusesAFileWithNoExtensionAtAll(t *testing.T) {
	// otherwise `payload` sails past a list that names only images
	err := validateFilePick(fileOption("jpg", nil), FileAsset{Filename: "payload", SizeBytes: 10})
	if !errors.Is(err, ErrProductOptionInvalid) {
		t.Errorf("err = %v, want ErrProductOptionInvalid for an extension-less filename", err)
	}
	// and a double extension is judged on the last one, which is what a server would execute
	if err := validateFilePick(fileOption("jpg", nil), FileAsset{Filename: "art.jpg.php"}); !errors.Is(err, ErrProductOptionInvalid) {
		t.Errorf("err = %v, want the LAST suffix to decide", err)
	}
}

func TestValidateFilePickAllowsAnythingWhenNoListIsSet(t *testing.T) {
	if err := validateFilePick(fileOption("", nil), FileAsset{Filename: "whatever.xyz", SizeBytes: 10}); err != nil {
		t.Errorf("an unconstrained option rejected a file: %v", err)
	}
}

func TestValidateFilePickEnforcesTheSizeCap(t *testing.T) {
	opt := fileOption("", bytes(100))
	if err := validateFilePick(opt, FileAsset{Filename: "a.jpg", SizeBytes: 100}); err != nil {
		t.Errorf("a file exactly at the cap was rejected: %v", err)
	}
	if err := validateFilePick(opt, FileAsset{Filename: "a.jpg", SizeBytes: 101}); !errors.Is(err, ErrProductOptionInvalid) {
		t.Errorf("err = %v, want ErrProductOptionInvalid one byte over the cap", err)
	}
}

func TestValidateFilePickTreatsAnAbsentOrZeroCapAsUncapped(t *testing.T) {
	for _, cap := range []*int64{nil, bytes(0)} {
		if err := validateFilePick(fileOption("", cap), FileAsset{Filename: "a.jpg", SizeBytes: 1 << 30}); err != nil {
			t.Errorf("cap %v rejected a large file, want uncapped: %v", cap, err)
		}
	}
}

func TestValidateFilePickChecksSizeBeforeType(t *testing.T) {
	// both wrong - the size message is the one that tells the customer something actionable
	err := validateFilePick(fileOption("jpg", bytes(10)), FileAsset{Filename: "big.svg", SizeBytes: 999})
	if !errors.Is(err, ErrProductOptionInvalid) {
		t.Fatalf("err = %v, want ErrProductOptionInvalid", err)
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("err = %q, want the size failure reported first", err)
	}
}

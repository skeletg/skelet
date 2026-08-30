package medialibrary

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Directory paths come off the admin file browser as free text, so the sanitizer is the only thing
// between a pasted path and the object keys it would otherwise reach
func TestSanitizeDirectoryPathNeutralizesTraversal(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"/", ""},
		{"///", ""},
		{"catalog", "catalog"},
		{"/catalog/banners/", "catalog/banners"},
		{"  catalog/banners  ", "catalog/banners"},
		{"catalog//banners", "catalog/banners"},
		{`catalog\banners`, "catalog-banners"},    // splitting happens on / first, so a windows path collapses into one directory
		{"../../etc", "etc"},                      // the traversal segments drop out entirely
		{"catalog/../../../root", "catalog/root"}, //
		{"./catalog", "catalog"},
		{"cat a log/ban ners", "cat-a-log/ban-ners"}, // spaces collapse to the key-safe separator
		{"CATALOG/Banners", "CATALOG/Banners"},       // directory case is preserved, unlike asset keys
		{"..", ""},
		{"....", ""},
	} {
		got, err := sanitizeDirectoryPath(tc.in)
		if err != nil {
			t.Errorf("sanitizeDirectoryPath(%q) = %v, want %q", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("sanitizeDirectoryPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.Contains(got, "..") {
			t.Errorf("sanitizeDirectoryPath(%q) = %q, which still carries a traversal segment", tc.in, got)
		}
	}
}

// path.Base drops any directory the client tried to smuggle in, so a filename can never steer the key
func TestSanitizeFilenameStripsDirectoriesAndRequiresAnExtension(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"photo.jpg", "photo.jpg"},
		{"My Photo.JPG", "My-Photo.jpg"}, // only the extension is lowercased, and it must not be appended twice
		{"IMG_1234.JPG", "IMG_1234.jpg"}, // straight off a phone, which is where the doubled extension showed up
		{"Scan.PDF", "Scan.pdf"},
		{"clip.MOV", "clip.mov"},
		{"shot.PnG", "shot.png"},
		{"archive.tar.GZ", "archive.tar.gz"}, // only the last segment is an extension
		{"../../etc/passwd.png", "passwd.png"},
		{`C:\Users\admin\shot.png`, "shot.png"},
		{"  spaced name .pdf", "spaced-name.pdf"},
		{"...hidden.png", "hidden.png"},
		{"a/b/c/deep.webp", "deep.webp"},
	} {
		got, err := sanitizeFilename(tc.in)
		if err != nil {
			t.Errorf("sanitizeFilename(%q) = %v, want %q", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("sanitizeFilename(%q) = %q, which still carries a path separator", tc.in, got)
		}
	}

	for _, in := range []string{"", "   ", ".", "..", "/", "noextension", ".gitignore", "....."} {
		if got, err := sanitizeFilename(in); !errors.Is(err, ErrInvalidFilename) {
			t.Errorf("sanitizeFilename(%q) = (%q, %v), want ErrInvalidFilename", in, got, err)
		}
	}
}

func TestNewServiceNormalizesTheRootPrefix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"media", "media"},
		{"/media/", "media"},
		{"  /media/library/  ", "media/library"},
	} {
		if got := NewService(nil, nil, tc.in).rootPrefix; got != tc.want {
			t.Errorf("NewService rootPrefix from %q = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The prefix is what every list, delete and rename call scopes itself to, so a missing or doubled
// slash silently widens or breaks the whole directory operation
func TestObjectPrefixAndMarkerKey(t *testing.T) {
	rooted := &Service{rootPrefix: "media"}
	bare := &Service{rootPrefix: ""}

	for _, tc := range []struct {
		svc  *Service
		dir  string
		want string
	}{
		{rooted, "", "media/"},
		{rooted, "catalog", "media/catalog/"},
		{rooted, "catalog/banners", "media/catalog/banners/"},
		{bare, "", ""},
		{bare, "catalog", "catalog/"},
	} {
		if got := tc.svc.objectPrefix(tc.dir); got != tc.want {
			t.Errorf("objectPrefix(root=%q, %q) = %q, want %q", tc.svc.rootPrefix, tc.dir, got, tc.want)
		}
	}

	if got, want := rooted.markerKey("catalog"), "media/catalog/.keep"; got != want {
		t.Errorf("markerKey = %q, want %q", got, want)
	}
	if got, want := bare.markerKey(""), ".keep"; got != want {
		t.Errorf("markerKey at the bare root = %q, want %q", got, want)
	}
}

// The key's extension comes from the validated MIME type, never from the filename - an "evil.php"
// upload declared as image/png must land as a .png object
func TestBuildObjectKeyTakesItsExtensionFromTheMimeType(t *testing.T) {
	s := &Service{rootPrefix: "media"}
	id := uuid.MustParse("018f0000-0000-7000-8000-000000000001")

	for _, tc := range []struct {
		dir, filename, mime, want string
	}{
		{"", "Photo Of Me.JPEG", "image/jpeg", "media/" + id.String() + "-photo-of-me.jpg"},
		{"catalog", "banner.png", "image/png", "media/catalog/" + id.String() + "-banner.png"},
		{"catalog", "evil.php", "image/png", "media/catalog/" + id.String() + "-evil.png"},
		{"docs", "Terms & Conditions.pdf", "application/pdf", "media/docs/" + id.String() + "-terms-conditions.pdf"},
		{"", "clip.mov", "video/quicktime", "media/" + id.String() + "-clip.mov"},
		{"", "----.png", "image/png", "media/" + id.String() + "-asset.png"}, // nothing survives sanitizing
		{"", "no-ext", "image/webp", "media/" + id.String() + "-no-ext.webp"},
	} {
		got := s.buildObjectKey(tc.dir, id, tc.filename, tc.mime)
		if got != tc.want {
			t.Errorf("buildObjectKey(%q, %q, %q) = %q, want %q", tc.dir, tc.filename, tc.mime, got, tc.want)
		}
	}
}

// RequestUpload runs the filename through the sanitizer and then into the key, so the two together
// are what an admin actually sees - a doubled ".JPG.jpg" showed up here first
func TestSanitizedFilenameFeedsASingleExtensionIntoTheKey(t *testing.T) {
	s := &Service{rootPrefix: "media"}
	for _, tc := range []struct{ raw, mime, wantSuffix string }{
		{"IMG_1234.JPG", "image/jpeg", "-img_1234.jpg"},
		{"Holiday Snap.JPEG", "image/jpeg", "-holiday-snap.jpg"},
		{"Terms.PDF", "application/pdf", "-terms.pdf"},
		{"clip.MOV", "video/quicktime", "-clip.mov"},
	} {
		filename, err := sanitizeFilename(tc.raw)
		if err != nil {
			t.Fatalf("sanitizeFilename(%q): %v", tc.raw, err)
		}
		key := s.buildObjectKey("", uuid.Nil, filename, tc.mime)
		if !strings.HasSuffix(key, tc.wantSuffix) {
			t.Errorf("%q -> filename %q -> key %q, want the key to end in %q", tc.raw, filename, key, tc.wantSuffix)
		}
	}
}

// Two uploads of the same filename must not collide - the per-asset uuid is what keeps them apart
func TestBuildObjectKeyIsUniquePerAsset(t *testing.T) {
	s := &Service{rootPrefix: "media"}
	a := s.buildObjectKey("catalog", uuid.New(), "banner.png", "image/png")
	b := s.buildObjectKey("catalog", uuid.New(), "banner.png", "image/png")
	if a == b {
		t.Errorf("two assets with the same filename produced the same key %q", a)
	}
}

// Only what the storage layer can actually serve gets a presign - an unknown type would otherwise
// land as an extension-less object the transform endpoint can't read
func TestIsAllowedMime(t *testing.T) {
	for _, mime := range []string{"image/jpeg", "image/png", "image/webp", "image/gif", "video/mp4", "application/pdf"} {
		if !isAllowedMime(mime) {
			t.Errorf("isAllowedMime(%q) = false, want true", mime)
		}
	}
	for _, mime := range []string{"", "text/html", "application/x-php", "application/octet-stream", "image/svg+xml"} {
		if isAllowedMime(mime) {
			t.Errorf("isAllowedMime(%q) = true, want false", mime)
		}
	}
}

func TestExtensionForMime(t *testing.T) {
	for mime, want := range map[string]string{
		"image/jpeg":      ".jpg",
		"image/png":       ".png",
		"video/quicktime": ".mov",
		"application/pdf": ".pdf", // not a storage type, so medialibrary supplies it
		"text/html":       "",
	} {
		if got := extensionForMime(mime); got != want {
			t.Errorf("extensionForMime(%q) = %q, want %q", mime, got, want)
		}
	}
}

// MimeGroup drives the width/height requirement in ConfirmUpload, so a non-image must not be grouped
// as one and then be rejected for lacking dimensions
func TestAssetMimeGroupAndPath(t *testing.T) {
	for mime, want := range map[string]MimeGroup{
		"image/png":       MimeGroupImage,
		"image/jpeg":      MimeGroupImage,
		"application/pdf": MimeGroupDocument,
		"video/mp4":       MimeGroupDocument, // videos are not images for the dimension gate
		"":                MimeGroupDocument,
		"image":           MimeGroupDocument, // too short to be the "image/" prefix
	} {
		if got := (Asset{MimeType: mime}).MimeGroup(); got != want {
			t.Errorf("MimeGroup(%q) = %q, want %q", mime, got, want)
		}
	}

	if got := (Asset{Filename: "a.png"}).Path(); got != "a.png" {
		t.Errorf("Path at the root = %q, want %q", got, "a.png")
	}
	if got := (Asset{DirectoryPrefix: "catalog", Filename: "a.png"}).Path(); got != "catalog/a.png" {
		t.Errorf("Path = %q, want %q", got, "catalog/a.png")
	}
}

func TestDirectoryNameLabelsTheRoot(t *testing.T) {
	if got := directoryName(""); got != "Library" {
		t.Errorf("directoryName(\"\") = %q, want %q", got, "Library")
	}
	if got := directoryName("catalog/banners"); got != "banners" {
		t.Errorf("directoryName = %q, want %q", got, "banners")
	}
}

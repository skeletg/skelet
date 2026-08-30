package admin

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
)

// Bulk mutations take id lists off the client, and one bad entry has to fail the whole call rather than silently operate on a shorter list
func TestParseUUIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got, err := parseUUIDs([]string{a.String(), b.String()})
	if err != nil {
		t.Fatalf("parseUUIDs: %v", err)
	}
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("parseUUIDs = %v, want [%v %v] in order", got, a, b)
	}

	for name, in := range map[string][]string{
		"one bad entry": {a.String(), "not-a-uuid"},
		"empty entry":   {a.String(), ""},
		"only bad":      {"nope"},
		"truncated":     {a.String()[:len(a.String())-1]},
	} {
		if got, err := parseUUIDs(in); err == nil {
			t.Errorf("%s: parseUUIDs = %v, want an error rather than a partial list", name, got)
		}
	}
}

// The error names the offending value, which is what makes a failed bulk action debuggable from the admin toast
func TestParseUUIDsErrorNamesTheBadValue(t *testing.T) {
	_, err := parseUUIDs([]string{uuid.NewString(), "banana"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error = %v, want it to name the unparseable value", err)
	}
}

func TestParseUUIDsOnAnEmptyList(t *testing.T) {
	got, err := parseUUIDs(nil)
	if err != nil {
		t.Fatalf("parseUUIDs(nil): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parseUUIDs(nil) = %v, want nothing", got)
	}
}

// An empty string is a cleared optional field, not a malformed id - the distinction is why this exists alongside the silent parser
func TestParseUUIDOptDistinguishesUnsetFromInvalid(t *testing.T) {
	id := uuid.New()
	s := id.String()
	got, err := parseUUIDOpt(&s)
	if err != nil || got == nil || *got != id {
		t.Errorf("parseUUIDOpt = (%v, %v), want %v", got, err, id)
	}

	empty := ""
	for name, in := range map[string]*string{"nil": nil, "empty": &empty} {
		got, err := parseUUIDOpt(in)
		if err != nil || got != nil {
			t.Errorf("%s: parseUUIDOpt = (%v, %v), want (nil, nil)", name, got, err)
		}
	}

	bad := "not-a-uuid"
	got, err = parseUUIDOpt(&bad)
	if err == nil {
		t.Errorf("parseUUIDOpt(%q) = %v, want the parse error surfaced", bad, got)
	}
	if got != nil {
		t.Errorf("parseUUIDOpt returned %v alongside the error, want nil", got)
	}
}

func TestBlogDerefHelpers(t *testing.T) {
	no, yes := false, true
	if got := boolDeref(nil, true); !got {
		t.Error("boolDeref(nil, true) = false, want the default")
	}
	if got := boolDeref(&no, true); got {
		t.Error("an explicit false was overridden by the default")
	}
	if got := boolDeref(&yes, false); !got {
		t.Error("an explicit true was overridden by the default")
	}

	n, zero := int32(7), int32(0)
	if got := intDeref(nil, 20); got != 20 {
		t.Errorf("intDeref(nil, 20) = %d, want 20", got)
	}
	if got := intDeref(&n, 20); got != 7 {
		t.Errorf("intDeref = %d, want the supplied 7", got)
	}
	if got := intDeref(&zero, 20); got != 0 {
		t.Errorf("an explicit 0 became %d, want 0 - a page size of zero is a real choice", got)
	}
}

// Post bodies come through the JSON scalar, and "absent" has to reach the domain as nil so an untouched body isn't overwritten with an empty document
func TestRawJSONTreatsNilAndNullAsAbsent(t *testing.T) {
	for name, in := range map[string]any{"nil": nil} {
		if got := rawJSON(in); got != nil {
			t.Errorf("%s: rawJSON = %q, want nil", name, got)
		}
	}
	// a decoded JSON null is also absent, not the literal null document
	var decodedNull any
	if err := json.Unmarshal([]byte("null"), &decodedNull); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := rawJSON(decodedNull); got != nil {
		t.Errorf("rawJSON(decoded null) = %q, want nil", got)
	}
}

func TestRawJSONEncodesRealContent(t *testing.T) {
	for name, in := range map[string]any{
		"document": map[string]any{"type": "doc", "content": []any{}},
		"array":    []any{float64(1), float64(2)},
		"string":   "plain",
		"number":   float64(3),
		"bool":     false,
	} {
		raw := rawJSON(in)
		if raw == nil {
			t.Errorf("%s: rawJSON = nil, want it encoded", name)
			continue
		}
		if !json.Valid(raw) {
			t.Errorf("%s: rawJSON = %q, which is not valid JSON", name, raw)
			continue
		}
		if got := unmarshalJSON(raw); !reflect.DeepEqual(got, in) {
			t.Errorf("%s: round-tripped to %#v, want %#v", name, got, in)
		}
	}
}

// An empty column and a malformed one both have to read as GraphQL null rather than taking the query down
func TestUnmarshalJSONFailsSoft(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"nil":       nil,
		"empty":     json.RawMessage(""),
		"malformed": json.RawMessage("{not json"),
	} {
		if got := unmarshalJSON(raw); got != nil {
			t.Errorf("%s: unmarshalJSON = %#v, want nil", name, got)
		}
	}
	// the null literal decodes to nil too
	if got := unmarshalJSON(json.RawMessage("null")); got != nil {
		t.Errorf("unmarshalJSON(null) = %#v, want nil", got)
	}
}

// Post status gates whether a page is publicly readable, so an unrecognized value must fall back to draft rather than publishing something
func TestStrLowerStatusFailsClosedToDraft(t *testing.T) {
	want := map[model.BlogPostStatus]string{
		model.BlogPostStatusDraft:     "draft",
		model.BlogPostStatusScheduled: "scheduled",
		model.BlogPostStatusPublished: "published",
		model.BlogPostStatusArchived:  "archived",
	}
	seen := map[string]model.BlogPostStatus{}
	for in, wantOut := range want {
		got := strLowerStatus(in)
		if got != wantOut {
			t.Errorf("strLowerStatus(%q) = %q, want %q", in, got, wantOut)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, got)
		}
		seen[got] = in
	}
	for _, in := range []model.BlogPostStatus{"", "LIVE", "published", "Draft"} {
		if got := strLowerStatus(in); got != "draft" {
			t.Errorf("strLowerStatus(%q) = %q, want the draft fallback so nothing publishes by accident", in, got)
		}
	}
}

// Metadata nil means "leave it alone" while an empty list means "clear it", and collapsing the two would make a clear impossible
func TestBlogMetadataFromInput(t *testing.T) {
	if got := blogMetadataFromInput(nil); got != nil {
		t.Errorf("blogMetadataFromInput(nil) = %v, want nil so an omitted field is left untouched", got)
	}
	got := blogMetadataFromInput([]*model.MetadataInput{})
	if got == nil {
		t.Error("an empty list = nil, want an empty map so the caller can tell a clear from an omission")
	}
	if len(got) != 0 {
		t.Errorf("an empty list = %v, want it empty", got)
	}

	got = blogMetadataFromInput([]*model.MetadataInput{
		{Key: "og:title", Value: "Winter tires"},
		nil, // a nil entry must not panic
		{Key: "og:type", Value: "article"},
	})
	if len(got) != 2 || got["og:title"] != "Winter tires" || got["og:type"] != "article" {
		t.Errorf("blogMetadataFromInput = %v, want both entries and the nil skipped", got)
	}
}

// A repeated key is last-write-wins, which is what the admin's own editor produces when a row is duplicated
func TestBlogMetadataFromInputLastWriteWins(t *testing.T) {
	got := blogMetadataFromInput([]*model.MetadataInput{
		{Key: "og:title", Value: "first"},
		{Key: "og:title", Value: "second"},
	})
	if len(got) != 1 || got["og:title"] != "second" {
		t.Errorf("blogMetadataFromInput = %v, want the last value to win", got)
	}
}

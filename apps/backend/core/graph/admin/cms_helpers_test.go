package admin

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
)

// Bulk page actions take id lists off the client, and a bad entry has to fail the call rather than act on a shorter list
func TestParseCMSPageIDs(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	got, err := parseCMSPageIDs([]string{a.String(), b.String()})
	if err != nil {
		t.Fatalf("parseCMSPageIDs: %v", err)
	}
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("parseCMSPageIDs = %v, want [%v %v] in order", got, a, b)
	}

	if _, err := parseCMSPageIDs([]string{a.String(), "not-a-uuid"}); err == nil {
		t.Error("a bad id was accepted, want the whole call refused")
	} else if !strings.Contains(err.Error(), "not-a-uuid") {
		t.Errorf("error = %v, want it to name the bad id", err)
	}

	empty, err := parseCMSPageIDs(nil)
	if err != nil {
		t.Fatalf("parseCMSPageIDs(nil): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("parseCMSPageIDs(nil) = %v, want an empty non-nil slice", empty)
	}
}

// The attribute's input type decides both the editor control and how its value is validated, so a collapsed pair silently changes a field's type
func TestCMSInputTypeMappingIsInjectiveBothWays(t *testing.T) {
	domainTypes := []cms.InputType{
		cms.InputText, cms.InputTextarea, cms.InputRichText, cms.InputNumber,
		cms.InputBoolean, cms.InputDate, cms.InputDateTime, cms.InputURL,
		cms.InputSelect, cms.InputMultiSelect, cms.InputAsset, cms.InputAssets,
		cms.InputRelationship, cms.InputJSON,
	}

	seenGQL := map[model.CMSInputType]cms.InputType{}
	for _, in := range domainTypes {
		gql := toCMSInputType(in)
		if prev, dup := seenGQL[gql]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, gql)
		}
		seenGQL[gql] = in
	}
	if len(seenGQL) != len(domainTypes) {
		t.Errorf("%d distinct GraphQL enums for %d domain types", len(seenGQL), len(domainTypes))
	}

	// and the reverse direction has to land back on the same domain string
	for _, in := range domainTypes {
		gql := toCMSInputType(in)
		if back := cmsInputTypeToString(gql); back != string(in) {
			t.Errorf("%q round-tripped through %q to %q", in, gql, back)
		}
	}
}

// A field type nobody recognizes must render as plain text - the one control that can display any stored value without losing it
func TestCMSInputTypeFallsBackToText(t *testing.T) {
	if got := toCMSInputType(cms.InputType("hologram")); got != model.CMSInputTypeText {
		t.Errorf("an unknown domain type = %q, want TEXT", got)
	}
	if got := toCMSInputType(""); got != model.CMSInputTypeText {
		t.Errorf("an empty domain type = %q, want TEXT", got)
	}
	for _, in := range []model.CMSInputType{"", "HOLOGRAM", "text"} {
		if got := cmsInputTypeToString(in); got != "text" {
			t.Errorf("cmsInputTypeToString(%q) = %q, want text", in, got)
		}
	}
}

// Every GraphQL enum the schema declares needs a string, or saving that field type writes the wrong one
func TestCMSInputTypeToStringCoversTheWholeEnum(t *testing.T) {
	seen := map[string]model.CMSInputType{}
	for _, in := range model.AllCMSInputType {
		got := cmsInputTypeToString(in)
		if got == "" {
			t.Errorf("cmsInputTypeToString(%q) = empty", in)
			continue
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, got)
		}
		seen[got] = in
		// and the string has to map back to the same enum
		if back := toCMSInputType(cms.InputType(got)); back != in {
			t.Errorf("%q became %q which maps back to %q", in, got, back)
		}
	}
	if len(seen) != len(model.AllCMSInputType) {
		t.Errorf("%d distinct strings for %d schema enums", len(seen), len(model.AllCMSInputType))
	}
}

// A relationship target picks which table the reference resolves against, so a wrong fallback points a field at the wrong entity
func TestCMSTargetToString(t *testing.T) {
	want := map[model.CMSRelationshipTarget]string{
		model.CMSRelationshipTargetCmsPage:    "cms_page",
		model.CMSRelationshipTargetBlogPost:   "blog_post",
		model.CMSRelationshipTargetProduct:    "product",
		model.CMSRelationshipTargetCollection: "collection",
		model.CMSRelationshipTargetCategory:   "category",
	}
	seen := map[string]model.CMSRelationshipTarget{}
	for in, wantOut := range want {
		got := cmsTargetToString(in)
		if got != wantOut {
			t.Errorf("cmsTargetToString(%q) = %q, want %q", in, got, wantOut)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, got)
		}
		seen[got] = in
	}
	for _, in := range []model.CMSRelationshipTarget{"", "CUSTOMER", "cms_page"} {
		if got := cmsTargetToString(in); got != "cms_page" {
			t.Errorf("cmsTargetToString(%q) = %q, want the cms_page fallback", in, got)
		}
	}
}

// Every target the schema declares needs its own string, or two targets resolve against one table
func TestCMSTargetToStringCoversTheWholeEnum(t *testing.T) {
	seen := map[string]model.CMSRelationshipTarget{}
	for _, in := range model.AllCMSRelationshipTarget {
		got := cmsTargetToString(in)
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", in, prev, got)
		}
		seen[got] = in
	}
	if len(seen) != len(model.AllCMSRelationshipTarget) {
		t.Errorf("%d distinct strings for %d schema targets", len(seen), len(model.AllCMSRelationshipTarget))
	}
}

// Nil means "leave the metadata alone" while an empty list means "clear it", so collapsing them makes a clear impossible
func TestCMSMetadataFromInput(t *testing.T) {
	if got := cmsMetadataFromInput(nil); got != nil {
		t.Errorf("cmsMetadataFromInput(nil) = %v, want nil for an omitted field", got)
	}
	got := cmsMetadataFromInput([]*model.MetadataInput{})
	if got == nil {
		t.Error("an empty list = nil, want an empty map so a clear is expressible")
	}
	if len(got) != 0 {
		t.Errorf("an empty list = %v, want it empty", got)
	}

	got = cmsMetadataFromInput([]*model.MetadataInput{
		{Key: "og:title", Value: "About us"},
		nil, // must not panic
		{Key: "robots", Value: "noindex"},
		{Key: "og:title", Value: "About"}, // last write wins
	})
	if len(got) != 2 {
		t.Fatalf("cmsMetadataFromInput = %v, want two keys", got)
	}
	if got["og:title"] != "About" {
		t.Errorf("og:title = %q, want the last value", got["og:title"])
	}
	if got["robots"] != "noindex" {
		t.Errorf("robots = %q, want noindex", got["robots"])
	}
}

func TestUUIDPtrStr(t *testing.T) {
	if got := uuidPtrStr(nil); got != nil {
		t.Errorf("uuidPtrStr(nil) = %v, want nil", got)
	}
	id := uuid.New()
	got := uuidPtrStr(&id)
	if got == nil || *got != id.String() {
		t.Errorf("uuidPtrStr = %v, want %q", got, id)
	}
}

func TestPageInputsCannotSetThePublishState(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[model.CreateCMSPageInput](), reflect.TypeFor[model.UpdateCMSPageInput]()} {
		for _, f := range []string{"IsPublished", "PublishedAt", "ClearPublished"} {
			if _, ok := typ.FieldByName(f); ok {
				t.Errorf("%s exposes %s", typ.Name(), f)
			}
		}
	}
	if _, ok := reflect.TypeFor[model.CreateCMSPageTypeInput]().FieldByName("IsSystem"); ok {
		t.Error("CreateCMSPageTypeInput lets the API mint a system type")
	}
}

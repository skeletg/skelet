package catalog

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func i32(n int32) *int32 { return &n }

// Single vs multi decides how many selections an option may hold, so an input type that answers neither would slip past every cardinality check
func TestBundleInputTypeSelectionClass(t *testing.T) {
	for _, tc := range []struct {
		in            BundleInputType
		single, multi bool
	}{
		{BundleInputRadio, true, false},
		{BundleInputDropdown, true, false},
		{BundleInputCheckbox, false, true},
		{BundleInputMultiselect, false, true},
	} {
		if got := tc.in.IsSingleSelect(); got != tc.single {
			t.Errorf("%q.IsSingleSelect() = %v, want %v", tc.in, got, tc.single)
		}
		if got := tc.in.IsMultiSelect(); got != tc.multi {
			t.Errorf("%q.IsMultiSelect() = %v, want %v", tc.in, got, tc.multi)
		}
		// every known type is exactly one of the two
		if tc.in.IsSingleSelect() == tc.in.IsMultiSelect() {
			t.Errorf("%q answers both or neither, want exactly one class", tc.in)
		}
		if !tc.in.IsKnown() {
			t.Errorf("%q.IsKnown() = false, want true", tc.in)
		}
	}
}

// An unknown type must claim NEITHER class - claiming single would silently cap a real option at one selection, claiming multi would let a radio hold several
func TestUnknownBundleInputTypeClaimsNoClass(t *testing.T) {
	for _, in := range []BundleInputType{"", "RADIO", "toggle", "select"} {
		if in.IsKnown() {
			t.Errorf("%q.IsKnown() = true, want false", in)
		}
		if in.IsSingleSelect() || in.IsMultiSelect() {
			t.Errorf("%q claims a selection class (single=%v multi=%v), want neither",
				in, in.IsSingleSelect(), in.IsMultiSelect())
		}
	}
}

// The bounds have to be internally coherent before the input type is even considered
func TestValidateBundleLimitsRejectsIncoherentBounds(t *testing.T) {
	for name, tc := range map[string]struct {
		inputType BundleInputType
		min       int32
		max       *int32
	}{
		"negative min":         {BundleInputCheckbox, -1, nil},
		"zero max":             {BundleInputCheckbox, 0, i32(0)},
		"negative max":         {BundleInputCheckbox, 0, i32(-3)},
		"max below min":        {BundleInputCheckbox, 3, i32(2)},
		"max below min by one": {BundleInputCheckbox, 2, i32(1)},
	} {
		err := validateBundleLimits(tc.inputType, tc.min, tc.max)
		if err == nil {
			t.Errorf("%s was accepted, want it refused", name)
			continue
		}
		if !errors.Is(err, ErrInvalidBundleLimits) {
			t.Errorf("%s: error = %v, want ErrInvalidBundleLimits", name, err)
		}
	}
}

// A radio or dropdown holds at most one selection, so any bound above 1 describes an option the UI can't render and the cart would refuse at add time
func TestValidateBundleLimitsRejectsMultiBoundsOnSingleSelect(t *testing.T) {
	for _, single := range []BundleInputType{BundleInputRadio, BundleInputDropdown} {
		if err := validateBundleLimits(single, 2, nil); err == nil {
			t.Errorf("%q with min 2 was accepted, want it refused", single)
		}
		if err := validateBundleLimits(single, 0, i32(2)); err == nil {
			t.Errorf("%q with max 2 was accepted, want it refused", single)
		}
		// the legal shapes for a single-select
		for _, ok := range []struct {
			min int32
			max *int32
		}{{0, nil}, {1, nil}, {0, i32(1)}, {1, i32(1)}} {
			if err := validateBundleLimits(single, ok.min, ok.max); err != nil {
				t.Errorf("%q with min %d max %v = %v, want nil", single, ok.min, ok.max, err)
			}
		}
	}
}

// Multi-select is where wide bounds are legal - capping them like a radio would break "pick any three"
func TestValidateBundleLimitsAllowsWideBoundsOnMultiSelect(t *testing.T) {
	for _, multi := range []BundleInputType{BundleInputCheckbox, BundleInputMultiselect} {
		for _, ok := range []struct {
			min int32
			max *int32
		}{{0, nil}, {1, nil}, {3, nil}, {0, i32(5)}, {2, i32(5)}, {3, i32(3)}} {
			if err := validateBundleLimits(multi, ok.min, ok.max); err != nil {
				t.Errorf("%q with min %d max %v = %v, want nil", multi, ok.min, ok.max, err)
			}
		}
	}
}

// An unknown type skips the single-select clause, so only the coherence rules apply - it must not accidentally get the strict cap
func TestValidateBundleLimitsOnAnUnknownType(t *testing.T) {
	if err := validateBundleLimits(BundleInputType("toggle"), 4, i32(9)); err != nil {
		t.Errorf("an unknown type with coherent bounds = %v, want nil", err)
	}
	if err := validateBundleLimits(BundleInputType("toggle"), 9, i32(4)); err == nil {
		t.Error("an unknown type with max below min was accepted")
	}
}

// A nil max is "unbounded", which is different from a zero max - collapsing them would make every unbounded option invalid
func TestValidateBundleLimitsTreatsNilMaxAsUnbounded(t *testing.T) {
	if err := validateBundleLimits(BundleInputCheckbox, 0, nil); err != nil {
		t.Errorf("nil max = %v, want nil (unbounded)", err)
	}
	if err := validateBundleLimits(BundleInputCheckbox, 0, i32(0)); err == nil {
		t.Error("a max of 0 was accepted, want it refused - it would allow no selections at all")
	}
}

// Slug stands in for a readable label - Category carries its name in localized Content, not on the row
func cat(id uuid.UUID, parent *uuid.UUID, slug string) Category {
	return Category{ID: id, ParentID: parent, Slug: slug}
}

func names(cs []Category) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Slug)
	}
	return out
}

// The nav tree is assembled from a flat query, so the parent wiring is what turns rows into menus
func TestBuildCategoryTreeNests(t *testing.T) {
	rootA, rootB := uuid.New(), uuid.New()
	child, grandchild := uuid.New(), uuid.New()
	all := []Category{
		cat(rootA, nil, "tires"),
		cat(child, &rootA, "winter"),
		cat(grandchild, &child, "studded"),
		cat(rootB, nil, "wheels"),
	}

	roots := buildCategoryTree(all, nil)
	if got := names(roots); len(got) != 2 || got[0] != "tires" || got[1] != "wheels" {
		t.Fatalf("roots = %v, want [tires wheels]", got)
	}
	if got := names(roots[0].Children); len(got) != 1 || got[0] != "winter" {
		t.Fatalf("children of tires = %v, want [winter]", got)
	}
	if got := names(roots[0].Children[0].Children); len(got) != 1 || got[0] != "studded" {
		t.Errorf("children of winter = %v, want [studded]", got)
	}
	if len(roots[1].Children) != 0 {
		t.Errorf("wheels has %d children, want none", len(roots[1].Children))
	}
}

// The flat query's order is the caller's chosen sort, so the tree has to preserve it per level
func TestBuildCategoryTreePreservesInputOrderPerLevel(t *testing.T) {
	root := uuid.New()
	all := []Category{
		cat(root, nil, "root"),
		cat(uuid.New(), &root, "first"),
		cat(uuid.New(), &root, "second"),
		cat(uuid.New(), &root, "third"),
	}
	got := names(buildCategoryTree(all, nil)[0].Children)
	if len(got) != 3 || got[0] != "first" || got[1] != "second" || got[2] != "third" {
		t.Errorf("children = %v, want the input order [first second third]", got)
	}
}

// A category whose parent isn't in the slice is unreachable from the roots, so it drops out rather than being silently promoted to a top-level nav entry
func TestBuildCategoryTreeDropsUnreachableBranches(t *testing.T) {
	root, orphanParent := uuid.New(), uuid.New()
	all := []Category{
		cat(root, nil, "root"),
		cat(uuid.New(), &orphanParent, "orphan"),
	}
	roots := buildCategoryTree(all, nil)
	if got := names(roots); len(got) != 1 || got[0] != "root" {
		t.Errorf("roots = %v, want only [root] - an orphan must not become a root", got)
	}
}

// Asking for one subtree returns just that branch, which is what the children resolver renders
func TestBuildCategoryTreeFromASpecificParent(t *testing.T) {
	root, child := uuid.New(), uuid.New()
	all := []Category{
		cat(root, nil, "root"),
		cat(child, &root, "child"),
		cat(uuid.New(), &child, "grandchild"),
		cat(uuid.New(), nil, "unrelated root"),
	}

	sub := buildCategoryTree(all, &root)
	if got := names(sub); len(got) != 1 || got[0] != "child" {
		t.Fatalf("subtree of root = %v, want [child]", got)
	}
	if got := names(sub[0].Children); len(got) != 1 || got[0] != "grandchild" {
		t.Errorf("subtree keeps nesting = %v, want [grandchild]", got)
	}
}

func TestBuildCategoryTreeOnTrivialInput(t *testing.T) {
	if got := buildCategoryTree(nil, nil); len(got) != 0 {
		t.Errorf("buildCategoryTree(nil) = %v, want nothing", names(got))
	}
	// only nested rows means no roots to render
	parent := uuid.New()
	if got := buildCategoryTree([]Category{cat(uuid.New(), &parent, "child")}, nil); len(got) != 0 {
		t.Errorf("a slice with no roots = %v, want nothing", names(got))
	}
	if got := buildCategoryTree([]Category{cat(uuid.New(), nil, "root")}, &parent); len(got) != 0 {
		t.Errorf("an unknown parent = %v, want nothing", names(got))
	}
}

package front

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/front/model"
	"github.com/skeletg/skelet/internal/menu"
	"github.com/skeletg/skelet/internal/urlrewrite"
)

func menuItem(id uuid.UUID, parent *uuid.UUID, label string, sortOrder int32) menu.MenuItem {
	return menu.MenuItem{ID: id, ParentID: parent, Label: label, SortOrder: sortOrder, URL: "/" + label}
}

func childLabels(items []*model.MenuItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Label)
	}
	return out
}

// The converter builds every node before wiring parents, so the rows can arrive in any order the query returns them
func TestToFrontMenuModelBuildsTheTreeRegardlessOfRowOrder(t *testing.T) {
	root, child, grandchild := uuid.New(), uuid.New(), uuid.New()
	items := []menu.MenuItem{
		menuItem(grandchild, &child, "grandchild", 0), // deepest row first
		menuItem(child, &root, "child", 0),
		menuItem(root, nil, "root", 0),
	}

	got := toFrontMenuModel(menu.Menu{ID: uuid.New(), Slug: "main", Name: "Main"}, items, nil)
	if len(got.Items) != 1 {
		t.Fatalf("%d roots, want 1: %v", len(got.Items), childLabels(got.Items))
	}
	if got.Items[0].Label != "root" {
		t.Fatalf("root = %q, want root", got.Items[0].Label)
	}
	if len(got.Items[0].Children) != 1 || got.Items[0].Children[0].Label != "child" {
		t.Fatalf("children = %v, want [child]", childLabels(got.Items[0].Children))
	}
	kids := got.Items[0].Children[0].Children
	if len(kids) != 1 || kids[0].Label != "grandchild" {
		t.Errorf("grandchildren = %v, want [grandchild]", childLabels(kids))
	}
	if got.Slug != "main" || got.Name != "Main" {
		t.Errorf("menu = {%q %q}, want {main Main}", got.Slug, got.Name)
	}
}

// A parent_id left behind by a deleted row must surface its orphan at top level rather than dropping the nav entry
func TestToFrontMenuModelSurfacesOrphansAtTopLevel(t *testing.T) {
	orphan := uuid.New()
	missingParent := uuid.New()
	root := uuid.New()
	items := []menu.MenuItem{
		menuItem(root, nil, "root", 0),
		menuItem(orphan, &missingParent, "orphan", 1),
	}

	got := toFrontMenuModel(menu.Menu{ID: uuid.New()}, items, nil)
	if len(got.Items) != 2 {
		t.Fatalf("%d roots, want the orphan surfaced alongside the real root: %v", len(got.Items), childLabels(got.Items))
	}
	labels := childLabels(got.Items)
	if labels[0] != "root" || labels[1] != "orphan" {
		t.Errorf("roots = %v, want [root orphan]", labels)
	}
	// the stale parent id is still reported, so the dashboard can show what it points at
	if got.Items[1].ParentID == nil || *got.Items[1].ParentID != missingParent.String() {
		t.Errorf("orphan parentId = %v, want the stale id %q", got.Items[1].ParentID, missingParent)
	}
}

// Sort order drives the nav, and equal orders fall back to the id so two siblings never swap between requests
func TestSortMenuItemTreeIsDeterministic(t *testing.T) {
	lowID, highID := "00000000-0000-0000-0000-000000000001", "ffffffff-0000-0000-0000-000000000001"
	items := []*model.MenuItem{
		{ID: highID, Label: "same-order-high", SortOrder: 1},
		{ID: "aaaaaaaa-0000-0000-0000-000000000001", Label: "second", SortOrder: 2},
		{ID: lowID, Label: "same-order-low", SortOrder: 1},
		{ID: "bbbbbbbb-0000-0000-0000-000000000001", Label: "first", SortOrder: 0},
	}

	sortMenuItemTree(items)

	if items[0].Label != "first" {
		t.Errorf("head = %q, want the sortOrder 0 item", items[0].Label)
	}
	if items[1].ID != lowID || items[2].ID != highID {
		t.Errorf("the sortOrder 1 tie broke to %q then %q, want the lower id first", items[1].ID, items[2].ID)
	}
	if items[3].Label != "second" {
		t.Errorf("tail = %q, want the sortOrder 2 item", items[3].Label)
	}
}

// Children get the same ordering, or a sub-menu renders in insertion order
func TestSortMenuItemTreeRecursesIntoChildren(t *testing.T) {
	items := []*model.MenuItem{{
		ID: uuid.NewString(), Label: "root",
		Children: []*model.MenuItem{
			{ID: uuid.NewString(), Label: "third", SortOrder: 3},
			{ID: uuid.NewString(), Label: "first", SortOrder: 1, Children: []*model.MenuItem{
				{ID: uuid.NewString(), Label: "deep-b", SortOrder: 2},
				{ID: uuid.NewString(), Label: "deep-a", SortOrder: 1},
			}},
			{ID: uuid.NewString(), Label: "second", SortOrder: 2},
		},
	}}

	sortMenuItemTree(items)

	if got := childLabels(items[0].Children); got[0] != "first" || got[1] != "second" || got[2] != "third" {
		t.Errorf("children = %v, want [first second third]", got)
	}
	deep := childLabels(items[0].Children[0].Children)
	if deep[0] != "deep-a" || deep[1] != "deep-b" {
		t.Errorf("grandchildren = %v, want [deep-a deep-b]", deep)
	}
}

// The converter re-sorts rather than trusting the repo's ORDER BY, so the tree is right even when a caller assembles the rows itself
func TestToFrontMenuModelSortsWhatItBuilds(t *testing.T) {
	root := uuid.New()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	items := []menu.MenuItem{
		menuItem(root, nil, "root", 0),
		menuItem(c, &root, "c", 3),
		menuItem(a, &root, "a", 1),
		menuItem(b, &root, "b", 2),
	}
	got := toFrontMenuModel(menu.Menu{ID: uuid.New()}, items, nil)
	if labels := childLabels(got.Items[0].Children); labels[0] != "a" || labels[1] != "b" || labels[2] != "c" {
		t.Errorf("children = %v, want [a b c]", labels)
	}
}

// An external URL is what the admin typed, so it wins over whatever url_rewrites resolved for the target
func TestToFrontMenuItemModelPrefersTheExplicitURL(t *testing.T) {
	id := uuid.New()
	it := menu.MenuItem{ID: id, Label: "Blog", URL: "https://external.invalid/blog"}

	got := toFrontMenuItemModel(it, "/resolved/path")
	if got.URL == nil || *got.URL != "https://external.invalid/blog" {
		t.Errorf("url = %v, want the explicit external URL", got.URL)
	}
}

// A target-linked item has no URL of its own, so the resolved canonical path is the only thing to render
func TestToFrontMenuItemModelUsesTheResolvedPath(t *testing.T) {
	targetID := uuid.New()
	it := menu.MenuItem{ID: uuid.New(), Label: "Tires", TargetType: menu.TargetTypeCategory, TargetID: &targetID}

	got := toFrontMenuItemModel(it, "/c/tires")
	if got.URL == nil || *got.URL != "/c/tires" {
		t.Errorf("url = %v, want the resolved path", got.URL)
	}
	if got.TargetType == nil || *got.TargetType != model.MenuItemTargetTypeCategory {
		t.Errorf("targetType = %v, want CATEGORY", got.TargetType)
	}
	if got.TargetID == nil || *got.TargetID != targetID.String() {
		t.Errorf("targetId = %v, want %q", got.TargetID, targetID)
	}
}

// A target whose rewrite is missing must leave url nil so the storefront can skip the link rather than render href=""
func TestToFrontMenuItemModelLeavesURLNilWithNothingToRender(t *testing.T) {
	targetID := uuid.New()
	it := menu.MenuItem{ID: uuid.New(), Label: "Tires", TargetType: menu.TargetTypeCategory, TargetID: &targetID}

	got := toFrontMenuItemModel(it, "")
	if got.URL != nil {
		t.Errorf("url = %v, want nil when neither an explicit nor a resolved URL exists", got.URL)
	}
}

// A URL-only item carries no target, and an empty TargetType must stay nil rather than defaulting to PRODUCT
func TestToFrontMenuItemModelLeavesTargetNilForAURLItem(t *testing.T) {
	got := toFrontMenuItemModel(menu.MenuItem{ID: uuid.New(), Label: "Sale", URL: "/sale"}, "")
	if got.TargetType != nil {
		t.Errorf("targetType = %v, want nil for a free-form URL item", got.TargetType)
	}
	if got.TargetID != nil {
		t.Errorf("targetId = %v, want nil", got.TargetID)
	}
	if got.Children == nil {
		t.Error("Children = nil, want an empty slice for the non-null field")
	}
}

// Menu targets and url_rewrites share a vocabulary, so a missed pair means the item's canonical path is never looked up
func TestMenuTargetToURLRewriteEntityCoversEveryTargetType(t *testing.T) {
	want := map[menu.TargetType]urlrewrite.EntityType{
		menu.TargetTypeProduct:      urlrewrite.EntityTypeProduct,
		menu.TargetTypeCategory:     urlrewrite.EntityTypeCategory,
		menu.TargetTypeCollection:   urlrewrite.EntityTypeCollection,
		menu.TargetTypeBlogPost:     urlrewrite.EntityTypeBlogPost,
		menu.TargetTypeBlogCategory: urlrewrite.EntityTypeBlogCategory,
		menu.TargetTypeBlogTag:      urlrewrite.EntityTypeBlogTag,
		menu.TargetTypeBlogAuthor:   urlrewrite.EntityTypeBlogAuthor,
		menu.TargetTypeCMSPage:      urlrewrite.EntityTypeCMSPage,
	}
	for tt, wantEntity := range want {
		if got := menuTargetToURLRewriteEntity(tt); got != wantEntity {
			t.Errorf("menuTargetToURLRewriteEntity(%q) = %q, want %q", tt, got, wantEntity)
		}
	}
	// every valid target type must map to something, or its items silently lose their URLs
	for tt := range want {
		if !tt.IsValid() {
			t.Errorf("%q is in the mapping but menu says it is not a valid target type", tt)
		}
		if menuTargetToURLRewriteEntity(tt) == "" {
			t.Errorf("%q maps to the empty entity type, so its canonical path is never resolved", tt)
		}
	}
	// an unknown one yields the empty entity, which the caller treats as nothing to resolve
	for _, tt := range []menu.TargetType{"", "wishlist", "PRODUCT"} {
		if got := menuTargetToURLRewriteEntity(tt); got != "" {
			t.Errorf("menuTargetToURLRewriteEntity(%q) = %q, want the empty entity", tt, got)
		}
	}
}

// Each domain target needs its own schema enum, or the storefront renders the wrong link kind
func TestFrontMenuTargetTypeToModelIsInjective(t *testing.T) {
	targets := []menu.TargetType{
		menu.TargetTypeProduct, menu.TargetTypeCategory, menu.TargetTypeCollection,
		menu.TargetTypeBlogPost, menu.TargetTypeBlogCategory, menu.TargetTypeBlogTag,
		menu.TargetTypeBlogAuthor, menu.TargetTypeCMSPage,
	}
	seen := make(map[model.MenuItemTargetType]menu.TargetType, len(targets))
	for _, tt := range targets {
		got := frontMenuTargetTypeToModel(tt)
		if prev, dup := seen[got]; dup {
			t.Errorf("%q and %q both map to %q", tt, prev, got)
		}
		seen[got] = tt
	}
	if len(seen) != len(targets) {
		t.Errorf("%d distinct enums for %d target types", len(seen), len(targets))
	}
	// the fallback is PRODUCT, and the caller only reaches it for a non-empty target type the DB CHECK already constrains
	if got := frontMenuTargetTypeToModel("wishlist"); got != model.MenuItemTargetTypeProduct {
		t.Errorf("an unknown target = %q, want the PRODUCT fallback", got)
	}
}

// The resolved map is keyed by item id, so a URL meant for one item must not land on another
func TestToFrontMenuModelAppliesResolvedURLsPerItem(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	catA, catB := uuid.New(), uuid.New()
	items := []menu.MenuItem{
		{ID: first, Label: "Tires", TargetType: menu.TargetTypeCategory, TargetID: &catA, SortOrder: 0},
		{ID: second, Label: "Wheels", TargetType: menu.TargetTypeCategory, TargetID: &catB, SortOrder: 1},
	}
	resolved := map[uuid.UUID]string{first: "/c/tires", second: "/c/wheels"}

	got := toFrontMenuModel(menu.Menu{ID: uuid.New()}, items, resolved)
	if len(got.Items) != 2 {
		t.Fatalf("%d roots, want 2", len(got.Items))
	}
	if got.Items[0].URL == nil || *got.Items[0].URL != "/c/tires" {
		t.Errorf("first url = %v, want /c/tires", got.Items[0].URL)
	}
	if got.Items[1].URL == nil || *got.Items[1].URL != "/c/wheels" {
		t.Errorf("second url = %v, want /c/wheels", got.Items[1].URL)
	}
}

func TestToFrontMenuModelHandlesAnEmptyMenu(t *testing.T) {
	got := toFrontMenuModel(menu.Menu{ID: uuid.New(), Slug: "empty"}, nil, nil)
	if got.Items == nil {
		t.Error("Items = nil, want an empty slice for the non-null field")
	}
	if len(got.Items) != 0 {
		t.Errorf("Items = %v, want none", childLabels(got.Items))
	}
}

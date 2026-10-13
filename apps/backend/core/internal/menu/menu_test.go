package menu

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Only the handful of methods the validation paths touch are implemented - the embedded interface
// keeps the other twenty compiling, and a nil-pointer panic would name any method a test wrongly reaches
type fakeRepo struct {
	Repository
	menu        Menu
	items       map[uuid.UUID]MenuItem
	created     []CreateMenuItemInput
	updated     []UpdateMenuItemInput
	assignments []uuid.UUID
	cleared     int
	menuErr     error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{menu: Menu{ID: uuid.New(), Slug: "main", Name: "Main"}, items: map[uuid.UUID]MenuItem{}}
}

func (r *fakeRepo) GetMenuByID(_ context.Context, id uuid.UUID) (Menu, error) {
	if r.menuErr != nil {
		return Menu{}, r.menuErr
	}
	if id != r.menu.ID {
		return Menu{}, ErrMenuNotFound
	}
	return r.menu, nil
}

func (r *fakeRepo) GetMenuItemByID(_ context.Context, id uuid.UUID) (MenuItem, error) {
	it, ok := r.items[id]
	if !ok {
		return MenuItem{}, ErrMenuItemNotFound
	}
	return it, nil
}

func (r *fakeRepo) CreateMenuItem(_ context.Context, in CreateMenuItemInput) (MenuItem, error) {
	r.created = append(r.created, in)
	item := MenuItem{ID: uuid.New(), MenuID: in.MenuID, ParentID: in.ParentID, Label: in.Label,
		TargetType: in.TargetType, TargetID: in.TargetID, URL: in.URL, SortOrder: in.SortOrder}
	r.items[item.ID] = item
	return item, nil
}

func (r *fakeRepo) UpdateMenuItem(_ context.Context, in UpdateMenuItemInput) (MenuItem, error) {
	r.updated = append(r.updated, in)
	return r.items[in.ID], nil
}

func (r *fakeRepo) CreateMenu(_ context.Context, in CreateMenuInput) (Menu, error) {
	return Menu{ID: uuid.New(), Slug: in.Slug, Name: in.Name}, nil
}

func (r *fakeRepo) UpdateMenu(_ context.Context, in UpdateMenuInput) (Menu, error) {
	m := r.menu
	if in.Slug != nil {
		m.Slug = *in.Slug
	}
	if in.Name != nil {
		m.Name = *in.Name
	}
	return m, nil
}

func (r *fakeRepo) DeleteAllMenuChannelAssignments(context.Context, uuid.UUID) error {
	r.cleared++
	r.assignments = nil
	return nil
}

func (r *fakeRepo) InsertMenuChannelAssignment(_ context.Context, _, channelID uuid.UUID) error {
	r.assignments = append(r.assignments, channelID)
	return nil
}

func (r *fakeRepo) UpsertMenuItemTranslation(_ context.Context, itemID uuid.UUID, locale, label string) (MenuItemTranslation, error) {
	return MenuItemTranslation{ItemID: itemID, Locale: locale, Label: label}, nil
}

// Builds a chain root -> child -> grandchild... of the requested length and returns the deepest item
func (r *fakeRepo) chain(n int) MenuItem {
	var parent *uuid.UUID
	var last MenuItem
	for range n {
		item := MenuItem{ID: uuid.New(), MenuID: r.menu.ID, ParentID: parent, Label: "level"}
		r.items[item.ID] = item
		id := item.ID
		parent = &id
		last = item
	}
	return last
}

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	return NewService(repo), repo
}

func TestValidateSlug(t *testing.T) {
	for _, ok := range []string{"main", "main-nav", "footer_links", "nav2", "a"} {
		if err := validateSlug(ok); err != nil {
			t.Errorf("validateSlug(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "Main", "main nav", "main/nav", "main.nav", "menü", "main!"} {
		if err := validateSlug(bad); err == nil {
			t.Errorf("validateSlug(%q) = nil, want an error", bad)
		}
	}
}

func TestBadSlugIsAValidationError(t *testing.T) {
	svc, _ := newTestService()
	bad := "Main.Nav"
	_, createErr := svc.CreateMenu(context.Background(), CreateMenuInput{Slug: bad, Name: "Main"})
	_, updateErr := svc.UpdateMenu(context.Background(), UpdateMenuInput{ID: uuid.New(), Slug: &bad})
	for _, err := range []error{createErr, updateErr} {
		if !errors.Is(err, ErrInvalidSlug) {
			t.Fatalf("got %v, want ErrInvalidSlug", err)
		}
		if c := apperr.ClassOf(err); c != apperr.ClassValidation {
			t.Errorf("class = %v, want ClassValidation", c)
		}
	}
}

// A menu item points at an entity or at a free-form URL, never both and never neither - the DB has
// the same CHECK, and this is what turns it into a readable message
func TestValidateTargetIsAnExclusiveOr(t *testing.T) {
	id := uuid.New()

	if err := validateTarget(TargetTypeProduct, &id, ""); err != nil {
		t.Errorf("an entity target = %v, want nil", err)
	}
	if err := validateTarget("", nil, "https://example.invalid"); err != nil {
		t.Errorf("a URL item = %v, want nil", err)
	}

	for name, tc := range map[string]struct {
		tt  TargetType
		tid *uuid.UUID
		url string
	}{
		"neither":                 {"", nil, ""},
		"blank url and no target": {"", nil, "   "},
		"both":                    {TargetTypeProduct, &id, "https://example.invalid"},
	} {
		if err := validateTarget(tc.tt, tc.tid, tc.url); !errors.Is(err, ErrTargetXorURL) {
			t.Errorf("%s = %v, want ErrTargetXorURL", name, err)
		}
	}

	if err := validateTarget(TargetType("wishlist"), &id, ""); !errors.Is(err, ErrInvalidTargetType) {
		t.Errorf("an unknown target type = %v, want ErrInvalidTargetType", err)
	}
	// a target type without an id is not a target at all, so it needs a URL
	if err := validateTarget(TargetTypeProduct, nil, ""); !errors.Is(err, ErrTargetXorURL) {
		t.Errorf("a target type with no id = %v, want ErrTargetXorURL", err)
	}
}

func TestTargetTypeIsValid(t *testing.T) {
	for _, tt := range []TargetType{TargetTypeProduct, TargetTypeCategory, TargetTypeCollection,
		TargetTypeBlogPost, TargetTypeBlogCategory, TargetTypeBlogTag, TargetTypeBlogAuthor, TargetTypeCMSPage} {
		if !tt.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", tt)
		}
	}
	for _, tt := range []TargetType{"", "product ", "PRODUCT", "wishlist"} {
		if tt.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", tt)
		}
	}
}

// The front resolver renders whatever this returns, so a missing translation has to fall back rather
// than render an empty nav item
func TestResolveItemLabel(t *testing.T) {
	item := MenuItem{Label: "Shop"}
	translations := []MenuItemTranslation{{Locale: "de", Label: "Laden"}, {Locale: "fr", Label: "Boutique"}}

	for name, tc := range map[string]struct {
		locale string
		want   string
	}{
		"matching locale":     {"de", "Laden"},
		"other locale":        {"fr", "Boutique"},
		"unknown locale":      {"es", "Shop"},
		"no locale requested": {"", "Shop"},
	} {
		if got := ResolveItemLabel(item, translations, tc.locale); got != tc.want {
			t.Errorf("%s: ResolveItemLabel = %q, want %q", name, got, tc.want)
		}
	}
	if got := ResolveItemLabel(item, nil, "de"); got != "Shop" {
		t.Errorf("with no translations at all = %q, want the base label", got)
	}
}

func TestCreateMenuRequiresASlugAndName(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	if _, err := svc.CreateMenu(ctx, CreateMenuInput{Slug: "Main Nav", Name: "Main"}); err == nil {
		t.Error("a bad slug was accepted")
	}
	if _, err := svc.CreateMenu(ctx, CreateMenuInput{Slug: "main", Name: "   "}); err == nil {
		t.Error("a blank name was accepted")
	}
	if _, err := svc.CreateMenu(ctx, CreateMenuInput{Slug: "main", Name: "Main"}); err != nil {
		t.Errorf("the valid menu = %v, want nil", err)
	}
}

// Nil fields mean "leave alone", so an update that only renames must not have to re-supply the slug
func TestUpdateMenuValidatesOnlyTheSuppliedFields(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	name, blank, badSlug := "Renamed", "  ", "Bad Slug"

	if _, err := svc.UpdateMenu(ctx, UpdateMenuInput{Name: &name}); err != nil {
		t.Errorf("a name-only update = %v, want nil", err)
	}
	if _, err := svc.UpdateMenu(ctx, UpdateMenuInput{Slug: &badSlug}); err == nil {
		t.Error("a bad slug was accepted")
	}
	if _, err := svc.UpdateMenu(ctx, UpdateMenuInput{Name: &blank}); err == nil {
		t.Error("a blank name was accepted")
	}
}

func TestCreateMenuItemValidatesBeforeWriting(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	target := uuid.New()

	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{MenuID: repo.menu.ID, Label: "  ", URL: "/x"}); err == nil {
		t.Error("a blank label was accepted")
	}
	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{MenuID: repo.menu.ID, Label: "Both",
		TargetType: TargetTypeProduct, TargetID: &target, URL: "/x"}); !errors.Is(err, ErrTargetXorURL) {
		t.Error("an item with both a target and a URL was accepted")
	}
	if len(repo.created) != 0 {
		t.Fatalf("%d items reached the repository despite validation failing", len(repo.created))
	}

	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{MenuID: repo.menu.ID, Label: "Shop", URL: "/shop"}); err != nil {
		t.Fatalf("the valid item = %v, want nil", err)
	}
	if len(repo.created) != 1 {
		t.Errorf("%d items created, want 1", len(repo.created))
	}
}

// Parent ids arrive from the dashboard's drag-and-drop, so a stale or cross-menu one is routine input
func TestCreateMenuItemRejectsBadParents(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	missing := uuid.New()
	foreign := MenuItem{ID: uuid.New(), MenuID: uuid.New(), Label: "elsewhere"}
	repo.items[foreign.ID] = foreign

	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{
		MenuID: repo.menu.ID, Label: "Child", URL: "/c", ParentID: &missing,
	}); !errors.Is(err, ErrMenuItemNotFound) {
		t.Errorf("an unknown parent = %v, want ErrMenuItemNotFound", err)
	}
	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{
		MenuID: repo.menu.ID, Label: "Child", URL: "/c", ParentID: &foreign.ID,
	}); !errors.Is(err, ErrParentInOtherMenu) {
		t.Errorf("a parent from another menu = %v, want ErrParentInOtherMenu", err)
	}
}

// MaxDepth is 5, so an item may hang off a parent that sits at level 4 but not one at level 5
func TestParentDepthIsCappedAtMaxDepth(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	deepestAllowed := repo.chain(MaxDepth - 1)
	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{
		MenuID: repo.menu.ID, Label: "Child", URL: "/c", ParentID: &deepestAllowed.ID,
	}); err != nil {
		t.Errorf("a parent at level %d = %v, want the child accepted", MaxDepth-1, err)
	}

	svc, repo = newTestService()
	tooDeep := repo.chain(MaxDepth)
	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{
		MenuID: repo.menu.ID, Label: "Child", URL: "/c", ParentID: &tooDeep.ID,
	}); !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("a parent at level %d = %v, want ErrDepthExceeded", MaxDepth, err)
	}
}

// Reparenting an item under its own descendant would orphan the whole subtree from the root and make
// the tree walk never terminate
func TestUpdateMenuItemRefusesToReparentUnderItsOwnDescendant(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()

	root := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, Label: "root", URL: "/"}
	repo.items[root.ID] = root
	child := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, ParentID: &root.ID, Label: "child", URL: "/c"}
	repo.items[child.ID] = child
	grandchild := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, ParentID: &child.ID, Label: "grandchild", URL: "/g"}
	repo.items[grandchild.ID] = grandchild

	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: root.ID, ParentID: &grandchild.ID}); !errors.Is(err, ErrParentCycle) {
		t.Errorf("reparenting root under its grandchild = %v, want ErrParentCycle", err)
	}
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: root.ID, ParentID: &root.ID}); !errors.Is(err, ErrParentCycle) {
		t.Errorf("making an item its own parent = %v, want ErrParentCycle", err)
	}
	if len(repo.updated) != 0 {
		t.Errorf("%d updates reached the repository despite the cycle", len(repo.updated))
	}

	// the legitimate direction is still allowed
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: grandchild.ID, ParentID: &root.ID}); err != nil {
		t.Errorf("reparenting a grandchild under root = %v, want nil", err)
	}
}

// nil means "keep what's there", so an update carrying only a label must not trip the target XOR
func TestUpdateMenuItemMergesTheTargetFieldsBeforeValidating(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	target := uuid.New()
	item := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, Label: "Shop", TargetType: TargetTypeProduct, TargetID: &target}
	repo.items[item.ID] = item

	label := "Renamed"
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: item.ID, Label: &label}); err != nil {
		t.Errorf("a label-only update = %v, want nil", err)
	}
	blank := "  "
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: item.ID, Label: &blank}); err == nil {
		t.Error("a blank label was accepted")
	}

	// adding a URL on top of the existing entity target breaks the XOR
	url := "https://example.invalid"
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: item.ID, URL: &url}); !errors.Is(err, ErrTargetXorURL) {
		t.Errorf("adding a URL to an entity-targeted item = %v, want ErrTargetXorURL", err)
	}
	// swapping to a URL item means clearing the target in the same call
	none := TargetType("")
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: item.ID, TargetType: &none, URL: &url}); err != nil {
		t.Errorf("swapping an entity target for a URL = %v, want nil", err)
	}
}

func TestUpdateMenuItemRejectsUnknownItems(t *testing.T) {
	svc, _ := newTestService()
	label := "x"
	if _, err := svc.UpdateMenuItem(context.Background(), UpdateMenuItemInput{ID: uuid.New(), Label: &label}); !errors.Is(err, ErrMenuItemNotFound) {
		t.Errorf("an unknown item = %v, want ErrMenuItemNotFound", err)
	}
}

// The drag-reorder ships the whole list, so one item belonging to another menu has to stop the batch
// rather than quietly re-home it
func TestReorderRefusesItemsFromAnotherMenu(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	mine := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, Label: "mine", URL: "/m"}
	theirs := MenuItem{ID: uuid.New(), MenuID: uuid.New(), Label: "theirs", URL: "/t"}
	repo.items[mine.ID] = mine
	repo.items[theirs.ID] = theirs

	err := svc.ReorderMenuItems(ctx, repo.menu.ID, []ReorderMenuItem{{ID: mine.ID, SortOrder: 0}, {ID: theirs.ID, SortOrder: 1}})
	if !errors.Is(err, ErrParentInOtherMenu) {
		t.Errorf("Reorder across menus = %v, want ErrParentInOtherMenu", err)
	}
	if len(repo.updated) != 1 {
		t.Errorf("%d updates ran, want only the item that passed the check before the batch stopped", len(repo.updated))
	}
}

// Dropping an item back to the top level is the common reorder, and it only works if the nil parent
// is translated into an explicit clear
func TestReorderClearsTheParentWhenAnItemMovesToTheRoot(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	root := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, Label: "root", URL: "/"}
	repo.items[root.ID] = root
	nested := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, ParentID: &root.ID, Label: "nested", URL: "/n"}
	repo.items[nested.ID] = nested

	if err := svc.ReorderMenuItems(ctx, repo.menu.ID, []ReorderMenuItem{{ID: nested.ID, SortOrder: 3}}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if len(repo.updated) != 1 {
		t.Fatalf("%d updates ran, want 1", len(repo.updated))
	}
	got := repo.updated[0]
	if !got.ClearParent || got.ParentID != nil {
		t.Errorf("update = {parent %v clear %v}, want the parent explicitly cleared", got.ParentID, got.ClearParent)
	}
	if got.SortOrder == nil || *got.SortOrder != 3 {
		t.Errorf("sortOrder = %v, want 3", got.SortOrder)
	}

	// an item that was already at the root must not be flagged as a clear it doesn't need
	repo.updated = nil
	if err := svc.ReorderMenuItems(ctx, repo.menu.ID, []ReorderMenuItem{{ID: root.ID, SortOrder: 1}}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if repo.updated[0].ClearParent {
		t.Error("a root-level item was flagged ClearParent, want no parent change")
	}
}

// The admin UI posts the full assignment list per save, so the replace has to be a clean sweep and
// a duplicated channel must not become two rows
func TestSetMenuStoresReplacesAndDeduplicates(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	a, b := uuid.New(), uuid.New()

	if err := svc.SetMenuStores(ctx, repo.menu.ID, []uuid.UUID{a, b, a, b, a}); err != nil {
		t.Fatalf("SetMenuStores: %v", err)
	}
	if len(repo.assignments) != 2 || repo.assignments[0] != a || repo.assignments[1] != b {
		t.Errorf("assignments = %v, want [%v %v]", repo.assignments, a, b)
	}
	if repo.cleared != 1 {
		t.Errorf("cleared %d times, want exactly 1 before the inserts", repo.cleared)
	}

	// an empty list detaches the menu from every channel
	if err := svc.SetMenuStores(ctx, repo.menu.ID, nil); err != nil {
		t.Fatalf("SetMenuStores with an empty list: %v", err)
	}
	if len(repo.assignments) != 0 {
		t.Errorf("assignments = %v, want none", repo.assignments)
	}
}

func TestSetMenuStoresRejectsAnUnknownMenu(t *testing.T) {
	svc, repo := newTestService()
	if err := svc.SetMenuStores(context.Background(), uuid.New(), []uuid.UUID{uuid.New()}); !errors.Is(err, ErrMenuNotFound) {
		t.Errorf("SetMenuStores on an unknown menu = %v, want ErrMenuNotFound", err)
	}
	if repo.cleared != 0 {
		t.Error("assignments were cleared for a menu that does not exist")
	}
}

type stubLocales struct{ known map[string]bool }

func (s stubLocales) EnsureExists(_ context.Context, locale string) error {
	if !s.known[locale] {
		return errors.New("unknown locale " + locale)
	}
	return nil
}

// A translation row keyed on a locale the store doesn't have is invisible forever, so the write is
// gated on the locale actually existing
func TestUpsertTranslationValidatesItsInputs(t *testing.T) {
	svc, _ := newTestService()
	svc.SetLocaleValidator(stubLocales{known: map[string]bool{"de": true}})
	ctx := context.Background()
	id := uuid.New()

	if _, err := svc.UpsertMenuItemTranslation(ctx, id, "  ", "Laden"); err == nil {
		t.Error("a blank locale was accepted")
	}
	if _, err := svc.UpsertMenuItemTranslation(ctx, id, "de", "   "); err == nil {
		t.Error("a blank label was accepted")
	}
	if _, err := svc.UpsertMenuItemTranslation(ctx, id, "es", "Tienda"); err == nil {
		t.Error("a locale the store doesn't have was accepted")
	}
	if _, err := svc.UpsertMenuItemTranslation(ctx, id, "de", "Laden"); err != nil {
		t.Errorf("the valid translation = %v, want nil", err)
	}
}

// The storefront reads menus through a cache the service is responsible for dropping, so every
// mutation has to announce itself
func TestMutationsPublishMenuUpdated(t *testing.T) {
	svc, repo := newTestService()
	bus := events.NewBus()
	var seen atomic.Int64
	var slugs sync.Map
	bus.Subscribe(EventMenuUpdated, func(_ context.Context, e events.Event) error {
		seen.Add(1)
		if u, ok := e.(*MenuUpdated); ok {
			slugs.Store(u.Slug, true)
		}
		return nil
	})
	svc.SetEventBus(bus)
	ctx := context.Background()

	if _, err := svc.CreateMenu(ctx, CreateMenuInput{Slug: "footer", Name: "Footer"}); err != nil {
		t.Fatalf("CreateMenu: %v", err)
	}
	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{MenuID: repo.menu.ID, Label: "Shop", URL: "/shop"}); err != nil {
		t.Fatalf("CreateMenuItem: %v", err)
	}
	if err := bus.Drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if got := seen.Load(); got != 2 {
		t.Errorf("%d menuUpdated events, want one per mutation", got)
	}
	for _, slug := range []string{"footer", repo.menu.Slug} {
		if _, ok := slugs.Load(slug); !ok {
			t.Errorf("no menuUpdated carried slug %q, so the storefront cache for it never drops", slug)
		}
	}
}

func (r *fakeRepo) InTx(_ context.Context, fn func(Repository) error) error { return fn(r) }

func (r *fakeRepo) ListMenuItemsByMenu(_ context.Context, menuID uuid.UUID) ([]MenuItem, error) {
	var out []MenuItem
	for _, it := range r.items {
		if it.MenuID == menuID {
			out = append(out, it)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteMenuItemTranslation(context.Context, uuid.UUID, string) error { return nil }

// Reorder, store assignment and translations skipped the publish, so the storefront kept serving the old menu until its TTL ran out
func TestEveryWritePublishesMenuUpdated(t *testing.T) {
	svc, repo := newTestService()
	bus := events.NewBus()
	var seen atomic.Int64
	bus.Subscribe(EventMenuUpdated, func(context.Context, events.Event) error { seen.Add(1); return nil })
	svc.SetEventBus(bus)
	ctx := context.Background()
	item, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{MenuID: repo.menu.ID, Label: "Shop", URL: "/shop"})
	if err != nil {
		t.Fatal(err)
	}
	_ = bus.Drain(ctx)
	seen.Store(0)

	for name, op := range map[string]func() error{
		"reorder": func() error {
			return svc.ReorderMenuItems(ctx, repo.menu.ID, []ReorderMenuItem{{ID: item.ID, SortOrder: 3}})
		},
		"set stores":         func() error { return svc.SetMenuStores(ctx, repo.menu.ID, []uuid.UUID{uuid.New()}) },
		"upsert translation": func() error { _, err := svc.UpsertMenuItemTranslation(ctx, item.ID, "de", "Laden"); return err },
		"delete translation": func() error { return svc.DeleteMenuItemTranslation(ctx, item.ID, "de") },
	} {
		before := seen.Load()
		if err := op(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_ = bus.Drain(ctx)
		if seen.Load() != before+1 {
			t.Errorf("%s published %d menuUpdated events, want 1", name, seen.Load()-before)
		}
	}
}

// Only the parent's depth was checked, so a three-level branch could land under a deep parent
func TestMovingABranchCountsItsOwnDepth(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	parent := repo.chain(MaxDepth - 2) // children of this sit at MaxDepth-1
	branch := repo.chain(1)
	branch.URL = "/b"
	repo.items[branch.ID] = branch
	child := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, ParentID: &branch.ID, Label: "c", URL: "/c"}
	repo.items[child.ID] = child
	grandchild := MenuItem{ID: uuid.New(), MenuID: repo.menu.ID, ParentID: &child.ID, Label: "g", URL: "/g"}
	repo.items[grandchild.ID] = grandchild

	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: branch.ID, ParentID: &parent.ID}); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("a three-level branch under level %d = %v, want ErrDepthExceeded", MaxDepth-2, err)
	}
	if _, err := svc.UpdateMenuItem(ctx, UpdateMenuItemInput{ID: grandchild.ID, ParentID: &parent.ID}); err != nil {
		t.Fatalf("a leaf under level %d = %v, want it accepted", MaxDepth-2, err)
	}
}

func TestLinkURLsAreSchemeChecked(t *testing.T) {
	for url, ok := range map[string]bool{
		"/sale": true, "sale": true, "#top": true, "?q=1": true, "https://shop.test/x": true, "HTTP://shop.test": true,
		"mailto:hi@shop.test": true, "tel:+381111": true,
		"javascript:alert(1)": false, "JavaScript:alert(1)": false, "data:text/html,<script>": false,
		"vbscript:x": false, "java\tscript:alert(1)": false, " javascript:alert(1)": false,
	} {
		err := validateTarget("", nil, url)
		if (err == nil) != ok {
			t.Errorf("%q = %v, want accepted %v", url, err, ok)
		}
		if !ok && !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q refused with %v, want ErrInvalidURL", url, err)
		}
	}
}

func TestValidationErrorsAreTyped(t *testing.T) {
	svc, repo := newTestService()
	ctx := context.Background()
	if _, err := svc.CreateMenu(ctx, CreateMenuInput{Slug: "x", Name: " "}); !errors.Is(err, ErrNameRequired) {
		t.Errorf("blank name = %v", err)
	}
	if _, err := svc.CreateMenuItem(ctx, CreateMenuItemInput{MenuID: repo.menu.ID, Label: " ", URL: "/x"}); !errors.Is(err, ErrLabelRequired) {
		t.Errorf("blank label = %v", err)
	}
	if _, err := svc.UpsertMenuItemTranslation(ctx, uuid.New(), " ", "x"); !errors.Is(err, ErrLocaleRequired) {
		t.Errorf("blank locale = %v", err)
	}
}

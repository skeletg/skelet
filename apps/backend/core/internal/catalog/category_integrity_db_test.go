package catalog

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type catalogRig struct {
	t      *testing.T
	conn   *sql.DB
	svc    *Service
	repo   *postgresRepository
	bus    *events.Bus
	sfx    string
	mu     sync.Mutex
	events []events.Event
}

func newCatalogRig(t *testing.T) *catalogRig {
	t.Helper()
	conn := testdb.Require(t)
	tx := txrunner.New(conn)
	repo := NewRepository(db.New(conn), tx)
	bus := events.NewBus()
	r := &catalogRig{t: t, conn: conn, repo: repo, bus: bus, sfx: uuid.NewString()[:8]}
	r.svc = NewService(Deps{Repo: repo, Tx: tx, Bus: bus})
	record := func(_ context.Context, e events.Event) error {
		r.mu.Lock()
		r.events = append(r.events, e)
		r.mu.Unlock()
		return nil
	}
	for _, et := range []string{EventCategoryUpdated, EventCollectionUpdated, EventProductUpdated} {
		bus.Subscribe(et, record)
	}
	return r
}

func (r *catalogRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.conn.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *catalogRig) id(q string, args ...any) uuid.UUID {
	r.t.Helper()
	var id uuid.UUID
	if err := r.conn.QueryRow(q, args...).Scan(&id); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
	return id
}

// Rows go in parent-first, so the cleanup unlinks every parent before deleting
func (r *catalogRig) category(parent *uuid.UUID, name string) uuid.UUID {
	r.t.Helper()
	slug := name + "-" + r.sfx
	path := slug
	if parent != nil {
		var pp string
		if err := r.conn.QueryRow(`SELECT url_path FROM catalog_categories WHERE id = $1`, *parent).Scan(&pp); err != nil {
			r.t.Fatal(err)
		}
		path = pp + "/" + slug
	}
	id := r.id(`INSERT INTO catalog_categories (parent_id, slug, url_path, is_published) VALUES ($1, $2, $3, TRUE) RETURNING id`, parent, slug, path)
	r.t.Cleanup(func() {
		_, _ = r.conn.Exec(`UPDATE catalog_categories SET parent_id = NULL WHERE id = $1 OR parent_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_product_categories WHERE category_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, id)
	})
	return id
}

func (r *catalogRig) product(sku, template string) uuid.UUID {
	r.t.Helper()
	typeID := r.id(`INSERT INTO catalog_product_types (slug, name, url_template) VALUES ($1, 'rig', $2) RETURNING id`, "rig-"+sku+"-"+r.sfx, template)
	id := r.id(`INSERT INTO catalog_products (product_type_id, sku, url, kind, url_auto, default_price) VALUES ($1, $2, $3, 'simple', $4, 1000) RETURNING id`,
		typeID, sku+"-"+r.sfx, strings.ToLower(sku)+"-"+r.sfx, template != "")
	r.t.Cleanup(func() {
		_, _ = r.conn.Exec(`DELETE FROM catalog_product_categories WHERE product_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_product_channel_listings WHERE product_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM inventory_items WHERE product_id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, id)
		_, _ = r.conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
	})
	return id
}

func (r *catalogRig) channel() uuid.UUID {
	r.t.Helper()
	id := r.id(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'rig', 'USD') RETURNING id`, "rig-"+r.sfx+"-"+uuid.NewString()[:4])
	r.t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM channels WHERE id = $1`, id) })
	return id
}

func (r *catalogRig) urlPath(id uuid.UUID) string {
	r.t.Helper()
	var p string
	if err := r.conn.QueryRow(`SELECT url_path FROM catalog_categories WHERE id = $1`, id).Scan(&p); err != nil {
		r.t.Fatal(err)
	}
	return p
}

func (r *catalogRig) productURL(id uuid.UUID) string {
	r.t.Helper()
	var u string
	if err := r.conn.QueryRow(`SELECT url FROM catalog_products WHERE id = $1`, id).Scan(&u); err != nil {
		r.t.Fatal(err)
	}
	return u
}

// Events published so far, once every async handler has returned
func (r *catalogRig) published() []events.Event {
	r.t.Helper()
	if err := r.bus.WaitIdle(context.Background()); err != nil {
		r.t.Fatal(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

func (r *catalogRig) reset() {
	_ = r.bus.WaitIdle(context.Background())
	r.mu.Lock()
	r.events = nil
	r.mu.Unlock()
}

func TestACategoryCannotMoveUnderItselfOrADescendant(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	root := r.category(nil, "root")
	mid := r.category(&root, "mid")
	leaf := r.category(&mid, "leaf")

	for name, parent := range map[string]uuid.UUID{"itself": root, "a child": mid, "a grandchild": leaf} {
		p := parent
		if _, err := r.svc.UpdateCategory(ctx, UpdateCategoryInput{ID: root, ParentID: &p}); !errors.Is(err, ErrCategoryCycle) {
			t.Errorf("moving root under %s: err = %v, want ErrCategoryCycle", name, err)
		}
	}
	var parent uuid.NullUUID
	if err := r.conn.QueryRow(`SELECT parent_id FROM catalog_categories WHERE id = $1`, root).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent.Valid {
		t.Fatalf("a refused move still re-parented root under %s", parent.UUID)
	}

	other := r.category(nil, "other")
	if _, err := r.svc.UpdateCategory(ctx, UpdateCategoryInput{ID: mid, ParentID: &other}); err != nil {
		t.Fatalf("a legal move was refused: %v", err)
	}
	if got, want := r.urlPath(leaf), r.urlPath(other)+"/mid-"+r.sfx+"/leaf-"+r.sfx; got != want {
		t.Errorf("leaf path = %q, want %q - the subtree didn't move with its root", got, want)
	}
}

func TestTheTreeDepthIsCapped(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	var parent *uuid.UUID
	var chain []uuid.UUID
	for i := range maxCategoryDepth - 1 {
		id := r.category(parent, "d"+strings.Repeat("x", i%3)+uuid.NewString()[:6])
		chain = append(chain, id)
		parent = &chain[len(chain)-1]
	}
	in := CreateCategoryInput{ParentID: parent, Slug: "last-fits-" + r.sfx, Locale: "en", Name: "fits"}
	last, err := r.svc.CreateCategory(ctx, in)
	if err != nil {
		t.Fatalf("the deepest allowed level was refused: %v", err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM catalog_categories WHERE id = $1`, last.ID) })
	in = CreateCategoryInput{ParentID: &last.ID, Slug: "too-deep-" + r.sfx, Locale: "en", Name: "deep"}
	if _, err := r.svc.CreateCategory(ctx, in); !errors.Is(err, ErrCategoryTooDeep) {
		t.Errorf("create past the cap: err = %v, want ErrCategoryTooDeep", err)
	}

	branch := r.category(nil, "branch")
	r.category(&branch, "twig")
	if _, err := r.svc.UpdateCategory(ctx, UpdateCategoryInput{ID: branch, ParentID: &chain[len(chain)-1]}); !errors.Is(err, ErrCategoryTooDeep) {
		t.Errorf("moving a two-level branch under the deepest node: err = %v, want ErrCategoryTooDeep", err)
	}
}

// Rows written before the cycle guard existed must not hang the storefront or the move cascade
func TestAPreexistingCycleDoesNotHangTheTreeWalks(t *testing.T) {
	r := newCatalogRig(t)
	a := r.category(nil, "cyc-a")
	b := r.category(&a, "cyc-b")
	if _, err := r.conn.Exec(`UPDATE catalog_categories SET parent_id = $1 WHERE id = $2`, b, a); err != nil {
		t.Skipf("schema refuses a parent cycle, nothing left to guard: %v", err)
	}
	ch := r.channel()
	r.exec(`INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published) VALUES ($1, $3, TRUE), ($2, $3, TRUE)`, a, b, ch)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	visible, err := r.repo.FilterFullyVisibleCategoryIDsForChannel(ctx, []uuid.UUID{a, b}, ch)
	if err != nil {
		t.Fatalf("visibility walk: %v", err)
	}
	if len(visible) != 0 {
		t.Errorf("a cycle never reaches a root, so it must read as hidden - got %v", visible)
	}
	if _, err := r.repo.ListProductIDsInPrimaryCategoryDescendants(ctx, a); err != nil {
		t.Errorf("descendant walk: %v", err)
	}
	if _, err := r.repo.CategorySubtreeHeight(ctx, a); err != nil {
		t.Errorf("height walk: %v", err)
	}
	if _, err := r.repo.RewriteCategorySubtreeURLPaths(ctx, a, "cyc"); err != nil {
		t.Errorf("path rewrite: %v", err)
	}
	all := []Category{{ID: a, ParentID: &b}, {ID: b, ParentID: &a}}
	if got := buildCategoryTree(all, &a); len(got) > 2 {
		t.Errorf("tree from a cycle node kept nesting: %d", len(got))
	}
}

// The rename commits with an outbox row, and only the relay re-renders the products - each one announced so search and caches follow
func TestARenameQueuesTheProductURLRegenerationDurably(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	ob := outbox.New(db.New(r.conn), r.bus)
	RegisterOutboxTypes(ob)
	r.svc.SetOutbox(ob)
	r.svc.RegisterSubscribers(r.bus)
	for ob.DrainPending(ctx) > 0 {
	}

	root := r.category(nil, "shoes")
	leaf := r.category(&root, "running")
	pid := r.product("RUN", "{category}/{name}")
	r.exec(`INSERT INTO catalog_product_categories (product_id, category_id, is_primary) VALUES ($1, $2, TRUE)`, pid, leaf)
	if err := r.svc.regenerateProductURLPath(ctx, pid, true); err != nil {
		t.Fatal(err)
	}
	before := r.productURL(pid)
	if !strings.HasPrefix(before, "shoes-"+r.sfx+"/running-"+r.sfx+"/") {
		t.Fatalf("fixture url = %q, want it under the category path", before)
	}
	r.reset()

	slug := "footwear-" + r.sfx
	if _, err := r.svc.UpdateCategory(ctx, UpdateCategoryInput{ID: root, Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	if got := r.urlPath(leaf); got != slug+"/running-"+r.sfx {
		t.Errorf("descendant path = %q, the rename didn't reach it", got)
	}
	if got := r.productURL(pid); got != before {
		t.Errorf("product url changed inside the request (%q) - the regeneration belongs on the outbox", got)
	}
	var updated []uuid.UUID
	for _, e := range r.published() {
		if u, ok := e.(*CategoryUpdated); ok {
			updated = append(updated, u.CategoryID)
		}
	}
	if !slices.Contains(updated, root) || !slices.Contains(updated, leaf) {
		t.Errorf("category.updated went out for %v, want the renamed root and the moved leaf", updated)
	}
	r.reset()

	if n := ob.DrainPending(ctx); n == 0 {
		t.Fatal("the rename enqueued nothing for the relay")
	}
	if got := r.productURL(pid); !strings.HasPrefix(got, slug+"/running-"+r.sfx+"/") {
		t.Errorf("after the relay the product url = %q, want it under %q", got, slug)
	}
	announced := false
	for _, e := range r.published() {
		if u, ok := e.(*ProductUpdated); ok && u.ProductID == pid {
			announced = true
		}
	}
	if !announced {
		t.Error("the regenerated product published no product.updated")
	}
}

func TestDeleteCategoryRefusesOneWithChildren(t *testing.T) {
	r := newCatalogRig(t)
	parent := r.category(nil, "has-kids")
	r.category(&parent, "kid")
	if err := r.svc.DeleteCategory(context.Background(), parent); !errors.Is(err, ErrCategoryHasChildren) {
		t.Fatalf("err = %v, want ErrCategoryHasChildren", err)
	}
	if err := r.svc.DeleteCategory(context.Background(), uuid.New()); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("deleting an unknown category: err = %v, want ErrCategoryNotFound", err)
	}
}

func TestRemovingThePrimaryCategoryAnnouncesIt(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	primary, other := r.category(nil, "prim"), r.category(nil, "second")
	pid := r.product("PRIM", "{category}/{name}")
	r.exec(`INSERT INTO catalog_product_categories (product_id, category_id, is_primary) VALUES ($1, $2, TRUE), ($1, $3, FALSE)`, pid, primary, other)
	r.reset()

	if _, err := r.svc.RemoveProductFromCategory(ctx, pid, primary); err != nil {
		t.Fatal(err)
	}
	var changed []string
	for _, e := range r.published() {
		if u, ok := e.(*ProductUpdated); ok && u.ProductID == pid {
			changed = u.ChangedFields
		}
	}
	if !slices.Contains(changed, "primaryCategory") {
		t.Errorf("product.updated changed = %v, want primaryCategory", changed)
	}
}

// Re-adding a member moves it instead of failing on the primary key
func TestAddingAnExistingMemberUpdatesIt(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	cat := r.category(nil, "member")
	pid := r.product("MEMB", "")
	r.exec(`INSERT INTO catalog_product_categories (product_id, category_id, position) VALUES ($1, $2, 0)`, pid, cat)

	if _, err := r.svc.AddProductToCategory(ctx, pid, cat, 5, true); err != nil {
		t.Fatalf("re-adding a member: %v", err)
	}
	var pos int
	var primary bool
	if err := r.conn.QueryRow(`SELECT position, is_primary FROM catalog_product_categories WHERE product_id = $1 AND category_id = $2`, pid, cat).Scan(&pos, &primary); err != nil {
		t.Fatal(err)
	}
	if pos != 5 || !primary {
		t.Errorf("membership = position %d primary %v, want 5 and primary", pos, primary)
	}
}

func TestCoverWritesMapMissingRowsAndRaces(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	missing := uuid.New()
	if _, err := r.repo.SetCategoryCover(ctx, missing, uuid.New(), "k", "image/png", 1, ""); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("set cover on a missing category: %v", err)
	}
	if _, err := r.repo.ClearCategoryCover(ctx, missing); !errors.Is(err, ErrCategoryNotFound) {
		t.Errorf("clear cover on a missing category: %v", err)
	}
	if _, err := r.repo.SetCollectionCover(ctx, missing, uuid.New(), "k", "image/png", 1, ""); !errors.Is(err, ErrCollectionNotFound) {
		t.Errorf("set cover on a missing collection: %v", err)
	}

	cat := r.category(nil, "covered")
	first, second := uuid.New(), uuid.New()
	if _, err := r.repo.SetCategoryCover(ctx, cat, first, "k1", "image/png", 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.repo.SetCategoryCover(ctx, cat, second, "k2", "image/png", 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.repo.ConfirmCategoryCover(ctx, cat, first, 10, 10); !errors.Is(err, ErrCategoryCoverNotPending) {
		t.Errorf("confirming a cover that was replaced meanwhile: %v, want ErrCategoryCoverNotPending", err)
	}
	if _, err := r.repo.ConfirmCategoryCover(ctx, cat, second, 10, 10); err != nil {
		t.Fatalf("confirming the current cover: %v", err)
	}
	if _, err := r.repo.ConfirmCategoryCover(ctx, cat, second, 10, 10); !errors.Is(err, ErrCategoryCoverNotPending) {
		t.Errorf("confirming twice: %v, want ErrCategoryCoverNotPending", err)
	}

	col := r.id(`INSERT INTO catalog_collections (slug) VALUES ($1) RETURNING id`, "covered-col-"+r.sfx)
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM catalog_collections WHERE id = $1`, col) })
	if _, err := r.repo.SetCollectionCover(ctx, col, first, "k1", "image/png", 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.repo.ConfirmCollectionCover(ctx, col, second, 10, 10); !errors.Is(err, ErrCollectionCoverNotPending) {
		t.Errorf("confirming another collection cover id: %v, want ErrCollectionCoverNotPending", err)
	}
}

// Storefront menus follow the channel override, then the global position, then the slug
func TestStorefrontCategoriesComeBackInMerchandisingOrder(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	ch := r.channel()
	parent := r.category(nil, "aaa-parent")
	r.exec(`UPDATE catalog_categories SET position = -100 WHERE id = $1`, parent)
	b, a, c := r.category(&parent, "b"), r.category(&parent, "a"), r.category(&parent, "c")
	r.exec(`UPDATE catalog_categories SET position = 1 WHERE id = ANY($1)`, []uuid.UUID{a, b})
	r.exec(`UPDATE catalog_categories SET position = 5 WHERE id = $1`, c)
	for _, id := range []uuid.UUID{parent, a, b, c} {
		r.exec(`INSERT INTO catalog_category_channel_listings (category_id, channel_id, is_published) VALUES ($1, $2, TRUE)`, id, ch)
	}
	r.exec(`UPDATE catalog_category_channel_listings SET sort_order_override = 0 WHERE category_id = $1`, c)
	want := []uuid.UUID{c, a, b}

	kids, err := r.repo.ListPublishedCategoryChildrenForChannel(ctx, ch, parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := categoryIDs(kids); !slices.Equal(got, want) {
		t.Errorf("children order = %v, want override first then position then slug %v", got, want)
	}
	all, err := r.repo.ListPublishedCategoriesForChannel(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	if got := categoryIDs(all); !slices.Equal(got, append([]uuid.UUID{parent}, want...)) {
		t.Errorf("flat order = %v", got)
	}
	roots, err := r.repo.ListPublishedRootCategoriesForChannel(ctx, ch)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].ID != parent {
		t.Errorf("roots = %v, want only the parent", categoryIDs(roots))
	}
}

// Every write the storefront renders from has to announce itself, or its pages stay cached
func TestStorefrontVisibleWritesPublishUpdates(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	cat := r.category(nil, "evented")
	ch := r.channel()
	col := r.id(`INSERT INTO catalog_collections (slug) VALUES ($1) RETURNING id`, "evented-col-"+r.sfx)
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM catalog_collections WHERE id = $1`, col) })
	pid := r.product("EVT", "")
	r.exec(`INSERT INTO catalog_collection_products (collection_id, product_id, sort_order) VALUES ($1, $2, 0)`, col, pid)

	for name, write := range map[string]func() error{
		"category listing": func() error {
			_, err := r.svc.SetCategoryChannelListing(ctx, SetCategoryChannelListingInput{CategoryID: cat, ChannelID: ch, IsPublished: true})
			return err
		},
		"category listing cleared": func() error { _, err := r.svc.ClearCategoryChannelListing(ctx, cat, ch); return err },
		"category content": func() error {
			_, err := r.svc.UpsertCategoryContent(ctx, cat, CategoryContentInput{Locale: "en", Name: "Evented"})
			return err
		},
		"category content deleted": func() error { return r.svc.DeleteCategoryContent(ctx, cat, "en") },
		"category facets":          func() error { return r.svc.SetCategoryFacetOverrides(ctx, cat, nil) },
		"category cover cleared":   func() error { _, err := r.svc.ClearCategoryCover(ctx, cat); return err },
		"collection content": func() error {
			_, err := r.svc.UpsertCollectionContent(ctx, col, CollectionContentInput{Locale: "en", Name: "Evented"})
			return err
		},
		"collection content deleted": func() error { return r.svc.DeleteCollectionContent(ctx, col, "en") },
		"collection order":           func() error { _, err := r.svc.ReorderCollectionProducts(ctx, col, []uuid.UUID{pid}); return err },
		"collection facets":          func() error { return r.svc.SetCollectionFacetOverrides(ctx, col, nil) },
		"collection cover cleared":   func() error { _, err := r.svc.ClearCollectionCover(ctx, col); return err },
	} {
		r.reset()
		if err := write(); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got := false
		for _, e := range r.published() {
			switch u := e.(type) {
			case *CategoryUpdated:
				got = got || u.CategoryID == cat
			case *CollectionUpdated:
				got = got || u.CollectionID == col
			}
		}
		if !got {
			t.Errorf("%s published no update event", name)
		}
	}
}

func TestReorderingACollectionIsAllOrNothing(t *testing.T) {
	r := newCatalogRig(t)
	ctx := context.Background()
	col := r.id(`INSERT INTO catalog_collections (slug) VALUES ($1) RETURNING id`, "reorder-col-"+r.sfx)
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM catalog_collections WHERE id = $1`, col) })
	a, b := r.product("RA", ""), r.product("RB", "")
	r.exec(`INSERT INTO catalog_collection_products (collection_id, product_id, sort_order) VALUES ($1, $2, 0), ($1, $3, 1)`, col, a, b)

	if _, err := r.svc.ReorderCollectionProducts(ctx, col, []uuid.UUID{b, uuid.New(), a}); !errors.Is(err, ErrCollectionProductNotFound) {
		t.Errorf("reorder naming a non-member: err = %v, want ErrCollectionProductNotFound", err)
	}
	var first uuid.UUID
	if err := r.conn.QueryRow(`SELECT product_id FROM catalog_collection_products WHERE collection_id = $1 ORDER BY sort_order LIMIT 1`, col).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if first != a {
		t.Error("a refused reorder still moved the products it reached first")
	}
	if _, err := r.svc.ReorderCollectionProducts(ctx, col, make([]uuid.UUID, MaxUnpagedRows+1)); !errors.Is(err, ErrTooManyItems) {
		t.Errorf("an oversized reorder: err = %v, want ErrTooManyItems", err)
	}
}

func TestFacetOverridesNamingAnUnknownAttributeAreRefused(t *testing.T) {
	r := newCatalogRig(t)
	cat := r.category(nil, "facets")
	err := r.svc.SetCategoryFacetOverrides(context.Background(), cat, []FacetOverride{{AttributeID: uuid.New()}})
	if !errors.Is(err, ErrAttributeNotFound) {
		t.Errorf("err = %v, want ErrAttributeNotFound", err)
	}
	if err := r.svc.SetCategoryFacetOverrides(context.Background(), cat, make([]FacetOverride, maxFacetOverrides+1)); !errors.Is(err, ErrTooManyItems) {
		t.Errorf("an oversized override set: err = %v, want ErrTooManyItems", err)
	}
}

// A category whose only translation is in a third locale still gets a name, from one batched read
func TestTheLastResortTranslationIsBatched(t *testing.T) {
	r := newCatalogRig(t)
	cat := r.category(nil, "only-fr")
	r.exec(`INSERT INTO locales (code, name) VALUES ('fr', 'fr') ON CONFLICT DO NOTHING`)
	r.exec(`INSERT INTO catalog_category_content (category_id, locale, name) VALUES ($1, 'fr', 'Seulement')`, cat)
	cats := []Category{{ID: cat}}
	if err := r.svc.populateCategoryLocale(context.Background(), cats, "en"); err != nil {
		t.Fatal(err)
	}
	if cats[0].Content == nil || cats[0].Content.Name != "Seulement" {
		t.Errorf("content = %+v, want the fr row as the last resort", cats[0].Content)
	}
}

func TestChangedCategoryFieldsNameEveryInput(t *testing.T) {
	s, p, b := "x", int32(1), true
	m := Metadata{}
	got := changedCategoryFields(UpdateCategoryInput{Slug: &s, ClearParent: true, Position: &p, IsPublished: &b, SEONoindex: &b, Metadata: &m, PrivateMetadata: &m})
	want := []string{"slug", "parent_id", "position", "is_published", "seo_noindex", "metadata", "private_metadata"}
	if !slices.Equal(got, want) {
		t.Errorf("changed = %v, want %v", got, want)
	}
}

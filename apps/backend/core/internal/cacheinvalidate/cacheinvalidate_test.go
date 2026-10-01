package cacheinvalidate

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/cms"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/menu"
	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func TestMain(m *testing.M) {
	batchWindow = time.Hour // tests flush explicitly, so the timer never races an assertion
	os.Exit(m.Run())
}

type purgeCall struct {
	surface string
	tags    []string
	paths   []string
	reason  string
}

type fakePurger struct {
	mu       sync.Mutex
	purges   []purgeCall
	surfaces []string
}

func (f *fakePurger) Purge(_ context.Context, surface string, tags, paths []string, reason string, _ *uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.purges = append(f.purges, purgeCall{surface, tags, paths, reason})
}
func (f *fakePurger) PurgeSurface(_ context.Context, key, _ string, _ *uuid.UUID) {
	f.surfaces = append(f.surfaces, key)
}

func (f *fakePurger) on(surface string) []purgeCall {
	var out []purgeCall
	for _, p := range f.purges {
		if p.surface == surface {
			out = append(out, p)
		}
	}
	return out
}

type fakeQueue struct {
	enqueued []purgeCall
	err      error
}

func (f *fakeQueue) EnqueuePaths(_ context.Context, surface string, paths, tags []string, _ string, _ *uuid.UUID) (int, error) {
	f.enqueued = append(f.enqueued, purgeCall{surface: surface, tags: tags, paths: paths})
	return len(paths), f.err
}

type fakePaths struct{}

func (fakePaths) CanonicalPaths(context.Context, urlrewrite.EntityType, uuid.UUID) []string {
	return []string{"/p/thing"}
}

type fakeConfig struct {
	strings  map[string]string
	bools    map[string]bool
	onChange []func(context.Context, config.Definition)
}

func (f *fakeConfig) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f.strings[path], nil
}
func (f *fakeConfig) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return f.bools[path], nil
}
func (f *fakeConfig) OnChange(fn func(context.Context, config.Definition)) {
	f.onChange = append(f.onChange, fn)
}

type rig struct {
	bus    *events.Bus
	purger *fakePurger
	queue  *fakeQueue
	cfg    *fakeConfig
	batch  *Batcher
}

func newRig(mode string, toggles map[string]bool) rig {
	r := rig{bus: events.NewBus(), purger: &fakePurger{}, queue: &fakeQueue{},
		cfg: &fakeConfig{strings: map[string]string{catalog.CacheProductMode: mode}, bools: toggles}}
	r.batch = Register(r.bus, r.purger, r.queue, fakePaths{}, r.cfg)
	return r
}

func (r rig) fire(t *testing.T, e events.Event) error {
	t.Helper()
	err := r.bus.DispatchAsyncNow(context.Background(), e)
	r.batch.Flush(context.Background())
	return err
}

// cache/product/mode picks off, invalidate or rebuild, and each trigger has its own toggle
func TestProductModeBranching(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         string
		triggerOn    bool
		wantPurges   int
		wantEnqueued int
	}{
		{"mode off purges nothing", "off", true, 0, 0},
		{"mode unset purges nothing", "", true, 0, 0},
		{"trigger off purges nothing", "invalidate", false, 0, 0},
		{"invalidate purges", "invalidate", true, 1, 0},
		{"rebuild enqueues instead", "rebuild", true, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(tc.mode, map[string]bool{catalog.CacheProductOnUpdate: tc.triggerOn})
			_ = r.fire(t, &catalog.ProductUpdated{ProductID: uuid.New()})

			if got := len(r.purger.on(catalog.CacheSurfaceProduct)); got != tc.wantPurges {
				t.Errorf("product purges = %d, want %d", got, tc.wantPurges)
			}
			if len(r.queue.enqueued) != tc.wantEnqueued {
				t.Errorf("enqueued = %d, want %d", len(r.queue.enqueued), tc.wantEnqueued)
			}
			if len(r.purger.on(catalog.CacheSurfaceCatalog)) == 0 {
				t.Error("the grid bust must fire whatever the product mode")
			}
		})
	}
}

func TestStockChangeFollowsItsOwnToggle(t *testing.T) {
	r := newRig("invalidate", map[string]bool{catalog.CacheProductOnStock: true})
	_ = r.fire(t, &inventory.StockChanged{ProductID: uuid.New()})
	if len(r.purger.on(catalog.CacheSurfaceProduct)) != 1 {
		t.Errorf("stock change purges = %+v, want the one PDP purge", r.purger.purges)
	}
}

// A deleted product can't be re-rendered, so rebuild mode must still invalidate
func TestDeleteNeverRebuilds(t *testing.T) {
	r := newRig("rebuild", map[string]bool{catalog.CacheProductOnDelete: true})
	_ = r.fire(t, &catalog.ProductDeleted{ProductID: uuid.New()})
	if len(r.queue.enqueued) != 0 {
		t.Fatalf("delete enqueued %d rebuilds, want 0", len(r.queue.enqueued))
	}
	if len(r.purger.on(catalog.CacheSurfaceProduct)) != 1 {
		t.Fatal("delete should still invalidate the PDP")
	}
}

// A failed rebuild enqueue used to vanish - returned, the bus logs it and an outbox delivery retries
func TestAFailedRebuildEnqueueIsReported(t *testing.T) {
	r := newRig("rebuild", map[string]bool{catalog.CacheProductOnUpdate: true})
	r.queue.err = errors.New("db down")
	if err := r.fire(t, &catalog.ProductUpdated{ProductID: uuid.New()}); err == nil {
		t.Error("the enqueue failure was swallowed")
	}
}

func TestEveryNavChangeBustsTheHeader(t *testing.T) {
	for _, e := range []events.Event{
		&menu.MenuUpdated{},
		&channel.Updated{ChannelID: uuid.New()},
		&channel.LocalesChanged{ChannelID: uuid.New()},
		&channel.Deleted{ChannelID: uuid.New()},
		&catalog.CategoryCreated{CategoryID: uuid.New()},
		&catalog.CategoryUpdated{CategoryID: uuid.New()},
		&catalog.CategoryDeleted{CategoryID: uuid.New()},
		&catalog.CollectionCreated{CollectionID: uuid.New()},
		&catalog.CollectionUpdated{CollectionID: uuid.New()},
		&catalog.CollectionDeleted{CollectionID: uuid.New()},
	} {
		r := newRig("", nil)
		_ = r.fire(t, e)
		if !slices.Equal(r.purger.surfaces, []string{menu.CacheSurface}) {
			t.Errorf("%s: busted %v, want the nav surface", e.Type(), r.purger.surfaces)
		}
	}
}

func TestEntityPurgesCarryTagsAndPaths(t *testing.T) {
	id := uuid.New()
	for _, c := range []struct {
		event     events.Event
		surface   string
		wantTags  []string
		wantPaths []string
	}{
		{&cms.PagePublished{PageID: id}, cms.CacheSurface, []string{cmsPageTag(id), tagCMSAll}, []string{"/p/thing"}},
		{&cms.PageUnpublished{PageID: id}, cms.CacheSurface, []string{cmsPageTag(id), tagCMSAll}, []string{"/p/thing"}},
		{&catalog.CategoryUpdated{CategoryID: id}, catalog.CacheSurfaceCatalog, []string{tagBrowse, categoryTag(id)}, []string{"/p/thing"}},
		{&catalog.CategoryDeleted{CategoryID: id}, catalog.CacheSurfaceCatalog, []string{tagBrowse, categoryTag(id)}, nil},
		{&catalog.CollectionCreated{CollectionID: id}, catalog.CacheSurfaceCatalog, []string{tagCollections, collectionTag(id)}, []string{"/p/thing"}},
		{&catalog.CollectionProductsChanged{CollectionID: id}, catalog.CacheSurfaceCatalog, []string{tagBrowse, tagCollections, collectionTag(id)}, []string{"/p/thing"}},
		{&urlrewrite.Changed{Path: "old-name"}, "", nil, []string{"/old-name"}},
	} {
		r := newRig("", nil)
		_ = r.fire(t, c.event)
		got := r.purger.on(c.surface)
		if len(got) != 1 {
			t.Errorf("%s: %d purges on %q, want 1", c.event.Type(), len(got), c.surface)
			continue
		}
		wantTags := slices.Sorted(slices.Values(c.wantTags))
		if !slices.Equal(got[0].tags, wantTags) || !slices.Equal(got[0].paths, c.wantPaths) {
			t.Errorf("%s: tags %v paths %v, want %v %v", c.event.Type(), got[0].tags, got[0].paths, wantTags, c.wantPaths)
		}
	}
}

// The storefront caches public settings under one tag - a private setting never reaches it, so saving one purges nothing
func TestSavingAPublicSettingPurgesTheStorefrontsCopy(t *testing.T) {
	save := func(def config.Definition) []purgeCall {
		r := newRig("", nil)
		for _, fn := range r.cfg.onChange {
			fn(context.Background(), def)
		}
		r.batch.Flush(context.Background())
		return r.purger.on(settingsSurface)
	}
	if got := save(config.Definition{Path: "checkout/guest/enabled", Public: true}); len(got) != 1 || !slices.Equal(got[0].tags, []string{tagPublicSettings}) {
		t.Errorf("public setting purges = %+v, want one purge of %s", got, tagPublicSettings)
	}
	if got := save(config.Definition{Path: "ai/llm/api_key"}); len(got) != 0 {
		t.Errorf("a private setting purged the storefront: %+v", got)
	}
}

// A bulk import fires one event per product - the grid tag must go out once, not once per product
func TestABurstOfEventsSendsOnePurgePerSurface(t *testing.T) {
	r := newRig("invalidate", map[string]bool{catalog.CacheProductOnUpdate: true})
	for range 200 {
		_ = r.bus.DispatchAsyncNow(context.Background(), &catalog.ProductUpdated{ProductID: uuid.New()})
	}
	r.batch.Flush(context.Background())
	grids, pdps := r.purger.on(catalog.CacheSurfaceCatalog), r.purger.on(catalog.CacheSurfaceProduct)
	if len(grids) != 1 || !slices.Equal(grids[0].tags, []string{tagBrowse}) {
		t.Errorf("grid purges = %+v, want one catalog:browse", grids)
	}
	if len(pdps) != 1 || len(pdps[0].tags) != 200 {
		t.Errorf("PDP purges = %d carrying %d tags, want one carrying all 200", len(pdps), len(pdps[0].tags))
	}
	if pdps[0].reason != "product updated" {
		t.Errorf("reason = %q, want each distinct cause once", pdps[0].reason)
	}
}

func TestTheBatchSendsOnItsOwnAfterTheWindow(t *testing.T) {
	purger := &fakePurger{}
	b := NewBatcher(purger, 10*time.Millisecond)
	b.Purge("catalog", []string{tagBrowse}, nil, "x")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		purger.mu.Lock()
		n := len(purger.purges)
		purger.mu.Unlock()
		if n == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("the batch never sent after its window")
}

func productTagged(calls []purgeCall, id uuid.UUID) bool {
	for _, c := range calls {
		if slices.Contains(c.tags, productTag(id)) {
			return true
		}
	}
	return false
}

func TestAListingChangeReachesTheProductPage(t *testing.T) {
	id := uuid.New()
	r := newRig("invalidate", map[string]bool{})
	_ = r.fire(t, &catalog.ProductListingChanged{ProductID: id, IsPublished: false})
	if !productTagged(r.purger.on(catalog.CacheSurfaceProduct), id) {
		t.Error("an unpublished listing left the PDP cached")
	}
	r = newRig("invalidate", map[string]bool{catalog.CacheProductOnUpdate: true})
	_ = r.fire(t, &catalog.ProductListingChanged{ProductID: id, IsPublished: true})
	if !productTagged(r.purger.on(catalog.CacheSurfaceProduct), id) {
		t.Error("a published listing edit didn't purge the PDP")
	}
	if len(r.purger.on(catalog.CacheSurfaceCatalog)) == 0 {
		t.Error("the grids weren't busted")
	}
}

func TestAttributeChangesPurgeTheProductsCarryingThem(t *testing.T) {
	id := uuid.New()
	r := newRig("invalidate", map[string]bool{})
	_ = r.fire(t, &catalog.AttributeUpdated{ProductIDs: []uuid.UUID{id}, ChangedFields: []string{catalog.AttributeFieldIsPublic}})
	if !productTagged(r.purger.on(catalog.CacheSurfaceProduct), id) {
		t.Error("an isPublic flip left the PDP cached")
	}
	if len(r.purger.on(catalog.CacheSurfaceCatalog)) == 0 {
		t.Error("the grids weren't busted")
	}
	r = newRig("invalidate", map[string]bool{})
	_ = r.fire(t, &catalog.AttributeUpdated{ProductIDs: []uuid.UUID{id}, ChangedFields: []string{"name"}})
	if productTagged(r.purger.on(catalog.CacheSurfaceProduct), id) {
		t.Error("a rename purged the PDP with the update toggle off")
	}
	r = newRig("invalidate", map[string]bool{})
	_ = r.fire(t, &catalog.AttributeDeleted{ProductIDs: []uuid.UUID{id}})
	if !productTagged(r.purger.on(catalog.CacheSurfaceProduct), id) {
		t.Error("a deleted attribute left the PDP cached")
	}
}

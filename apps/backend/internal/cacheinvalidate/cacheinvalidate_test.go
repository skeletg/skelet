package cacheinvalidate

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/urlrewrite"
)

type purgeCall struct {
	surface string
	tags    []string
	paths   []string
}

type fakePurger struct {
	purges   []purgeCall
	surfaces []string
}

func (f *fakePurger) Purge(_ context.Context, surface string, tags, paths []string, _ string, _ *uuid.UUID) {
	f.purges = append(f.purges, purgeCall{surface, tags, paths})
}
func (f *fakePurger) PurgeSurface(_ context.Context, key, _ string, _ *uuid.UUID) {
	f.surfaces = append(f.surfaces, key)
}

type fakeQueue struct{ enqueued []purgeCall }

func (f *fakeQueue) EnqueuePaths(_ context.Context, surface string, paths, tags []string, _ string, _ *uuid.UUID) (int, error) {
	f.enqueued = append(f.enqueued, purgeCall{surface, tags, paths})
	return len(paths), nil
}

type fakePaths struct{}

func (fakePaths) CanonicalPaths(context.Context, urlrewrite.EntityType, uuid.UUID) []string {
	return []string{"/p/thing"}
}

type fakeConfig struct {
	strings map[string]string
	bools   map[string]bool
}

func (f fakeConfig) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f.strings[path], nil
}
func (f fakeConfig) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return f.bools[path], nil
}

// The whole reason this package exists: cache/product/mode picks off, invalidate or rebuild, and each
// trigger has its own toggle - a package-level function is what keeps that reachable from a test
func TestProductModeBranching(t *testing.T) {
	for _, tc := range []struct {
		name               string
		mode               string
		triggerOn          bool
		wantPurges         int
		wantEnqueued       int
		wantBrowseGridBust bool
	}{
		{"mode off purges nothing", "off", true, 0, 0, true},
		{"mode unset purges nothing", "", true, 0, 0, true},
		{"trigger off purges nothing", "invalidate", false, 0, 0, true},
		{"invalidate purges", "invalidate", true, 1, 0, true},
		{"rebuild enqueues instead", "rebuild", true, 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			purger, queue := &fakePurger{}, &fakeQueue{}
			cfg := fakeConfig{
				strings: map[string]string{catalog.CacheProductMode: tc.mode},
				bools:   map[string]bool{catalog.CacheProductOnUpdate: tc.triggerOn},
			}
			bus := events.NewBus()
			Register(bus, purger, queue, fakePaths{}, cfg)

			_ = bus.DispatchAsyncNow(context.Background(), &catalog.ProductUpdated{ProductID: uuid.New()})

			// the grid bust is ungated on purpose - it rides the same event
			var pdp, grid int
			for _, p := range purger.purges {
				switch p.surface {
				case "product":
					pdp++
				case "catalog":
					grid++
				}
			}
			if pdp != tc.wantPurges {
				t.Errorf("product purges = %d, want %d", pdp, tc.wantPurges)
			}
			if len(queue.enqueued) != tc.wantEnqueued {
				t.Errorf("enqueued = %d, want %d", len(queue.enqueued), tc.wantEnqueued)
			}
			if tc.wantBrowseGridBust && grid == 0 {
				t.Error("catalog grid bust should fire regardless of the product mode")
			}
		})
	}
}

// A deleted product can't be re-rendered, so rebuild mode must still invalidate
func TestDeleteNeverRebuilds(t *testing.T) {
	purger, queue := &fakePurger{}, &fakeQueue{}
	cfg := fakeConfig{
		strings: map[string]string{catalog.CacheProductMode: "rebuild"},
		bools:   map[string]bool{catalog.CacheProductOnDelete: true},
	}
	bus := events.NewBus()
	Register(bus, purger, queue, fakePaths{}, cfg)

	_ = bus.DispatchAsyncNow(context.Background(), &catalog.ProductDeleted{ProductID: uuid.New()})

	if len(queue.enqueued) != 0 {
		t.Fatalf("delete enqueued %d rebuilds, want 0 - a deleted product cannot be re-rendered", len(queue.enqueued))
	}
	if len(purger.purges) == 0 {
		t.Fatal("delete should still invalidate")
	}
}

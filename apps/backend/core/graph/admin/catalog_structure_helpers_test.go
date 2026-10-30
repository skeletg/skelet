package admin

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
)

// What this pins: setBundleStructure is gated on one bundle, so an id from any other bundle must read as not found
// rather than be edited - only the bundle's own ids and the tempIds its adds named resolve
func TestBundleRefsOnlyResolveTheGatedBundle(t *testing.T) {
	own, created := uuid.New(), uuid.New()
	refs := bundleRefs{options: map[string]uuid.UUID{own.String(): own, "tmp_1": created}, selections: map[string]uuid.UUID{}}
	if id, err := refs.option(own.String()); err != nil || id != own {
		t.Fatalf("own option = %v, %v", id, err)
	}
	if id, err := refs.option("tmp_1"); err != nil || id != created {
		t.Fatalf("temp option = %v, %v", id, err)
	}
	if _, err := refs.option(uuid.NewString()); !errors.Is(err, catalog.ErrBundleOptionNotFound) {
		t.Fatalf("foreign option = %v, want not found", err)
	}
	if _, err := refs.selection(uuid.NewString()); !errors.Is(err, catalog.ErrBundleSelectionNotFound) {
		t.Fatalf("foreign selection = %v, want not found", err)
	}
}

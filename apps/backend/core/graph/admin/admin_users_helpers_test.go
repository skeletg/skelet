package admin

import (
	"testing"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
)

func TestPermissionPickerDropsRowsTheRegistryNoLongerDeclares(t *testing.T) {
	got := registeredPermissionGroups([]adminpkg.PermissionGroup{
		{Slug: "orders", Permissions: []adminpkg.Permission{{Slug: "orders.view"}, {Slug: "orders.renamed_away"}}},
		{Slug: "removed_plugin", Permissions: []adminpkg.Permission{{Slug: "removed_plugin.manage"}}},
	})
	if len(got) != 1 || got[0].Slug != "orders" {
		t.Fatalf("groups: got %+v, want only orders", got)
	}
	if len(got[0].Permissions) != 1 || got[0].Permissions[0].Slug != "orders.view" {
		t.Errorf("permissions: got %+v, want only orders.view", got[0].Permissions)
	}
}

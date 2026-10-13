package admin

import (
	"testing"

	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// config holds the receiver's API keys - remote_providers.view shows the provider, only manage shows its credentials
func TestAViewOnlyAdminDoesNotSeeProviderConfig(t *testing.T) {
	f := newEscalationFixture(t)
	conn := testdb.Require(t)
	svc := remoteprovider.NewService(remoteprovider.NewRepository(db.New(conn), nil))
	svc.AllowPrivateTargets = true
	own := f.channel
	res, err := svc.Create(t.Context(), remoteprovider.CreateInput{Kind: remoteprovider.KindTaxProvider, Slug: "cfg-scope", Name: "T", TargetURL: "https://tax.example.test/hook", Config: []byte(`{"apiKey":"sk_live_1"}`), ChannelID: &own})
	if err != nil {
		t.Fatal(err)
	}
	q := &queryResolver{&Resolver{AdminSvc: f.svc, RemoteProviderSvc: svc}}

	_, viewer := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "remote_providers.view", ChannelID: &own}))
	one, err := q.RemoteProvider(viewer, res.Provider.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	list, err := q.RemoteProviders(viewer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if one.Config != nil || len(list) != 1 || list[0].Config != nil {
		t.Errorf("a view-only admin read the config: single %v, list %d", one.Config, len(list))
	}

	_, manager := f.admin(t, f.role(t,
		adminpkg.RolePermissionScopeInput{PermissionSlug: "remote_providers.view", ChannelID: &own},
		adminpkg.RolePermissionScopeInput{PermissionSlug: "remote_providers.manage", ChannelID: &own}))
	if got, err := q.RemoteProvider(manager, res.Provider.ID.String()); err != nil || got.Config == nil {
		t.Errorf("a manager lost the config: %v, %v", got, err)
	}
}

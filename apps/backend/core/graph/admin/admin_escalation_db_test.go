package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	adminpkg "github.com/skeletg/skelet/apps/backend/core/internal/admin"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type escalationFixture struct {
	r       *mutationResolver
	svc     *adminpkg.Service
	channel uuid.UUID
}

func newEscalationFixture(t *testing.T) escalationFixture {
	t.Helper()
	conn := testdb.Require(t)
	var ch uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&ch); err != nil {
		t.Skipf("no channel: %v", err)
	}
	if err := permission.Sync(context.Background(), db.New(conn), txrunner.New(conn)); err != nil {
		t.Fatalf("sync permissions: %v", err)
	}
	svc := adminpkg.NewService(adminpkg.NewRepository(db.New(conn), txrunner.New(conn)))
	ints := integration.NewService(integration.NewRepository(db.New(conn), txrunner.New(conn)))
	return escalationFixture{r: &mutationResolver{&Resolver{AdminSvc: svc, IntegrationSvc: ints}}, svc: svc, channel: ch}
}

func (f escalationFixture) role(t *testing.T, scopes ...adminpkg.RolePermissionScopeInput) adminpkg.Role {
	t.Helper()
	slug := "esc-" + uuid.NewString()[:8]
	role, err := f.svc.CreateRole(context.Background(), adminpkg.CreateRoleInput{Name: slug, Slug: slug, Scopes: scopes})
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	return role
}

func (f escalationFixture) admin(t *testing.T, roles ...adminpkg.Role) (uuid.UUID, context.Context) {
	t.Helper()
	slugs := make([]string, len(roles))
	for i, r := range roles {
		slugs[i] = r.Slug
	}
	u, err := f.svc.CreateAdmin(context.Background(), "esc-"+uuid.NewString()[:8]+"@example.invalid", "Correct-Horse-9-battery", "E", "S", slugs)
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return u.ID, auth.WithAdmin(context.Background(), &auth.Claims{AdminID: u.ID, Email: u.Email})
}

func scopeIn(slug string, ch *uuid.UUID) *model.RolePermissionScopeInput {
	in := &model.RolePermissionScopeInput{PermissionSlug: slug}
	if ch != nil {
		s := ch.String()
		in.ChannelID = &s
	}
	return in
}

func TestAChannelScopedAdminsManageCannotReachAdminsOrRoles(t *testing.T) {
	f := newEscalationFixture(t)
	ch := f.channel
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "admins.manage", ChannelID: &ch}))

	if _, err := f.r.CreateRole(ctx, model.CreateRoleInput{Name: "x", Slug: "esc-" + uuid.NewString()[:8], Scopes: []*model.RolePermissionScopeInput{scopeIn("admins.manage", &ch)}}); err == nil {
		t.Error("a channel-scoped admins.manage created a role - roles are install-wide")
	}
	if _, err := f.r.CreateAdmin(ctx, model.CreateAdminInput{Email: "esc-" + uuid.NewString()[:8] + "@example.invalid", Password: "Correct-Horse-9-battery", FirstName: "a", LastName: "b"}); err == nil {
		t.Error("a channel-scoped admins.manage created an admin")
	}
}

func TestAGlobalAdminsManageHandsOutNothingItLacks(t *testing.T) {
	f := newEscalationFixture(t)
	mine := f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "admins.manage"}, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.view"})
	_, ctx := f.admin(t, mine)

	if _, err := f.r.CreateRole(ctx, model.CreateRoleInput{Name: "x", Slug: "esc-" + uuid.NewString()[:8], Scopes: []*model.RolePermissionScopeInput{scopeIn("orders.refund", nil)}}); err == nil {
		t.Error("created a role granting orders.refund, which the caller doesn't hold")
	}
	ch := f.channel
	if _, err := f.r.CreateRole(ctx, model.CreateRoleInput{Name: "x", Slug: "esc-" + uuid.NewString()[:8], Scopes: []*model.RolePermissionScopeInput{scopeIn("orders.view", &ch)}}); err != nil {
		t.Errorf("a narrower grant than the caller's own = %v, want allowed", err)
	}
	wider := f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.refund"})
	if _, err := f.r.CreateAdmin(ctx, model.CreateAdminInput{Email: "esc-" + uuid.NewString()[:8] + "@example.invalid", Password: "Correct-Horse-9-battery", FirstName: "a", LastName: "b", RoleSlugs: []string{wider.Slug}}); err == nil {
		t.Error("created an admin with a role wider than the caller's own")
	}
	if _, err := f.r.UpdateRole(ctx, mine.ID.String(), model.UpdateRoleInput{Scopes: []*model.RolePermissionScopeInput{scopeIn("admins.manage", nil)}}); err == nil {
		t.Error("edited a role the caller holds - that's how admins.manage grows itself")
	}
	if _, err := f.r.UpdateRole(ctx, wider.ID.String(), model.UpdateRoleInput{Scopes: []*model.RolePermissionScopeInput{scopeIn("orders.view", nil)}}); err == nil {
		t.Error("edited a role wider than the caller's own")
	}
	targetID, _ := f.admin(t, wider)
	off := false
	if _, err := f.r.UpdateAdmin(ctx, targetID.String(), model.UpdateAdminInput{IsActive: &off}); err == nil {
		t.Error("deactivated an admin who holds more than the caller")
	}
	if _, err := f.r.DeleteAdmin(ctx, targetID.String()); err == nil {
		t.Error("deleted an admin who holds more than the caller")
	}
}

func TestAnIntegrationTokenCarriesNothingItsMinterLacks(t *testing.T) {
	f := newEscalationFixture(t)
	ch := f.channel
	chs := ch.String()
	_, scoped := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "admins.manage", ChannelID: &ch}, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.view", ChannelID: &ch}))
	if _, err := f.r.CreateIntegration(scoped, model.CreateIntegrationInput{Name: "x", PermissionSlugs: []string{"orders.view"}, ChannelID: &chs}); err == nil {
		t.Error("a channel-scoped admins.manage minted an integration token")
	}

	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "admins.manage"}, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.view", ChannelID: &ch}))
	if _, err := f.r.CreateIntegration(ctx, model.CreateIntegrationInput{Name: "x", PermissionSlugs: []string{"orders.view"}}); err == nil {
		t.Error("minted an unpinned orders.view token from a channel-scoped orders.view")
	}
	pinned, err := f.r.CreateIntegration(ctx, model.CreateIntegrationInput{Name: "x", PermissionSlugs: []string{"orders.view"}, ChannelID: &chs})
	if err != nil {
		t.Fatalf("a token pinned inside the caller's own scope = %v, want allowed", err)
	}
	clear := true
	if _, err := f.r.UpdateIntegration(ctx, model.UpdateIntegrationInput{ID: pinned.Integration.ID, ClearChannel: &clear}); err == nil {
		t.Error("unpinned a token past the caller's own scope")
	}

	super := auth.WithAdmin(context.Background(), &auth.Claims{AdminID: uuid.New(), IsSuperuser: true})
	wide, err := f.r.CreateIntegration(super, model.CreateIntegrationInput{Name: "wide", PermissionSlugs: []string{"orders.refund"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.r.RegenerateIntegrationToken(ctx, wide.Integration.ID); err == nil {
		t.Error("regenerated a token wider than the caller, taking over its secret")
	}
}

func TestRoleGrantsCannotBeStrippedFromAMemberWhoOutranksTheEditor(t *testing.T) {
	f := newEscalationFixture(t)
	_, ctx := f.admin(t, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "admins.manage"}, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.view"}))
	shared := f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.view"})
	f.admin(t, shared, f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.refund"}))

	if _, err := f.r.UpdateRole(ctx, shared.ID.String(), model.UpdateRoleInput{Scopes: []*model.RolePermissionScopeInput{}}); err == nil {
		t.Error("stripped a role's grants from a member who holds orders.refund, which the editor lacks")
	}
	name := "renamed"
	if _, err := f.r.UpdateRole(ctx, shared.ID.String(), model.UpdateRoleInput{Name: &name}); err != nil {
		t.Errorf("renaming leaves every member's grants alone = %v, want allowed", err)
	}
	junior := f.role(t, adminpkg.RolePermissionScopeInput{PermissionSlug: "orders.view"})
	f.admin(t, junior)
	if _, err := f.r.UpdateRole(ctx, junior.ID.String(), model.UpdateRoleInput{Scopes: []*model.RolePermissionScopeInput{}}); err != nil {
		t.Errorf("a role whose members the editor outranks = %v, want allowed", err)
	}
}

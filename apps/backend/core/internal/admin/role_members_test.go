package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type memberRepo struct {
	Repository
	members []uuid.UUID
	grants  map[uuid.UUID][]PermissionGrant
}

func (m *memberRepo) RoleMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return m.members, nil
}

func (m *memberRepo) GetByID(_ context.Context, id uuid.UUID) (AdminUser, error) {
	return AdminUser{ID: id}, nil
}

func (m *memberRepo) ListAllGrants(_ context.Context, id uuid.UUID) ([]PermissionGrant, error) {
	return m.grants[id], nil
}

func TestRoleGrantsCannotChangeUnderAMemberWhoOutranksTheCaller(t *testing.T) {
	caller, junior, senior := uuid.New(), uuid.New(), uuid.New()
	repo := &memberRepo{grants: map[uuid.UUID][]PermissionGrant{
		caller: {global("admins.manage"), global("orders.view")},
		junior: {global("orders.view")},
		senior: {global("orders.view"), global("orders.refund")},
	}}
	svc := NewService(repo)
	ctx := auth.WithRequestCache(context.Background())

	repo.members = []uuid.UUID{junior}
	if err := svc.RequireOutranksMembers(ctx, caller, uuid.New()); err != nil {
		t.Fatalf("every member is within the caller's grants: %v", err)
	}
	repo.members = []uuid.UUID{junior, senior}
	if err := svc.RequireOutranksMembers(ctx, caller, uuid.New()); err == nil {
		t.Fatal("changed a role held by an admin who holds orders.refund, which the caller lacks")
	}
}

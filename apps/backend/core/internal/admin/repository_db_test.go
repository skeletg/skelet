package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The service tests run on memRepo, which only mirrors these conditional updates - this holds the SQL to the same contract

func dbRepo(t *testing.T) (Repository, *db.Queries, context.Context) {
	t.Helper()
	conn := testdb.Require(t)
	q := db.New(conn)
	return NewRepository(q, txrunner.New(conn)), q, context.Background()
}

func seedRepoAdmin(t *testing.T, repo Repository, ctx context.Context) AdminUser {
	t.Helper()
	u, err := repo.Create(ctx, CreateAdminUserInput{Email: "repo-" + uuid.NewString()[:8] + "@example.test", PasswordHash: "x", FirstName: "R", LastName: "T"})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	t.Cleanup(func() { _ = repo.Delete(context.Background(), u.ID) })
	return u
}

func TestRepoEmailIsCaseInsensitive(t *testing.T) {
	repo, _, ctx := dbRepo(t)
	u := seedRepoAdmin(t, repo, ctx)

	got, err := repo.GetByEmail(ctx, "REPO-"+u.Email[len("repo-"):])
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetByEmail in another case = (%v, %v)", got.ID, err)
	}
	if _, err := repo.Create(ctx, CreateAdminUserInput{Email: "Repo-" + u.Email[len("repo-"):], PasswordHash: "x"}); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("case variant insert err = %v, want ErrEmailTaken", err)
	}
	if _, err := repo.GetByEmail(ctx, "nobody-"+uuid.NewString()+"@example.test"); !errors.Is(err, ErrAdminNotFound) {
		t.Errorf("unknown email err = %v, want ErrAdminNotFound", err)
	}
	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, ErrAdminNotFound) {
		t.Errorf("unknown id err = %v, want ErrAdminNotFound", err)
	}
}

func TestRepoTotpAttemptsLockAndLapse(t *testing.T) {
	repo, _, ctx := dbRepo(t)
	conn := testdb.Require(t)
	u := seedRepoAdmin(t, repo, ctx)

	for i := range 3 {
		if ok, err := repo.ClaimTotpAttempt(ctx, u.ID, 3, time.Hour); err != nil || !ok {
			t.Fatalf("claim %d = (%v, %v), want allowed", i+1, ok, err)
		}
	}
	if ok, _ := repo.ClaimTotpAttempt(ctx, u.ID, 3, time.Hour); ok {
		t.Fatal("a claim past the limit was allowed")
	}
	if _, err := conn.ExecContext(ctx, `UPDATE admin_users SET totp_locked_until = now() - interval '1 second' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.ClaimTotpAttempt(ctx, u.ID, 3, time.Hour); !ok {
		t.Fatal("a lapsed lockout still refused")
	}
	if ok, _ := repo.ClaimTotpAttempt(ctx, u.ID, 3, time.Hour); !ok {
		t.Fatal("the count did not restart after the lockout lapsed")
	}
	if err := repo.ClearTotpFailures(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if ok, _ := repo.ClaimTotpAttempt(ctx, u.ID, 3, time.Hour); !ok {
			t.Fatalf("claim %d after clearing refused", i+1)
		}
	}
}

func TestRepoTotpStepIsSingleUse(t *testing.T) {
	repo, _, ctx := dbRepo(t)
	u := seedRepoAdmin(t, repo, ctx)

	for _, c := range []struct {
		step int64
		want bool
	}{{100, true}, {100, false}, {99, false}, {101, true}} {
		if ok, err := repo.AcceptTotpStep(ctx, u.ID, c.step); err != nil || ok != c.want {
			t.Errorf("AcceptTotpStep(%d) = (%v, %v), want %v", c.step, ok, err, c.want)
		}
	}
}

func TestRepoEnableTotpNeedsPendingAndOnlyOnce(t *testing.T) {
	repo, _, ctx := dbRepo(t)
	u := seedRepoAdmin(t, repo, ctx)

	if ok, _ := repo.EnableTotp(ctx, u.ID, []byte(`[]`), 1); ok {
		t.Fatal("enabled with nothing pending")
	}
	if err := repo.SetTotpPending(ctx, u.ID, "enc"); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.EnableTotp(ctx, u.ID, []byte(`["h"]`), 5); err != nil || !ok {
		t.Fatalf("EnableTotp = (%v, %v)", ok, err)
	}
	if ok, _ := repo.AcceptTotpStep(ctx, u.ID, 5); ok {
		t.Error("the enabling step was accepted again at login")
	}
	if err := repo.SetTotpPending(ctx, u.ID, "enc2"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := repo.EnableTotp(ctx, u.ID, []byte(`["h2"]`), 6); ok {
		t.Error("a second enable replaced the live factor")
	}
}

func TestRepoRecoveryRedeemIsCompareAndSwap(t *testing.T) {
	repo, _, ctx := dbRepo(t)
	u := seedRepoAdmin(t, repo, ctx)
	if err := repo.SetTotpPending(ctx, u.ID, "enc"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EnableTotp(ctx, u.ID, []byte(`["a","b","c"]`), 1); err != nil {
		t.Fatal(err)
	}

	if ok, err := repo.RedeemTotpRecoveryCode(ctx, u.ID, []byte(`["a", "b", "c"]`), []byte(`["b","c"]`)); err != nil || !ok {
		t.Fatalf("first redeem = (%v, %v)", ok, err)
	}
	if ok, _ := repo.RedeemTotpRecoveryCode(ctx, u.ID, []byte(`["a","b","c"]`), []byte(`["a","c"]`)); ok {
		t.Fatal("a redeem off the old list overwrote the new one")
	}
	got, _ := repo.GetByID(ctx, u.ID)
	if len(got.TotpRecoveryCodes) != 2 || got.TotpRecoveryCodes[0] != "b" {
		t.Errorf("codes = %v, want [b c]", got.TotpRecoveryCodes)
	}
}

func TestRepoDeleteRole(t *testing.T) {
	repo, q, ctx := dbRepo(t)
	perms, err := q.ListPermissionsByGroupID(ctx, mustFirstGroup(t, q, ctx))
	if err != nil || len(perms) == 0 {
		t.Fatalf("no permission to grant: %v", err)
	}
	role, err := repo.CreateRole(ctx, CreateRoleInput{Name: "Del", Slug: "del-" + uuid.NewString()[:8], Scopes: []RolePermissionScopeInput{{PermissionSlug: perms[0].Slug}}})
	if err != nil {
		t.Fatal(err)
	}
	u := seedRepoAdmin(t, repo, ctx)
	if err := repo.SetRoles(ctx, u.ID, []string{role.Slug}); err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteRole(ctx, role.ID); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("delete with a member err = %v, want ErrRoleInUse", err)
	}
	if err := repo.SetRoles(ctx, u.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteRole(ctx, role.ID); err != nil {
		t.Fatalf("delete an empty role: %v", err)
	}
	if err := repo.DeleteRole(ctx, role.ID); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("delete a gone role err = %v, want ErrRoleNotFound", err)
	}
	if err := repo.SetRoles(ctx, u.ID, []string{role.Slug}); !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("assign a gone role err = %v, want ErrRoleNotFound", err)
	}
}

func mustFirstGroup(t *testing.T, q *db.Queries, ctx context.Context) uuid.UUID {
	t.Helper()
	groups, err := q.ListPermissionGroups(ctx)
	if err != nil || len(groups) == 0 {
		t.Fatalf("no permission groups: %v", err)
	}
	return groups[0].ID
}

package adminprefs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The service tests run on a fake - this holds the SQL to the same contract

func dbRepo(t *testing.T) (Repository, *db.Queries, context.Context) {
	t.Helper()
	q := db.New(testdb.Require(t))
	return NewRepository(q), q, context.Background()
}

func seedAdmin(t *testing.T, q *db.Queries, ctx context.Context) uuid.UUID {
	t.Helper()
	a, err := q.CreateAdminUser(ctx, db.CreateAdminUserParams{Email: "prefs-" + uuid.NewString()[:8] + "@example.test", PasswordHash: "x", FirstName: "P", LastName: "R"})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	t.Cleanup(func() { _ = q.DeleteAdminUser(context.Background(), a.ID) })
	return a.ID
}

func TestRepoUpsertReplacesValueAndBumpsUpdatedAt(t *testing.T) {
	repo, q, ctx := dbRepo(t)
	admin := seedAdmin(t, q, ctx)

	if v, err := repo.Get(ctx, admin, "nav:pins"); err != nil || v != nil {
		t.Fatalf("unset key = (%s, %v), want (nil, nil)", v, err)
	}
	if err := repo.Set(ctx, admin, "nav:pins", json.RawMessage(`["/orders"]`)); err != nil {
		t.Fatal(err)
	}
	first, _ := repo.List(ctx, admin)
	time.Sleep(10 * time.Millisecond)
	if err := repo.Set(ctx, admin, "nav:pins", json.RawMessage(`["/orders","/webhooks"]`)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.List(ctx, admin)
	if err != nil || len(got) != 1 {
		t.Fatalf("List = (%d rows, %v), want one", len(got), err)
	}
	var pins []string
	if json.Unmarshal(got[0].Value, &pins) != nil || len(pins) != 2 {
		t.Errorf("value = %s, want the second write", got[0].Value)
	}
	if !got[0].UpdatedAt.After(first[0].UpdatedAt) {
		t.Errorf("updated_at stayed %v on overwrite", got[0].UpdatedAt)
	}
}

func TestRepoRowsArePerAdminAndCountExcludesTheKey(t *testing.T) {
	repo, q, ctx := dbRepo(t)
	a, b := seedAdmin(t, q, ctx), seedAdmin(t, q, ctx)
	for _, key := range []string{"table:orders", "table:products", "nav:pins"} {
		if err := repo.Set(ctx, a, key, json.RawMessage(`true`)); err != nil {
			t.Fatal(err)
		}
	}

	if got, _ := repo.List(ctx, b); len(got) != 0 {
		t.Errorf("admin B lists %d of admin A's rows", len(got))
	}
	if v, _ := repo.Get(ctx, b, "nav:pins"); v != nil {
		t.Error("admin B read admin A's key")
	}
	if n, err := repo.CountOthers(ctx, a, "nav:pins"); err != nil || n != 2 {
		t.Errorf("CountOthers(existing key) = (%d, %v), want 2", n, err)
	}
	if n, _ := repo.CountOthers(ctx, a, "views:/orders"); n != 3 {
		t.Errorf("CountOthers(new key) = %d, want 3", n)
	}
	if err := repo.Delete(ctx, a, "nav:pins"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.List(ctx, a); len(got) != 2 || got[0].Key != "table:orders" {
		t.Errorf("after delete = %+v, want the two table keys in key order", got)
	}
}

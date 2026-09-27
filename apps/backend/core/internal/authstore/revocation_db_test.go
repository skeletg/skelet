package authstore

import (
	"context"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// Gated on DATABASE_URL (disposable marker) - the guard is worth nothing unless a logout actually publishes the session it killed, and nothing else would catch that link going quiet

type recordingRevocations struct{ killed []uuid.UUID }

func (r *recordingRevocations) Revoke(_ context.Context, id uuid.UUID) {
	r.killed = append(r.killed, id)
}
func (r *recordingRevocations) Revoked(context.Context, uuid.UUID) bool { return false }

func adminStore(t *testing.T) (*auth.RefreshStore, *recordingRevocations, *db.Queries, context.Context) {
	t.Helper()
	conn := testdb.Require(t)
	q := db.New(conn)
	list := &recordingRevocations{}
	// No Valkey client, so the refresh backend runs on Postgres and the whole path stays in one database
	store := auth.NewRefreshStore(NewRefreshBackend(nil, q, "admin"), NewSessionLog(q), "admin")
	store.SetRevocations(list)
	return store, list, q, context.Background()
}

// Seeds an admin the session rows can hang off, since admin_sessions.admin_id is a real reference
func seedAdmin(t *testing.T, q *db.Queries, ctx context.Context) uuid.UUID {
	t.Helper()
	email := "revoke-" + uuid.NewString()[:8] + "@example.test"
	admin, err := q.CreateAdminUser(ctx, db.CreateAdminUserParams{
		Email: email, PasswordHash: "x", FirstName: "Rev", LastName: "Oke",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	t.Cleanup(func() { _ = q.DeleteAdminUser(ctx, admin.ID) })
	return admin.ID
}

func TestLogoutPublishesTheSessionItKilled(t *testing.T) {
	store, list, q, ctx := adminStore(t)
	adminID := seedAdmin(t, q, ctx)

	token, sessionID, err := store.Create(ctx, adminID, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sessionID == uuid.Nil {
		t.Fatal("no session row was created, so there is nothing to revoke")
	}

	if err := store.Revoke(ctx, token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if len(list.killed) != 1 || list.killed[0] != sessionID {
		t.Fatalf("published %v, want exactly [%s]", list.killed, sessionID)
	}
}

func TestLogoutEverywherePublishesEverySession(t *testing.T) {
	store, list, q, ctx := adminStore(t)
	adminID := seedAdmin(t, q, ctx)

	want := map[uuid.UUID]bool{}
	for range 3 {
		_, sessionID, err := store.Create(ctx, adminID, "127.0.0.1", "agent-"+uuid.NewString()[:6])
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		want[sessionID] = true
	}

	if err := store.RevokeAll(ctx, adminID); err != nil {
		t.Fatalf("RevokeAll: %v", err)
	}
	if len(list.killed) != len(want) {
		t.Fatalf("published %d sessions, want %d - a device left out keeps working on the Signals API", len(list.killed), len(want))
	}
	for _, id := range list.killed {
		if !want[id] {
			t.Errorf("published %s, which was not one of this admin's sessions", id)
		}
	}
}

func TestLogoutOtherDevicesKeepsTheCurrentOne(t *testing.T) {
	store, list, q, ctx := adminStore(t)
	adminID := seedAdmin(t, q, ctx)

	keepToken, keepSession, err := store.Create(ctx, adminID, "127.0.0.1", "this-device")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, otherSession, err := store.Create(ctx, adminID, "127.0.0.1", "other-device")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.RevokeExcept(ctx, adminID, auth.HashToken(keepToken)); err != nil {
		t.Fatalf("RevokeExcept: %v", err)
	}
	if len(list.killed) != 1 || list.killed[0] != otherSession {
		t.Fatalf("published %v, want exactly [%s]", list.killed, otherSession)
	}
	for _, id := range list.killed {
		if id == keepSession {
			t.Error("published the session doing the revoking, which would log it out of the Signals API")
		}
	}
}

package authstore

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The guard runs on every admin request, so it asks for the one session rather than listing them all
func TestSessionActiveLooksUpTheOneSession(t *testing.T) {
	store, _, q, ctx := adminStore(t)
	a, b := seedAdmin(t, q, ctx), seedAdmin(t, q, ctx)
	_, own, err := store.Create(ctx, a, "127.0.0.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	_, other, err := store.Create(ctx, b, "127.0.0.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		session uuid.UUID
		want    bool
	}{{"own session", own, true}, {"another admin's session", other, false}, {"unknown session", uuid.New(), false}} {
		if got, err := store.SessionActive(ctx, a, c.session); err != nil || got != c.want {
			t.Errorf("%s: SessionActive = (%v, %v), want %v", c.name, got, err, c.want)
		}
	}
}

func putRefresh(t *testing.T, conn *sql.DB, q *db.Queries, set string) string {
	t.Helper()
	hash := "prune-" + uuid.NewString()
	if err := NewPostgresRefreshBackend(q, "customer").Put(t.Context(), hash, uuid.New(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if set != "" {
		if _, err := conn.Exec(`UPDATE auth_refresh_tokens SET `+set+` WHERE token_hash = $1`, hash); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM auth_refresh_tokens WHERE token_hash = $1`, hash) })
	return hash
}

func TestPruneDropsDeadSessionsAndLapsedSuccessors(t *testing.T) {
	store, _, q, ctx := adminStore(t)
	conn := testdb.Require(t)
	admin := seedAdmin(t, q, ctx)
	_, fresh, _ := store.Create(ctx, admin, "127.0.0.1", "ua")
	_, stale, _ := store.Create(ctx, admin, "127.0.0.1", "ua")
	if _, err := conn.Exec(`UPDATE admin_sessions SET last_active_at = now() - interval '31 days' WHERE id = $1`, stale); err != nil {
		t.Fatal(err)
	}
	live := putRefresh(t, conn, q, "")
	expired := putRefresh(t, conn, q, "expires_at = now() - interval '1 minute'")
	inGraceAftermath := putRefresh(t, conn, q, "rotated_at = now() - interval '1 minute', successor_token = 'raw', grace_expires_at = now() - interval '30 seconds'")
	pastWindow := putRefresh(t, conn, q, "rotated_at = now() - interval '25 hours'")

	if _, err := Prune(ctx, q, time.Now()); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[uuid.UUID]bool{fresh: true, stale: false} {
		if got, _ := store.SessionActive(ctx, admin, id); got != want {
			t.Errorf("session %s active = %v after prune, want %v", id, got, want)
		}
	}
	state := func(hash string) (exists bool, successor sql.NullString) {
		err := conn.QueryRow(`SELECT successor_token FROM auth_refresh_tokens WHERE token_hash = $1`, hash).Scan(&successor)
		return err == nil, successor
	}
	if ok, _ := state(live); !ok {
		t.Error("a live refresh token was pruned")
	}
	if ok, _ := state(expired); ok {
		t.Error("an expired refresh token survived")
	}
	if ok, s := state(inGraceAftermath); !ok || s.Valid {
		t.Errorf("rotated row inside the reuse window: kept=%v successor=%v, want kept with the raw successor cleared", ok, s)
	}
	if ok, _ := state(pastWindow); ok {
		t.Error("a rotated row past the reuse window survived")
	}
}

func TestPostgresRotationHandsOutOneSuccessorThenForgetsIt(t *testing.T) {
	_, _, q, ctx := adminStore(t)
	conn := testdb.Require(t)
	backend := NewPostgresRefreshBackend(q, "customer")
	old := putRefresh(t, conn, q, "")
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM auth_refresh_tokens WHERE token_hash LIKE 'next-%'`) })

	won, err := backend.Rotate(ctx, old, "next-"+uuid.NewString(), "raw-1", time.Hour, 30*time.Second)
	if err != nil || won.Outcome != auth.RotateWon {
		t.Fatalf("first rotation = (%+v, %v), want won", won, err)
	}
	again, err := backend.Rotate(ctx, old, "next-"+uuid.NewString(), "raw-2", time.Hour, 30*time.Second)
	if err != nil || again.Outcome != auth.RotateGrace || again.Successor != "raw-1" {
		t.Fatalf("second rotation inside grace = (%+v, %v), want the same successor", again, err)
	}

	if _, err := conn.Exec(`UPDATE auth_refresh_tokens SET grace_expires_at = now() - interval '1 second' WHERE token_hash = $1`, old); err != nil {
		t.Fatal(err)
	}
	late, err := backend.Rotate(ctx, old, "next-"+uuid.NewString(), "raw-3", time.Hour, 30*time.Second)
	if err != nil || late.Outcome != auth.RotateInvalid {
		t.Fatalf("rotation after grace = (%+v, %v), want invalid", late, err)
	}
	var successor sql.NullString
	_ = conn.QueryRow(`SELECT successor_token FROM auth_refresh_tokens WHERE token_hash = $1`, old).Scan(&successor)
	if successor.Valid {
		t.Error("the raw successor outlived its grace window")
	}
}

// Past the window a replay must read as a stale token, as on Valkey, not as theft that revokes every session
func TestRotatedOwnerOnlyInsideTheReuseWindow(t *testing.T) {
	_, _, q, ctx := adminStore(t)
	conn := testdb.Require(t)
	backend := NewPostgresRefreshBackend(q, "customer")
	recent := putRefresh(t, conn, q, "rotated_at = now() - interval '1 hour'")
	old := putRefresh(t, conn, q, "rotated_at = now() - interval '25 hours'")

	if _, ok := backend.RotatedOwner(ctx, recent); !ok {
		t.Error("a token rotated an hour ago is not remembered")
	}
	if _, ok := backend.RotatedOwner(ctx, old); ok {
		t.Error("a token rotated 25 hours ago still trips the reuse tripwire")
	}
}

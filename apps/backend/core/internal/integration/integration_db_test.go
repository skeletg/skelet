package integration

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type dbRig struct {
	conn  *sql.DB
	svc   *Service
	slugs []string
}

func newDBRig(t *testing.T) dbRig {
	t.Helper()
	conn := testdb.Require(t)
	rows, err := conn.Query(`SELECT slug FROM permissions ORDER BY sort_order, name LIMIT 3`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var slugs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		slugs = append(slugs, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(slugs) < 3 {
		t.Skip("no permissions seeded")
	}
	return dbRig{conn: conn, svc: NewService(NewRepository(db.New(conn), txrunner.New(conn))), slugs: slugs}
}

func (r dbRig) create(t *testing.T, slugs ...string) CreateIntegrationResult {
	t.Helper()
	res, err := r.svc.Create(context.Background(), CreateIntegrationInput{Name: "it-" + uuid.NewString()[:8], Permissions: slugs})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM integrations WHERE id = $1`, res.Integration.ID) })
	return res
}

func slugsOf(ps []Permission) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Slug
	}
	return out
}

// The token lookup aggregates the grant set in one query - it has to carry exactly what the per-integration read does
func TestTheTokenLookupCarriesTheSameGrantsAsARead(t *testing.T) {
	r := newDBRig(t)
	ctx := context.Background()
	res := r.create(t, r.slugs[2], r.slugs[0])
	byToken, err := r.svc.Validate(ctx, res.RawToken)
	if err != nil {
		t.Fatal(err)
	}
	byID, err := r.svc.Get(ctx, res.Integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(byToken.Permissions, byID.Permissions) || len(byToken.Permissions) != 2 {
		t.Fatalf("by token %+v, by id %+v - want the same two grants in the same order", byToken.Permissions, byID.Permissions)
	}
	if byToken.Permissions[0].Name == "" || byToken.Permissions[0].ID == uuid.Nil {
		t.Errorf("grant %+v lost its id or name", byToken.Permissions[0])
	}

	none := r.create(t)
	if i, err := r.svc.Validate(ctx, none.RawToken); err != nil || len(i.Permissions) != 0 {
		t.Fatalf("a token with no grants = %+v, %v - want it found with an empty set", i.Permissions, err)
	}
}

func TestListGroupsEachIntegrationsGrants(t *testing.T) {
	r := newDBRig(t)
	a := r.create(t, r.slugs[0], r.slugs[1])
	b := r.create(t, r.slugs[2])
	c := r.create(t)
	list, err := r.svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID][]string{a.Integration.ID: {r.slugs[0], r.slugs[1]}, b.Integration.ID: {r.slugs[2]}, c.Integration.ID: nil}
	seen := 0
	for _, i := range list {
		if w, ok := want[i.ID]; ok {
			seen++
			if got := slugsOf(i.Permissions); !slices.Equal(got, w) {
				t.Errorf("%s grants = %v, want %v", i.Name, got, w)
			}
		}
	}
	if seen != 3 {
		t.Fatalf("listed %d of the 3 integrations", seen)
	}
}

// One bad slug must leave nothing behind - no half-created token, no half-replaced grant set
func TestAnUnknownPermissionChangesNothing(t *testing.T) {
	r := newDBRig(t)
	ctx := context.Background()
	name := "it-" + uuid.NewString()[:8]
	if _, err := r.svc.Create(ctx, CreateIntegrationInput{Name: name, Permissions: []string{r.slugs[0], "no.such.permission"}}); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("create = %v, want ErrUnknownPermission", err)
	}
	var n int
	if err := r.conn.QueryRow(`SELECT count(*) FROM integrations WHERE name = $1`, name).Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d rows left behind (%v)", n, err)
	}

	res := r.create(t, r.slugs[0])
	renamed := "renamed"
	if _, err := r.svc.Update(ctx, UpdateIntegrationInput{ID: res.Integration.ID, Name: &renamed, PermissionSlugs: []string{"no.such.permission"}}); !errors.Is(err, ErrUnknownPermission) {
		t.Fatalf("update = %v, want ErrUnknownPermission", err)
	}
	got, err := r.svc.Get(ctx, res.Integration.ID)
	if err != nil || got.Name == renamed || !slices.Equal(slugsOf(got.Permissions), []string{r.slugs[0]}) {
		t.Fatalf("after a refused update: %+v, %v - want the name and grants untouched", got, err)
	}
}

func TestAMissingIntegrationIsNotFound(t *testing.T) {
	r := newDBRig(t)
	ctx := context.Background()
	id := uuid.New()
	name := "n"
	for op, err := range map[string]error{
		"get":        func() error { _, err := r.svc.Get(ctx, id); return err }(),
		"update":     func() error { _, err := r.svc.Update(ctx, UpdateIntegrationInput{ID: id, Name: &name}); return err }(),
		"set active": func() error { _, err := r.svc.SetActive(ctx, id, false); return err }(),
		"regenerate": func() error { _, err := r.svc.RegenerateToken(ctx, id); return err }(),
		"delete":     r.svc.Delete(ctx, id),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s = %v, want ErrNotFound", op, err)
		}
	}
}

func TestPinningAndUnpinningAChannel(t *testing.T) {
	r := newDBRig(t)
	ctx := context.Background()
	var ch uuid.UUID
	if err := r.conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&ch); err != nil {
		t.Skipf("no channel: %v", err)
	}
	res := r.create(t, r.slugs[0])
	pinned, err := r.svc.Update(ctx, UpdateIntegrationInput{ID: res.Integration.ID, ChannelID: &ch, SetChannel: true})
	if err != nil || pinned.ChannelID == nil || *pinned.ChannelID != ch {
		t.Fatalf("pin = %+v, %v", pinned.ChannelID, err)
	}
	if v, _ := r.svc.Validate(ctx, res.RawToken); v.ChannelID == nil || *v.ChannelID != ch {
		t.Fatalf("the token lookup lost the pin: %v", v.ChannelID)
	}
	kept, err := r.svc.Update(ctx, UpdateIntegrationInput{ID: res.Integration.ID, Description: new(string)})
	if err != nil || kept.ChannelID == nil {
		t.Fatalf("an unrelated edit dropped the pin: %+v, %v", kept.ChannelID, err)
	}
	unpinned, err := r.svc.Update(ctx, UpdateIntegrationInput{ID: res.Integration.ID, SetChannel: true})
	if err != nil || unpinned.ChannelID != nil {
		t.Fatalf("unpin = %v, %v - want the token global again", unpinned.ChannelID, err)
	}
}

// Update writes back every column it read - without the row lock, an edit landing mid-update was silently reverted
func TestConcurrentEditsToDifferentFieldsBothLand(t *testing.T) {
	r := newDBRig(t)
	ctx := context.Background()
	res := r.create(t)
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE integrations SET description = 'from the other admin' WHERE id = $1`, res.Integration.ID); err != nil {
		t.Fatal(err)
	}
	renamed := "renamed"
	done := make(chan error, 1)
	go func() {
		_, err := r.svc.Update(ctx, UpdateIntegrationInput{ID: res.Integration.ID, Name: &renamed})
		done <- err
	}()
	waitForLockWait(t, r.conn)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Get(ctx, res.Integration.ID)
	if err != nil || got.Name != renamed || got.Description != "from the other admin" {
		t.Fatalf("after both edits: name %q description %q (%v), want both", got.Name, got.Description, err)
	}
}

func waitForLockWait(t *testing.T, conn *sql.DB) {
	t.Helper()
	for range 200 {
		var n int
		if err := conn.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the update never queued behind the open transaction")
}

// An outage must surface as the outage - "not found" would send an admin hunting for a token that is still there
func TestADeadDatabaseIsNotNotFound(t *testing.T) {
	testdb.Require(t)
	dsn, _ := testdb.URL("DATABASE_URL")
	dead, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	_ = dead.Close()
	svc := NewService(NewRepository(db.New(dead), txrunner.New(dead)))
	if _, err := svc.Get(context.Background(), uuid.New()); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("get on a closed pool = %v, want the connection error", err)
	}
	if _, err := svc.Validate(context.Background(), tokenPrefix+"x"); err == nil || errors.Is(err, errTokenRefused) {
		t.Fatalf("validate on a closed pool = %v, want the connection error", err)
	}
}

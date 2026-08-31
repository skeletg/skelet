package config

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	dbsqlc "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// The resolution ladder lives half in SQL - GetCandidates returns every row that COULD satisfy a scope and the service picks among them, so its own doc says the ladder "can't drift between the SQL and a test double" - these run the real query, which is the only way to hold that promise  Settings are global rows with no fixtures to hang off, so each test uses a unique path and deletes its own rows

func repoDB(t *testing.T) (*sql.DB, Repository) {
	t.Helper()
	conn := testdb.Require(t)
	return conn, NewRepository(dbsqlc.New(conn), tx.New(conn))
}

// uniquePath keeps concurrent runs and leftover dev rows from colliding
func uniquePath(t *testing.T, conn *sql.DB) string {
	t.Helper()
	p := "e2etest/cfg/" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM config_values WHERE path = $1`, p) })
	return p
}

func someChannelID(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("no channel to scope config against: %v", err)
	}
	return id
}

// twoChannelIDs skips when the database has only one store to compare against
func twoChannelIDs(t *testing.T, conn *sql.DB) []uuid.UUID {
	t.Helper()
	rows, err := conn.Query(`SELECT id FROM channels LIMIT 2`)
	if err != nil {
		t.Fatalf("list channels: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan channel: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate channels: %v", err)
	}
	if len(ids) < 2 {
		t.Skip("need two channels to check isolation")
	}
	return ids
}

func valuesByScope(rows []StoredValue) map[string]string {
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[FormatScope(r.Scope)] = r.Value
	}
	return out
}

// A write round-trips through the real columns, including the nullable channel and locale
func TestRepositoryUpsertRoundTrips(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	path := uniquePath(t, conn)
	ch := someChannelID(t, conn)

	def, err := repo.Upsert(ctx, DefaultScope, path, "platform")
	if err != nil {
		t.Fatalf("upsert default: %v", err)
	}
	if def.Value != "platform" || def.Scope.Kind != ScopeDefault || def.Scope.ChannelID != nil {
		t.Errorf("stored default = %+v, want a channel-less default row", def)
	}

	chScope := Scope{Kind: ScopeChannel, ChannelID: &ch}
	got, err := repo.Upsert(ctx, chScope, path, "per-store")
	if err != nil {
		t.Fatalf("upsert channel: %v", err)
	}
	if got.Value != "per-store" || got.Scope.ChannelID == nil || *got.Scope.ChannelID != ch {
		t.Errorf("stored channel row = %+v, want it carrying channel %s", got, ch)
	}

	// upserting the same scope REPLACES rather than adding a second row
	if _, err := repo.Upsert(ctx, chScope, path, "revised"); err != nil {
		t.Fatalf("re-upsert channel: %v", err)
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM config_values WHERE path = $1`, path).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 2 {
		t.Errorf("%d rows for one path across two scopes, want 2 - the upsert must replace in place", n)
	}
}

// The global half of the round trip above, which it never exercised: a default-scope row carries
// channel_id NULL, and the unique index has to treat that NULL as colliding with itself. When it
// did not, ON CONFLICT could never fire, every save added another row, and reads answered from
// whichever duplicate the plan reached first - a saved setting looked like it had reverted
func TestRepositoryUpsertReplacesAtDefaultScope(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	path := uniquePath(t, conn)

	for _, want := range []string{"first", "second", "third"} {
		if _, err := repo.Upsert(ctx, DefaultScope, path, want); err != nil {
			t.Fatalf("upsert %q: %v", want, err)
		}
	}

	var rows int
	var value string
	if err := conn.QueryRow(
		`SELECT count(*), max(value) FROM config_values WHERE path = $1`, path,
	).Scan(&rows, &value); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("%d rows after three saves of one global setting, want 1 - the upsert must replace in place", rows)
	}
	if value != "third" {
		t.Errorf("stored value = %q, want the last one written", value)
	}

	// and the read path has to agree - one candidate, carrying the newest value
	got, err := repo.GetCandidates(ctx, DefaultScope, path)
	if err != nil {
		t.Fatalf("GetCandidates: %v", err)
	}
	if len(got) != 1 || got[0].Value != "third" {
		t.Errorf("candidates = %+v, want exactly the last value written", got)
	}
}

// The candidate set is what the ladder picks from, so asking at channel scope must return BOTH the channel row and the default - returning only one would collapse the fallback
func TestRepositoryGetCandidatesReturnsTheWholeLadder(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	path := uniquePath(t, conn)
	ch := someChannelID(t, conn)
	chScope := Scope{Kind: ScopeChannel, ChannelID: &ch}

	if _, err := repo.Upsert(ctx, DefaultScope, path, "platform"); err != nil {
		t.Fatalf("seed default: %v", err)
	}
	if _, err := repo.Upsert(ctx, chScope, path, "per-store"); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	got, err := repo.GetCandidates(ctx, chScope, path)
	if err != nil {
		t.Fatalf("GetCandidates: %v", err)
	}
	byScope := valuesByScope(got)
	if byScope[ScopeDefault] != "platform" {
		t.Errorf("candidates missing the default row: %v", byScope)
	}
	if byScope[FormatScope(chScope)] != "per-store" {
		t.Errorf("candidates missing the channel row: %v", byScope)
	}

	// asking at DEFAULT scope must not drag the channel row in, or a platform read would see a store's value
	got, err = repo.GetCandidates(ctx, DefaultScope, path)
	if err != nil {
		t.Fatalf("GetCandidates at default: %v", err)
	}
	for _, r := range got {
		if r.Scope.ChannelID != nil {
			t.Errorf("a default-scope read returned channel row %+v, want only platform rows", r)
		}
	}
}

// Another channel's row must never be a candidate - that would leak one store's setting into another
func TestRepositoryGetCandidatesIsChannelIsolated(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	path := uniquePath(t, conn)

	ids := twoChannelIDs(t, conn)
	mine, theirs := ids[0], ids[1]

	if _, err := repo.Upsert(ctx, Scope{Kind: ScopeChannel, ChannelID: &theirs}, path, "not-mine"); err != nil {
		t.Fatalf("seed other channel: %v", err)
	}
	got, err := repo.GetCandidates(ctx, Scope{Kind: ScopeChannel, ChannelID: &mine}, path)
	if err != nil {
		t.Fatalf("GetCandidates: %v", err)
	}
	for _, r := range got {
		if r.Value == "not-mine" {
			t.Errorf("another channel's row surfaced as a candidate: %+v", r)
		}
	}
}

// Delete is scope-precise - clearing a store's override must leave the platform default standing
func TestRepositoryDeleteIsScoped(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	path := uniquePath(t, conn)
	ch := someChannelID(t, conn)
	chScope := Scope{Kind: ScopeChannel, ChannelID: &ch}

	if _, err := repo.Upsert(ctx, DefaultScope, path, "platform"); err != nil {
		t.Fatalf("seed default: %v", err)
	}
	if _, err := repo.Upsert(ctx, chScope, path, "per-store"); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	if err := repo.Delete(ctx, chScope, path); err != nil {
		t.Fatalf("delete channel row: %v", err)
	}
	got, err := repo.GetCandidates(ctx, chScope, path)
	if err != nil {
		t.Fatalf("GetCandidates: %v", err)
	}
	byScope := valuesByScope(got)
	if _, still := byScope[FormatScope(chScope)]; still {
		t.Error("the channel row survived its delete")
	}
	if byScope[ScopeDefault] != "platform" {
		t.Errorf("the platform default was collateral damage: %v", byScope)
	}

	// deleting a row that isn't there is a no-op rather than an error
	if err := repo.Delete(ctx, chScope, path); err != nil {
		t.Errorf("deleting an absent row = %v, want nil", err)
	}
}

// The service reads through the real SQL here, so this is the ladder end to end rather than against a double - channel row beats platform row beats the code default
func TestServiceResolutionLadderOverRealSQL(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	isolateRegistries(t)
	RegisterSection(Section{Slug: "e2etest", Label: "Test"})
	RegisterGroup(Group{Section: "e2etest", Slug: "cfg", Label: "Cfg", Scoping: ScopingChannel})

	path := uniquePath(t, conn)
	// the registry path has to match the row path, so register the generated one
	_, _, field, ok := ParsePath(path)
	if !ok {
		t.Fatalf("generated path %q is not well formed", path)
	}
	Register(Definition{Path: path, Type: TypeString, Default: "code-default", Label: field, Scoping: ScopingChannel})

	svc := NewService(repo)
	ch := someChannelID(t, conn)
	chScope := Scope{Kind: ScopeChannel, ChannelID: &ch}

	// nothing stored - the code default
	got, err := svc.Get(ctx, path, chScope)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != "code-default" || !got.IsDefault {
		t.Errorf("with nothing stored = %+v, want the registry default", got)
	}

	// a platform row wins over the code default
	if _, err := svc.Set(ctx, path, "platform", DefaultScope); err != nil {
		t.Fatalf("Set default: %v", err)
	}
	got, _ = svc.Get(ctx, path, chScope)
	if got.Value != "platform" || got.IsDefault {
		t.Errorf("with a platform row = %+v, want the stored platform value", got)
	}

	// and the store's own row wins over that
	if _, err := svc.Set(ctx, path, "per-store", chScope); err != nil {
		t.Fatalf("Set channel: %v", err)
	}
	got, _ = svc.Get(ctx, path, chScope)
	if got.Value != "per-store" {
		t.Errorf("with a channel row = %+v, want the store's value", got)
	}
	// the platform scope still reads its own row, unaffected
	got, _ = svc.Get(ctx, path, DefaultScope)
	if got.Value != "platform" {
		t.Errorf("platform read = %q, want the override invisible there", got.Value)
	}

	// clearing the override falls back one rung, not all the way to code
	if _, err := svc.Reset(ctx, path, chScope); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	got, _ = svc.Get(ctx, path, chScope)
	if got.Value != "platform" {
		t.Errorf("after Reset = %q, want the platform row back", got.Value)
	}
}

// Rows whose path left the registry are invisible to every read, so they need their own listing before anything can clean them up - and Prune must take only those
func TestServiceOrphanPruneOverRealSQL(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	isolateRegistries(t)
	RegisterSection(Section{Slug: "e2etest", Label: "Test"})
	RegisterGroup(Group{Section: "e2etest", Slug: "cfg", Label: "Cfg"})

	kept := uniquePath(t, conn)
	orphan := uniquePath(t, conn)
	_, _, keptField, _ := ParsePath(kept)
	Register(Definition{Path: kept, Type: TypeString, Default: "", Label: keptField})

	svc := NewService(repo)
	if _, err := svc.Set(ctx, kept, "still-declared", DefaultScope); err != nil {
		t.Fatalf("set kept: %v", err)
	}
	// written straight through the repo, since the service refuses an unregistered path
	if _, err := repo.Upsert(ctx, DefaultScope, orphan, "left-behind"); err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	orphans, err := svc.ListOrphans(ctx)
	if err != nil {
		t.Fatalf("ListOrphans: %v", err)
	}
	var sawOrphan, sawKept bool
	for _, o := range orphans {
		if o.Path == orphan {
			sawOrphan = true
		}
		if o.Path == kept {
			sawKept = true
		}
	}
	if !sawOrphan {
		t.Errorf("the undeclared path %q is missing from %d orphans", orphan, len(orphans))
	}
	if sawKept {
		t.Errorf("the registered path %q was reported as an orphan", kept)
	}

	if _, err := svc.Prune(ctx); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM config_values WHERE path = $1`, orphan).Scan(&n); err != nil {
		t.Fatalf("count orphan: %v", err)
	}
	if n != 0 {
		t.Errorf("%d orphan rows survived Prune", n)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM config_values WHERE path = $1`, kept).Scan(&n); err != nil {
		t.Fatalf("count kept: %v", err)
	}
	if n != 1 {
		t.Errorf("Prune deleted the registered path's row (%d left)", n)
	}
}

// A secret is encrypted in the COLUMN and plaintext to the caller - the unit tests prove this against a double, this proves the bytes that actually land in Postgres
func TestSecretIsCiphertextInTheColumn(t *testing.T) {
	conn, repo := repoDB(t)
	ctx := context.Background()
	isolateRegistries(t)
	RegisterSection(Section{Slug: "e2etest", Label: "Test"})
	RegisterGroup(Group{Section: "e2etest", Slug: "cfg", Label: "Cfg"})

	path := uniquePath(t, conn)
	_, _, field, _ := ParsePath(path)
	Register(Definition{Path: path, Type: TypeSecret, Label: field})

	svc := NewService(repo)
	svc.SetCipher(testCipher(t))

	const plaintext = "sk_live_never_in_the_column"
	entry, err := svc.Set(ctx, path, plaintext, DefaultScope)
	if err != nil {
		t.Fatalf("Set secret: %v", err)
	}
	if entry.Value != plaintext {
		t.Errorf("returned value = %q, want the plaintext for the admin's is-it-set check", entry.Value)
	}

	var stored string
	if err := conn.QueryRow(`SELECT value FROM config_values WHERE path = $1`, path).Scan(&stored); err != nil {
		t.Fatalf("read the raw column: %v", err)
	}
	if stored == plaintext {
		t.Fatal("the secret is plaintext in the config_values column")
	}

	got, err := svc.Get(ctx, path, DefaultScope)
	if err != nil {
		t.Fatalf("Get secret: %v", err)
	}
	if got.Value != plaintext {
		t.Errorf("Get = %q, want the decrypted plaintext", got.Value)
	}
}

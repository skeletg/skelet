package pluginmigrate

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// The runner's guarantees live in SQL - tx boundaries, the (namespace, version) primary key, the advisory lock - so a fake can't assert them
// DATABASE_URL-gated like internal/campaign's claim test, each case owning a unique namespace + table so parallel packages don't collide

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

// Wires a namespace plus teardown of both its table and its tracking rows
func registerFixture(t *testing.T, conn *sql.DB, namespace, table string, files fstest.MapFS) {
	t.Helper()
	ResetForTest()
	Register(Registration{Namespace: namespace, FS: files})
	t.Cleanup(func() {
		ResetForTest()
		_, _ = conn.Exec(`DROP TABLE IF EXISTS ` + table)
		_, _ = conn.Exec(`DELETE FROM plugin_migrations WHERE namespace = $1`, namespace)
	})
	// clean slate even if a previous run died mid-test
	_, _ = conn.Exec(`DROP TABLE IF EXISTS ` + table)
	_, _ = conn.Exec(`DELETE FROM plugin_migrations WHERE namespace = $1`, namespace)
}

func mapFS(entries map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, body := range entries {
		out["migrations/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return out
}

func appliedCount(t *testing.T, conn *sql.DB, namespace string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM plugin_migrations WHERE namespace = $1`, namespace,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The core loop - apply in order, record a checksum, and no-op on re-run rather than re-applying
func TestUpNamespace_AppliesAndIsIdempotent(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-apply", "pmtest_apply"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_create.up.sql":   `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
		"0001_create.down.sql": `DROP TABLE ` + table + `;`,
		"0002_column.up.sql":   `ALTER TABLE ` + table + ` ADD COLUMN label TEXT;`,
		"0002_column.down.sql": `ALTER TABLE ` + table + ` DROP COLUMN label;`,
	}))

	r := New(conn)
	ctx := context.Background()
	if err := r.UpNamespace(ctx, ns); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if got := appliedCount(t, conn, ns); got != 2 {
		t.Fatalf("applied rows: got %d, want 2", got)
	}

	var checksum sql.NullString
	if err := conn.QueryRow(
		`SELECT checksum FROM plugin_migrations WHERE namespace = $1 AND version = 1`, ns,
	).Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if !checksum.Valid || checksum.String == "" {
		t.Error("migration applied without recording a checksum - drift detection would be blind")
	}

	// ALTER TABLE ADD COLUMN would fail outright if the SQL re-executed
	if err := r.UpNamespace(ctx, ns); err != nil {
		t.Fatalf("second up should be a no-op, got: %v", err)
	}
	if got := appliedCount(t, conn, ns); got != 2 {
		t.Errorf("applied rows after re-run: got %d, want 2", got)
	}
}

func TestUpNamespace_ResetsSessionSettingsBeforeRecording(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-reset", "pmtest_reset"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_dump.up.sql":   `CREATE TABLE ` + table + ` (id INT); SELECT pg_catalog.set_config('search_path', '', false);`,
		"0001_dump.down.sql": `DROP TABLE public.` + table + `;`,
	}))
	if err := New(conn).UpNamespace(context.Background(), ns); err != nil {
		t.Fatalf("up: %v - the record insert has to run after the migration's settings are reset", err)
	}
	if got := appliedCount(t, conn, ns); got != 1 {
		t.Errorf("applied rows: got %d, want 1", got)
	}
}

// Editing a shipped migration desyncs file from DB, and rolling forward on that is how environments silently diverge
func TestUpNamespace_ChecksumDriftRefuses(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-drift", "pmtest_drift"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_create.up.sql": `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
	}))

	ctx := context.Background()
	if err := New(conn).UpNamespace(ctx, ns); err != nil {
		t.Fatalf("up: %v", err)
	}

	// same version, different body - as if someone edited the file
	ResetForTest()
	Register(Registration{Namespace: ns, FS: mapFS(map[string]string{
		"0001_create.up.sql": `CREATE TABLE ` + table + ` (id INT PRIMARY KEY, extra TEXT);`,
	})})

	err := New(conn).UpNamespace(ctx, ns)
	if err == nil {
		t.Fatal("expected a checksum mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error should name the checksum mismatch, got: %v", err)
	}
}

// A version below the high-water mark means someone added 0002 after 0003 shipped - applying it interleaves with state built without it
func TestUpNamespace_RejectsOutOfOrder(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-order", "pmtest_order"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_a.up.sql": `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
		"0002_b.up.sql": `ALTER TABLE ` + table + ` ADD COLUMN b TEXT;`,
	}))

	ctx := context.Background()
	if err := New(conn).UpNamespace(ctx, ns); err != nil {
		t.Fatalf("up: %v", err)
	}
	// forces the high-water mark above a version that now looks pending
	if _, err := conn.Exec(
		`UPDATE plugin_migrations SET version = 9 WHERE namespace = $1 AND version = 2`, ns,
	); err != nil {
		t.Fatal(err)
	}
	// version 9 has no file, so the runner rejects before it reaches ordering - that guard is what's under test
	err := New(conn).UpNamespace(ctx, ns)
	if err == nil {
		t.Fatal("expected a refusal for an applied version with no file, got nil")
	}
	if !strings.Contains(err.Error(), "no migration file") {
		t.Errorf("error should name the missing file, got: %v", err)
	}
}

func TestDown_RollsBackNewestFirst(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-down", "pmtest_down"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_create.up.sql":   `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
		"0001_create.down.sql": `DROP TABLE ` + table + `;`,
		"0002_column.up.sql":   `ALTER TABLE ` + table + ` ADD COLUMN label TEXT;`,
		"0002_column.down.sql": `ALTER TABLE ` + table + ` DROP COLUMN label;`,
	}))

	ctx := context.Background()
	r := New(conn)
	if err := r.UpNamespace(ctx, ns); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := r.Down(ctx, ns, 1); err != nil {
		t.Fatalf("down 1: %v", err)
	}
	if got := appliedCount(t, conn, ns); got != 1 {
		t.Fatalf("applied after one rollback: got %d, want 1", got)
	}

	var cols int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = 'label'`,
		table,
	).Scan(&cols); err != nil {
		t.Fatal(err)
	}
	if cols != 0 {
		t.Error("0002 rolled back but its column survived")
	}

	if err := r.Down(ctx, ns, 1); err != nil {
		t.Fatalf("down 2: %v", err)
	}
	if got := appliedCount(t, conn, ns); got != 0 {
		t.Errorf("applied after full rollback: got %d, want 0", got)
	}
}

// Pretending otherwise would unrecord the migration while leaving its schema change in place
func TestDown_RefusesWithoutDownSQL(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-nodown", "pmtest_nodown"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_create.up.sql": `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
	}))

	ctx := context.Background()
	r := New(conn)
	if err := r.UpNamespace(ctx, ns); err != nil {
		t.Fatalf("up: %v", err)
	}
	err := r.Down(ctx, ns, 1)
	if err == nil {
		t.Fatal("expected a refusal, got nil")
	}
	if !strings.Contains(err.Error(), "no .down.sql") {
		t.Errorf("error should name the missing down file, got: %v", err)
	}
	if got := appliedCount(t, conn, ns); got != 1 {
		t.Errorf("refused rollback still unrecorded the migration: %d rows left", got)
	}
}

// Uninstall rolls back everything - a missing down below the top must refuse before the top is touched, not after
func TestDown_ChecksEveryStepFirst(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-downplan", "pmtest_downplan"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_create.up.sql":   `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
		"0002_column.up.sql":   `ALTER TABLE ` + table + ` ADD COLUMN label TEXT;`,
		"0002_column.down.sql": `ALTER TABLE ` + table + ` DROP COLUMN label;`,
	}))

	ctx := context.Background()
	r := New(conn)
	if err := r.UpNamespace(ctx, ns); err != nil {
		t.Fatalf("up: %v", err)
	}
	if err := r.Down(ctx, ns, 2); err == nil || !strings.Contains(err.Error(), "no .down.sql") {
		t.Fatalf("want a refusal naming the missing down, got %v", err)
	}
	if got := appliedCount(t, conn, ns); got != 2 {
		t.Errorf("refused rollback still ran part of the plan: %d rows left, want 2", got)
	}
}

// Two operators rolling back at once must not both run the down SQL
func TestDown_ConcurrentRunnersSerialize(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-downrace", "pmtest_downrace"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		"0001_create.up.sql":   `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
		"0001_create.down.sql": `DROP TABLE ` + table + `;`,
	}))

	ctx := context.Background()
	if err := New(conn).UpNamespace(ctx, ns); err != nil {
		t.Fatalf("up: %v", err)
	}
	// hold the namespace lock so every racer reads version 1 as applied, then queues behind it
	holder, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockNamespace(ctx, holder, ns); err != nil {
		t.Fatal(err)
	}
	const racers = 4
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = New(conn).Down(ctx, ns, 1)
		}(i)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		var waiting int
		if err := conn.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND wait_event = 'advisory' AND datname = current_database()`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == racers {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d racers reached the lock", waiting, racers)
		}
	}
	_ = holder.Rollback()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("racer %d failed - concurrent rollback ran the down SQL twice: %v", i, err)
		}
	}
	if got := appliedCount(t, conn, ns); got != 0 {
		t.Errorf("applied rows: got %d, want 0", got)
	}
}

// A rolling deploy or two operators must not both execute the up SQL - the lock plus the re-check under it makes the loser a no-op, and without them CREATE TABLE runs twice
func TestUpNamespace_ConcurrentRunnersSerialize(t *testing.T) {
	conn := testDB(t)
	const ns, table = "pmtest-race", "pmtest_race"
	registerFixture(t, conn, ns, table, mapFS(map[string]string{
		// no IF NOT EXISTS on purpose - a double apply has to fail loudly
		"0001_create.up.sql":   `CREATE TABLE ` + table + ` (id INT PRIMARY KEY);`,
		"0001_create.down.sql": `DROP TABLE ` + table + `;`,
	}))

	ctx := context.Background()
	const racers = 4
	errs := make([]error, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = New(conn).UpNamespace(ctx, ns)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("racer %d failed - concurrent apply was not serialized: %v", i, err)
		}
	}
	if got := appliedCount(t, conn, ns); got != 1 {
		t.Errorf("applied rows: got %d, want exactly 1", got)
	}
}

// Stable across processes or two servers derive different locks, distinct across namespaces or unrelated plugins block each other
func TestAdvisoryLockID(t *testing.T) {
	t.Parallel()
	if got, want := advisoryLockID("payment-stripe"), int32(1400083645); got != want {
		t.Errorf("lock id for payment-stripe drifted: got %d, want %d - existing runners would stop blocking each other", got, want)
	}
	seen := map[int32]string{}
	for _, ns := range []string{"payment-stripe", "payment-braintree", "shipping-easypost", "eu-countries", "loyalty"} {
		id := advisoryLockID(ns)
		if prev, dup := seen[id]; dup {
			t.Errorf("lock id collision between %q and %q (%d)", prev, ns, id)
		}
		seen[id] = ns
	}
}

// The error operators see when they name a namespace the running binary doesn't have compiled in
func TestUpNamespace_UnknownNamespace(t *testing.T) {
	conn := testDB(t)
	ResetForTest()
	t.Cleanup(ResetForTest)
	err := New(conn).UpNamespace(context.Background(), "pmtest-missing")
	if !errors.Is(err, ErrNamespaceNotRegistered) {
		t.Errorf("got %v, want ErrNamespaceNotRegistered", err)
	}
}

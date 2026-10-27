package migrate

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

// Each test migrates a schema of its own, named on the connection's search_path, so the clone's real tables stay out of reach
type rig struct {
	conn   *sql.DB
	dsn    string
	schema string
}

func newRig(t *testing.T) rig {
	t.Helper()
	conn := testdb.Require(t)
	dsn, reason := testdb.URL("DATABASE_URL")
	if dsn == "" {
		t.Skip(reason)
	}
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	schema := "migrate_" + hex.EncodeToString(suffix)
	if _, err := conn.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return rig{conn: conn, dsn: u.String(), schema: schema}
}

func (r rig) pool(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", r.dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func (r rig) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := r.conn.QueryRow(`SELECT count(*) FROM ` + r.schema + `.` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r rig) recorded(t *testing.T) []string {
	t.Helper()
	rows, err := r.conn.Query(`SELECT version FROM ` + r.schema + `.schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func files(bodies map[string]string) fstest.MapFS {
	out := fstest.MapFS{}
	for name, body := range bodies {
		out[name] = &fstest.MapFile{Data: []byte("-- migrate:up\n" + body + "\n-- migrate:down\n")}
	}
	return out
}

// Neither file survives a second run: the table already exists and the counter would read 2
var counted = files(map[string]string{
	"000001_table.sql": `CREATE TABLE counter (n int NOT NULL);`,
	"000002_row.sql":   `INSERT INTO counter VALUES (1);`,
})

func TestPostgresAppliesInVersionOrderAndThenNothing(t *testing.T) {
	r := newRig(t)
	db := r.pool(t)
	ctx := context.Background()

	got, err := Postgres(ctx, db, "test", counted)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"000001", "000002"}) {
		t.Fatalf("applied %v, want both in order", got)
	}
	again, err := Postgres(ctx, db, "test", counted)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run applied %v (%v), want nothing", again, err)
	}
	if n := r.count(t, "counter"); n != 1 {
		t.Errorf("counter holds %d rows, want 1", n)
	}
	if v := r.recorded(t); !slices.Equal(v, []string{"000001", "000002"}) {
		t.Errorf("schema_migrations = %v", v)
	}
}

func TestPostgresAppliesEachFileOnceAcrossConcurrentBoots(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()

	var mu sync.Mutex
	var all []string
	var wg sync.WaitGroup
	for range 6 {
		db := r.pool(t)
		wg.Go(func() {
			got, err := Postgres(ctx, db, "test", counted)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			all = append(all, got...)
			mu.Unlock()
		})
	}
	wg.Wait()
	slices.Sort(all)
	if !slices.Equal(all, []string{"000001", "000002"}) {
		t.Errorf("the boots applied %v between them, want each file once", all)
	}
	if n := r.count(t, "counter"); n != 1 {
		t.Errorf("counter holds %d rows, want 1", n)
	}
}

func TestPostgresRecordsAFileThatEmptiesTheSearchPath(t *testing.T) {
	r := newRig(t)
	got, err := Postgres(context.Background(), r.pool(t), "test", files(map[string]string{
		"000001_dump.sql": "SET lock_timeout = 0;\nSELECT pg_catalog.set_config('search_path', '', false);",
	}))
	if err != nil || len(got) != 1 {
		t.Fatalf("applied %v (%v) - the version insert has to run after the file's settings are reset", got, err)
	}
	if v := r.recorded(t); !slices.Equal(v, []string{"000001"}) {
		t.Errorf("schema_migrations = %v", v)
	}
}

func TestPostgresRollsBackAFailingFileAndKeepsTheOnesBefore(t *testing.T) {
	r := newRig(t)
	_, err := Postgres(context.Background(), r.pool(t), "test", files(map[string]string{
		"000001_ok.sql":     `CREATE TABLE kept (id int);`,
		"000002_broken.sql": `CREATE TABLE dropped (id int); SELECT no_such_column FROM kept;`,
	}))
	if err == nil || !strings.Contains(err.Error(), "000002_broken.sql") {
		t.Fatalf("err = %v, want the failing file named", err)
	}
	if v := r.recorded(t); !slices.Equal(v, []string{"000001"}) {
		t.Errorf("schema_migrations = %v, want only the file that committed", v)
	}
	var exists bool
	if err := r.conn.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, r.schema+".dropped").Scan(&exists); err != nil || exists {
		t.Errorf("the failing file's table survived (%v)", err)
	}
}

func TestPostgresLeavesAnEmptyFileForWhenItHasSQL(t *testing.T) {
	r := newRig(t)
	db := r.pool(t)
	ctx := context.Background()
	got, err := Postgres(ctx, db, "test", files(map[string]string{"000001_new.sql": "-- written by migrate new\n"}))
	if err != nil || len(got) != 0 {
		t.Fatalf("applied %v (%v), want the empty file skipped", got, err)
	}
	filled := files(map[string]string{"000001_new.sql": `CREATE TABLE filled (id int);`})
	if got, err := Postgres(ctx, db, "test", filled); err != nil || !slices.Equal(got, []string{"000001"}) {
		t.Fatalf("applied %v (%v) once the file had SQL, want it applied then", got, err)
	}
}

func TestPostgresRefusesAMigrationThatChangedAfterItRan(t *testing.T) {
	r := newRig(t)
	db := r.pool(t)
	ctx := context.Background()
	if _, err := Postgres(ctx, db, "test", counted); err != nil {
		t.Fatal(err)
	}
	edited := files(map[string]string{
		"000001_table.sql": `CREATE TABLE counter (n int NOT NULL);`,
		"000002_row.sql":   `INSERT INTO counter VALUES (1), (2);`,
	})
	if _, err := Postgres(ctx, db, "test", edited); err == nil || !strings.Contains(err.Error(), "000002_row.sql changed") {
		t.Fatalf("err = %v, want the edited file refused", err)
	}
	if n := r.count(t, "counter"); n != 1 {
		t.Errorf("counter holds %d rows - the edit must not run", n)
	}
}

func TestPostgresLeavesRowsDbmateWroteUnchecked(t *testing.T) {
	r := newRig(t)
	db := r.pool(t)
	ctx := context.Background()
	if _, err := Postgres(ctx, db, "test", fstest.MapFS{}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES ('000001')`); err != nil {
		t.Fatal(err)
	}
	got, err := Postgres(ctx, db, "test", files(map[string]string{"000001_table.sql": `CREATE TABLE never (id int);`}))
	if err != nil || len(got) != 0 {
		t.Fatalf("applied %v (%v), want the dbmate row read as applied whatever the file says", got, err)
	}
}

func TestPostgresRefusesTransactionFalse(t *testing.T) {
	r := newRig(t)
	_, err := Postgres(context.Background(), r.pool(t), "test", fstest.MapFS{
		"000001_concurrently.sql": {Data: []byte("-- migrate:up transaction:false\nSELECT 1;\n-- migrate:down\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "transaction:false") {
		t.Fatalf("err = %v, want transaction:false refused", err)
	}
	if v := r.recorded(t); len(v) != 0 {
		t.Errorf("schema_migrations = %v", v)
	}
}

func TestWithLockSerializesItsCallers(t *testing.T) {
	r := newRig(t)
	db := r.pool(t)
	ctx := context.Background()
	var mu sync.Mutex
	inside, most := 0, 0
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := WithLock(ctx, db, "test", func(context.Context) error {
				mu.Lock()
				inside++
				most = max(most, inside)
				mu.Unlock()
				_, _ = r.conn.Exec(`SELECT pg_sleep(0.05)`)
				mu.Lock()
				inside--
				mu.Unlock()
				return nil
			}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if most != 1 {
		t.Errorf("%d callers held the lock at once", most)
	}
}

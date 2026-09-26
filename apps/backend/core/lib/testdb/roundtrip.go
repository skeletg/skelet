package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var migrationName = regexp.MustCompile(`^(\d+)_.+\.(up|down)\.sql$`)

// Rebuilds the schema from empty, recording it after every migration, then walks each migration above floor down and up against those records
// conn must be disposable - the walk starts by dropping the public schema
func MigrationRoundTrip(t *testing.T, conn *sql.DB, dir string, floor int) {
	t.Helper()
	if os.Getenv("SKELET_TESTDB_SHARED") != "" {
		t.Skip("the shared database is the template itself - never drop its schema")
	}
	ups, downs := map[int]string{}, map[int]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		if m[2] == "up" {
			ups[v] = filepath.Join(dir, e.Name())
		} else {
			downs[v] = filepath.Join(dir, e.Name())
		}
	}
	var versions []int
	for v := range ups {
		if v > floor && downs[v] == "" {
			t.Errorf("migration %d has no .down.sql", v)
		}
		versions = append(versions, v)
	}
	slices.Sort(versions)

	if _, err := conn.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	recorded := map[int][]string{}
	for _, v := range versions {
		runFile(t, conn, ups[v])
		recorded[v] = schemaSnapshot(t, conn)
	}
	walked := slices.DeleteFunc(slices.Clone(versions), func(v int) bool { return v <= floor })
	if len(walked) == 0 {
		t.Skip("no migrations above the baseline")
	}
	below := func(v int) int { return versions[slices.Index(versions, v)-1] }

	for _, v := range slices.Backward(walked) {
		runFile(t, conn, downs[v])
		sameSchema(t, fmt.Sprintf("after %s", filepath.Base(downs[v])), recorded[below(v)], schemaSnapshot(t, conn))
	}
	for _, v := range walked {
		runFile(t, conn, ups[v])
		sameSchema(t, fmt.Sprintf("after re-running %s", filepath.Base(ups[v])), recorded[v], schemaSnapshot(t, conn))
	}
}

func sameSchema(t *testing.T, step string, want, got []string) {
	t.Helper()
	for _, line := range want {
		if !slices.Contains(got, line) {
			t.Errorf("%s: missing %s", step, line)
		}
	}
	for _, line := range got {
		if !slices.Contains(want, line) {
			t.Errorf("%s: unexpected %s", step, line)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

func runFile(t *testing.T, conn *sql.DB, path string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := conn.Exec(string(body)); err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
}

func schemaSnapshot(t *testing.T, conn *sql.DB) []string {
	t.Helper()
	const q = `
SELECT 'column ' || table_name || '.' || column_name || ' ' || data_type || ' null=' || is_nullable || ' default=' || COALESCE(column_default, '')
FROM information_schema.columns WHERE table_schema = 'public'
UNION ALL
SELECT 'index ' || indexdef FROM pg_indexes WHERE schemaname = 'public'
UNION ALL
SELECT 'constraint ' || cl.relname || '.' || c.conname || ' ' || pg_get_constraintdef(c.oid)
FROM pg_constraint c JOIN pg_class cl ON cl.oid = c.conrelid JOIN pg_namespace n ON n.oid = cl.relnamespace
WHERE n.nspname = 'public'
UNION ALL
SELECT 'function ' || pg_get_functiondef(p.oid) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'public' AND p.prokind IN ('f', 'p') AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = p.oid AND d.deptype = 'e')
UNION ALL
SELECT 'trigger ' || pg_get_triggerdef(tg.oid) FROM pg_trigger tg JOIN pg_class cl ON cl.oid = tg.tgrelid JOIN pg_namespace n ON n.oid = cl.relnamespace
WHERE n.nspname = 'public' AND NOT tg.tgisinternal
UNION ALL
SELECT 'view ' || viewname || ' ' || definition FROM pg_views WHERE schemaname = 'public'
UNION ALL
SELECT 'type ' || ty.typname || ' ' || string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder) FROM pg_type ty JOIN pg_enum e ON e.enumtypid = ty.oid
JOIN pg_namespace n ON n.oid = ty.typnamespace WHERE n.nspname = 'public' GROUP BY ty.typname
UNION ALL
SELECT 'sequence ' || sequencename FROM pg_sequences WHERE schemaname = 'public'`
	rows, err := conn.Query(q)
	if err != nil {
		t.Fatalf("snapshot schema: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("snapshot schema: %v", err)
		}
		out = append(out, strings.TrimSpace(line))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("snapshot schema: %v", err)
	}
	slices.Sort(out)
	return out
}

package migfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadSplitsSectionsAndListOrdersLikeDbmate(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "000002_two.sql", "-- migrate:up\nCREATE TABLE a (id int);\n\n-- migrate:down\nDROP TABLE a;\n\n-- migrate:up\nCREATE TABLE b (id int);\n\n-- migrate:down\nDROP TABLE b;\n")
	write(t, dir, "20261016153000_later.sql", "-- migrate:up transaction:false\nSELECT 1;\n\n-- migrate:down\n")
	write(t, dir, "000001_first.sql", "-- migrate:up\nSELECT 0;\n-- migrate:down\nSELECT 0;\n")
	write(t, dir, "README.md", "not a migration")

	files, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, f := range files {
		versions = append(versions, f.Version)
	}
	// a timestamped migration sorts after the numbered ones, as dbmate applies it
	if got := strings.Join(versions, ","); got != "000001,000002,20261016153000" {
		t.Fatalf("versions = %s", got)
	}
	two := files[1]
	if len(two.Ups) != 2 || two.Ups[1] != "CREATE TABLE b (id int);" {
		t.Errorf("ups = %q", two.Ups)
	}
	if two.Down() != "DROP TABLE b;\nDROP TABLE a;" {
		t.Errorf("down = %q, want the sections undone last first", two.Down())
	}
	if files[2].Up() != "SELECT 1;" || files[2].Down() != "" {
		t.Errorf("options on the marker leaked into the body: up %q down %q", files[2].Up(), files[2].Down())
	}
}

func TestListRefusesADuplicateVersion(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "000034_delivery_options.sql", "-- migrate:up\nSELECT 1;\n-- migrate:down\n")
	write(t, dir, "000034_integration_actor.sql", "-- migrate:up\nSELECT 2;\n-- migrate:down\n")
	if _, err := List(dir); err == nil || !strings.Contains(err.Error(), "000034") {
		t.Errorf("List = %v, want the shared version named", err)
	}
}

func TestReadRefusesAFileWithoutAnUpSection(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "000001_bare.sql", "CREATE TABLE a (id int);\n")
	if _, err := Read(filepath.Join(dir, "000001_bare.sql")); err == nil {
		t.Error("a file with no markers read as a migration")
	}
}

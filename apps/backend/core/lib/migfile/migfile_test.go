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
	if !files[2].NoTransaction || files[0].NoTransaction || two.NoTransaction {
		t.Errorf("transaction:false read as %v %v %v, want only the third file", files[0].NoTransaction, two.NoTransaction, files[2].NoTransaction)
	}
}

func TestListFSReadsTheSameAsADirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "000002_two.sql", "-- migrate:up\nSELECT 2;\n-- migrate:down\n")
	write(t, dir, "000001_one.sql", "-- migrate:up\nSELECT 1;\n-- migrate:down\n")
	write(t, dir, "embed.go", "package migrations")

	fromDir, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	fromFS, err := ListFS(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(fromFS) != 2 || len(fromDir) != 2 {
		t.Fatalf("read %d from the FS and %d from the directory, want 2 each", len(fromFS), len(fromDir))
	}
	for i := range fromFS {
		if fromFS[i].Version != fromDir[i].Version || fromFS[i].Up() != fromDir[i].Up() {
			t.Errorf("file %d: FS %s %q, directory %s %q", i, fromFS[i].Version, fromFS[i].Up(), fromDir[i].Version, fromDir[i].Up())
		}
	}
	if fromFS[0].Path != "000001_one.sql" || fromDir[0].Path != filepath.Join(dir, "000001_one.sql") {
		t.Errorf("paths %q and %q", fromFS[0].Path, fromDir[0].Path)
	}
}

func TestBlankSeesThroughComments(t *testing.T) {
	for section, want := range map[string]bool{
		"":                            true,
		"-- a note\n\n  -- another":   true,
		"-- a note\nCREATE TABLE a()": false,
		"SELECT 1 -- trailing":        false,
	} {
		if got := Blank(section); got != want {
			t.Errorf("Blank(%q) = %v, want %v", section, got, want)
		}
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

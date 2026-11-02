package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// rm channel kept only .skelet-apps current, so where the lists and the old profile disagree the lists are the truth
func TestMigrateFoldsTheRootFilesIntoOneProfile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".skelet-profile": "SKELET_TARGET=dev\nSKELET_DOCKER_APPS=dashboard,gone-shop\nSKELET_TOOLS_SELECTED=mailpit\n",
		".skelet-apps":    "dashboard, vetrina\n",
		".skelet-tools":   "mailpit,grafana\n",
	})
	moved, err := Migrate(root)
	if err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	p, err := Load(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{TargetKey: "dev", DockerKey: "dashboard,vetrina", ToolsKey: "mailpit,grafana"} {
		if p[k] != want {
			t.Errorf("%s = %q, want %q", k, p[k], want)
		}
	}
	for _, f := range []string{".skelet-profile", ".skelet-apps", ".skelet-tools"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			t.Errorf("%s should be gone after the move", f)
		}
	}
	if moved, err := Migrate(root); moved || err != nil {
		t.Errorf("a second run should find nothing to move, got %v, %v", moved, err)
	}
}

func TestMigrateTakesListsWithNoProfile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".skelet-apps": "dashboard\n"})
	if _, err := Migrate(root); err != nil {
		t.Fatal(err)
	}
	p, _ := Load(Path(root))
	if docker, ok := p.List(DockerKey); !ok || len(docker) != 1 || docker[0] != "dashboard" {
		t.Errorf("docker apps %v, %v", docker, ok)
	}
	if _, ok := p[ToolsKey]; ok {
		t.Error("a list that wasn't there shouldn't become an empty answer")
	}
}

func TestMigrateLeavesAnExistingProfileAlone(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{File: "SKELET_TARGET=prod\n", ".skelet-profile": "SKELET_TARGET=dev\n", ".skelet-apps": "dashboard\n"})
	if _, err := Migrate(root); err != nil {
		t.Fatal(err)
	}
	if p, _ := Load(Path(root)); p[TargetKey] != "prod" || p[DockerKey] != "" {
		t.Errorf("the existing profile changed: %v", p)
	}
	if _, err := os.Stat(filepath.Join(root, ".skelet-profile")); err == nil {
		t.Error("the stale root file should be cleared")
	}
}

func TestUpdateKeepsTheOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile")
	writeFiles(t, filepath.Dir(path), map[string]string{"profile": "B=1\nA=2\n"})
	if err := Update(path, func(p Profile) { p["A"] = "3"; p["C"] = "4" }); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if got := strings.SplitN(string(raw), "\n", 2)[1]; got != "B=1\nA=3\nC=4\n" {
		t.Errorf("wrote %q", got)
	}
}

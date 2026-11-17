package build

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMain(m *testing.M) {
	regen = func(string) int { panic("a test reached skelet gen") }
	image = func(string, string, string) int { panic("a test reached docker") }
	pnpmIn = func(string, string, ...string) error { panic("a test reached pnpm") }
	os.Exit(m.Run())
}

type calls struct{ log []string }

func (c *calls) fake() {
	regen = func(string) int { c.log = append(c.log, "gen"); return 0 }
	image = func(_, name, tag string) int { c.log = append(c.log, "image "+name+":"+tag); return 0 }
	pnpmIn = func(_, app string, args ...string) error {
		c.log = append(c.log, "pnpm "+app+" "+args[0]+" "+args[1])
		return nil
	}
}

// With no names, core and the dashboard - regenerated first, so the plugin set is the one built
func TestBuildRegeneratesThenBuildsCoreAndTheDashboard(t *testing.T) {
	c := &calls{}
	c.fake()
	if code := Run(t.TempDir(), nil); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := []string{"gen", "image core:prod", "pnpm dashboard run build"}
	if !reflect.DeepEqual(c.log, want) {
		t.Errorf("ran %v, want %v", c.log, want)
	}
}

func TestBuildTakesNamesAndATag(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "apps", "channels", "shop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "apps", "channels", "shop", "package.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &calls{}
	c.fake()
	if code := Run(root, []string{"signals", "shop", "--tag", "v2"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := []string{"gen", "image signals:v2", "pnpm shop run build"}
	if !reflect.DeepEqual(c.log, want) {
		t.Errorf("ran %v, want %v", c.log, want)
	}

	c.log = nil
	if code := Run(root, []string{"nothing"}); code != 2 || len(c.log) != 0 {
		t.Errorf("an unknown name: exit %d after %v", code, c.log)
	}
}

package storefronts

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func put(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestList(t *testing.T) {
	root := t.TempDir()
	put(t, root, "templates/vetrina/package.json")
	put(t, root, "templates/astro-shop/package.json")
	put(t, root, "apps/channels/shop/package.json")
	put(t, root, "apps/channels/README.md")
	// A directory without a package.json isn't a storefront - packages/ and leftovers alike
	if err := os.MkdirAll(filepath.Join(root, "apps/channels/half"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := List(root)
	want := []Storefront{{Name: "shop", Dir: "apps/channels/shop"}, {Name: "astro-shop", Dir: "templates/astro-shop"}, {Name: "vetrina", Dir: "templates/vetrina"}}
	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
	if s, ok := Find(root, "vetrina"); !ok || !s.Template() || s.Dir != "templates/vetrina" {
		t.Errorf("Find vetrina = %v %v", s, ok)
	}
	if s, ok := Find(root, "shop"); !ok || s.Template() {
		t.Errorf("Find shop = %v %v", s, ok)
	}
	if _, ok := Find(root, "half"); ok {
		t.Error("a directory with no package.json was found")
	}
	if !Exists(root, "half") || Exists(root, "nope") {
		t.Error("Exists")
	}
	if !slices.Equal(Names(root), []string{"shop", "astro-shop", "vetrina"}) {
		t.Errorf("Names = %v", Names(root))
	}
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestKnownFollowsSparseCheckout(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"templates/astro-shop", "templates/" + DefaultTemplate, "packages/core", "apps/channels/mine", "apps/channels/gone"} {
		put(t, root, f+"/package.json")
	}
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "base")
	git(t, root, "sparse-checkout", "set", "--no-cone", "/*", "!/templates/"+DefaultTemplate+"/", "!/packages/core/")
	if err := os.RemoveAll(filepath.Join(root, "apps/channels/gone")); err != nil {
		t.Fatal(err)
	}
	if got, want := Excluded(root), []string{"packages/core", "templates/" + DefaultTemplate}; !slices.Equal(got, want) {
		t.Errorf("Excluded = %v, want %v", got, want)
	}
	known := Known(root)
	want := []Storefront{
		{Name: "mine", Dir: "apps/channels/mine"},
		{Name: "astro-shop", Dir: "templates/astro-shop"},
		{Name: DefaultTemplate, Dir: "templates/" + DefaultTemplate, Excluded: true},
	}
	if !slices.Equal(known, want) {
		t.Errorf("Known = %v, want %v", known, want)
	}
	if s, ok := Default(known); !ok || s.Name != DefaultTemplate {
		t.Errorf("Default = %v %v, want the default template though it isn't checked out", s, ok)
	}
	if s, ok := Default(known[:2]); !ok || s.Name != "astro-shop" {
		t.Errorf("Default without the default template = %v %v, want the first template", s, ok)
	}
	if _, ok := Default(known[:1]); ok {
		t.Error("Default found a template among none")
	}
	if plain := t.TempDir(); len(Known(plain)) != 0 || len(Excluded(plain)) != 0 {
		t.Error("an empty directory outside git knows storefronts")
	}
}

func TestPinTakesOnlyTheWorkspaceLinks(t *testing.T) {
	in := `{
  "dependencies": {
    "@skelet/storefront-core": "workspace:*",
    "@skelet/storefront-react": "0.3.0",
    "react": "catalog:"
  },
  "devDependencies": { "@skelet/storefront-next":"workspace:^" }
}`
	got := Pin(in, "0.3.1")
	for _, want := range []string{`"@skelet/storefront-core": "0.3.1"`, `"@skelet/storefront-react": "0.3.0"`, `"react": "catalog:"`, `"@skelet/storefront-next":"0.3.1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

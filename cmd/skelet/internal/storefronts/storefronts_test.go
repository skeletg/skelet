package storefronts

import (
	"os"
	"path/filepath"
	"slices"
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
	want := []Storefront{{"shop", "apps/channels/shop"}, {"astro-shop", "templates/astro-shop"}, {"vetrina", "templates/vetrina"}}
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

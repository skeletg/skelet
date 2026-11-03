package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShopsBehindTheBackendAreFlagged(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("CHANGELOG.md", "- [0.3.2](changelog/0.3.2.md) - 2026-11-20\n- [0.3.1](changelog/0.3.1.md) - 2026-11-01\n")
	shop := func(name, core, next string) {
		write("apps/channels/"+name+"/package.json", `{"dependencies": {"@skelet/storefront-core": "`+core+`", "@skelet/storefront-next": "`+next+`"}}`)
	}
	shop("behind", "0.3.1", "0.3.1")
	shop("current", "0.3.2", "^0.3.2")
	shop("linked", "workspace:*", "workspace:*")
	shop("mixed", "0.3.1", "0.3.2")
	write("templates/vetrina/package.json", `{"dependencies": {"@skelet/storefront-core": "workspace:*"}}`)

	var out strings.Builder
	r := &report{out: &out}
	sharedPackages(r, root)
	got := out.String()
	for _, want := range []string{
		"warn  behind takes @skelet/storefront-* 0.3.1 from npm, but this checkout is 0.3.2",
		"skelet pnpm behind add @skelet/storefront-core@0.3.2 @skelet/storefront-next@0.3.2",
		"ok    current takes @skelet/storefront-* 0.3.2 from npm",
		"ok    linked links @skelet/storefront-* to packages/",
		"warn  mixed mixes @skelet/storefront-* versions",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "vetrina") {
		t.Errorf("templates link packages/ by design and shouldn't be reported:\n%s", got)
	}
	if r.warn != 2 || r.ok != 2 {
		t.Errorf("want 2 ok and 2 warnings, got %d and %d", r.ok, r.warn)
	}
}

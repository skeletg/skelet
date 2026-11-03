package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const forkManifest = `{
  "name": "eu-shop",
  "dependencies": {
    "@skelet/storefront-core": "workspace:*",
    "@skelet/storefront-next": "workspace:*",
    "next": "catalog:"
  }
}
`

func releasedCheckout(t *testing.T) (root, fork string) {
	t.Helper()
	root = t.TempDir()
	put(t, root, "CHANGELOG.md", "# Changelog\n\n## Releases\n\n- [0.3.1](changelog/0.3.1.md) - 2026-11-01\n- [0.3.0](changelog/0.3.0.md) - 2026-10-01\n", 0o644)
	put(t, root, "packages/storefront-core/cart.ts", "export const cart = 1\n", 0o644)
	put(t, root, "templates/vetrina/page.tsx", "export default 1\n", 0o644)
	put(t, root, "packages/storefront-core/graphql/schema.types.ts", "export type Query = { cart: Cart }\n", 0o644)
	put(t, root, "packages/storefront-core/cart/queries.gql.ts", "export const CartDocument = {}\n", 0o644)
	put(t, root, "templates/vetrina/src/graphql/schema.types.ts", "export type Query = { cart: Cart }\n", 0o644)
	git(t, root, "init", "-q")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "release")
	git(t, root, "tag", "v0.3.1")
	fork = t.TempDir()
	put(t, fork, "package.json", forkManifest, 0o644)
	return root, fork
}

func onNpm(ok bool, err error) func() {
	saved := published
	published = func(string) (bool, error) { return ok, err }
	return func() { published = saved }
}

func manifest(t *testing.T, fork string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fork, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAForkAtTheReleaseTakesItFromNpm(t *testing.T) {
	defer onNpm(true, nil)()
	root, fork := releasedCheckout(t)
	version, kept := pinPackages(root, "templates/vetrina", fork, false)
	if version != "0.3.1" || kept != "" {
		t.Fatalf("got %q, kept %q - want 0.3.1 from npm", version, kept)
	}
	got := manifest(t, fork)
	for _, want := range []string{`"@skelet/storefront-core": "0.3.1"`, `"@skelet/storefront-next": "0.3.1"`, `"next": "catalog:"`} {
		if !strings.Contains(got, want) {
			t.Errorf("package.json should have %s:\n%s", want, got)
		}
	}
}

func TestAForkKeepsTheWorkspaceWhenTheCheckoutIsAheadOfItsRelease(t *testing.T) {
	defer onNpm(true, nil)()
	for _, changed := range []string{"packages/storefront-core/cart.ts", "templates/vetrina/page.tsx"} {
		root, fork := releasedCheckout(t)
		put(t, root, changed, "export const moved = 2\n", 0o644)
		if version, kept := pinPackages(root, "templates/vetrina", fork, false); version != "" || !strings.Contains(kept, "changed since v0.3.1") {
			t.Errorf("%s changed: got %q, kept %q", changed, version, kept)
		}
		if manifest(t, fork) != forkManifest {
			t.Errorf("%s changed: the fork's package.json should be untouched", changed)
		}
	}
}

func TestAForkStillTakesNpmAfterAPluginRegeneratesTheSchemaTypes(t *testing.T) {
	defer onNpm(true, nil)()
	root, fork := releasedCheckout(t)
	plugin := "export type Query = { cart: Cart; affirmClientConfig?: AffirmClientConfig }\nexport type AffirmClientConfig = { publicApiKey: string }\n"
	put(t, root, "packages/storefront-core/graphql/schema.types.ts", plugin, 0o644)
	put(t, root, "templates/vetrina/src/graphql/schema.types.ts", plugin, 0o644)
	put(t, root, "packages/storefront-core/cart/queries.gql.ts", "import type { AffirmClientConfig } from \"../graphql/schema.types\"\nexport const CartDocument = {}\n", 0o644)
	if version, kept := pinPackages(root, "templates/vetrina", fork, false); version != "0.3.1" || kept != "" {
		t.Errorf("regenerated codegen output shouldn't count as the checkout moving on: got %q, kept %q", version, kept)
	}
}

func TestAForkKeepsTheWorkspaceWhenNpmCantServeTheRelease(t *testing.T) {
	for name, c := range map[string]struct {
		ok   bool
		err  error
		want string
	}{
		"not published": {false, nil, "isn't on npm"},
		"unreachable":   {false, errors.New("dial tcp: no route"), "couldn't ask npm"},
	} {
		func() {
			defer onNpm(c.ok, c.err)()
			root, fork := releasedCheckout(t)
			if version, kept := pinPackages(root, "templates/vetrina", fork, false); version != "" || !strings.Contains(kept, c.want) {
				t.Errorf("%s: got %q, kept %q", name, version, kept)
			}
		}()
	}
}

func TestAForkKeepsTheWorkspaceWhenAskedOrUnsure(t *testing.T) {
	defer onNpm(true, nil)()
	root, fork := releasedCheckout(t)
	if _, kept := pinPackages(root, "templates/vetrina", fork, true); !strings.Contains(kept, "--workspace") {
		t.Errorf("--workspace should keep the link, kept %q", kept)
	}
	git(t, root, "tag", "-d", "v0.3.1")
	if _, kept := pinPackages(root, "templates/vetrina", fork, false); !strings.Contains(kept, "no v0.3.1 tag") {
		t.Errorf("with no tag to compare against the link should stay, kept %q", kept)
	}
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte("# Changelog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, kept := pinPackages(root, "templates/vetrina", fork, false); !strings.Contains(kept, "no release") {
		t.Errorf("before the first release the link should stay, kept %q", kept)
	}
	if manifest(t, fork) != forkManifest {
		t.Error("a fork that keeps the link should be untouched")
	}
}

func TestAForkWithNoWorkspaceLinksIsLeftAlone(t *testing.T) {
	defer onNpm(true, nil)()
	root, fork := releasedCheckout(t)
	pinned := strings.ReplaceAll(forkManifest, "workspace:*", "0.3.0")
	put(t, fork, "package.json", pinned, 0o644)
	if version, kept := pinPackages(root, "", fork, false); version != "" || kept != "" || manifest(t, fork) != pinned {
		t.Errorf("a copy already on npm should keep its versions, got %q, kept %q", version, kept)
	}
}

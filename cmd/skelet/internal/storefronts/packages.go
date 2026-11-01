package storefronts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var sharedPackage = regexp.MustCompile(`"(@skelet/storefront-[a-z-]+)"(\s*:\s*)"([^"]*)"`)

// SharedDeps is the @skelet/storefront-* a storefront depends on, each with the range it asks for - workspace:* links packages/
func SharedDeps(root, dir string) map[string]string {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), "package.json"))
	var manifest struct{ Dependencies, DevDependencies map[string]string }
	if err != nil || json.Unmarshal(raw, &manifest) != nil {
		return nil
	}
	out := map[string]string{}
	for _, deps := range []map[string]string{manifest.Dependencies, manifest.DevDependencies} {
		for name, spec := range deps {
			if strings.HasPrefix(name, "@skelet/storefront-") {
				out[name] = spec
			}
		}
	}
	return out
}

// Pin points a package.json's workspace links to the shared packages at version on npm, leaving the rest as written
func Pin(manifest, version string) string {
	return sharedPackage.ReplaceAllStringFunc(manifest, func(dep string) string {
		m := sharedPackage.FindStringSubmatch(dep)
		if !strings.HasPrefix(m[3], "workspace:") {
			return dep
		}
		return `"` + m[1] + `"` + m[2] + `"` + version + `"`
	})
}

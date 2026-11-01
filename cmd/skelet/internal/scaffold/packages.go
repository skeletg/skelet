package scaffold

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/skeletg/skelet/cmd/skelet/internal/changelog"
	"github.com/skeletg/skelet/cmd/skelet/internal/gitx"
	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

// A var, so a test never reaches the registry
var published = func(version string) (bool, error) {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://registry.npmjs.org/@skelet%2fstorefront-core/" + version)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("npm answered %s", resp.Status)
}

// A checkout ahead of its release may use what npm doesn't have yet, so the fork keeps linking packages/ and says why
func pinPackages(root, template, dir string, workspace bool) (version, kept string) {
	manifest := filepath.Join(dir, "package.json")
	raw, err := os.ReadFile(manifest)
	if err != nil || storefronts.Pin(string(raw), "") == string(raw) {
		return "", ""
	}
	if workspace {
		return "", "--workspace asked for it"
	}
	release := changelog.Latest(root)
	if release == "" {
		return "", "CHANGELOG.md lists no release yet"
	}
	tag := "v" + release
	if !gitx.HasTag(root, tag) {
		return "", "there's no " + tag + " tag here to check packages/ against"
	}
	paths := []string{"packages"}
	if template != "" {
		paths = append(paths, template)
	}
	switch changed, err := gitx.ChangedSince(root, tag, paths...); {
	case err != nil:
		return "", fmt.Sprintf("couldn't compare packages/ with %s (%v)", tag, err)
	case changed:
		return "", fmt.Sprintf("packages/ or the template changed since %s, so npm's %s may not match them", tag, release)
	}
	switch ok, err := published(release); {
	case err != nil:
		return "", fmt.Sprintf("couldn't ask npm for %s (%v)", release, err)
	case !ok:
		return "", release + " isn't on npm"
	}
	if err := writeKeepingMode(manifest, storefronts.Pin(string(raw), release)); err != nil {
		return "", fmt.Sprintf("couldn't write %s (%v)", manifest, err)
	}
	return release, ""
}

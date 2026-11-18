package pluginset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

// UIManifest is the file at a plugin's root naming the npm packages that hold its dashboard and storefront halves -
// Go modules carry no TypeScript worth shipping, so the UI is published to npm and skelet plugin add installs it
const UIManifest = "skelet-plugin.json"

type UI struct {
	Dashboard  string `json:"dashboard,omitempty"`
	Storefront string `json:"storefront,omitempty"`
}

// An npm package name, scoped or not, and optionally a registry version, range or tag. Nothing with a colon or a slash
// after the @ - no npm: alias, URL, git or file source - since the spec comes from a plugin's own manifest and goes to
// pnpm add. cmd/skelet/internal/plugins holds the same pattern, and its tests hold the two together
var packageSpec = regexp.MustCompile(`^(@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*(@[\^~]?[A-Za-z0-9][A-Za-z0-9.*+_-]*)?$`)

// ValidPackageSpec reports whether spec is one a plugin's manifest or plugins.json may name
func ValidPackageSpec(spec string) bool { return packageSpec.MatchString(spec) }

// ReadUI reads a plugin's UI manifest - the zero UI when it has none, an error when it's there and malformed
func ReadUI(dir string) (UI, error) {
	raw, err := os.ReadFile(filepath.Join(dir, UIManifest))
	if errors.Is(err, fs.ErrNotExist) {
		return UI{}, nil
	}
	if err != nil {
		return UI{}, err
	}
	// A misspelt key would otherwise read as a plugin with no UI
	var ui UI
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ui); err != nil {
		return UI{}, fmt.Errorf("%s: %w", UIManifest, err)
	}
	for _, spec := range []string{ui.Dashboard, ui.Storefront} {
		if spec != "" && !packageSpec.MatchString(spec) {
			return UI{}, fmt.Errorf("%s: %q is not an npm package name, with an optional @version", UIManifest, spec)
		}
	}
	return ui, nil
}

// SetUI records a plugin's UI packages, leaving its decision and module as they are
func (s *Set) SetUI(slug string, ui UI) {
	if s.Plugins == nil {
		s.Plugins = map[string]Entry{}
	}
	e := s.Plugins[slug]
	e.Dashboard, e.Storefront = ui.Dashboard, ui.Storefront
	s.Plugins[slug] = e
}

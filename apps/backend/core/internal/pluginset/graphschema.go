package pluginset

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	beginMarker = "  # BEGIN plugin schema"
	endMarker   = "  # END plugin schema"
)

// Globbing plugins/*/graphql/<surface> in the gqlgen configs would make the schema directory-driven while registration stays import-driven, so a disabled plugin's fields stay advertised with nothing behind them - writing the enabled set into both configs makes the two agree by construction
func WriteGraphSchemas(root string) error {
	set, err := Load(root)
	if err != nil {
		return err
	}
	for _, surface := range []string{"admin", "front"} {
		var lines []string
		for _, slug := range set.EnabledSlugs() {
			// In-tree plugins are globbed where they sit; a module plugin's SDL was copied into
			// StageDir by Stage, because its real home is a versioned path in the module cache
			base := filepath.Join(pluginsDir, slug)
			if !set.InTree(slug) {
				base = filepath.Join(StageDir, slug)
			}
			pattern := filepath.Join(base, "graphql", surface, "*.graphqls")
			matches, err := filepath.Glob(filepath.Join(root, pattern))
			if err != nil {
				return err
			}
			if len(matches) > 0 {
				lines = append(lines, "  - "+filepath.ToSlash(pattern))
			}
		}
		if err := replaceRegion(filepath.Join(root, "gqlgen."+surface+".yml"), lines); err != nil {
			return err
		}
	}
	return nil
}

// Rewrites only what sits between the markers, so the surrounding config stays hand-written
func replaceRegion(path string, lines []string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(raw)
	begin := strings.Index(src, beginMarker)
	if begin < 0 {
		return fmt.Errorf("%s: no %q marker - the plugin schema list has to live in a generated region", path, strings.TrimSpace(beginMarker))
	}
	end := strings.Index(src[begin:], endMarker)
	if end < 0 {
		return fmt.Errorf("%s: %q without a matching END", path, strings.TrimSpace(beginMarker))
	}
	end += begin

	var body strings.Builder
	body.WriteString(beginMarker + " - generated from plugins/plugins.json by cmd/plugingen\n")
	for _, l := range lines {
		body.WriteString(l + "\n")
	}
	return os.WriteFile(path, []byte(src[:begin]+body.String()+src[end:]), 0o644)
}

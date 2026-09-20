// Package pluginpath translates kebab-case plugin slugs to Go package names, shared by cmd/plugingraph and the CLI scaffolder so they agree
package pluginpath

import "strings"

func PackageName(slug string) string {
	s := strings.ReplaceAll(slug, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

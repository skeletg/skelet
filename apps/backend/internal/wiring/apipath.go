package wiring

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Moves a GraphQL endpoint off a known URL - it cuts automated scanning, and that is all:
// authorization is still per resolver, and the segment leaks through Referer and history
const (
	adminAPIBase = "/api/admin"
	frontAPIBase = "/api/front"

	AdminSlugEnv = "ADMIN_API_SLUG"
	FrontSlugEnv = "FRONT_API_SLUG"
)

var apiSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,63}$`) // survives a URL path, a shell var and an nginx location unquoted

func APIPaths(getenv func(string) string) (admin, front string, err error) { // refuses a malformed slug rather than mounting a route nobody can reach
	admin, err = apiPath(adminAPIBase, AdminSlugEnv, getenv(AdminSlugEnv))
	if err != nil {
		return "", "", err
	}
	front, err = apiPath(frontAPIBase, FrontSlugEnv, getenv(FrontSlugEnv))
	if err != nil {
		return "", "", err
	}
	return admin, front, nil
}

func apiPath(base, envName, slug string) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return base + "/graphql", nil // unset keeps the historical path, so each surface is opt-in
	}
	if !apiSlugPattern.MatchString(slug) {
		return "", fmt.Errorf("%s must be 8-64 chars of letters, digits, - or _ and start alphanumeric, got %q", envName, slug)
	}
	return base + "/" + slug + "/graphql", nil
}

func adminAPIPath() string { // for the boot log and the dev playground - the mount resolves its own
	p, _, err := APIPaths(os.Getenv)
	if err != nil {
		return adminAPIBase + "/graphql"
	}
	return p
}

func frontAPIPath() string {
	_, p, err := APIPaths(os.Getenv)
	if err != nil {
		return frontAPIBase + "/graphql"
	}
	return p
}

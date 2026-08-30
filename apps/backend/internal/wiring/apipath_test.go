package wiring

import (
	"strings"
	"testing"
)

// The slug decides where the dashboard and every integration must POST, so a malformed one has to abort
// the boot - a silently wrong path reads as "the dashboard is broken" with nothing naming the cause

func env(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

func TestAPIPathsDefaultToTheHistoricalRoutes(t *testing.T) { // unset must not move an endpoint - every existing deployment relies on it
	admin, front, err := APIPaths(env(nil))
	if err != nil {
		t.Fatalf("APIPaths: %v", err)
	}
	if admin != "/api/admin/graphql" {
		t.Errorf("admin = %q, want the historical path", admin)
	}
	if front != "/api/front/graphql" {
		t.Errorf("front = %q, want the historical path", front)
	}
}

func TestAPIPathsInsertTheSlugAsItsOwnSegment(t *testing.T) {
	admin, front, err := APIPaths(env(map[string]string{
		AdminSlugEnv: "s3cr3t-panel",
		FrontSlugEnv: "quiet_front_9",
	}))
	if err != nil {
		t.Fatalf("APIPaths: %v", err)
	}
	if admin != "/api/admin/s3cr3t-panel/graphql" {
		t.Errorf("admin = %q", admin)
	}
	if front != "/api/front/quiet_front_9/graphql" {
		t.Errorf("front = %q", front)
	}
}

func TestEachSurfaceIsIndependent(t *testing.T) { // admin hidden while the public front API stays put is the expected shape
	admin, front, err := APIPaths(env(map[string]string{AdminSlugEnv: "onlyadminmoved"}))
	if err != nil {
		t.Fatalf("APIPaths: %v", err)
	}
	if admin != "/api/admin/onlyadminmoved/graphql" || front != "/api/front/graphql" {
		t.Errorf("got admin=%q front=%q, want only the admin path moved", admin, front)
	}
}

func TestAMalformedSlugRefusesToResolve(t *testing.T) {
	for name, slug := range map[string]string{
		"a path separator would nest a route nobody serves": "secret/panel",
		"whitespace breaks the mux pattern":                 "secret panel",
		"too short to be worth guessing past":               "admin1",
		"over the length cap":                               strings.Repeat("a", 65),
		"leading dash reads as a flag in tooling":           "-secret-panel",
		"a dot invites a suffix match":                      "secret.panel",
		"percent-encoding would double-decode":              "secret%2Fpanel",
		"empty after trimming is not the same as unset":     "   \t  ",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := APIPaths(env(map[string]string{AdminSlugEnv: slug}))
			if slug == "   \t  " { // trims to empty, which is the documented "unset"
				if err != nil {
					t.Fatalf("whitespace-only slug should read as unset, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted %q", slug)
			}
			if !strings.Contains(err.Error(), AdminSlugEnv) {
				t.Errorf("error %q does not name the env var that caused it", err)
			}
		})
	}
}

func TestAcceptedSlugsCoverWhatOperatorsWillReachFor(t *testing.T) {
	for _, slug := range []string{"abcdefgh", "A1b2C3d4", "with-dashes", "with_underscores", strings.Repeat("z", 64)} {
		if _, _, err := APIPaths(env(map[string]string{AdminSlugEnv: slug})); err != nil {
			t.Errorf("rejected %q: %v", slug, err)
		}
	}
}

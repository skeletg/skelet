package route

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../../../.."

// The front door and bin/ agree in both directions, and bin/repo-check can count the routes by grepping this file
func TestTableAndBinAgree(t *testing.T) {
	routed := map[string]bool{}
	for _, r := range routes {
		routed[r.Script] = true
		fi, err := os.Stat(filepath.Join(repoRoot, "bin", r.Script))
		if err != nil || fi.Mode()&0o111 == 0 {
			t.Errorf("%s %s routes to bin/%s, which is missing or not executable", r.Verb, r.Sub, r.Script)
		}
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "_") && !routed[e.Name()] {
			t.Errorf("bin/%s has no verb", e.Name())
		}
	}

	src, err := os.ReadFile("table.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(regexp.MustCompile(`Script: "[a-z][a-z0-9-]*"`).FindAll(src, -1)); n != len(routes) {
		t.Errorf("bin/repo-check's grep finds %d routes in table.go, the table holds %d - keep one Script: per route", n, len(routes))
	}
}

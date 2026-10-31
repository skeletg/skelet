package route

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const repoRoot = "../../../.."

// The front door and bin/ agree in both directions, and skelet check structure can count the routes by grepping this file
func TestTableAndBinAgree(t *testing.T) {
	routed := map[string]bool{}
	scripts := 0
	for _, r := range routes {
		if r.Go != nil {
			if r.Script != "" || r.About == "" {
				t.Errorf("%s is a Go verb, so it takes About and no Script", r.Verb)
			}
			continue
		}
		scripts++
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
		// bin/check only hands hooks from before skelet hooks to the CLI, and has no verb of its own
		if !strings.HasPrefix(e.Name(), "_") && !routed[e.Name()] && e.Name() != "check" {
			t.Errorf("bin/%s has no verb", e.Name())
		}
	}

	src, err := os.ReadFile("table.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(regexp.MustCompile(`Script: "[a-z][a-z0-9-]*"`).FindAll(src, -1)); n != scripts {
		t.Errorf("skelet check structure's grep finds %d routes in table.go, the table holds %d - keep one Script: per route", n, scripts)
	}
}

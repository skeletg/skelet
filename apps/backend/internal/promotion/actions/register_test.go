package actions

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/skeletg/skelet/internal/promotion"
)

// Every rule type in the package declares Slug, so counting those is a source of truth independent of the add()
// calls themselves - comparing the registry against builtins would only ever compare the list to itself, and a
// new type that never calls add is exactly the mistake worth catching, because it shows up in production as a
// rule that quietly matches nothing
func TestEveryRuleTypeIsRegistered(t *testing.T) {
	declared := countSlugMethods(t, ".")

	r := promotion.NewRegistry()
	Register(r)
	if got := len(r.AllActions()); got != declared {
		t.Errorf("%d types declare Slug but %d registered - a file is missing its add() call", declared, got)
	}
	Register(r) // idempotent, or the registry panics on the duplicate slug
}

var slugMethod = regexp.MustCompile(`func \([A-Za-z]\w*\) Slug\(\) string`)

func countSlugMethods(t *testing.T, dir string) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n += len(slugMethod.FindAll(src, -1))
	}
	return n
}

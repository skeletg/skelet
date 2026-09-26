package admin

import (
	"os"
	"strings"
	"testing"
)

// auth.GrantCheckerFrom reads the checker the gRPC service interceptor puts on the context - core's
// admin GraphQL never sets one, so there it falls back to deny-all and every non-superuser is refused,
// while superusers bypass the checker and never notice. Resolvers here check through r.AdminSvc
func TestAdminResolversNeverUseTheContractGrantChecker(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "auth.GrantCheckerFrom(") {
				t.Errorf("%s:%d checks through auth.GrantCheckerFrom, which denies everyone but superusers on this surface - use r.AdminSvc", name, i+1)
			}
		}
	}
}

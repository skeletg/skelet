package route

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/skeletg/skelet/cmd/skelet/internal/gates"
	"github.com/skeletg/skelet/cmd/skelet/internal/scaffold"
	"github.com/skeletg/skelet/cmd/skelet/internal/stack"
	"github.com/skeletg/skelet/cmd/skelet/internal/testrun"
)

var update = flag.Bool("update", false, "rewrite testdata/usage.golden")

func TestUsageGolden(t *testing.T) {
	if *update {
		if err := os.WriteFile("testdata/usage.golden", []byte(Usage()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/usage.golden")
	if err != nil {
		t.Fatal(err)
	}
	if Usage() != string(want) {
		t.Errorf("the usage screen changed - review it, then go test ./internal/route -update\n%s", Usage())
	}
}

func TestHelp(t *testing.T) {
	root := fakeRoot(t)
	cases := []struct{ verb, want string }{
		{"add", "skelet add   channel <name> [--template <template> | --from <storefront>] [--tracked] [--workspace]\n\n" + scaffold.AddAbout + "\n"},
		{"new", "skelet new   channel <name> [--template <template> | --from <storefront>] [--tracked] [--workspace]\n\n" + scaffold.AddAbout + "\n"},
		{"rm", "skelet rm   channel <name> [-y] [--volumes]\n\n" + scaffold.RemoveAbout + "\n"},
		{"up", stack.UpAbout + "\n"},
		{"down", stack.DownAbout + "\n"},
		{"pnpm", "skelet pnpm   <app|package> <pnpm args> | --lockfile\n\n"},
		{"db", "skelet db   " + subs["db"] + "\n\n"},
	}
	for _, c := range cases {
		if a := Help(root, c.verb); a.Stdout != c.want || a.Code != 0 {
			t.Errorf("help %s = %+v, want %q", c.verb, a, c.want)
		}
	}
	// A subcommand with a route of its own describes itself; one without falls back to its verb
	for _, c := range []struct{ args, want string }{
		{"test e2e", "skelet test e2e\n\n" + testrun.E2EAbout + "\n"},
		{"check pins", "skelet check pins\n\n" + gates.PinsAbout + "\n"},
		{"test ./internal/x", "skelet test   " + subs["test"] + "\n\n" + testrun.TestAbout + "\n"},
		{"db nope", "skelet db   " + subs["db"] + "\n\n"},
	} {
		f := strings.Fields(c.args)
		if a := Resolve(root, append([]string{"help"}, f...)); a.Stdout != c.want || a.Code != 0 {
			t.Errorf("help %s = %+v, want %q", c.args, a, c.want)
		}
	}
	if a := Help(root, "nope"); a.Code != 2 || a.Stderr != `Unknown command "nope" - try: skelet help` {
		t.Errorf("help nope = %+v", a)
	}
}

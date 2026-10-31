// Package route maps a skelet command line onto the bin/ script that carries it out
package route

import "github.com/skeletg/skelet/apps/skelet/internal/setup"

// Route sends a verb, or a verb's subcommand, to bin/<Script> - or, for a verb written in Go, to Go. bin/repo-check
// greps the Script fields of this file, so every script the front door reaches is named here, one route per line
type Route struct {
	Verb    string
	Sub     string
	Flag    string
	Script  string
	Prepend []string
	Go      func(root string, args []string) int
	About   string
}

// AnyApp is the pnpm route taken by any storefront name - the storefront exists when apps/channels/<name> does
const AnyApp = "*"

var routes = []Route{
	{Verb: "setup", Go: setup.Run, About: setup.About},
	{Verb: "doctor", Script: "doctor"},
	{Verb: "up", Script: "dev"},
	{Verb: "serve", Script: "serve"},
	{Verb: "down", Script: "down"},
	{Verb: "stop", Script: "stop"},
	{Verb: "restart", Script: "restart"},
	{Verb: "ps", Script: "ps"},
	{Verb: "logs", Script: "logs"},
	{Verb: "sh", Script: "bash"},
	{Verb: "go", Script: "go"},
	{Verb: "cli", Script: "cli"},
	{Verb: "psql", Script: "psql"},
	{Verb: "lint", Script: "lint"},
	{Verb: "search", Script: "opensearch"},
	{Verb: "release", Script: "push-images"},
	{Verb: "hooks", Script: "install-hooks"},
	{Verb: "fixperms", Script: "fixperms"},
	{Verb: "provider", Script: "provider-check"},
	{Verb: "changelog", Script: "changelog"},
	{Verb: "add", Script: "new"},
	{Verb: "rm", Script: "remove"},
	{Verb: "pnpm", Sub: "dashboard", Script: "dash"},
	{Verb: "pnpm", Sub: AnyApp, Script: "channel"},
	{Verb: "db", Sub: "migrate", Script: "migrate"},
	{Verb: "db", Sub: "migrate", Flag: "--signals", Script: "migrate-signals"},
	{Verb: "db", Sub: "export", Script: "db-export"},
	{Verb: "db", Sub: "import", Script: "db-import"},
	{Verb: "db", Sub: "shell", Script: "psql"},
	{Verb: "gen", Script: "codegen"},
	{Verb: "gen", Sub: "sqlc", Script: "sqlc"},
	{Verb: "gen", Sub: "gqlgen", Script: "gqlgen"},
	{Verb: "gen", Sub: "buf", Script: "buf"},
	{Verb: "check", Script: "check"},
	{Verb: "check", Sub: "pins", Script: "pin-check"},
	{Verb: "check", Sub: "structure", Script: "repo-check"},
	{Verb: "check", Sub: "contracts", Script: "contract-check"},
	{Verb: "check", Sub: "codegen", Script: "codegen", Prepend: []string{"--check"}},
	{Verb: "test", Script: "test"},
	{Verb: "test", Sub: "e2e", Script: "test-e2e"},
	{Verb: "test", Sub: "storefront", Script: "test-storefront"},
}

// The shape of each compound verb's arguments, for help and the usage errors
var subs = map[string]string{
	"setup":     setup.Flags,
	"db":        "migrate [--signals] up|rollback|status|new <name> | export | import | shell",
	"gen":       "(all) | sqlc | gqlgen | buf",
	"check":     "(all) | pins | structure | contracts | codegen",
	"test":      "(all) | e2e | storefront [name] [playwright args] | [pkgs] [--changed[=REF]] [--keep] [--no-frontend]",
	"logs":      "[service]",
	"pnpm":      "<app> <pnpm args>",
	"npm":       "<app> <pnpm args>",
	"add":       "channel <name> [--from <storefront>] [--tracked]",
	"new":       "channel <name> [--from <storefront>] [--tracked]",
	"rm":        "channel <name> [-y] [--volumes]",
	"changelog": "(preview) | check | release <version> [date]",
}

// Routes is the table, for the tests and gates that hold it to bin/
func Routes() []Route { return routes }

func lookup(verb, sub string) (Route, bool) {
	for _, r := range routes {
		if r.Verb == verb && r.Sub == sub && r.Flag == "" {
			return r, true
		}
	}
	return Route{}, false
}

func flagged(verb, sub, flag string) (Route, bool) {
	for _, r := range routes {
		if r.Verb == verb && r.Sub == sub && r.Flag == flag {
			return r, true
		}
	}
	return Route{}, false
}

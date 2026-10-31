// Package route maps a skelet command line onto the bin/ script that carries it out
package route

import (
	"github.com/skeletg/skelet/cmd/skelet/internal/changelog"
	"github.com/skeletg/skelet/cmd/skelet/internal/container"
	"github.com/skeletg/skelet/cmd/skelet/internal/db"
	"github.com/skeletg/skelet/cmd/skelet/internal/doctor"
	"github.com/skeletg/skelet/cmd/skelet/internal/fixperms"
	"github.com/skeletg/skelet/cmd/skelet/internal/gates"
	"github.com/skeletg/skelet/cmd/skelet/internal/gen"
	"github.com/skeletg/skelet/cmd/skelet/internal/hooks"
	"github.com/skeletg/skelet/cmd/skelet/internal/lint"
	"github.com/skeletg/skelet/cmd/skelet/internal/release"
	"github.com/skeletg/skelet/cmd/skelet/internal/search"
	"github.com/skeletg/skelet/cmd/skelet/internal/setup"
	"github.com/skeletg/skelet/cmd/skelet/internal/stack"
	"github.com/skeletg/skelet/cmd/skelet/internal/testrun"
)

// Route sends a verb, or a verb's subcommand, to bin/<Script> - or, for a verb written in Go, to Go. skelet check structure
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
	{Verb: "doctor", Go: doctor.Run, About: doctor.About},
	{Verb: "up", Script: "dev"},
	{Verb: "serve", Go: stack.Serve, About: stack.ServeAbout},
	{Verb: "down", Go: stack.Down, About: stack.DownAbout},
	{Verb: "stop", Go: stack.Stop, About: stack.StopAbout},
	{Verb: "restart", Go: stack.Restart, About: stack.RestartAbout},
	{Verb: "ps", Go: stack.Ps, About: stack.PsAbout},
	{Verb: "logs", Go: stack.Logs, About: stack.LogsAbout},
	{Verb: "sh", Go: container.Shell, About: container.ShellAbout},
	{Verb: "go", Script: "go"},
	{Verb: "cli", Go: container.CLI, About: container.CLIAbout},
	{Verb: "psql", Go: db.Shell, About: db.ShellAbout},
	{Verb: "lint", Go: lint.Run, About: lint.About},
	{Verb: "search", Go: search.Run, About: search.About},
	{Verb: "release", Go: release.Run, About: release.About},
	{Verb: "hooks", Go: hooks.Run, About: hooks.About},
	{Verb: "fixperms", Go: fixperms.Run, About: fixperms.About},
	{Verb: "provider", Go: container.Provider, About: container.ProviderAbout},
	{Verb: "changelog", Go: changelog.Run, About: changelog.About},
	{Verb: "add", Script: "new"},
	{Verb: "rm", Script: "remove"},
	{Verb: "pnpm", Sub: "dashboard", Script: "dash"},
	{Verb: "pnpm", Sub: AnyApp, Script: "channel"},
	{Verb: "db", Sub: "migrate", Go: db.Migrate, About: db.MigrateAbout},
	{Verb: "db", Sub: "migrate", Flag: "--signals", Go: db.MigrateSignals, About: db.MigrateAbout},
	{Verb: "db", Sub: "export", Go: db.Export, About: db.ExportAbout},
	{Verb: "db", Sub: "import", Go: db.Import, About: db.ImportAbout},
	{Verb: "db", Sub: "shell", Go: db.Shell, About: db.ShellAbout},
	{Verb: "gen", Go: gen.Run, About: gen.About},
	{Verb: "gen", Sub: "sqlc", Script: "sqlc"},
	{Verb: "gen", Sub: "gqlgen", Script: "gqlgen"},
	{Verb: "gen", Sub: "buf", Script: "buf"},
	{Verb: "check", Go: gates.Check, About: gates.CheckAbout},
	{Verb: "check", Sub: "pins", Go: gates.Pins, About: gates.PinsAbout},
	{Verb: "check", Sub: "structure", Go: gates.Repo, About: gates.RepoAbout},
	{Verb: "check", Sub: "contracts", Go: gates.Contracts, About: gates.ContractsAbout},
	{Verb: "check", Sub: "codegen", Go: gen.Check, About: gen.About},
	{Verb: "test", Go: testrun.Test, About: testrun.TestAbout},
	{Verb: "test", Sub: "e2e", Go: testrun.E2E, About: testrun.E2EAbout},
	{Verb: "test", Sub: "storefront", Go: testrun.Storefront, About: testrun.StorefrontAbout},
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

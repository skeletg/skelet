package route

import (
	"slices"
	"strings"

	"github.com/skeletg/skelet/cmd/skelet/internal/storefronts"
)

// Action is what a command line resolves to: a verb to run with Args, or output and an exit code
type Action struct {
	Go     func(root string, args []string) int
	Args   []string
	Notice string
	Stdout string
	Stderr string
	Code   int
}

// The verbs that are another verb's old name - they still work, after a notice on stderr
var aliases = map[string]string{"npm": "pnpm", "new": "add"}

func Resolve(root string, args []string) Action {
	verb := "help"
	if len(args) > 0 && args[0] != "" {
		verb = args[0]
	}
	var rest []string
	if len(args) > 0 {
		rest = args[1:]
	}
	switch verb {
	case "help", "-h", "--help":
		if len(rest) > 0 && rest[0] != "" {
			return Help(root, rest[0], rest[1:]...)
		}
		return Action{Stdout: Usage()}
	case "pnpm", "npm":
		a := pnpmRoute(root, rest)
		if verb == "npm" {
			a.Notice = "(skelet npm is skelet pnpm now - running pnpm)"
		}
		return a
	case "db":
		return dbRoute(rest)
	case "gen", "check", "test":
		return withSubs(verb, rest)
	case "new":
		if len(rest) > 0 && rest[0] != "channel" && !strings.HasPrefix(rest[0], "-") {
			return die(newProject)
		}
		a := simple("add", rest)
		if len(rest) > 0 && rest[0] == "channel" {
			a.Notice = "(skelet new channel is skelet add channel now - running it)"
		}
		return a
	}
	return simple(verb, rest)
}

// What the docs' skelet new <dir> meets when it's typed as ./skelet in a clone, where new is add channel's old name
const newProject = "skelet new <dir> starts a project, and only the installed CLI does that (the README's Quickstart). " +
	"To add a storefront to this one: skelet add channel <name>"

func simple(verb string, rest []string) Action {
	r, ok := lookup(verb, "")
	if !ok {
		return die(`Unknown command "` + verb + `" - try: skelet help`)
	}
	return to(r, rest)
}

// to is the one place a route becomes an Action, so a Go verb's handler survives every path - sub-routes, flags, apps
func to(r Route, args []string) Action { return Action{Go: r.Go, Args: args} }

func withSubs(verb string, rest []string) Action {
	if len(rest) > 0 && rest[0] != "" {
		if r, ok := lookup(verb, rest[0]); ok {
			return to(r, rest[1:])
		}
	}
	return simple(verb, rest)
}

func dbRoute(rest []string) Action {
	sub := ""
	if len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}
	switch sub {
	case "migrate":
		args := slices.DeleteFunc(slices.Clone(rest), func(a string) bool { return a == "--signals" })
		if len(args) != len(rest) {
			r, _ := flagged("db", "migrate", "--signals")
			return to(r, args)
		}
		r, _ := lookup("db", "migrate")
		return to(r, args)
	case "export", "import", "shell":
		r, _ := lookup("db", sub)
		return to(r, rest)
	}
	return die("skelet db " + subs["db"])
}

func pnpmRoute(root string, rest []string) Action {
	app := ""
	if len(rest) > 0 {
		app, rest = rest[0], rest[1:]
	}
	switch app {
	case "dashboard":
		r, _ := lookup("pnpm", "dashboard")
		return to(r, rest)
	case "":
		return die("skelet pnpm <app|package> <pnpm args> - " + appsList(root))
	}
	if _, ok := storefronts.Find(root, app); ok {
		r, _ := lookup("pnpm", AnyApp)
		return to(r, append([]string{app}, rest...))
	}
	if slices.Contains(packageNames(root), app) {
		r, _ := lookup("pnpm", AnyPackage)
		return to(r, append([]string{app}, rest...))
	}
	return die(`skelet pnpm: unknown app or package "` + app + `" - ` + appsList(root))
}

func appsList(root string) string {
	s := "apps: " + strings.Join(append([]string{"dashboard"}, storefronts.Names(root)...), ", ")
	if p := packageNames(root); len(p) > 0 {
		s += "; packages: " + strings.Join(p, ", ")
	}
	return s
}

// packageNames is the workspace packages by their directory name - storefront-core, not packages/storefront-core
func packageNames(root string) []string {
	var out []string
	for _, dir := range storefronts.Packages(root) {
		out = append(out, strings.TrimPrefix(dir, "packages/"))
	}
	return out
}

func die(msg string) Action { return Action{Stderr: msg, Code: 2} }

package route

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Action is what a command line resolves to: a bin/ script to exec with Args, a Go verb to run with them, or output
// and an exit code
type Action struct {
	Script string
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
		a := pnpm(root, rest)
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
func to(r Route, args []string) Action { return Action{Script: r.Script, Go: r.Go, Args: args} }

func withSubs(verb string, rest []string) Action {
	if len(rest) > 0 && rest[0] != "" {
		if r, ok := lookup(verb, rest[0]); ok {
			return to(r, append(slices.Clone(r.Prepend), rest[1:]...))
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

func pnpm(root string, rest []string) Action {
	app := ""
	if len(rest) > 0 {
		app, rest = rest[0], rest[1:]
	}
	switch app {
	case "dashboard":
		r, _ := lookup("pnpm", "dashboard")
		return to(r, rest)
	case "":
		return die("skelet pnpm <app> <pnpm args> - apps: " + appsList(root))
	}
	if fi, err := os.Stat(filepath.Join(root, "apps", "channels", app)); err != nil || !fi.IsDir() {
		return die(`skelet pnpm: unknown app "` + app + `" - apps: ` + appsList(root))
	}
	r, _ := lookup("pnpm", AnyApp)
	return to(r, append([]string{app}, rest...))
}

func appsList(root string) string {
	out := []string{"dashboard"}
	matches, _ := filepath.Glob(filepath.Join(root, "apps", "channels", "*", "package.json"))
	for _, m := range matches {
		out = append(out, filepath.Base(filepath.Dir(m)))
	}
	return strings.Join(out, ", ")
}

func die(msg string) Action { return Action{Stderr: msg, Code: 2} }

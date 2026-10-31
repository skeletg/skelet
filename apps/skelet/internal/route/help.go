package route

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Usage() string {
	return `skelet - the dev commands for this repo

Common
  setup      configure this clone (run this first)
  up         start the dev stack          [--lean skips OpenSearch, Signals, dev tools]
  serve      a frontend on the host       [skelet serve dashboard]
  down       stop and remove containers
  logs       tail service logs            [skelet logs app]
  check      every gate that can fail a commit
  doctor     diagnose your environment

Containers
  sh         shell into the app container
  go         go inside the app            [skelet go test ./internal/...]
  cli        the ops CLI                  [skelet cli admin create-superuser]
  pnpm       pnpm in a frontend           [skelet pnpm dashboard run lint]
  psql       psql on the dev database

Database
  db         ` + subs["db"] + `

Codegen
  gen        ` + subs["gen"] + `

Quality
  test       go test + frontend typechecks + unit tests   [test e2e: money paths, test storefront: browser]
  lint       gofmt + golangci-lint + eslint
  changelog  ` + subs["changelog"] + `

Scaffold
  add        ` + subs["add"] + `
  rm         ` + subs["rm"] + `

Rarely
  ps, stop, restart, search, release, fixperms, hooks, provider, version

  skelet help <command>   detail on one command
  bin/<script>            the script a verb runs, if you need the source
`
}

// Help prints a verb's argument shape, the script it runs and that script's own header comment, so the
// help never restates what a script already says about itself
func Help(root, verb string) Action {
	var b strings.Builder
	sub := subs[verb]
	if sub != "" {
		fmt.Fprintf(&b, "skelet %s   %s\n\n", verb, sub)
	}
	target := verb
	if a, ok := aliases[verb]; ok {
		target = a
	}
	r, ok := lookup(target, "")
	if !ok {
		if sub != "" {
			return Action{Stdout: b.String()}
		}
		return die(`Unknown command "` + verb + `" - try: skelet help`)
	}
	fmt.Fprintf(&b, "runs bin/%s\n\n", r.Script)
	path := filepath.Join(root, "bin", r.Script)
	data, err := os.ReadFile(path)
	if err != nil {
		return die(err.Error())
	}
	text := Header(string(data))
	if text == "" {
		text = fmt.Sprintf("(no description - it is %d lines, just read bin/%s)", strings.Count(string(data), "\n"), r.Script)
	}
	b.WriteString(text + "\n")
	return Action{Stdout: b.String()}
}

// Header is a script's leading comment: every # line after the shebang, up to the first `set ` line
func Header(script string) string {
	var out []string
	for i, line := range strings.Split(script, "\n") {
		if i == 0 {
			continue
		}
		if strings.HasPrefix(line, "set ") {
			break
		}
		if rest, ok := strings.CutPrefix(line, "#"); ok {
			out = append(out, strings.TrimPrefix(rest, " "))
		}
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

package route

import "fmt"

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
`
}

// Help prints a verb's argument shape and what it does. help <verb> <sub> is the subcommand's own, when it has a route
func Help(root, verb string, sub ...string) Action {
	target := verb
	if a, ok := aliases[verb]; ok {
		target = a
	}
	if len(sub) > 0 {
		if r, ok := lookup(target, sub[0]); ok {
			return describe(fmt.Sprintf("skelet %s %s\n\n", verb, sub[0]), r)
		}
	}
	head := ""
	if shape := subs[verb]; shape != "" {
		head = fmt.Sprintf("skelet %s   %s\n\n", verb, shape)
	}
	r, ok := lookup(target, "")
	if !ok {
		if head != "" {
			return Action{Stdout: head}
		}
		return die(`Unknown command "` + verb + `" - try: skelet help`)
	}
	return describe(head, r)
}

func describe(head string, r Route) Action {
	return Action{Stdout: head + r.About + "\n"}
}

// Package container holds the verbs that run something inside the app container - exec'd into it when it's running,
// else a one-off run removed afterwards
package container

import (
	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const (
	GoAbout = `go in the app container, from core's module - skelet go test ./internal/..., skelet go build ./... -
or with --signals or --sdk first, from that module: skelet go --signals test ./...`
	ShellAbout = `A bash shell in the app container. Arguments go to bash.`
	CLIAbout   = `The ops CLI in the app container - skelet cli admin list, skelet cli job run <name>. It asks for a
terminal only when stdin is one, so piping and CI work.`
	ProviderAbout = `Conformance-checks a remote provider endpoint against the contract core enforces. The endpoint can
live anywhere - this only speaks signed HTTP.

  skelet provider --url https://tax.example.com/skelet --secret rpsec_... --kind tax_provider

Kinds and payload shapes: docs/remote-providers.md. With no --payload it posts the published example
for that kind, which the checker carries itself.`
)

func Shell(root string, args []string) int {
	return app(root, true, append([]string{"bash"}, args...)).Replace()
}

func CLI(root string, args []string) int { return CLICall(root, args...).Replace() }

// CLICall is the ops CLI as a command, for setup, which runs it and carries on
func CLICall(root string, args ...string) sys.Call {
	return app(root, true, append([]string{"go", "run", "./cmd/cli"}, args...))
}

// The checker's flags are a published contract - remote-provider repos run it from their own CI - so they pass
// through untouched
func Provider(root string, args []string) int {
	return app(root, false, append([]string{"go", "run", "./cmd/providercheck"}, args...)).Replace()
}

// Go is go in the app container, from core's module or the one --signals or --sdk names
func Go(root string, args []string) int {
	dir := "/var/www/backend/core"
	if len(args) > 0 {
		switch args[0] {
		case "--signals":
			dir, args = "/var/www/backend/signals", args[1:]
		case "--sdk":
			dir, args = "/var/www/backend/sdk", args[1:]
		}
	}
	return In(root, compose.Target{Service: "app", Flags: append(compose.UserFlags(), "-w", dir)}, append([]string{"go"}, args...)...).Replace()
}

func app(root string, tty bool, cmd []string) sys.Call {
	return In(root, compose.Target{Service: "app", TTY: tty, Flags: compose.UserFlags()}, cmd...)
}

// In is cmd in the target's container: exec'd when it's running, else a one-off run, which gets the uid and gid a
// created container runs as
func In(root string, t compose.Target, cmd ...string) sys.Call {
	lib := compose.Load(root)
	args, creates := lib.ExecOrRun(t, cmd...)
	env := lib.Env()
	if creates {
		env = lib.CreateEnv()
	}
	return compose.Call(root, env, args...)
}

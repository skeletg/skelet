// Package container holds the verbs that run something inside the app container - exec'd into it when it's running,
// else a one-off run removed afterwards
package container

import (
	"github.com/skeletg/skelet/cmd/skelet/internal/compose"
	"github.com/skeletg/skelet/cmd/skelet/internal/sys"
)

const (
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

func app(root string, tty bool, cmd []string) sys.Call {
	lib := compose.Load(root)
	args, creates := lib.ExecOrRun(compose.Target{Service: "app", TTY: tty, Flags: compose.UserFlags()}, cmd...)
	env := lib.Env()
	if creates {
		env = lib.CreateEnv()
	}
	return compose.Call(root, env, args...)
}

- **The pre-commit hook finds the skelet CLI on its own.** `skelet hooks` writes a hook that runs
  `skelet check` through the CLI on your PATH, `~/.local/bin/skelet`, the one it was installed with,
  or `go run` from the checkout - git GUIs often run hooks without your shell's PATH. The hook an
  earlier `skelet hooks` wrote is updated the next time you run any `skelet` command, and a hook of
  your own is no longer overwritten unless you pass `--force`. It also installs into worktrees.

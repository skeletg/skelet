- **`./skelet` and `bin/` are gone.** Install the CLI the README's way - inside a project it runs
  that project's own code, and one from an earlier release still gets its `./skelet`. With only Go,
  `go -C cmd/skelet run . <verb>` does the same. A pre-commit hook from an older `skelet hooks`,
  which ran a script that's gone, is rewritten by any `skelet` command, and `skelet doctor` fails
  on one that's left.

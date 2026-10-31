- **The docs, messages and help say `skelet`, the installed CLI, instead of `./skelet`.** In a
  project it runs that project's own code; with only Go, `go -C cmd/skelet run . <verb>` does the
  same.
- **`skelet doctor` warns when the `skelet` on your PATH is a link to a project's `./skelet`,**
  which projects no longer have. The installer replaces such a link with `--force`.

- **The docs, messages and help say `skelet`, the installed CLI, instead of `./skelet`.** In a
  project it runs that project's own code; with only Go, `go -C cmd/skelet run . <verb>` does the
  same.
- **`./skelet` says it's going away when you run it by hand,** and `skelet doctor` warns when the
  `skelet` on your PATH is a link to one - that stops working when the file goes. The installer
  replaces such a link with `--force`.

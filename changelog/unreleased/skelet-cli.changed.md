- **Every dev command is part of the skelet CLI.** In a project, the installed `skelet` builds
  that project's own `cmd/skelet` on first use - with host Go when there is one, otherwise inside
  the `golang` image, so Docker alone is still enough - and runs it, so a command always matches the
  code you're on. Verbs, flags and exit codes are as they were.
- **Storefronts are scaffolded with `skelet add channel <name>`**, the pair to `rm channel`.
  `new channel` keeps working and says it's `add channel` now.

- **`./skelet` is a Go program now.** The front door builds the CLI in `apps/skelet` on its first
  run - with host Go when there is one, otherwise inside the `golang` image, so Docker alone is
  still enough - and caches it under `.skelet/bin/`. Every verb, flag, message and exit code is the
  same as before, and each one still runs its `bin/` script.
- **Storefronts are scaffolded with `./skelet add channel <name>`**, the pair to `rm channel`.
  `new channel` keeps working and says it's `add channel` now.

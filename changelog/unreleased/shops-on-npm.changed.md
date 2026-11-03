- **A new shop takes the storefront packages from npm.** `skelet add channel` and `skelet setup` pin
  the copy's `@skelet/storefront-*` to the checkout's release on npm, where they used to link
  `packages/`, so merging a later release no longer changes a shop until you bump them.
  `skelet doctor` says when a shop is behind the backend, with the command that bumps it. A
  checkout ahead of its last release still links `packages/`, as does `--workspace`. pnpm takes a
  `@skelet/*` release the day it's published.

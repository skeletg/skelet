- **The storefront packages publish to npm.** `@skelet/storefront-core`, `-next`, `-astro` and `-react` go out with
  every release at its version, compiled to JavaScript with their types and licensed MIT, so a shop in a repo of its
  own can install them. A Skelet checkout keeps reading the source. The release workflow publishes through npm's
  trusted publishing, with no token stored - `docs/releasing.md` has the one-time setup.

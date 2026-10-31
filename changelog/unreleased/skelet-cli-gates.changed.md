- **`skelet check structure` looks in every tracked file for a `bin/` script that no longer exists**,
  not only in Markdown and YAML, so a Go or TypeScript message pointing at a retired script fails the
  gate instead of reaching someone.

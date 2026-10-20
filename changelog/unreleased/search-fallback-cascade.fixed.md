- **A struggling OpenSearch no longer takes checkout down with it.** When OpenSearch keeps failing,
  the storefront stops asking it for a few seconds and lets a single request check whether it's
  back. Postgres still answers in its place, but for at most a quarter of `DB_MAX_CONNS` at a
  time, so carts and checkout keep their connections. Past that, a search comes back empty
  instead of queueing.
- A browse that fails on every search tier is no longer cached as an empty catalogue for a minute.

- **The dashboard and the storefronts deploy as builds, not images.** `pnpm run build` makes the dashboard's static
  files - with a `dist/_headers` carrying its security headers - and a Next storefront's self-contained server in
  `.next/standalone`, so either goes on any host or platform. Each takes its settings from its own `.env.production`,
  which `skelet setup` writes with the secrets it shares with core. Their prod Dockerfiles, the dashboard's nginx and
  their compose services are gone, and `skelet release` builds only `core` and `signals`. The bundled Caddy edge
  serves the dashboard's `dist/` and proxies the shop to the storefront's server on the host. `skelet add channel`
  no longer writes compose files, so `--tracked` is gone.

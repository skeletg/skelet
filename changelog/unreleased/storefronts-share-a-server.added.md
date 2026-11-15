- **Several storefronts on one server.** `skelet setup` with a server target gives each storefront its own `PORT`,
  writes the bundled edge a site for every storefront past the first in `deploy/caddy/sites/` (its host is
  `<NAME>_DOMAIN` in `env/prod-caddy.env`), and leaves `STOREFRONT_REVALIDATE_URL` empty so core purges each channel
  at its own URL. A storefront's `pnpm run start` now runs its built server with its `.env.production`, and the
  dashboard build writes a `dist/_redirects` beside `dist/_headers`.

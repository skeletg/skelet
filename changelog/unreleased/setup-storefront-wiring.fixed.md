- **A small or prod setup serves the storefront you picked.** The demo storefront always started and the edge always
  proxied to it, whatever setup was told. It now runs under its own compose profile like astro-shop, and setup writes
  env files only for the storefronts you picked, turns their profiles on in `COMPOSE_PROFILES`, and points
  `STOREFRONT_REVALIDATE_URL` and the edge's new `STOREFRONT_UPSTREAM` at the one it serves. A value you set by hand
  stays.
- **The dashboard previews your first shop.** `skelet add channel` on a dev clone sets `VITE_STOREFRONT_ORIGIN` to the
  new shop's port when `.env` names none, instead of leaving previews on the template's `:3000`.

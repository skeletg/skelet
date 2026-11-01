- **A storefront's dev container can no longer change the checkout.** It mounts the repo read-only, writing only its own
  directory and `packages/`, with the root `.env` and the rest of `apps/` hidden as well as `env/`. It installs frozen,
  `skelet pnpm <name> add|remove|update` runs over the workspace's manifests in a throwaway container, and the new
  `skelet pnpm --lockfile` refreshes the lockfile by itself. `skelet doctor` notes a shop in the override still
  mounting the checkout writable - recreate it from the template's block. Run `skelet up` to pick it up.
- **Open redirects.** A `returnTo` with a tab, a newline or a backslash in it is refused, a posted `locale` no longer
  decides where an action lands, and a locale is only ever a locale code in front of a path.
- **Masquerade links install only masquerade sessions.** The backend marks the tokens it mints for an operator, and the
  storefront's `/masquerade` landing refuses any other - a customer's own token in a crafted link would have signed
  whoever clicked it into the sender's account. It redirects to a relative `/`.
- **The backend proxy keeps to its routes.** `/t/%2e%2e/api/...` matched a proxied prefix and resolved outside it.
- **A purge drops everything it names.** Both templates' revalidate routes kept only the first 64 tags and paths; the
  body is bounded by size now. A warm request stays on the storefront's origin and follows no redirect, since it carries
  the revalidate secret.
- **The session refresh logs a shopper out only when the backend says the token is no good** - not on a rate limit or
  an outage - names the shopper's address to the backend, and hands the rotated refresh token to the render too.
- **astro-shop's route cache kept one channel's page for another host** when hosts map to channels; pages now vary by
  host there. Its pages are dropped by menu and channel purges as well, kept for the admin's cache time, and render
  catalog reads without a shopper's session.
- **A cart the backend couldn't read is kept.** astro-shop deleted the cart cookie whenever reading the cart failed.
- **Shopper actions no longer purge every cached page.** Signing in or out, consent and placing an order re-render the
  shopper's own page instead of revalidating the whole site.
- **`skelet add channel`** copies into place last, so a failure leaves nothing half-made, renames the source only
  where its name stands whole, defaults to `nextjs-vetrina`, rewrites the ports of a template on its own (astro-shop's
  3001), and fails loudly when the lockfile can't be refreshed. `skelet rm channel` refuses a template setup left out of
  the working tree, which it used to strip from the tracked compose files.
- **`skelet setup`** offers a first shop only to a plain interactive dev setup, asks before where the frontends run so
  the shop goes with them, and refuses a shop name a compose service already has.
- **`skelet release`** builds only the frontends the clone runs when setup recorded a selection.

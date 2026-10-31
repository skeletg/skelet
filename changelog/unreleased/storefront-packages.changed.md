- **Storefronts are made from templates.** The demo storefront is now the `nextjs-vetrina` template under
  `templates/`, and `apps/channels/` holds a project's own shops. `skelet add channel <name>` copies
  a template there (`--template <name>`, or `--from <shop>` for one of your own), points its paths at
  its new home and adds it to the lockfile. The templates and the packages they build on stay
  upstream's, so merging a release brings their fixes without touching your shop. `skelet rm channel`
  leaves the templates alone.
- **The storefronts are one pnpm workspace.** `pnpm-workspace.yaml` and `pnpm-lock.yaml` at the root
  cover `apps/channels/*`, `templates/*` and the new `packages/*`, and the workspace's catalog keeps
  the versions they share in step, so a package and the app using it resolve the same React. The
  dashboard stays a project of its own.
- **A storefront's logic is `@skelet/storefront-next`.** Everything the demo kept in `app/lib` that isn't
  design - the GraphQL client and its operations, auth, cart, checkout, channel and locale, the server
  actions - now lives in `packages/storefront-next`, and the template imports it from there. A shop forked
  from the template keeps up with fixes to that code by merging a release, without touching its own
  pages. Codegen is the package's, run by `skelet gen`, and `skelet check` reads the shared lists from
  the package and fails when one goes missing.
- **A storefront's dev container mounts the whole repo** at `/var/www/skelet`, with `env/` hidden
  behind a tmpfs, and its prod image builds from the repo root with a `.dockerignore` of its own
  beside the Dockerfile. `skelet doctor` notes an override still mounting a storefront at
  `/var/www/storefront`, and a `.skelet-apps` naming a storefront that isn't there.
- **`vetrina-demo` is now `nextjs-vetrina`** everywhere it's named, since a template carries its framework
  in its name: the compose service and profile, the `skelet-nextjs-vetrina` image and its
  `NEXTJS_VETRINA_IMAGE` variable, and `env/{prod,small}-nextjs-vetrina.env`. Rename your `env/` files
  and update `.skelet-apps`, `docker-compose.override.yml` and any `--apps` list that names the old one.

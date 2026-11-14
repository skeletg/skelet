- **The frontends run on the host in dev, with `skelet dev`.** `skelet up` starts the backend and its datastores;
  `skelet dev` runs the apps the clone was set up with side by side, or the ones named, until Ctrl-C. Each listens on
  the port its `package.json` dev script names, and `skelet add channel` gives a new storefront the next free one.
  The dashboard and storefront dev containers, their Dockerfiles and node_modules volumes are gone, along with
  `skelet setup --docker-apps` and `skelet serve`. `skelet pnpm` always runs the host's pnpm, and `skelet test
  storefront` runs Playwright against the storefront's dev server. `skelet up` removes a frontend container left
  from before.

# Storefront dev image - astro dev over the bind-mounted workspace. The storefronts and the packages they share are one
# pnpm workspace rooted at the repo, so the image works from the root and runs this app inside it

# The packages' manifests alone, so the install layer below is keyed on them and not on every package source edit
FROM alpine:3.24 AS manifests
WORKDIR /w
COPY packages packages
RUN find packages -mindepth 2 -maxdepth 2 ! -name package.json -exec rm -rf {} +

FROM node:24-alpine

# Pin a locale Alpine actually has - stops bash whining about leaked LC_ALL
ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# git for the odd postinstall script, curl for healthchecks + debugging, watchexec for the dev loop below
RUN apk add --no-cache git curl watchexec

# pnpm from npm, not corepack - corepack fetches pnpm per HOME, and the service runs as your uid
# with HOME=/tmp, so every container would download it again
RUN npm install -g pnpm@12.6.0

# node_modules/.modules.yaml records the store path, and a later `pnpm add` in the container refuses a tree linked from
# any other one - so the path is pinned for build and runtime alike. The install runs before every start below, so
# pnpm's own check before a script is off - it can't vouch for a filtered install and would redo it whole. Astro's dev
# toolbar would otherwise start a background process the restart loop below can't see
ENV pnpm_config_store_dir=/pnpm/store \
    pnpm_config_verify_deps_before_run=false \
    HOME=/tmp \
    ASTRO_DEV_BACKGROUND=0

# The install runs as the uid the service does (compose passes both) - pnpm chmods every bin it
# links on each add, which only the owner may do, so a root-built tree takes no later install
ARG SKELET_UID=1000
ARG SKELET_GID=1000

WORKDIR /var/www/skelet

# The mkdir matters - an empty volume's mountpoint comes up root-owned unless the image has the dir. The workspace's
# node_modules holds every installed file; an app's or a package's is only links into it
RUN mkdir -p node_modules /pnpm/store \
    && chown -R "$SKELET_UID:$SKELET_GID" /var/www/skelet /pnpm/store

USER $SKELET_UID:$SKELET_GID

# Deps in their own layer keyed on the workspace file, the lockfile and the manifests - source is mounted at runtime.
# The app and every shared package, each with its own dependencies - a package's peers resolve from its own directory
COPY --chown=$SKELET_UID:$SKELET_GID pnpm-workspace.yaml pnpm-lock.yaml ./
COPY --chown=$SKELET_UID:$SKELET_GID templates/astro-shop/package.json templates/astro-shop/
COPY --chown=$SKELET_UID:$SKELET_GID --from=manifests /w/packages packages
RUN --mount=type=cache,id=pnpm-store-$SKELET_UID,target=/pnpm/store,uid=$SKELET_UID,gid=$SKELET_GID \
    pnpm install --frozen-lockfile --filter "./templates/astro-shop..." --filter "./packages/*"

# The dev server behind an install that re-runs whenever the dependencies change - the workspace's node_modules is a
# named volume Docker fills from this image only while it's empty, so a changed lockfile reaches the container through
# this install alone. Polled, because a Windows bind mount delivers no file events. No purge prompt, there is no TTY
EXPOSE 3000
CMD ["watchexec", "--poll", "2s", "--restart", \
     "--watch", "pnpm-workspace.yaml", "--watch", "pnpm-lock.yaml", "--watch", "templates/astro-shop/package.json", "--watch", "packages", \
     "--filter", "pnpm-workspace.yaml", "--filter", "pnpm-lock.yaml", "--filter", "**/package.json", "--", \
     "pnpm install --filter ./templates/astro-shop... --filter './packages/*' --config.confirm-modules-purge=false && cd templates/astro-shop && exec pnpm run dev --host 0.0.0.0 --port 3000"]

# Dashboard dev image - Vite + HMR over the bind-mounted source,
# node_modules in a named volume so the musl install isn't shadowed

FROM node:24-alpine

# Pin a locale Alpine actually has - stops bash whining about leaked LC_ALL
ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# git for the odd postinstall script, curl for healthchecks + debugging, watchexec for the dev loop below
RUN apk add --no-cache git curl watchexec

# pnpm from npm, not corepack - corepack fetches pnpm per HOME, and the service runs as your uid
# with HOME=/tmp, so every container would download it again
RUN npm install -g pnpm@12.6.0

# node_modules/.modules.yaml records the store path, and a later `pnpm add` in the container
# refuses a tree linked from any other one - so the path is pinned for build and runtime alike.
# The build fills it from a cache mount that never reaches the image; at runtime it starts empty
# and only holds what an add downloads
ENV pnpm_config_store_dir=/pnpm/store \
    HOME=/tmp

# The install runs as the uid the service does (compose passes both) - pnpm chmods every bin it
# links on each add, which only the owner may do, so a root-built tree takes no later install
ARG SKELET_UID=1000
ARG SKELET_GID=1000

WORKDIR /var/www/dashboard

# The mkdir matters - an empty volume's mountpoint comes up root-owned unless the image has the dir
RUN mkdir -p node_modules/.vite /pnpm/store \
    && chown -R "$SKELET_UID:$SKELET_GID" /var/www/dashboard /pnpm/store

USER $SKELET_UID:$SKELET_GID

# Deps in their own layer keyed on the lockfiles - source is mounted at runtime. The workspace
# file carries the overrides and build policy the lockfile was resolved under
COPY --chown=$SKELET_UID:$SKELET_GID apps/dashboard/package.json apps/dashboard/pnpm-lock.yaml apps/dashboard/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm-store-$SKELET_UID,target=/pnpm/store,uid=$SKELET_UID,gid=$SKELET_GID \
    pnpm install --frozen-lockfile

# The dev server behind an install that re-runs whenever the dependencies change. node_modules is a
# named volume, which Docker fills from this image only while it's empty, so a changed lockfile
# reaches the container through this install and nothing else. watchexec restarts the whole command
# on a change and, when the install fails, waits for the next one - polled, because a Windows bind
# mount delivers no file events. No purge prompt on the install, there is no TTY to answer it. One
# string rather than sh -c, because watchexec already runs it through its own shell. Same command as
# the storefront's dev image
#
# --host so Vite listens beyond the container's localhost - no `--` before it, pnpm hands
# that to the script verbatim and vite would take --host for a positional
EXPOSE 5173
CMD ["watchexec", "--poll", "2s", "--restart", \
     "--watch", "package.json", "--watch", "pnpm-lock.yaml", "--watch", "pnpm-workspace.yaml", "--", \
     "pnpm install --config.confirm-modules-purge=false && exec pnpm run dev --host --port 5173"]

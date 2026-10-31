# Storefront dev image - astro dev over the bind-mounted source, node_modules in a named volume
# so the musl install isn't shadowed. Same shape as vetrina-demo's, which explains each choice

FROM node:24-alpine

ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

RUN apk add --no-cache git curl watchexec

RUN npm install -g pnpm@12.6.0

ENV pnpm_config_store_dir=/pnpm/store \
    HOME=/tmp \
    # Astro detaches `astro dev` when it spots an AI agent - under watchexec it must stay in the foreground
    ASTRO_DEV_BACKGROUND=0

ARG SKELET_UID=1000
ARG SKELET_GID=1000

WORKDIR /var/www/storefront

RUN mkdir -p node_modules /pnpm/store \
    && chown -R "$SKELET_UID:$SKELET_GID" /var/www/storefront /pnpm/store

USER $SKELET_UID:$SKELET_GID

COPY --chown=$SKELET_UID:$SKELET_GID apps/channels/astro-shop/package.json apps/channels/astro-shop/pnpm-lock.yaml apps/channels/astro-shop/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm-store-$SKELET_UID,target=/pnpm/store,uid=$SKELET_UID,gid=$SKELET_GID \
    pnpm install --frozen-lockfile

# Reinstall-and-restart on a dependency change, as vetrina's image does. Note the dev server never
# caches routes - build and `pnpm start` to see the route cache and /api/revalidate at work
EXPOSE 3000
CMD ["watchexec", "--poll", "2s", "--restart", \
     "--watch", "package.json", "--watch", "pnpm-lock.yaml", "--watch", "pnpm-workspace.yaml", "--", \
     "pnpm install --config.confirm-modules-purge=false && exec pnpm run dev --host 0.0.0.0 --port 3000"]

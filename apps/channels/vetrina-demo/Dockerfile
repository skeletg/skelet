# Storefront dev image - next dev + HMR over the bind-mounted source,
# node_modules/.next in named volumes so the musl install isn't shadowed

FROM node:24-alpine

# Pin a locale Alpine actually has - stops bash whining about leaked LC_ALL
ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# git for the odd postinstall script, curl for healthchecks + debugging
RUN apk add --no-cache git curl

WORKDIR /var/www/storefront

# Deps in their own layer keyed on the lockfiles - source is mounted at runtime
COPY apps/channels/vetrina-demo/package.json apps/channels/vetrina-demo/package-lock.json ./
RUN npm ci --no-audit --no-fund

# Writable for any uid (the service runs as yours) - the mkdir matters - an
# empty volume's mountpoint comes up root-owned unless the image has the dir
RUN mkdir -p .next && chmod -R a+rwX /var/www/storefront /tmp

# -H 0.0.0.0 so Next listens beyond the container's localhost
EXPOSE 3000
CMD ["npm", "run", "dev", "--", "-H", "0.0.0.0", "-p", "3000"]

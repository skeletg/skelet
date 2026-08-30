# Dashboard dev image - Vite + HMR over the bind-mounted source,
# node_modules in a named volume so the musl install isn't shadowed

FROM node:24-alpine

# Pin a locale Alpine actually has - stops bash whining about leaked LC_ALL
ENV LC_ALL=C.UTF-8 \
    LANG=C.UTF-8

# git for the odd postinstall script, curl for healthchecks + debugging
RUN apk add --no-cache git curl

WORKDIR /var/www/dashboard

# Deps in their own layer keyed on the lockfiles - source is mounted at runtime
COPY apps/dashboard/package.json apps/dashboard/package-lock.json ./
RUN npm ci --no-audit --no-fund

# Writable for any uid (the service runs as yours) - the mkdir matters - an
# empty volume's mountpoint comes up root-owned unless the image has the dir
RUN mkdir -p node_modules/.vite && chmod -R a+rwX /var/www/dashboard /tmp

# --host so Vite listens beyond the container's localhost
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "--port", "5173"]

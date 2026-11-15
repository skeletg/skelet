# Deploying

Every app deploys on its own, the way it would from a repo of its own. Core and
Signals each have a `compose.yaml` and a `.env.example` at their root, beside
their `Dockerfile.prod`. The dashboard and the storefronts aren't containers at
all: each builds into what a web host serves, configured by its own
`.env.production`. They know each other only by addresses and shared secrets, so
any of them can share a box with the rest or sit somewhere else entirely. Every
backend service binds to loopback by default, so something has to sit in front
and terminate TLS.

## One backend

```bash
skelet setup core             # writes apps/backend/core/.env, secrets included
cd apps/backend/core
docker compose up -d
```

`skelet setup` with app names sets up a server with just those, and assumes nothing
else runs there. `skelet setup signals --shared core.env` does Signals on a machine
of its own: the values it shares with core ([env.md](env.md#the-files) lists them)
come from a copy of core's `.env`, or from what it already holds, or are asked for -
setup never makes them up, since a value core doesn't have breaks every call between
the two. Copying `.env.example` to `.env` and filling it in by hand works just as
well.

That `.env` holds everything the app reads, secrets included. `COMPOSE_PROFILES`
in it starts the datastores it can bring along - `bundled` for Postgres (and
ClickHouse on Signals), `valkey` for the Valkeys - and drops one once you point
its URL at a managed instance. A new image is a new `CORE_IMAGE` or
`SIGNALS_IMAGE` line and another `docker compose up -d`, which touches nothing
else.

Across machines, the addresses are settings:

| On | Setting | Points at |
|---|---|---|
| core | `SIGNALS_GRPC_ADDR` | Signals' contract, port 8092 |
| core | `SIGNALS_INGEST_URL`, `SIGNALS_URL` | Signals' `/t` and `/e`, port 8090 |
| Signals | `CORE_FRONT_API_URL` | core's storefront API |
| Signals | `INGEST_FALLBACK_URL` | core, for the beacons only it answers |
| Signals | `VALKEY_URL` | core's primary Valkey - optional, it's how Signals hears about revoked admin sessions at once |

Signals publishes 8090, 8091 and 8092 on `SIGNALS_BIND` (loopback by default) -
set it to an address core can reach. The contract runs plaintext unless TLS is
on ([env.md](env.md)); put the two on a private network either way.

## One box

`docker-compose.prod.yml` includes both backends, each still reading its own
`.env`, plus the optional edge. `docker compose -f docker-compose.prod.yml up -d`
starts it all, and naming services redeploys only those: `docker compose -f
docker-compose.prod.yml up -d signals ingest`.

The root `.env` is read first and wins over the backends' own, so it holds only
what the edge uses. `skelet setup core signals` writes both backends' files, with
the values they share copied straight across, and moves any backend key it finds in
the root `.env`.

`docker-compose.small.yml` is the minimum deployable tenant: core alone, its
`.env` set so one app container runs every backend role (API, WS, media
transforms, scheduler, webhook delivery, outbox relay) against one Postgres - no
worker or media replicas, `JOBS_EXTERNAL=false`, `VALKEY_URL` empty. No NATS, no
OpenSearch and no Signals. Its API listens on 18080 so it runs beside the dev
stack, and `skelet setup core --small` writes it.

`skelet release core signals` builds and pushes the backends' images and prints
the line each `.env` takes.

Both shapes need a second bucket besides the media one, with no public access,
named in `PRIVATE_S3_BUCKET` - exports, uploads and private media live there,
and production won't start without it. [env.md](env.md#private-store) has what
it needs.

## The frontends

Each frontend builds in its own directory, from settings in its own
`.env.production` - copy the `.env.production.example` beside it, or let `skelet
setup <name>` write it, taking the values it shares with core from core.

**The dashboard** is static files: `pnpm run build` in `apps/dashboard` makes
`dist/`, which any web server, CDN or static host serves - every path that isn't
a file falls back to `index.html`. Its address for core, `VITE_GRAPHQL_URL`, is
baked in: leave it relative and put the dashboard behind something that proxies
`/api/*` to core, or make it absolute and add the dashboard's origin to core's
`CORS_ALLOWED_ORIGINS`. The build also writes `dist/_headers`, the security
headers in the form Netlify and Cloudflare Pages read, with core's origin and
`UPLOAD_ORIGINS` already in its `connect-src` - another host should send the same
headers - and `dist/_redirects`, the `index.html` fallback in Netlify's form.
Cloudflare Pages falls back on its own and says in its build log that it skips
that rule.

**A Next storefront** is a Node server. `pnpm run build` leaves a self-contained
one in `.next/standalone`, its static files beside it, which runs anywhere with
Node: `pnpm run start` is `node --env-file-if-exists=.env.production
.next/standalone/server.js`, on the port `PORT` names (3000 by default). A
platform that builds Next itself, Vercel or Netlify, just needs the same settings
in its own environment. Build it from the repo root's workspace -
`pnpm install --frozen-lockfile --filter ./templates/nextjs-vetrina...` - or from
the storefront's directory when it takes the packages from npm.

**The Astro storefront** builds the same way, and `pnpm run start` runs
`dist/server/entry.mjs` with its `.env.production`.

**On a machine of your own**, `skelet setup` gives each storefront named on it
its own `PORT` - the one its dev script uses - so several share
a machine without colliding. The servers listen on every interface so the edge's
container can reach them, so keep their ports closed to the outside.

Deploy core before a frontend that needs something new from it. The other way
round is safe: `skelet check` holds core's GraphQL schemas compatible with the last
release, so a dashboard or storefront built against it keeps working on a newer core.

A storefront that isn't on the same private network as core needs
`TRUSTED_PROXY_KEY` on both, or core can't tell its shoppers apart for rate
limiting ([env.md](env.md#networking)).

## Managed datastores

Each backend's bundled datastores sit behind profiles in its own `.env`:
Postgres and its pooler (and ClickHouse on Signals) behind `bundled`, the
Valkeys behind `valkey`. Both are on in `.env.example`, so a self-hosted box
gets them by default.

Moving one to a managed service is dropping its profile and pointing the URL
outward, in the file of the app that uses it:

```bash
# apps/backend/core/.env
COMPOSE_PROFILES=valkey             # bundled dropped - Postgres is managed now
DATABASE_URL=postgres://...@your-managed-pg:5432/skelet?sslmode=require

# apps/backend/signals/.env
COMPOSE_PROFILES=valkey
SIGNALS_DB_URL=postgres://...@your-managed-pg:5432/skelet_signals?sslmode=require
SIGNALS_CH_URL=clickhouse://user:password@your-managed-ch:9440/signals?secure=true
```

On one box the profiles come from the root `.env` instead, since that's the
project compose runs. The containers behind a dropped profile then never start,
and their `*_PASSWORD` lines go unused.

The signals process migrates the spine on whatever `SIGNALS_CH_URL` names as it
boots, so the schema lands on the managed server without a second URL - a TLS
endpoint is `clickhouse://user:password@host:9440/signals?secure=true`.

Two things that bite:

- A profile is all or nothing: with `bundled` still on, every container behind
  it starts and refuses an empty password, including the ones whose URL you
  pointed elsewhere. Going half managed means keeping the passwords set.
- Every `depends_on` on those services carries `required: false`. Without it
  compose rejects the entire file the moment the profile is off, because the app
  tiers declare a dependency on a service that no longer exists.
- `envcheck` refuses to boot if `DATABASE_URL` has `sslmode=disable`. Managed
  providers want `require` or `verify-full` anyway; `ALLOW_INSECURE_DB=1` exists
  for the case where you terminate TLS yourself.

A managed commerce database needs the `pgvector` extension for [embeddings](embeddings.md). The big providers all
have it; where the app's user may not create extensions, run `CREATE EXTENSION vector;` once as the admin user.

## Moving to the pgvector image

The bundled commerce database runs `pgvector/pgvector`, Postgres 18 with the extension, built on Debian. A volume
the old `postgres:18-alpine` image made starts on it as it is, but the two sort text differently and Postgres can't
tell, so indexes on text columns would quietly come out in the wrong order. After the first start on the new image,
reindex once:

```bash
docker compose exec db psql -U skelet -d skelet -c 'REINDEX DATABASE skelet'
```

Take a dump first, as with any change to the database image.

A dashboard served from another origin than core's talks to the API directly, so
`CORS_ALLOWED_ORIGINS` has to name it, and `TRUSTED_PROXIES` has to name whatever
sits in front of the API, or forwarded client IPs are wrong.

Uploads go from the browser straight to object storage, so whichever host that
is has to be in `UPLOAD_ORIGINS`, or every CSP on the stack blocks the upload -
see [env.md](env.md#object-storage).

## Putting an edge in front

Three hostnames, whoever terminates TLS:

| Host | Upstream | Why it exists |
|---|---|---|
| apex, e.g. `shop.example.com` | the storefront's server | the storefront - every channel's host can point at the same upstream |
| `admin.` | the dashboard's `dist/`, and `/api/*` to `app:8080` | the admin SPA - proxying `/api/*` keeps it same-origin, so it needs no API host and no CORS |
| `api.` | `app:8080` | **inbound payment webhooks.** Providers POST to `/api/v1/payments/webhook/<provider>`, which is the URL you register with your payment provider. Skip this host and captures never settle |

`/t/*` is the Signals tier's, and `app` proxies it there per `SIGNALS_INGEST_URL`
- one public entry point, no routing needed. Route `/t/*` at `ingest:8091` to skip
the hop once that tier is scaled out, and `/media/t/*` at `media:8083` likewise.

HSTS belongs here and nowhere else. The backend and the dashboard's `_headers`
each ship their own copy of the CSP and Permissions-Policy so a deployment without
an edge still gets them, but they deliberately omit HSTS - asserting it over plain
HTTP would pin a browser to HTTPS for a year. `skelet check contracts` fails when
those copies drift apart.

**The bundled option.** `deploy/caddy/Caddyfile` has all of the above plus
automatic Let's Encrypt certs and renewal. It's behind a profile because it takes
`:80` and `:443`, which is the wrong move if you already run nginx, Traefik, or a
load balancer:

```bash
cp env/prod-caddy.env.example env/prod-caddy.env   # domains + ACME email
docker compose -f docker-compose.small.yml --profile edge up -d
```

It serves the dashboard from `apps/dashboard/dist`, so build that first, and
reaches the storefront's server on the same machine through
`host.docker.internal` - `STOREFRONT_UPSTREAM` in `env/prod-caddy.env`, which
`skelet setup` sets and which falls back to `host.docker.internal:3000`. The
vhost map is meant to be edited, so a storefront somewhere else is just another
upstream.

**More than one storefront on the machine.** The first takes the apex. Every
other one gets a file in `deploy/caddy/sites/`, which the Caddyfile imports: its
host is `<NAME>_DOMAIN` in `env/prod-caddy.env` (`OUTLET_DOMAIN` for a storefront
called `outlet`), and until you set it the edge answers it on `outlet.localhost`.
Setup writes the file once and leaves it to you from then on, and `skelet rm
channel` takes it away. With more than one storefront, setup also leaves
`STOREFRONT_REVALIDATE_URL` empty, so core purges each channel at its own public
URL and every purge reaches the storefront that serves it - give each channel its
URL in admin.

**More than one channel.** One storefront deployment serves every channel: add
each channel's host to `STOREFRONT_CHANNEL_HOSTS` in the storefront's
`.env.production` and point the host at the same upstream. The edge has to
pass the original `Host` through, which Caddy and nginx's
`proxy_set_header Host $host` both do. Each channel's pages and cache entries
live under their own path inside the app, so a new channel is a new host, not a
new deployment. `STOREFRONT_REVALIDATE_URL` pointing at that server purges
every channel; see [env.md](env.md#storefront-links).

**Keep it to one replica** on a server of your own. The rendered pages and the
data cache live in the storefront process, and nothing shares them between
processes. With two
replicas behind a load balancer, a purge lands on whichever one answers it and
the other keeps serving the old page until its own timer runs out. So run one
storefront replica per deployment, and give it more CPU before you give it a
twin. More replicas only work once a shared `cacheHandler` (Redis or similar)
is configured in `next.config.ts`, which the storefront doesn't ship.

## Operations

**Health.** The API serves `/health` (pings Postgres + Valkey) and `/readyz`
(503s while optional pieces are unconfigured); the worker, media, signals and
ingest tiers each serve `/healthz`. The compose healthchecks already point at all
of these.

**Backups.** skelet takes none - that belongs to your infrastructure. Turn on
point-in-time recovery if you're on managed Postgres, or run pgBackRest or wal-g
against the bundled containers, and back the ClickHouse spine up with its own
`BACKUP DATABASE`. What needs covering and what doesn't:
[backups.md](backups.md).

**Tracing and metrics.** OpenTelemetry, opt-in - nothing is exported until you
set the `OTEL_*` vars. Sentry activates when a DSN is set. In dev, the optional
`grafana` tool gives you a local Grafana to send them to
([workflow.md](workflow.md#optional-dev-tools)); in production, point the vars at the
collector you already run.

**Logs.** Every service writes structured JSON to stdout, and the compose files cap
Docker's copy at 10 MB x 3 per service, so that cap is also the retention. Shipping
and searching history is the deployment's job, not the repo's: point a collector
(Vector, Fluent Bit, Grafana Alloy, the OpenTelemetry Collector) at the container
logs and send them to whatever log store you already run - Grafana Loki and
VictoriaLogs are the usual self-hosted answers, both with a UI. Nothing in the
stack depends on it being there.

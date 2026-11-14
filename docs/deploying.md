# Deploying

Each backend deploys on its own, the way it would from a repo of its own: core
and Signals each have a `compose.yaml` and a `.env.example` at their root, beside
their `Dockerfile.prod`. They know each other only by the addresses in those
files, so they can share a box or sit on different servers. Every service binds
to loopback by default, so something has to sit in front and terminate TLS.

## One backend

```bash
cd apps/backend/core          # or apps/backend/signals
cp .env.example .env          # fill it in - skelet setup does it for you
docker compose up -d
```

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
`.env`, plus the frontends and the optional edge. `docker compose -f
docker-compose.prod.yml up -d` starts it all, and naming services redeploys
only those: `docker compose -f docker-compose.prod.yml up -d signals ingest`.

The root `.env` is read first and wins over the backends' own, so it holds only
what the frontend blocks and the edge use. `skelet setup` with the prod target
writes all three files and moves any backend key it finds in the root one.

`docker-compose.small.yml` is the minimum deployable tenant: core alone, its
`.env` set so one app container runs every backend role (API, WS, media
transforms, scheduler, webhook delivery, outbox relay) against one Postgres - no
worker or media replicas, `JOBS_EXTERNAL=false`, `VALKEY_URL` empty. No NATS, no
OpenSearch and no Signals. Its ports are offset (18080/18081/13000) so it runs
beside the dev stack, and `skelet setup` with the small target writes it.

Each storefront runs under a compose profile of its own name, so the demo one
also needs `nextjs-vetrina` in the root `.env`'s `COMPOSE_PROFILES`. `skelet
setup` copies only the env files of the storefronts you pick, turns their
profiles on, and points `STOREFRONT_REVALIDATE_URL` and the edge at the one it
serves.

`skelet release <app>...` builds and pushes the images you name - `core`,
`signals`, `dashboard` or a storefront - and prints the line each `.env` takes.

Both shapes need a second bucket besides the media one, with no public access,
named in `PRIVATE_S3_BUCKET` - exports, uploads and private media live there,
and production won't start without it. [env.md](env.md#private-store) has what
it needs.

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

If the frontends move to a platform like Vercel, `CORS_ALLOWED_ORIGINS` stops
being optional - the browser starts talking to the API origin directly instead of
being proxied to it by the dashboard's own nginx - and `TRUSTED_PROXIES` has to
name whatever sits in front of the API, or forwarded client IPs are wrong.

Uploads go from the browser straight to object storage, so whichever host that
is has to be in `UPLOAD_ORIGINS`, or every CSP on the stack blocks the upload -
see [env.md](env.md#object-storage).

## Putting an edge in front

Three hostnames, whoever terminates TLS. The upstream ports are the same in both
stacks (the small profile publishes them on offset loopback ports so it can run
beside the dev stack, but the in-network ports don't move):

| Host | Upstream | Why it exists |
|---|---|---|
| apex, e.g. `shop.example.com` | `<storefront>:3000`, its compose service | the storefront - every channel's host can point at the same upstream |
| `admin.` | `dashboard:8080` | the admin SPA - its own nginx proxies `/api/*` to the backend in-network, so it needs no API host |
| `api.` | `app:8080` | **inbound payment webhooks.** Providers POST to `/api/v1/payments/webhook/<provider>`, which is the URL you register with your payment provider. Skip this host and captures never settle |

`/t/*` is the Signals tier's, and `app` proxies it there per `SIGNALS_INGEST_URL`
- one public entry point, no routing needed. Route `/t/*` at `ingest:8091` to skip
the hop once that tier is scaled out, and `/media/t/*` at `media:8083` likewise.

HSTS belongs here and nowhere else. The backend and the dashboard's nginx each
ship their own copy of the CSP and Permissions-Policy so a deployment without an
edge still gets them, but they deliberately omit HSTS - asserting it over plain
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

The vhost map is meant to be edited, all but the apex's upstream: that one reads
`STOREFRONT_UPSTREAM` from `env/prod-caddy.env`, which `skelet setup` points at
the storefront you run, and falls back to `nextjs-vetrina:3000`. A shop of your
own needs a service in the prod or small file first - the template's block,
renamed along with its profile.

**More than one channel.** One storefront image and one container serve every
channel: add each channel's host to `STOREFRONT_CHANNEL_HOSTS` in the
storefront's env file and point the host at the same upstream. The edge has to
pass the original `Host` through, which Caddy and nginx's
`proxy_set_header Host $host` both do. Each channel's pages and cache entries
live under their own path inside the app, so a new channel is a new host, not a
new deployment. `STOREFRONT_REVALIDATE_URL` pointing at that service purges
every channel; see [env.md](env.md#storefront-links).

**Keep it to one replica.** The rendered pages and the data cache live in the
storefront process, and nothing shares them between containers. With two
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

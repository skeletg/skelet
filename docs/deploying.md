# Deploying

Two shapes, same images - the difference is env, not a build. Both bind every
service to loopback, so something has to sit in front and terminate TLS.

## The two shapes

`docker-compose.small.yml` is the minimum deployable tenant: one app container
running every backend role (API, WS, media transforms, scheduler, webhook
delivery, outbox relay) and one Postgres. No Valkey, no NATS, no OpenSearch, and
Signals off. `--profile signals` adds the suite: its process, ClickHouse for the
event spine, and the two migrations, with `SIGNALS_ENABLED=true` in
`env/small.env`. Its ports are offset (18080/18081/13000/15432) so it runs
beside the dev stack.

```bash
cp env/small.env.example env/small.env
cp env/small-vetrina-demo.env.example env/small-vetrina-demo.env
cp .env.example .env    # JWT_SECRET, SIGNALS_SERVICE_SECRET, CONFIG_ENCRYPTION_KEY, DB_PASSWORD
docker compose -f docker-compose.small.yml up -d
```

`docker-compose.prod.yml` is the full shape: the tiers split into their own
containers (worker, media, signals, ingest), pgbouncer per database, the split
Valkey pair, ClickHouse for the spine, and the optional NATS profile. Same env contract -
secrets in the root `.env`, everything else in `env/prod.env`. `./skelet release`
builds and pushes the four images it reads: the backend (`APP_IMAGE`, also the
worker and media tiers), the Signals module (`SIGNALS_IMAGE`, the signals and
ingest tiers plus the ClickHouse schema), the dashboard and each storefront.

Scaling from one to the other is that env changing: set `VALKEY_URL` for
multi-replica, then `JOBS_EXTERNAL` to split the
background tiers back out.

The root `.env` is the only file docker compose interpolates, which is why the
secrets that have to appear twice (a datastore *and* the URL reaching it) live
there rather than in `env/`. Everything else is bulk config handed to one
container through `env_file:`. See [env.md](env.md).

## Managed datastores

The bundled Postgres, Valkey and pgbouncer containers sit behind the `bundled`
compose profile, and `.env.example` ships `COMPOSE_PROFILES=bundled` - so a
self-hosted box gets them by default and nothing changes.

Moving to managed services is dropping that line and pointing the URLs outward:

```bash
# .env
# COMPOSE_PROFILES=bundled          <- removed
DATABASE_URL=postgres://...@your-managed-pg:5432/skelet?sslmode=require
SIGNALS_DB_URL=postgres://...@your-managed-pg:5432/skelet_signals?sslmode=require
VALKEY_URL=rediss://...
```

The six containers then never start, instead of idling beside the databases you
are actually paying for. Everything else - app, worker, media, signals, ingest -
is the same image and the same file.

Two things that bite:

- Every `depends_on` on those services carries `required: false`. Without it
  compose rejects the entire file the moment the profile is off, because the app
  tiers declare a dependency on a service that no longer exists.
- `envcheck` refuses to boot if `DATABASE_URL` has `sslmode=disable`. Managed
  providers want `require` or `verify-full` anyway; `ALLOW_INSECURE_DB=1` exists
  for the case where you terminate TLS yourself.

If the frontends move to a platform like Vercel, `CORS_ALLOWED_ORIGINS` stops
being optional - the browser starts talking to the API origin directly instead of
being proxied to it by the dashboard's own nginx - and `TRUSTED_PROXIES` has to
name whatever sits in front of the API, or forwarded client IPs are wrong.

## Putting an edge in front

Three hostnames, whoever terminates TLS. The upstream ports are the same in both
stacks (the small profile publishes them on offset loopback ports so it can run
beside the dev stack, but the in-network ports don't move):

| Host | Upstream | Why it exists |
|---|---|---|
| apex, e.g. `shop.example.com` | `vetrina-demo:3000` | the storefront |
| `admin.` | `dashboard:8080` | the admin SPA - its own nginx proxies `/api/*` to the backend in-network, so it needs no API host |
| `api.` | `app:8080` | **inbound payment webhooks.** Providers POST to `/api/v1/payments/webhook/<provider>`, which is the URL you register with Stripe/Braintree. Skip this host and captures never settle |

`/t/*` is the Signals tier's, and `app` proxies it there per `SIGNALS_INGEST_URL`
- one public entry point, no routing needed. Route `/t/*` at `ingest:8091` to skip
the hop once that tier is scaled out, and `/media/t/*` at `media:8083` likewise.

HSTS belongs here and nowhere else. The backend and the dashboard's nginx each
ship their own copy of the CSP and Permissions-Policy so a deployment without an
edge still gets them, but they deliberately omit HSTS - asserting it over plain
HTTP would pin a browser to HTTPS for a year. `./skelet check contracts` fails when
those copies drift apart.

**The bundled option.** `deploy/caddy/Caddyfile` has all of the above plus
automatic Let's Encrypt certs and renewal. It's behind a profile because it takes
`:80` and `:443`, which is the wrong move if you already run nginx, Traefik, or a
load balancer:

```bash
cp env/prod-caddy.env.example env/prod-caddy.env   # domains + ACME email
docker compose -f docker-compose.small.yml --profile edge up -d
```

The vhost map is the one thing no tool can discover, so that file is meant to be
edited - the storefront upstream in particular names the demo service rather than
deriving your shop's.

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
set the `OTEL_*` vars. Sentry activates when a DSN is set. In dev, `docker
compose --profile tracing up` starts Jaeger and the app sends traces to it out of
the box.

**Logs.** Container stdout is capped at 10 MB x 3 per service. For durable,
searchable history the `logs` profile ships stdout through Vector
(`deploy/vector/`) into ClickHouse - query it with `./skelet logs query`.

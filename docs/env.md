# Environment variables

**You should not need to read this to get started.** `./skelet setup` asks what you
want and writes these files for you, secrets included - see
[docs/workflow.md](workflow.md#first-run). Come here when you want to
change something afterwards, or to understand a variable the wizard did not ask
about.

Each file is a `.example` you copy and fill in; the copies are gitignored.

The examples stay deliberately thin - variable names, a default, and at
most a one-line hint. Everything that needs a paragraph lives here.

## The files

| File | Copy to | Read by |
|---|---|---|
| `../.env.example` | `../.env` | docker compose itself (prod and small) - see below |
| `go.env.example` | `go.env` | core in dev (`app`, `worker`, `media`, `migrate`) |
| `signals.env.example` | `signals.env` | the Signals processes in dev (`signals`, `ingest`) - `./skelet up` writes it and copies `JWT_SECRET` / `SIGNALS_SERVICE_SECRET` over from `go.env` |
| `db.env.example` | `db.env` | the dev Postgres container |
| `prod.env.example` | `prod.env` | `app`, `worker`, `media`, `signals`, `ingest`, `migrate` - one shared file |
| `prod-signals.env.example` | `prod-signals.env` | `signals` only, on top of `prod.env` - the retention archive's bucket and `CORE_FRONT_API_URL`; optional |
| `prod-vetrina-demo.env.example` | `prod-vetrina-demo.env` | the storefront container |
| `small.env.example` | `small.env` | the single app container in the small profile |
| `small-vetrina-demo.env.example` | `small-vetrina-demo.env` | its storefront container |
| `prod-caddy.env.example` | `prod-caddy.env` | the opt-in Caddy edge (`--profile edge`), in both prod and small |

The two `small.*` files belong to `docker-compose.small.yml`, the
minimum deployable tenant (one app container, one Postgres, no Valkey /
NATS / ClickHouse / OpenSearch). They follow the same split as the prod
pair: secrets and DSNs come from the root `.env`, everything else from
the file.

### Why the root .env exists

Compose interpolates `${VAR}` from the root `.env` (or the shell) and from
nowhere else - never from an `env_file`. That's the whole reason it's a
separate file.

Passwords are the one thing that has to appear twice: once on the
datastore container, once inside the URL that connects to it. Putting them
in the root `.env` lets compose derive both from a single value, so they
can't drift. A file per datastore would mean keeping each password in step
with a DSN in `prod.env` by hand, which is the failure this avoids.

The split is: secrets in the root `.env`, everything else in
`env/prod.env`.

Each URL stays overridable in full. Set `DATABASE_URL` in the root `.env`
and it wins over the derived one, so a managed or external datastore is a
one-line change and the matching `*_PASSWORD` just goes unused.

The two profile-gated passwords work as switches: set `NATS_PASSWORD` and
`SIGNALS_QUEUE_URL` derives itself, leave it unset and the queue stays
off. Same for `CLICKHOUSE_PASSWORD`. They can't use compose's
"required" marker, because compose interpolates every service whether or
not its profile is active - a missing value would break `up` for everyone
not running that profile. The check lives in the container's start command
instead, so it fires only when the profile is actually on.

The storefront still keeps its own file. That's a security boundary, not
duplication: the most internet-exposed container holds no DB password, S3
key or SMTP credential. The two values it shares with the backend
(`JWT_SECRET`, `STOREFRONT_REVALIDATE_SECRET`) come from the root `.env`
via compose, so they aren't written twice either.

One gotcha on a machine that runs both stacks: the root `.env` is shared
between `docker-compose.yml` and `docker-compose.prod.yml`, so dev values
there apply to prod commands too.

### Dev host ports

The dev stack publishes its datastores on `127.0.0.1` only, and nothing on
the host dials them - `./skelet psql` goes through `docker compose exec`. So
each one can move when something else already holds its port, most often a
local Postgres on 5432:

| Var | Default | Service |
|---|---|---|
| `DB_HOST_PORT` | 5432 | commerce Postgres |
| `SIGNALS_DB_HOST_PORT` | 5433 | Signals Postgres |
| `VALKEY_HOST_PORT` | 6379 | Valkey |
| `CLICKHOUSE_HOST_PORT` / `CLICKHOUSE_HTTP_HOST_PORT` | 9000 / 8123 | ClickHouse native / HTTP |
| `OPENSEARCH_HOST_PORT` | 9200 | OpenSearch |
| `NATS_HOST_PORT` | 4222 | NATS |
| `SIGNALS_HOST_PORT` | 8090 | Signals health |
| `GARAGE_ADMIN_HOST_PORT` | 9903 | Garage admin API |

`./skelet up` (and so `./skelet setup`) probes every port it's about to
publish before it starts anything. On a terminal it asks, port by port,
whether you've stopped the other thing - it checks again, and asks again if
you haven't - or offers to move a datastore to a free port and writes the
var above into the root `.env`. Without a terminal it lists the clashes and
exits. The app-facing ports (8080, 5173, 3000, 8025, 9900-9902) stay
fixed, because they're part of the origins and CSP the apps are built
around, so for those the only answer is to free them.

`./skelet up` also builds on every run (`docker compose up --build`). When
nothing changed that's a cache hit, and it's what keeps an image from an
older checkout from running on after a pull changed its Dockerfile or
`go.mod` - the project name is pinned, so another clone's image counts.

`prod.env` is shared by every backend role, which is why role-specific
settings belong in the compose service and not in here - the app has to
stay a server. That's `SKELET_ROLE` (`worker`, default `server`) and
`WORKER_PORT` (8081, its `/healthz`).

Prod boot fails while a REQUIRED var is blank. That's deliberate - a
silent default in production is worse than a refusal to start.

## Required to boot

| Var | Notes |
|---|---|
| `JWT_SECRET` | `openssl rand -hex 32`, in the root `.env`. Compose hands it to the backend, the Signals processes and the storefront, so it's written once. It signs sessions, and nothing else. The default value is rejected in production. |
| `SIGNALS_SERVICE_SECRET` | A second `openssl rand -hex 32`, in the root `.env`. Core signs every call on the Signals wire contract with it and the suite verifies with the same value. Compose hands it to core and the Signals processes alone: the storefront holds `JWT_SECRET`, and a storefront that could mint service tokens could call the contract as core. Production refuses it empty, short, or equal to `JWT_SECRET`. |
| `CONFIG_ENCRYPTION_KEY` | `openssl rand -base64 32`, in the root `.env`. Keep it stable - rotating it orphans every stored secret. |
| `CORS_ALLOWED_ORIGINS` | Storefront + dashboard origins, comma-separated. Empty in dev means localhost defaults. |
| `DB_PASSWORD` | Commerce Postgres. `DATABASE_URL` derives from it, or set that directly for an external database. |
| `SIGNALS_DB_PASSWORD` | Signals Postgres, same deal via `SIGNALS_DB_URL`. Only the Signals processes read it - see below. |

Media storage config (`S3_BUCKET_NAME`, `AWS_REGION`) is also checked at
boot. The bucket doesn't have to be reachable for the app to start, but
media breaks the moment anyone uploads or views an image.

## Datastores

### Commerce Postgres

`DATABASE_URL` is the source of truth for everything except Signals.

The bundled `db` service sits on the trusted compose network, so
`sslmode=disable` plus `ALLOW_INSECURE_DB=1` is accepted. Point at a
managed or remote Postgres and you should drop `ALLOW_INSECURE_DB` and
use `sslmode=require` (or `verify-full`) - the backend refuses to boot on
`sslmode=disable` without the escape hatch.

`DATABASE_REPLICA_URL` gives analytics reads their own small pool. Point
it at a read replica to keep reporting off the primary.

Pool sizes: `DB_MAX_CONNS` (25) and `ANALYTICS_DB_MAX_CONNS` (8).

### Signals Postgres

`SIGNALS_DB_URL` holds the event spine, Lens, identities and Pulse - campaigns,
flows and the email builder. It is the Signals processes' database: `signals`
and `ingest` open it, and `app` and `worker` never do - they reach everything in
it over the wire contract.

It does not need a separate server. A second database on the commerce
Postgres instance works exactly the same, which is the usual answer for
small deployments - a `CREATE DATABASE`, not another container.

`SIGNALS_GRPC_ADDR` is where `cmd/signals` serves the wire contract core calls
(`signals:8092` in compose). It is core's only address for Signals, and leaving
it unset means core makes no calls at all - the admin chat then drops its
Signals tools rather than offering ones that answer "unreachable". With it set,
`SIGNALS_SERVICE_SECRET` has to be too, or core refuses to boot rather than
sign calls the suite would reject.

Every call core makes carries a deadline - ten seconds unless the caller set a
shorter one, thirty minutes for the maintenance calls (a dump, a restore, a
roster reconcile) - so a hung suite cannot hang the request that asked, a
password reset most of all. When OpenTelemetry is on (`OTEL_*`), the trace
started at core's HTTP edge continues into the suite's handler.

`SIGNALS_REPLICA_URL` is the suite's read pool, bulkheaded off the ingest
writes. Unset means the Signals primary. It is NOT `DATABASE_REPLICA_URL`
- that's the commerce replica and doesn't hold spine tables.

`CORE_FRONT_API_URL` is where the suite reads core's public storefront API -
`http://app:8080/api/front/graphql` in compose, set in `signals.env` (dev) or
`prod-signals.env` (prod). The email builder's products
block prices and links products the way the storefront does, and this is the
one call the suite makes into core. Unset, the block renders nothing and every
other email renders as before.

### Connection pooler

Prod compose runs pgbouncer in front of each Postgres (edoburu/pgbouncer,
which listens on 5432 unless LISTEN_PORT says otherwise - the compose pins
6432), one instance per database so the bulkhead survives the pooler. It exists because server
connections are otherwise `replicas x app pool`: stock Postgres allows
100, and app + worker + ingest + media already reserve ~93 before a
second replica exists.

Behind it, the app-side pools become client slots that queue in
pgbouncer's memory, and `DEFAULT_POOL_SIZE` (20) is what actually reaches
Postgres. Size that one to the database's cores, not to traffic - past
roughly a dozen concurrent statements a 2-CPU Postgres context-switches
more than it works.

`POOL_MODE=transaction` is what makes any of that true, and it has three
consequences worth knowing:

- **Migrations bypass it.** golang-migrate holds a session-level advisory
  lock for the whole run, so `migrate` and `signals-migrate` keep the
  direct DSNs.
- **Server-side prepared statements are off.** The pooled DSNs carry
  `default_query_exec_mode=exec`, because a `PREPARE` lands on whichever
  server connection is free and the next `EXECUTE` may not find it.
  pgbouncer 1.21+ can track them itself via `max_prepared_statements`;
  set that and the parameter can go.

Setting `DATABASE_URL` or `SIGNALS_DB_URL` outright still wins over all of
it, which is how you point at a managed datastore - the pooler then has
nothing to do and you can drop its two services.

Pool sizes: `SIGNALS_DB_MAX_CONNS` (8) and `SIGNALS_READ_DB_MAX_CONNS` (8)
in the signals process; the ingest tier has its own,
`INGEST_SIGNALS_MAX_CONNS` (16), because beacons write inside the request
handler and it holds a connection per concurrent write.

Budget them against the server, not appetite. Stock Postgres allows 100
connections; app and worker hold 16 each, the signals worker another 16,
so one ingest replica at 16 leaves room for a second. Beyond that, raise
`max_connections` and the database's CPU first, or put the ingest queue
in front - it lands a whole batch as one statement, which is the actual
fix for beacon volume. More connections against a 2-core Postgres move
the wait rather than remove it.

### Valkey

`VALKEY_URL` is OPTIONAL. Leave it empty to run Postgres-only, which is a
reasonable choice for single-node and small deployments. Everything keeps
working, with these tradeoffs:

| Area | Without Valkey |
|---|---|
| caches | no-op, so every read hits Postgres (slower, still correct) |
| rate limits | enforced per process, so the real budget is rate x replicas (when enabled at all - see below) |
| login lockout | per process, same caveat - still enforced, not bypassed |
| auth tokens | refresh / verification / reset move to Postgres (`auth_tokens` + `auth_refresh_tokens`, swept hourly by the `auth-token-prune` job) |
| notifications | reach the replica that created them, not all replicas |
| draft previews | UNAVAILABLE - the only feature that hard-requires Valkey |

A malformed URL is still a boot error. That's a typo, not a choice.

Multi-replica deployments should keep Valkey: per-process rate limits and
single-replica notification fan-out both get weaker as you scale out.

In prod, set `VALKEY_PASSWORD` in the root `.env` and `VALKEY_URL` derives
from it. To run Postgres-only, set `VALKEY_URL=` explicitly - that wins
over the derived value.

`VALKEY_CACHE_URL` is OPTIONAL and points the catalog/CMS read caches and the
storefront response cache at a different instance from the auth primary. Leave
it empty and they share `VALKEY_URL`. The response cache is keyed per channel
and locale, so its footprint grows with the channel count: the bundled
`valkey-cache` takes `VALKEY_CACHE_MAXMEMORY` (default `512mb`) under a
`VALKEY_CACHE_MEMORY_LIMIT` container cap (default `640m`), and a many-channel
install should raise both.

`VALKEY_LIMITS_URL` is OPTIONAL and holds the rate-limit counters and login
lockouts. The prod stack points it at a small bundled `valkey-limits`
(`volatile-ttl`), so a flood on the evictable cache can't evict the counters
and quietly lift every limit. Leave it empty and the counters share the cache
instance; set it to `VALKEY_URL` and they share the primary. The Signals process reads its caches
off the same variable, and reads the killed-session list core publishes off
`VALKEY_URL`, so a logout refuses a still-valid service token on the contract
too. Without a `VALKEY_URL` that check is skipped, as it is on core's own surface.

### ClickHouse (the spine)

The event spine, `analytics_events` and `analytics_identities`, lives in
ClickHouse, and the Lens heatmap grid derives from those raw events. The
Signals processes refuse to boot without `SIGNALS_CH_URL`. Sessions, replay
snapshots, rollups and everything Pulse owns stay in the Signals Postgres.

In dev the server and its schema come up inside the `signals` compose profile,
so `./skelet up` starts them whenever `SIGNALS_ENABLED` is on, and `./skelet doctor`
fails if the suite is on and either the container or `signals.analytics_events`
is missing.

There are two URLs because the app driver and golang-migrate disagree on
DSN shape: `SIGNALS_CH_URL` and `SIGNALS_CH_MIGRATE_URL`. In prod the
migrate one-shot rewrites a `clickhouse://user:password@host:port/db` form of
`SIGNALS_CH_URL` itself, so `SIGNALS_CH_MIGRATE_URL` is only for a URL it can't
rewrite.

Set `CLICKHOUSE_PASSWORD` in the root `.env`; the server credentials and
both ClickHouse URLs derive from it. Backing the spine up is
`BACKUP DATABASE` on a schedule of your own, see [backups.md](backups.md).

The server is generic infrastructure, so each consumer gets its own
database. The compose file creates `signals` at first boot because the
spine is the first consumer. A second consumer (a log collector, say) gets
a `CREATE DATABASE` of its own.

Don't add `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT` - the env-created user
already has the DDL rights the Signals driver needs, and that flag grants
user administration on top. Bringing your own RBAC-managed user? Beyond
SELECT/INSERT on its database the spine needs CREATE DICTIONARY, SYSTEM
RELOAD DICTIONARY, ALTER DELETE (GDPR erasure), ALTER (retention partition
drops), dictGet (every read resolves identity through the dictionary),
CREATE TEMPORARY TABLE (segment members ride an external table), and
SELECT on `system.parts` (the retention sweep sizes partitions before
dropping them). A minimally-granted user passes boot and then fails at
runtime, so grant the whole list.

### NATS / JetStream (optional ingest queue)

Run with `--profile queue` and set `SIGNALS_QUEUE_URL`. Spine appends then
land on disk before the request returns, and the signals worker (REQUIRED
with this on) drains them into the engine. Unset means the direct spine
path.

Set `NATS_PASSWORD` in the root `.env`; the server credentials and
`SIGNALS_QUEUE_URL` both derive from it.

## Search

`OPENSEARCH_URL` is optional and prod compose ships no OpenSearch service.

Leaving it empty does more than disable the search box. Browse is the only
product-listing path, so every category, collection and PLP falls back to
the Postgres tier: ts_rank relevance instead of a tuned score, no
stemming, no merch boosts, and facets only on selectable attributes
(scalar ones like brand simply don't appear).

That tier is a real floor, not a stub - a downed cluster gives you a
slower store rather than an empty one. But it's meaningfully slower on
large catalogues, especially filtered browse. Point this at a cluster when
the catalogue grows.

Managed clusters need `OPENSEARCH_USERNAME` / `OPENSEARCH_PASSWORD`. A
private CA needs `OPENSEARCH_CA_CERT_FILE`.

## Object storage

S3-compatible, and required for media to work. There's no filesystem
driver.

Two shapes:

- **Real S3 / managed** - endpoints empty, `S3_USE_PATH_STYLE=false`.
- **Garage / MinIO** - both endpoints set, `S3_PUBLIC_URL_BASE` pointing
  at the web-endpoint vhost, `S3_USE_PATH_STYLE=true`.

`S3_PRESIGN_ENDPOINT_URL` is the browser-facing presign endpoint and falls
back to `S3_ENDPOINT_URL`. `S3_MAX_UPLOAD_BYTES` defaults to 10 MB.

In dev, `./skelet up` points all of it at the Garage sidecar, so leave it
blank in `go.env`.

Self-hosting object storage on the same box is fine for small single-server
deployments, but its volume is NOT in `pg_dump` - back it up separately or
a disk loss takes every product image with it.

`MEDIA_LIBRARY_ROOT_PREFIX` is OPTIONAL and scopes the media tier's library
lookups to one key prefix, for a bucket shared with something else. Empty
means the whole bucket, minus the prefixes the platform writes itself
(`catalog/`, `webforms/`, `data/`, `derived/`) - the library won't list,
upload into, rename or delete those, and won't delete its own root.

Moving a store between buckets, `skelet media-migrate` reads the S3 vars
above as the source and takes the destination from its own flags, falling back
to `DEST_AWS_ACCESS_KEY_ID` / `DEST_AWS_SECRET_ACCESS_KEY` when the flags are
omitted. Neither is needed in normal operation.

## Database backups

Environment-only by design: a backup destination editable at runtime would
let anyone with settings access redirect a full-database dump to a bucket
they control, past every permission scope. So there is no admin config for
this - only `BACKUP_*` env vars, readable solely by whoever controls the
deployment.

They go in `prod-signals.env`, which only the `signals` service loads, so the
archive credential never reaches the commerce containers.

- `BACKUP_S3_BUCKET` - the analytics retention archive's destination, not a
  database backup. skelet takes no backups; see [backups.md](backups.md) for
  how to set those up at the infrastructure level. Use a DEDICATED bucket and
  credential, not the media ones.
- `BACKUP_S3_ENDPOINT_URL`, `BACKUP_S3_REGION`, `BACKUP_S3_PATH_STYLE`,
  `BACKUP_S3_ACCESS_KEY_ID`, `BACKUP_S3_SECRET_ACCESS_KEY`, `BACKUP_S3_PREFIX`
  (default `db/`) - same shape rules as the media store above.

What lands there: with the
`signals/analytics/retention_action` setting on `archive`, the daily retention
jobs export aged spine events and send-log months as identity-stripped
`.jsonl.gz` files under `<prefix>archive/` before deleting the rows. With no
`BACKUP_S3_BUCKET` the sweep fails closed - it deletes nothing rather than
archiving nowhere.

## Email and SMS

`SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD`. In dev,
`./skelet up` points these at Mailpit; an unset `SMTP_HOST` prints emails to
stdout instead. Two processes send through them: `app` its own transactional
mail, and `signals` every campaign and flow mail - both read the same file, so
one relay serves the install.

`EMAIL_WEBHOOK_TOKEN` is the bearer token for the ESP bounce webhook at
`POST /webhooks/email/events`. Unset means that route 404s.

Twilio (`TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM`) backs the
Pulse `send_sms` node, read by the `signals` process. From is E.164.

## AI assistant keys

The provider and key normally live in settings (`ai/llm/provider`,
`ai/llm/api_key` - see [settings.md](settings.md)), because an operator can
change them without a deploy. The env vars are the fallback for when you would
rather keep the key out of the database, and they are read in this order:

1. the `ai/llm/api_key` setting
2. `AI_API_KEY` - provider-agnostic, wins over both below
3. `OPENAI_API_KEY` or `ANTHROPIC_API_KEY`, whichever matches the configured
   provider (`anthropic` is the default)

All three are OPTIONAL. With no key from any source the assistant stays off.

## Storefront links

`STOREFRONT_URL` is where customer email links land, `DASHBOARD_URL` where
admin password-reset links land. Include the scheme.

Cache purges (CMS publish, product and catalog edits, the cache screen) are
signed with `STOREFRONT_REVALIDATE_SECRET`, which matches the storefront's
own file. With `STOREFRONT_REVALIDATE_URL` set, that one storefront serves
every channel and gets a single request naming all the active ones - the
usual setup, with the in-network URL. Leave it empty and each active channel
is purged at its own public URL plus `/api/revalidate`, for channels served
by separate deployments; a channel with no public URL, or one on localhost,
is then not purged at all.

On the storefront side, one deployment serves as many channels as you point
at it. `STOREFRONT_CHANNEL_HOSTS` maps hosts to channel uuids
(`shop.example.com=<uuid>,de.shop.example.com=<uuid>`, admin > channels
shows the ids), and `STOREFRONT_CHANNEL_ID` is the channel for any host the
map doesn't name. Set only the id and every host serves that one channel. A
page request on a host neither covers is a 404, and so are the sitemaps and
robots.txt. Absolute URLs (canonicals, sitemap, JSON-LD) use the channel's
public URL, with `STOREFRONT_BASE_URL` as the fallback for a channel without
one. Neither is needed at build time: the image is the same for every
channel. `REVIEW_VOTER_TOKEN_SECRET` signs review-voter tokens and voting
500s without it.

`BACKEND_INTERNAL_URL` is where the storefront reaches the backend, and it's
read at runtime too, so compose sets it on the container rather than the
build. Besides GraphQL it carries the backend links the browser loads from the
shop's own origin: `/media/t/*` thumbnails, `/download/*`, `/unsubscribe/*`,
the `/subscribe/confirm/*` signup confirmations, the `/e/*` email pixels and clicks, and the `/t/*` beacons. Left unset it
falls back to `http://localhost:8080`, which only fits `pnpm run dev` on the
host.

A purge reaches one storefront process, since the page cache lives in it. Run
a single replica unless you've configured a shared cache handler; see
[deploying.md](deploying.md#putting-an-edge-in-front).

## Tiers you can split out

All of these are the same binary or an additive service. Everything works
in one process; splitting buys isolation.

| Tier | Switch | What it does |
|---|---|---|
| `worker` | `JOBS_EXTERNAL=true` | App stops claiming scheduler jobs, webhook deliveries and outbox rows, leaving them to the worker. |
| `signals` | `SIGNALS_ENABLED=true`, prod shape only | Serves the suite's own admin API on `:8090` and its wire contract on `:8092`, and performs the retention, forwarding and trait-compute work core's scheduler asks for. |
| `ingest` | route `/t/*` and `/e/*` | Serves the public surfaces - the beacons and the mail tracking routes - on `127.0.0.1:8091`. |
| `media` | route `/media/t/*` | Image resize + focal crop, so decode CPU doesn't land next to checkout. |

`JOBS_EXTERNAL` is read by the app AND the worker, so one setting
configures both - the worker ignores the flag and always runs its layer.
It's all DB-claim based, so leaving the flag false while a worker runs is
safe (both claim, work is partitioned), it just doesn't isolate anything.
Setting it true with NO worker running stops that background work entirely.

The Signals tier needs no such switch: core's scheduler owns the cadence
of every Signals job and the suite performs the work over the contract, so
one owner schedules and there is nothing to double-run.

`INGEST_FALLBACK_URL` points the ingest tier back at the app for the two
routes it can't serve alone - form submissions and back-in-stock signups,
which need the app's subscriber graph. Leave it unset and those two paths 404,
which the browser SDKs fail soft on; everything else on `/t/*` is unaffected.

`SIGNALS_INGEST_URL` is where `app` proxies `/t/*`, the beacon surface being the
suite's to serve - `http://signals:8090` by default. Unset, beacons answer as
they do with the suite switched off: a silent 204 on the POSTs and a 404 on the
SDK reads. The commerce beacons (`/t/forms*`, `/t/back-in-stock`) are `app`'s own
and are unaffected either way.

`SIGNALS_URL` is where `app` proxies `/e/*` - the open pixel and the click
redirect in every marketing mail, recorded against the send log they name.
`http://signals:8090` by default. The `ingest` tier serves the same routes, so
once `/t` points at a scaled-out ingest, `/e` can follow it: the two variables
are separate only so they need not move together. Unset, opens go unrecorded
and links in already-sent mail 404.

Two parity notes when `/t` routes to the ingest tier:

- `GEOIP_DB_PATH` is shared through `prod.env`, but the `.mmdb` file has
  to be mounted into the ingest container too, or country attribution
  silently stops for beacon traffic.
- `TRUSTED_PROXIES` must cover the ingest containers (the default
  `172.16.0.0/12` does). Otherwise the app sees them as the client on the
  proxied forms and back-in-stock paths, and its per-IP rate limits
  collapse into one bucket.

`/media/t/*` is served by `app` regardless, so routing it is a scaling choice.

Media tier knobs: `MEDIA_PORT` (8083), `MEDIA_MAX_SOURCE_PIXELS` (~40 MP -
uploads go straight to object storage via presigned PUT so nothing else
bounds their dimensions, and decode allocates roughly w*h*4 bytes no
matter how small the output; over the cap is a 422, not a retry),
`MEDIA_RENDER_SLOTS` (cmd/server defaults to half the CPUs since it has
requests to protect, the media tier to all of them; callers queue ~3s for
a slot then get a 503 with Retry-After, so a burst throttles before it
refuses), and `MEDIA_DB_MAX_CONNS` (focal points come from the media
library, so the tier wants `DATABASE_URL`; it runs without one, cover
crops just centre instead).

Worth knowing: warm derivatives are answered with a 302 to
`S3_PUBLIC_URL_BASE` so image bytes never pass through Go. With that
unset every hit gets proxied instead, and the tier buys you much less.

## Networking

`TRUSTED_PROXIES` is the list of proxy IPs and CIDRs whose
`X-Forwarded-For` is trusted. Empty means no proxy. Without it, per-IP
rate limits share one bucket.

`ALLOW_PRIVATE_WEBHOOK_TARGETS=true` bypasses the SSRF guard for
webhook/destination targets on private IPs. One policy for cmd/server and
the signals worker; unset means off outside development. Only for
receivers you host on the compose network.

`SEO_CRAWL_BYPASS_TOKEN` is OPTIONAL and is sent by the link-map crawler on
every request so a storefront behind basic auth, a staging password or a
maintenance gate still answers it. Empty means the crawler gets whatever an
anonymous visitor gets.

`PORT` overrides the port a process listens on. Each service has its own
default (8080 for the app), so you only need this when running one outside
compose.

## Observability and logs

`SENTRY_DSN` and the OTel vars are optional - blank means logs to stdout.
The dev Jaeger takes OTLP at `http://jaeger:4318`. `APP_VERSION` is the
release tag for Sentry and OTel; CI should inject the git sha.

Application logs go to stdout as JSON (text when `APP_ENV != production`),
and the compose files cap them at 10 MB x 3 files per service so they
can't fill the disk. That cap is also the retention: rotated-out lines are
GONE. If you need searchable history, point a collector (Vector, Fluent Bit,
Grafana Alloy) at the container log files or swap the logging driver.

OTLP carries traces and metrics only. It does NOT ship logs.

`LOG_LEVEL` takes `debug | info | warn | error`.

`OTEL_SERVICE_NAME` overrides the identity a process reports to OTel. Sibling
processes - `cmd/signals`, `cmd/ingest`, `cmd/media` - set it so their traces
don't all land under the app's name. Unset falls back to the built-in default.

`PPROF_ENABLED=true` mounts `/debug/pprof` on the service's own mux. Off by
default because these ports are published in dev, and worth switching on only
for as long as you are profiling.

## Rate limiting and login lockout

Both ship **off**. A fresh install shouldn't hand out 429s while you click
around, and a dev box has no proxy in front of it for per-IP keying to mean
much anyway.

```
RATE_LIMIT_ENABLED=true
```

That one flag turns on every per-IP limiter (admin GraphQL, storefront
GraphQL, plugin HTTP routes, media transforms, download/unsubscribe/email
tokens, the `/t/*` beacons) and the failed-login lockout. In production the
boot log warns while it's off, since a public deployment with no per-IP
budget is one scraper away from trouble. It's a warning and not a refusal
because your proxy or CDN may already be doing the metering.

Once it's on, each surface has its own per-minute budget:

| Var | Default | Surface |
|---|---|---|
| `GENERAL_RATE_LIMIT_PER_MIN` | 30 | admin + storefront resolver-level checks |
| `AUTH_RATE_LIMIT_PER_MIN` | 10 | login / register / password reset |
| `FRONT_RATE_LIMIT_PER_MIN` | 600 | the whole storefront GraphQL endpoint |
| `MEDIA_TRANSFORM_RATE_LIMIT_PER_MIN` | 120 | `/media/t/*` |
| `TOKEN_RATE_LIMIT_PER_MIN` | 60 | download, unsubscribe, signup confirmation, email open/click |
| `INGEST_RATE_LIMIT_PER_MIN` | 300 | `/t/*` beacons, per endpoint |
| `SIGNUP_CONFIRM_RATE_LIMIT_PER_HOUR` | 3 | signup-form confirmation emails, per address (an hourly budget, unlike the rest) |
| `PAYMENT_METHOD_SAVE_RATE_LIMIT_PER_HOUR` | 5 | saved-card attempts per customer, failures included (an hourly budget) - each is a gateway vault call, the card-testing surface |

Setting one of these does NOT turn the layer on - `RATE_LIMIT_ENABLED`
gates all of them, so a knob left over in an old env file can't quietly
start refusing traffic.

The lockout is separate, in case you want brute-force protection without
per-IP budgets: `LOGIN_LOCKOUT_ENABLED` (defaults to whatever
`RATE_LIMIT_ENABLED` is) and `LOGIN_LOCKOUT_THRESHOLD` (10 failures, then a
15 minute lock).

Two limits are per-feature and stay off by default on their own terms,
independent of all of the above:

- **Catalog query throttle** - Settings > General > Security, a per-IP soft
  budget for storefront catalog reads. Past the budget requests are
  *slowed*, past 3x refused. Meant to be flipped on mid-attack with no
  deploy, which is why `RATE_LIMIT_ENABLED` doesn't gate it. 0 = off.
- **Per-plugin hook budget** - `plugin/<slug>/rate_limit_per_min` in
  Settings, 0 = unlimited.

## Feature switches

All default to true.

| Var | Off means |
|---|---|
| `SIGNALS_ENABLED` | no Signals suite (Pulse campaigns/flows/the email builder, Lens, Synapse, Prism, spine ingestion, forwarding). Transactional email keeps sending from core's own templates. This is core's switch: it decides whether core dials the suite at all. The Signals processes have none of their own, so a deployed suite is on and cannot quietly drop writes while core keeps sending them. |
| `AI_ENABLED` | no LLM integrations, including the Settings > AI section |

`GEOIP_DB_PATH` points at a MaxMind GeoLite2 `.mmdb` for geo on web
events. A bad path is logged, not fatal - geo just doesn't appear.

`REQUEST_MAX_BODY_BYTES` defaults to 8 MB.

## Image tags

`APP_IMAGE`, `DASHBOARD_IMAGE` and `VETRINA_DEMO_IMAGE` are set in the
SHELL, not in an env file - compose interpolates them from the process
env. `./skelet release` prints the values after a push.

---

Adding or renaming a variable? Update this file too, and keep the
`.example` line to a name, a default and at most one line of hint.

# Environment variables

**You should not need to read this to get started.** `bin/setup` asks what you
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
| `go.env.example` | `go.env` | the backend in dev (`app` service) |
| `db.env.example` | `db.env` | the dev Postgres container |
| `prod.env.example` | `prod.env` | `app`, `worker`, `media`, `signals`, `ingest`, `migrate` - one shared file |
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

`prod.env` is shared by every backend role, which is why role-specific
settings belong in the compose service and not in here - the app has to
stay a server. That's `SKELET_ROLE` (`worker`, default `server`) and
`WORKER_PORT` (8081, its `/healthz`).

Prod boot fails while a REQUIRED var is blank. That's deliberate - a
silent default in production is worse than a refusal to start.

## Required to boot

| Var | Notes |
|---|---|
| `JWT_SECRET` | `openssl rand -hex 32`, in the root `.env`. Compose hands it to the backend and the storefront, so it's written once. The default value is rejected in production. |
| `CONFIG_ENCRYPTION_KEY` | `openssl rand -base64 32`, in the root `.env`. Keep it stable - rotating it orphans every stored secret. |
| `CORS_ALLOWED_ORIGINS` | Storefront + dashboard origins, comma-separated. Empty in dev means localhost defaults. |
| `DB_PASSWORD` | Commerce Postgres. `DATABASE_URL` derives from it, or set that directly for an external database. |
| `SIGNALS_DB_PASSWORD` | Signals Postgres, same deal via `SIGNALS_DB_URL`. Required even with Signals switched off - see below. |

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

`SIGNALS_DB_URL` holds the event spine, Lens and identities. The commerce
schema carries none of those tables, so there is no falling back to
`DATABASE_URL`.

**It's required even with `SIGNALS_ENABLED=false`.** The pool opens either
way, and retention still has to reach the spine so old data ages out with
the suite off. Turning Signals off saves the ingest and job load, not the
database.

It does not need a separate server. A second database on the commerce
Postgres instance works exactly the same, which is the usual answer for
small deployments - a `CREATE DATABASE`, not another container.

`SIGNALS_GRPC_ADDR` is where `cmd/signals` serves the wire contract core calls
(`signals:8092` in compose). It is core's only address for Signals, and leaving
it unset means core makes no calls at all.

`SIGNALS_REPLICA_URL` is the Signals read pool, bulkheaded off the ingest
writes. Unset means the Signals primary. It is NOT `DATABASE_REPLICA_URL`
- that's the commerce replica and doesn't hold spine tables.

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
- **Backups bypass it.** `pg_dump` needs a session its snapshot and `SET`s
  can outlive. The app reads `DATABASE_DIRECT_URL` /
  `SIGNALS_DIRECT_URL` for that, and falls back to the pooled URL when
  they're unset - which is correct when there's no pooler at all.
- **Server-side prepared statements are off.** The pooled DSNs carry
  `default_query_exec_mode=exec`, because a `PREPARE` lands on whichever
  server connection is free and the next `EXECUTE` may not find it.
  pgbouncer 1.21+ can track them itself via `max_prepared_statements`;
  set that and the parameter can go.

Setting `DATABASE_URL` or `SIGNALS_DB_URL` outright still wins over all of
it, which is how you point at a managed datastore - the pooler then has
nothing to do and you can drop its two services.

Pool sizes: `SIGNALS_DB_MAX_CONNS` (8) and `SIGNALS_READ_DB_MAX_CONNS` (8)
in the app and worker; the ingest tier has its own pair,
`INGEST_SIGNALS_MAX_CONNS` (16) and `INGEST_PRIMARY_MAX_CONNS` (4),
because beacons write to the spine inside the request handler and it
holds a connection per concurrent write.

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

`VALKEY_CACHE_URL` is OPTIONAL and points the catalog/CMS read caches at a
different instance from the one doing rate limits and pub/sub. Leave it empty
and they share `VALKEY_URL`. Worth splitting when cache eviction pressure
starts costing you rate-limit counters.

### ClickHouse (optional spine engine)

Unset (or `postgres`) keeps the spine on the Signals Postgres. Set
`SIGNALS_ENGINE=clickhouse` and point `SIGNALS_CH_URL` at a server for the
columnar engine on high-volume deployments.

In dev you don't pass `--profile clickhouse` yourself: `bin/_lib` turns that
profile on whenever the root `.env` sets `SIGNALS_ENGINE=clickhouse`, so the
server and its schema come up with `bin/dev` and survive `--lean` (the spine
needs them, they aren't sidecars). `bin/doctor` fails if the engine is set to
clickhouse and either the container or `signals.analytics_events` is missing.
`analytics_events` and
`analytics_identities` move there and the Lens heatmap grid derives from
raw events, while sessions, snapshots and rollups stay on Postgres - so
it's additive, not a replacement.

Existing Postgres spine history migrates with `bin/signals-ch-backfill`,
which is safe to re-run.

There are two URLs because the app driver and golang-migrate disagree on
DSN shape: `SIGNALS_CH_URL` and `SIGNALS_CH_MIGRATE_URL`.

Set `CLICKHOUSE_PASSWORD` in the root `.env`; the server credentials and
both ClickHouse URLs derive from it.

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

In dev, `bin/dev` points all of it at the Garage sidecar, so leave it
blank in `go.env`.

Self-hosting object storage on the same box is fine for small single-server
deployments, but its volume is NOT in `pg_dump` - back it up separately or
a disk loss takes every product image with it.

`MEDIA_LIBRARY_ROOT_PREFIX` is OPTIONAL and scopes the media tier's library
lookups to one key prefix, for a bucket shared with something else. Empty
means the whole bucket.

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

- `BACKUP_S3_BUCKET` - setting it enables the nightly `db-backup` job (and
  `skelet db backup` / `db backups` / `db restore <key> --yes`). Use a
  DEDICATED bucket and credential, not the media ones.
- `BACKUP_S3_ENDPOINT_URL`, `BACKUP_S3_REGION`, `BACKUP_S3_PATH_STYLE`,
  `BACKUP_S3_ACCESS_KEY_ID`, `BACKUP_S3_SECRET_ACCESS_KEY` - same shape
  rules as the media store above.
- `BACKUP_ENCRYPTION_PASSPHRASE` - encrypts every dump with age before it
  leaves the process. Strongly recommended: dumps carry all customer PII.
  Keep the passphrase somewhere durable OFF the host - losing it makes
  every backup unreadable. Prod boots warn when backups run unencrypted.
- `BACKUP_S3_PREFIX` (default `db/`) and `BACKUP_KEEP` (default 30, per
  database - commerce and signals dumps are pruned as separate families).
- `BACKUP_HEARTBEAT_URL` - pinged (GET, 10s timeout) only after a fully
  successful run, dumps uploaded and retention pruned. Point it at
  healthchecks.io or any dead-man's-switch monitor. This is not redundant
  with the scheduler's failure alerting: that fires when the job *fails*,
  so the one thing it cannot report is the job never running - the app
  being down, or nothing ever claiming it. A monitor that alerts when
  pings stop covers exactly that hole. A failed ping is logged as a
  warning and never fails the backup, since the dump is already uploaded
  by then. Restore procedure and the post-restore checklist:
  [docs/backups.md](backups.md).

This destination doubles as the analytics retention archive: with the
`pulse/analytics/retention_action` setting on `archive`, the daily retention
jobs export aged spine events and send-log months as identity-stripped
`.jsonl.gz` files under `<prefix>archive/` before deleting the rows (the
dump pruner never touches them, and with no `BACKUP_S3_BUCKET` the sweep
fails closed - it deletes nothing rather than archiving nowhere). Set the
`BACKUP_*` vars on the cmd/signals worker too when it owns retention via
`SIGNALS_JOBS_EXTERNAL`.

The app credential can delete objects (retention pruning), so it cannot
protect backups from a compromised host - pair it with bucket versioning
or object lock if you need that.

At 100GB-class sizes: raise the db-backup job timeout in the scheduler UI
(a big dump outgrows the 30m default), restore with `--jobs N` for parallel
pg_restore, and set `TMPDIR` to a mounted volume - restore spools the dump
to a temp file first. Past that scale, run physical/PITR backups
(pgBackRest, WAL-G, or managed snapshots) as the primary and treat these
dumps as the portable weekly export.

## Email and SMS

`SMTP_HOST` / `SMTP_PORT` / `SMTP_USERNAME` / `SMTP_PASSWORD`. In dev,
`bin/dev` points these at Mailpit; an unset `SMTP_HOST` prints emails to
stdout instead.

`EMAIL_WEBHOOK_TOKEN` is the bearer token for the ESP bounce webhook at
`POST /webhooks/email/events`. Unset means that route 404s.

Twilio (`TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `TWILIO_FROM`) backs the
Pulse `send_sms` node. From is E.164.

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

`STOREFRONT_REVALIDATE_URL` + `STOREFRONT_REVALIDATE_SECRET` wire CMS
publish to a storefront cache bust. The secret matches the storefront's
own file.

On the storefront side: `STOREFRONT_BASE_URL` drives canonical URLs,
sitemap and JSON-LD. `STOREFRONT_CHANNEL_ID` is the channel uuid this
deployment serves (admin > channels shows it); leave it empty to resolve
by Host against the channel's public URL, or fall back to the backend's
default channel. `REVIEW_VOTER_TOKEN_SECRET` signs review-voter tokens and
voting 500s without it.

## Tiers you can split out

All of these are the same binary or an additive service. Everything works
in one process; splitting buys isolation.

| Tier | Switch | What it does |
|---|---|---|
| `worker` | `JOBS_EXTERNAL=true` | App stops claiming scheduler jobs, webhook deliveries and outbox rows, leaving them to the worker. |
| `signals` | `SIGNALS_JOBS_EXTERNAL=true` | App defers Signals retention, spine forwarding and trait compute. The same process serves the suite's own admin API on `:8090` and its wire contract on `:8092`. |
| `ingest` | route `/t/*` | Serves the public tracker surface on `127.0.0.1:8091`. |
| `media` | route `/media/t/*` | Image resize + focal crop, so decode CPU doesn't land next to checkout. |

Both `JOBS_EXTERNAL` and `SIGNALS_JOBS_EXTERNAL` are read by the app AND
the worker, so one setting configures both - the worker ignores the flag
and always runs its layer. It's all DB-claim based, so leaving the flag
false while a worker runs is safe (both claim, work is partitioned), it
just doesn't isolate anything. Setting it true with NO worker running
stops that background work entirely.

For the signals worker specifically: set it whenever the worker is
deployed. The forwarding cursor is guarded against co-running, but two
sweeps still each post the same batch once.

`INGEST_FALLBACK_URL` points the ingest tier back at the app for the two
routes it can't serve alone - form submissions and back-in-stock signups,
which need the app's subscriber graph. Leave it unset and those two paths 404,
which the browser SDKs fail soft on; everything else on `/t/*` is unaffected.

Two parity notes when `/t` routes to the ingest tier:

- `GEOIP_DB_PATH` is shared through `prod.env`, but the `.mmdb` file has
  to be mounted into the ingest container too, or country attribution
  silently stops for beacon traffic.
- `TRUSTED_PROXIES` must cover the ingest containers (the default
  `172.16.0.0/12` does). Otherwise the app sees them as the client on the
  proxied forms and back-in-stock paths, and its per-IP rate limits
  collapse into one bucket.

The app keeps serving `/t/*` and `/media/t/*` regardless, so both are a
routing choice rather than a migration.

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
GONE. If you need searchable history, point a collector (Vector, Promtail,
Fluent Bit) at the container log files or swap the logging driver.

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
| `TOKEN_RATE_LIMIT_PER_MIN` | 60 | download, unsubscribe, email open/click |
| `INGEST_RATE_LIMIT_PER_MIN` | 300 | `/t/*` beacons, per endpoint |

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
| `SIGNALS_ENABLED` | no Signals suite (Pulse campaigns/flows, Lens, Synapse, Prism, spine ingestion, forwarding). Transactional email keeps sending, and still needs `SIGNALS_DB_URL` to record it. |
| `AI_ENABLED` | no LLM integrations, including the Settings > AI section |

`GEOIP_DB_PATH` points at a MaxMind GeoLite2 `.mmdb` for geo on web
events. A bad path is logged, not fatal - geo just doesn't appear.

`REQUEST_MAX_BODY_BYTES` defaults to 8 MB.

## Image tags

`APP_IMAGE`, `DASHBOARD_IMAGE` and `VETRINA_DEMO_IMAGE` are set in the
SHELL, not in an env file - compose interpolates them from the process
env. `bin/push-images` prints the values after a push.

---

Adding or renaming a variable? Update this file too, and keep the
`.example` line to a name, a default and at most one line of hint.

# Architecture

What Skelet is made of: where the code lives, what it runs on, how the GraphQL
surfaces and the Signals wire contract are laid out, and how the domain packages
are shaped.

## Project Layout

```
skelet/
├── proto/skelet/signals/v1/         ← the core-to-Signals wire contract
├── apps/
│   ├── backend/                     ← everything server-side: three Go modules side by side
│   │   ├── Dockerfile               ← the shared dev image
│   │   ├── core/                    ← core - go.mod github.com/skeletg/skelet/apps/backend/core
│   │   │   ├── cmd/                 ← one module, several binaries
│   │   │   │   ├── server/          ← the HTTP server
│   │   │   │   ├── cli/             ← the `skelet` CLI (Cobra)
│   │   │   │   ├── media/           ← the image tier you can split out
│   │   │   │   └── docsref/, plugingen/, …    ← codegen and check tools
│   │   │   ├── graph/
│   │   │   │   ├── schema/admin/*.graphqls   ← admin surface
│   │   │   │   ├── schema/front/*.graphqls   ← storefront surface
│   │   │   │   ├── admin/                    ← admin gqlgen output + resolvers
│   │   │   │   │   ├── generated.go          ← DO NOT EDIT
│   │   │   │   │   ├── model/models_gen.go   ← DO NOT EDIT
│   │   │   │   │   ├── helpers.go            ← shared admin mappers
│   │   │   │   │   ├── *_helpers.go          ← per-domain helpers
│   │   │   │   │   └── *.resolvers.go        ← edit resolver bodies here
│   │   │   │   └── front/                    ← storefront gqlgen output + resolvers
│   │   │   ├── gqlgen.admin.yml     ← admin pipeline config
│   │   │   ├── gqlgen.front.yml     ← storefront pipeline config
│   │   │   ├── internal/            ← ~110 domain packages, see the table below
│   │   │   │   ├── db/
│   │   │   │   │   ├── migrations/  ← the generated baselines - see database.md
│   │   │   │   │   ├── query/*.sql  ← sqlc input
│   │   │   │   │   └── sqlc/        ← sqlc output (DO NOT EDIT)
│   │   │   │   └── signalsclient/   ← core's plug into the suite, over the wire contract
│   │   │   ├── lib/                 ← the infrastructure the Signals module builds on - see below
│   │   │   │   ├── apperr/, auth/, config/, applog/, envcheck/, httpx/, observability/,
│   │   │   │   │   ratelimit/, testdb/
│   │   │   │   └── txrunner/, cache/, secrets/, urlguard/, otelx/, dbconn/, objstore/, email/,
│   │   │   │       emailtrack/, consentkind/
│   │   │   └── signalsapi/          ← the wire contract's vocabulary, its generated code under v1/
│   │   ├── sdk/                     ← the plugin SDK - go.mod github.com/skeletg/skelet/apps/backend/sdk
│   │   │   └── sdktest/, httpx/, api.json
│   │   └── signals/                 ← the Signals suite - go.mod github.com/skeletg/skelet/apps/backend/signals
│   │       ├── sqlc.yaml            ← the suite's queries against its own schema
│   │       ├── cmd/
│   │       │   ├── signals/         ← the contract server, the beacon surface, the scheduled upkeep
│   │       │   └── ingest/          ← the public tier alone, scaled out
│   │       └── internal/
│   │           ├── boot/            ← the suite's composition root
│   │           ├── analytics/       ← the event spine, its engines, traits, rosters
│   │           ├── geo/, uaclass/, archivefile/  ← IP country, device class, retention exports
│   │           ├── lens/, segment/, experiment/, destination/, behavioralsegment/
│   │           ├── sendlog/, suppression/  ← the email send log and the suppression list
│   │           ├── emailtemplate/, flow/, campaign/  ← Pulse: the email builder, the flow engine, broadcasts
│   │           ├── engagement/, sms/, corefront/     ← the send tracker, the text gateway, core's storefront API
│   │           ├── ingesthttp/, pulsehttp/  ← /t and /e
│   │           ├── signalsgrpc/     ← the wire contract's handlers
│   │           ├── signalsboot/     ← spine + queue + engine assembly
│   │           └── db/              ← migrations/, query/*.sql and sqlc/ (DO NOT EDIT)
│   ├── dashboard/                   ← React + Vite admin UI
│   │   ├── src/
│   │   │   ├── components/          ← shared UI primitives (shadcn)
│   │   │   ├── features/<domain>/
│   │   │   │   ├── *.tsx            ← pages, dialogs, list/detail
│   │   │   │   ├── *.graphql        ← operation docs (codegen input)
│   │   │   │   └── *.gql.ts         ← codegen output (mostly - some hand-written)
│   │   │   ├── graphql/schema.types.ts ← codegen baseline (DO NOT EDIT)
│   │   │   └── lib/
│   │   │       ├── apollo.ts        ← client + auth link + silent refresh
│   │   │       └── config-scope.tsx ← header-driven scope context
│   │   ├── codegen.ts               ← graphql-codegen; reads live backend schema
│   │   ├── vite.config.ts           ← dev proxy /api → :8080
│   │   └── package.json
│   └── channels/vetrina-demo/       ← Next.js 16 (App Router, RSC) demo storefront
│       ├── app/
│       │   ├── [locale]/            ← every page lives under the locale segment
│       │   │   ├── layout.tsx       ← the root layout - reads no cookie, so routes
│       │   │   │                        stay static until their own fetches say otherwise
│       │   │   ├── page.tsx         ← home
│       │   │   ├── [...slug]/       ← URL-resolver catch-all (CMS pages, products, …)
│       │   │   ├── account/         ← login, register, profile
│       │   │   └── blog/, cart/, checkout/, categories/, collections/, products/, search/, …
│       │   ├── sitemap.ts, robots.txt/, products/, categories/, collections/
│       │   │                        ← feeds, OUTSIDE [locale]: the proxy's matcher skips
│       │   │                          dotted paths, so they never get the locale rewrite
│       │   ├── api/                 ← route handlers (e.g. hud-debug)
│       │   ├── components/          ← server + client components, no vendored UI kit
│       │   ├── lib/
│       │   │   ├── graphql.ts       ← server-side gqlFetch (BACKEND_INTERNAL_URL)
│       │   │   ├── auth.ts          ← JWT cookie handling, getAccessToken
│       │   │   ├── locale/, channel/  ← active channel/locale resolution
│       │   │   ├── *.graphql        ← operation docs (codegen input)
│       │   │   └── *.gql.ts         ← codegen output
│       │   └── graphql/schema.types.ts ← codegen baseline (DO NOT EDIT)
│       ├── codegen.ts               ← graphql-codegen; hits /api/front/graphql
│       ├── next.config.ts
│       └── package.json
├── bin/                             ← all dev operations
├── env/                             ← *.env.example committed; *.env gitignored (docs/env.md documents every var)
├── .gitattributes                   ← LF normalization, prevents Windows CRLF
├── .gitignore                       ← excludes air binaries + dashboard build
├── docker-compose.yml               ← the dev stack
└── docker-compose.prod.yml          ← the prod stack
```

### What's in `internal/`

Around 110 packages, so here is the shape rather than the inventory - `ls
apps/backend/core/internal` is the list, and each package's `domain.go` is its
contract. Everything below is `apps/backend/core/internal/<name>`.

| Area | Packages |
|---|---|
| Selling | `catalog`, `pricing`, `promotion`, `cart`, `order`, `ordercapture`, `payment`, `paymentvault`, `shipping`, `tax`, `inventory`, `subscription`, `giftcard`, `storecredit`, `download`, `sales` |
| Customers | `customer`, `address`, `loyalty`, `referral`, `review`, `qna`, `wishlist`, `customerio` |
| Content | `cms`, `blog`, `blockkit`, `content`, `menu`, `form`, `webform`, `medialibrary`, `documents` |
| Merchandising | `search`, `searchmerch`, `urlrewrite`, `sitecrawl`, `linkgraph` |
| Marketing | `backinstock`, `ordermail`, `mailsend` - core's own transactional mail; campaigns, flows and the email builder are the suite's, and core hands them the moments a flow can start on over the contract |
| Signals (core's side) | `signalsclient` - the wire contract's client; the suite itself is `apps/backend/signals`, see below |
| Platform | `admin`, `authstore`, `permission`, `channel`, `country`, `currency`, `locale`, `consent`, `integration`, `audit`, `activity`, `notification`, `adminprefs`, `ai`, `chat` |
| Extension | `plugin`, `pluginset`, `pluginmigrate`, `remoteprovider`, `action` |
| Data movement | `dataio`, `datajob`, `datapatch`, `catalogio` |
| Infrastructure | `db`, `events`, `outbox`, `webhook`, `scheduler`, `media`, `mediahttp`, `configstore`, `cacheinvalidate`, `revalidate`, `circuit`, `graphqlx`, `health`, `wiring` |

`wiring` is the composition root: every service is constructed there, so it is
the fastest way to see what depends on what.

### What sits beside `internal/`

Go only lets `apps/backend/core/internal/*` be imported from within `apps/backend/`, so
anything a second module reaches has to sit outside it. Three kinds do, and the
layout keeps them apart because they carry different promises:

- `sdk/` is what plugin authors build against, its own module with a surface gate
  (`sdk/api.json`) and a version. A plugin that adds GraphQL resolvers also imports
  core's gqlgen model packages, which is the one other door.
- `signalsapi/` is the wire contract: the vocabulary both modules share, with the
  code generated from `proto/` under `signalsapi/v1/`, checked for breaking changes
  against `main` - the deployed core that a new Signals has to keep answering.
  `bin/contract-check` runs it in CI against `origin/main` and fails when that ref
  is missing, rather than passing with nothing to compare. Until `main` carries the
  proto, it compares against `develop` instead.
- `lib/` is the infrastructure the Signals module builds on: `apperr`, `auth`,
  `config`, `applog`, `envcheck`, `httpx`, `observability` (Sentry and the telemetry
  facade), `ratelimit`, `testdb`, `txrunner`, `cache`, `secrets`, `urlguard`, `otelx`,
  `dbconn` for opening a pool, `objstore` for the S3 client, `email` for the SMTP
  sender and the order snapshot a flow renders from, `emailtrack` for the tracking and
  unsubscribe tokens the suite signs and core verifies, `consentkind` for the consent
  kinds both sides name. Public to the compiler, internal in intent: it
  is the suite's slice of core, not an API for plugins or anyone else, and it is
  plumbing with no commerce knowledge.

`lib/` stays a bottom layer by rule: a package there may not import `internal/`,
and `depguard` refuses it with no exceptions. What needed the domain moved out or
turned into a seam: core's settings store is `internal/configstore` behind the
`config.Repository` interface, core's token and refresh persistence and the admin
session log are `internal/authstore` behind `auth.TokenBackend`, `auth.RefreshBackend`
and `auth.SessionLog`, the customer guards that read the resolved store live in
`internal/channel`, the admin Tools registrations for config and the cache live in
`wiring`. Only what both processes use lives in `lib/`: `graphqlx` is gqlgen wiring
core alone serves, so it stays in `internal/`, and what only the suite uses - `geo`,
`uaclass`, the `archivefile` writer - lives in the suite.
Plugins are denied all of `lib/` by the same linter, since the blanket `internal/*`
rule does not reach it, and the suite pins its own doors into core - `lib/`, `sdk/`
and the contract - with a test.

### The Signals module

`apps/backend/signals` is the suite as its own Go module. Its `go.mod` requires core's by
relative path (`replace ../backend`), and the dependency only ever points that
way: core reaches the suite over the wire contract through `signalsclient`, and
the compiler refuses the reverse, since nothing under `apps/backend/core/internal/`
is importable from another module. What the suite needs *of* core is pushed
across or asked for over the contract - channels, customers, settings. What it
needs *from* core's code is the public slice above.

The signals database is the suite's alone. Its migration stream lives in
`apps/backend/signals/internal/db/migrations` beside the queries against it, and core opens
no pool on it: Pulse - campaigns, flows and the email builder - runs in the suite,
answering the dashboard through core's relaying resolvers, sending through the same
SMTP relay, and reading who a customer is and what shop a channel is off the rosters
core pushes. Core raises the moments a flow can start on (an order placed, a
password reset, a product back in stock) through `Enroll`, and sends its own
transactional mail when no flow takes one. Each moment is keyed by what it
happened to, so a retried or replayed call starts one run and answers the same
count, and the customer and consent events that keep the roster current ride
core's outbox, so a suite that was down gets them on the relay's next pass. Core
signs every call with `SIGNALS_SERVICE_SECRET`, a secret the storefront never
holds, and every call carries a deadline. The one thing the suite still asks core
for is live catalog data for the builder's products block, over core's public
storefront API. `internal/boot` is the suite's composition root, the way `wiring`
is core's.

All three apps run in containers via `./skelet up` (Go + Postgres + Valkey
+ Garage + Mailpit + Vite + Next.js). The dashboard container's Vite
dev server proxies `/api/*` to the in-network backend
(`API_PROXY=http://app:8080`); host `apps/dashboard/node_modules` is
still installed for host-side tooling (`./skelet test`'s tsc, ESLint, the
IDE). Codegen flows backend → frontends: editing `*.graphqls`
or any `*.graphql` doc requires `./skelet gen` so dashboard TS bindings
stay in sync; the storefront has its own codegen step
(`./skelet npm <name> run codegen`).

## Stack

- **Backend**: Go + gqlgen (schema-first GraphQL) + Postgres + sqlc.
  Never write raw `db.QueryRow` calls - sqlc generates everything.
- **Backend infra**: pgx/v5, Valkey via go-redis, Cobra,
  air (hot reload).
- **Dashboard**: React + Vite + Apollo Client + Tailwind + shadcn/ui +
  Lexical (rich text) + DnD Kit.
- **Storefront**: Next.js 16 (App Router, RSC, React 19) + Tailwind v4 +
  `lucide-react`, and nothing else. No component kit is vendored - the
  primitives are hand-written against the design tokens in `app/globals.css`,
  and interaction that would normally pull a library in (carousels, drawers,
  the search modal) is CSS scroll-snap and `<dialog>`. No Apollo either: a
  thin server-side `gqlFetch` in `app/lib/graphql.ts` POSTs to
  `/api/front/graphql`, `cache: "no-store"` by default and `force-cache` with
  `next: { tags, revalidate }` when the caller opts into ISR. **Caveat:** Next.js 16 has breaking changes vs.
  the version in your training data - read
  `apps/channels/vetrina-demo/node_modules/next/dist/docs/` before writing code,
  and heed the deprecation notices.

### Datastores and sidecars

| Layer | What |
|---|---|
| Commerce DB | Postgres + sqlc |
| Analytics DB | A second Postgres (`signals-db`) for the suite, ClickHouse for the event spine |
| Refresh tokens, cache | Valkey (Redis-compatible) |
| Async / durable delivery | Transactional outbox (Postgres); optional NATS JetStream queue for beacon ingest |
| Search | OpenSearch (optional sidecar - the app runs without it) |
| Object storage | Amazon S3 (managed) or Garage (self-hosted/on-prem); Garage in dev |
| Email (dev) | Mailpit (SMTP catcher) |
| Observability | OpenTelemetry + Sentry, both opt-in; logs are structured JSON on stdout for whatever collector the deployment runs |
| Hot reload | air (backend), Vite (dashboard) |

## GraphQL surface

Two physically separate endpoints, each with its own schema, generated
executable, and Resolver root. They share the `internal/*` services but
not the GraphQL layer.

| Endpoint | Schema | Resolver |
|---|---|---|
| `POST /api/admin/graphql` | `apps/backend/core/graph/schema/admin/*.graphqls` | `apps/backend/core/graph/admin/*.resolvers.go` |
| `POST /api/front/graphql` | `apps/backend/core/graph/schema/front/*.graphqls` | `apps/backend/core/graph/front/*.resolvers.go` |

Storefront introspection cannot see admin operations. The dashboard at
`apps/dashboard/` talks to admin only; the storefront at
`apps/channels/vetrina-demo/` talks to front only.

Dev playgrounds: `/playground/admin` and `/playground/front`
(`APP_ENV=development` only).

JWT: `Authorization: Bearer <token>`, access TTL 15m, refresh TTL 30d
(hashed in Valkey under `refresh:<prefix>:<sha256>`). The `type` claim
is `"admin"` or `"customer"` - middleware injects the right claims
struct into ctx.

## Domain pattern

`domain.go` (types + Repository interface) → `repository.go` (postgres
impl) → `service.go` (business logic).

**Transactions**: use `tx.Manager` (a public package, since the Signals module opens transactions through it too). Repos expose
`WithTx(*sql.Tx) Repository`. Inside a `txMgr.InTransaction(ctx, fn)` closure,
**always** shadow the outer repo with the tx-bound one - a typo reaching
`s.repo` silently drops atomicity.

**Registry pattern** (config, permission, consent, action): domain
packages register definitions in `init()` (or via explicit `RegisterX()`
when handlers close over runtime services). Server calls `Validate()` +
`Sync()` at boot. Fail-fast on duplicates or unknown slugs.

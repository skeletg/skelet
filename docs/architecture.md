# Architecture

What Skelet is made of: where the code lives, what it runs on, and how the
two GraphQL surfaces and the domain packages are shaped.

## Project Layout

```
skelet/
├── apps/
│   ├── backend/                     ← Go (go.mod lives here)
│   │   ├── cmd/                     ← one module, several binaries
│   │   │   ├── server/              ← the HTTP server
│   │   │   ├── cli/                 ← the `skelet` CLI (Cobra)
│   │   │   ├── signals/, ingest/, media/  ← the tiers you can split out
│   │   │   └── docsref/, plugingen/, …    ← codegen and check tools
│   │   ├── graph/
│   │   │   ├── schema/admin/*.graphqls   ← admin surface
│   │   │   ├── schema/front/*.graphqls   ← storefront surface
│   │   │   ├── admin/                    ← admin gqlgen output + resolvers
│   │   │   │   ├── generated.go          ← DO NOT EDIT
│   │   │   │   ├── model/models_gen.go   ← DO NOT EDIT
│   │   │   │   ├── helpers.go            ← shared admin mappers
│   │   │   │   ├── *_helpers.go          ← per-domain helpers
│   │   │   │   └── *.resolvers.go        ← edit resolver bodies here
│   │   │   └── front/                    ← storefront gqlgen output + resolvers
│   │   ├── gqlgen.admin.yml         ← admin pipeline config
│   │   ├── gqlgen.front.yml         ← storefront pipeline config
│   │       └── internal/            ← ~110 domain packages, see the table below
│   │           └── db/
│   │               ├── migrations/  ← the generated baselines - see database.md
│   │               ├── query/*.sql  ← sqlc input
│   │               └── sqlc/        ← sqlc output (DO NOT EDIT)
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
apps/backend/internal` is the list, and each package's `domain.go` is its
contract. Everything below is `apps/backend/internal/<name>`.

| Area | Packages |
|---|---|
| Selling | `catalog`, `pricing`, `promotion`, `cart`, `order`, `payment`, `shipping`, `tax`, `inventory`, `subscription`, `giftcard`, `storecredit` |
| Customers | `customer`, `address`, `loyalty`, `referral`, `review`, `qna`, `wishlist`, `segment` |
| Content | `cms`, `blog`, `blockkit`, `content`, `menu`, `form`, `webform`, `medialibrary`, `documents` |
| Merchandising | `search`, `searchmerch`, `urlrewrite`, `sitecrawl`, `linkgraph` |
| Marketing | `campaign`, `flow`, `emailtemplate`, `emailtrack`, `suppression`, `backinstock`, `experiment` |
| Signals | `analytics`, `lens`, `signalsboot`, `ingesthttp`, `destination` |
| Platform | `admin`, `auth`, `permission`, `channel`, `config`, `consent`, `integration`, `audit`, `activity`, `notification`, `adminprefs`, `ai`, `chat` |
| Extension | `plugin`, `pluginset`, `pluginmigrate`, `remoteprovider`, `action` |
| Data movement | `dataio`, `datajob`, `datapatch`, `catalogio`, `backup` |
| Infrastructure | `db`, `tx`, `events`, `outbox`, `webhook`, `scheduler`, `cache`, `ratelimit`, `secrets`, `media`, `email`, `sms`, `geo`, `httpx`, `graphqlx`, `apperr`, `applog`, `observability`, `health`, `wiring` |

`wiring` is the composition root: every service is constructed there, so it is
the fastest way to see what depends on what.

All three apps run in containers via `bin/dev` (Go + Postgres + Valkey
+ Garage + Mailpit + Vite + Next.js). The dashboard container's Vite
dev server proxies `/api/*` to the in-network backend
(`API_PROXY=http://app:8080`); host `apps/dashboard/node_modules` is
still installed for host-side tooling (`bin/test`'s tsc, ESLint, the
IDE). Codegen flows backend → frontends: editing `*.graphqls`
or any `*.graphql` doc requires `bin/codegen` so dashboard TS bindings
stay in sync; the storefront has its own codegen step
(`bin/channel run codegen`).

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
  and heed the deprecation notices (see
  `apps/channels/vetrina-demo/AGENTS.md`).

### Datastores and sidecars

| Layer | What |
|---|---|
| Commerce DB | Postgres + sqlc |
| Analytics DB | A second Postgres (`signals-db`); the spine engine is pluggable, ClickHouse optional |
| Refresh tokens, cache | Valkey (Redis-compatible) |
| Async / durable delivery | Transactional outbox (Postgres); optional NATS JetStream queue for beacon ingest |
| Search | OpenSearch (optional sidecar - the app runs without it) |
| Object storage | Amazon S3 (managed) or Garage (self-hosted/on-prem); Garage in dev |
| Email (dev) | Mailpit (SMTP catcher) |
| Observability | OpenTelemetry + Sentry, both opt-in; Vector log shipping to ClickHouse (`logs` profile) |
| Hot reload | air (backend), Vite (dashboard) |

## GraphQL surface

Two physically separate endpoints, each with its own schema, generated
executable, and Resolver root. They share the `internal/*` services but
not the GraphQL layer.

| Endpoint | Schema | Resolver |
|---|---|---|
| `POST /api/admin/graphql` | `apps/backend/graph/schema/admin/*.graphqls` | `apps/backend/graph/admin/*.resolvers.go` |
| `POST /api/front/graphql` | `apps/backend/graph/schema/front/*.graphqls` | `apps/backend/graph/front/*.resolvers.go` |

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

**Transactions**: use `internal/tx.Manager`. Repos expose
`WithTx(*sql.Tx) Repository`. Inside a `txMgr.InTransaction(ctx, fn)` closure,
**always** shadow the outer repo with the tx-bound one - a typo reaching
`s.repo` silently drops atomicity.

**Registry pattern** (config, permission, consent, action): domain
packages register definitions in `init()` (or via explicit `RegisterX()`
when handlers close over runtime services). Server calls `Validate()` +
`Sync()` at boot. Fail-fast on duplicates or unknown slugs.

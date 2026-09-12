# Skelet - agent guide

Orientation for AI agents working in this repo. Read this first, then
[docs/concepts.md](docs/concepts.md) for the vocabulary and
[docs/README.md](docs/README.md) for everything else. This file is a map and a list of
things that are easy to get wrong - it does not repeat the docs.

## 1. What this is

A self-hosted ecommerce platform in one repo: a Go GraphQL backend, a React admin
dashboard, and one or more Next.js storefronts. Everything runs in Docker.

| Path | What |
|---|---|
| `apps/backend/` | Go, `go.mod` lives here. gqlgen + sqlc. Three GraphQL surfaces, plus the Signals wire contract |
| `apps/dashboard/` | React 19 + Vite admin SPA, strict TypeScript |
| `apps/channels/<name>/` | Next.js 16 storefronts, App Router, RSC |
| `bin/` | Every dev operation, one script each |
| `./skelet` | Front door - routes verbs onto `bin/*` |
| `docs/` | One file per question, indexed in `docs/README.md` |

Full tree: [docs/architecture.md](docs/architecture.md). What each root directory is
for: [docs/layout.md](docs/layout.md).

## 2. Commands

**Use `./skelet` or `bin/*`, not raw `docker compose` / `npm` / `go`.** The scripts
resolve the right container, the right database URL and the right node_modules.
`./skelet` with no arguments lists every verb; `./skelet help <verb>` explains one.

```bash
./skelet setup            # first run: picks apps, writes env, migrates, boots
./skelet up               # start the dev stack     (--lean skips OpenSearch + Jaeger)
./skelet check            # every gate that can fail a commit, under a second
./skelet test             # go test + frontend typechecks + unit tests
./skelet lint             # gofmt + golangci-lint (hard) + eslint (reports only)
./skelet gen              # ALL codegen: buf, sqlc, gqlgen, plugin bridge, docs, TS types
./skelet go test ./...    # go, inside the app container
./skelet cli <args>       # the ops CLI, inside the app container
./skelet psql             # psql on the dev commerce database
./skelet logs app         # tail one service
```

`bin/test-e2e` runs the money-path suite against throwaway databases - it is excluded
from `./skelet test` on purpose, because it writes real orders and never cleans up.

## 3. Generated code - never hand-edit

Editing any of these is always a mistake; regenerate instead.

| Generated | From | Regenerate with |
|---|---|---|
| `graph/{admin,front}/generated.go` | `graph/schema/**/*.graphqls` | `./skelet gen` |
| `graph/{admin,front}/model/models_gen.go` | same | `./skelet gen` |
| `internal/db/sqlc/**` | `internal/db/query/*.sql` | `./skelet gen` |
| `internal/db/signals/sqlc/**` | `internal/db/signals/query/*.sql` | `./skelet gen` |
| `internal/signalspb/**` | `proto/skelet/signals/v1/*.proto` | `./skelet gen` |
| `**/*.gql.ts`, `graphql/schema.types.ts` | `*.graphql` docs + the live schema | `./skelet gen` |
| `plugins/firstparty/`, `plugin_resolvers.go` | `plugins/plugins.json` | `./skelet gen` |
| `docs/{permissions,webhooks,settings}.md` | the code-defined registries | `./skelet gen` |
| `apps/dashboard/nginx.conf.template`, `deploy/caddy/Caddyfile` | `internal/httpx/securityheaders.go` | `./skelet gen` |

`./skelet check codegen` fails when committed output has drifted. The TS half needs a
running backend; `--no-frontend` is the half CI can run.

Write resolver bodies in `*.resolvers.go` only. gqlgen strips unrecognised methods from
those files on regeneration, so private helpers go in `helpers.go` or
`<domain>_helpers.go` in the same directory.

## 4. Database

Three independent migration streams, all applied by compose one-shots that gate the
backend roles - adding a migration means restarting the stack, not remembering to run
anything.

| Stream | Files |
|---|---|
| Commerce | `apps/backend/internal/db/migrations/` |
| Signals | `apps/backend/internal/db/signals/migrations/` |
| ClickHouse spine | `apps/backend/internal/analytics/spinech/migrations/` |

The baseline migration in each stream is generated rather than hand-written, so don't
edit one. A schema change is a new numbered migration on top, paired with its
`.down.sql`. Rows every install needs belong in the reference-data migration rather
than in a data patch. Layout and the regeneration procedure:
[docs/database.md](docs/database.md).

There are no hand-written database calls. Write SQL in `internal/db/query/*.sql` and
let sqlc generate the Go.

## 5. Testing

- Go: standard `testing`, `_test.go` beside the code. `./skelet test`
- Database-backed Go tests gate on `internal/testdb` and refuse to run against a
  database that is not marked disposable
- Dashboard: Vitest, `*.test.tsx` beside the component. `bin/dash run test`
- Storefront: Vitest. `bin/channel <name> run test`
- E2E money paths: `bin/test-e2e [--fresh]`, own databases, ~8 minutes

**Dashboard typecheck trap:** `tsc --noEmit` on its own is a no-op here. Use
`bin/dash run typecheck:all`, or `npx tsc -p tsconfig.app.json --noEmit`.

## 6. Formatting

`gofmt` and `golangci-lint` are hard gates on the Go side. On the TypeScript side eslint
reports but does not block, and **there is no prettier in the dashboard** - do not add
one, it rewrites whole files.

## 7. Patterns worth knowing

**Three GraphQL surfaces.** `/api/admin/graphql` and `/api/front/graphql` run in the
server process; `/api/signals/graphql` runs in `cmd/signals`. Each has entirely separate
schemas, generated executables and resolver trees. The admin middleware is non-blocking,
so an ungated admin resolver answers anonymous callers - every admin resolver must gate
explicitly, and `authz_coverage_test.go` enforces it. The Signals surface works the same
way, and so does the gRPC contract beside it: the interceptor injects, the handler refuses.

**Everything is channel-scoped.** Catalog data is global; per-channel availability is a
listing row. Permissions are scoped per channel too. When you add a resolver, decide its
scope deliberately - [docs/channels.md](docs/channels.md).

**Domain packages** under `internal/<domain>/` follow one shape: `domain.go` for types
and interfaces, `service.go` for behaviour, `repository.go` over sqlc, `events.go` for
subscribers. Cross-domain work goes through the event bus or the transactional outbox,
not direct imports.

**Dashboard staged save is the rule, not a pattern to opt into.** Field edits stage
locally and apply through the single page-header Save, which opens `SaveConfirmDialog`
with every before→after change. Never give a section its own Save button, never fire a
mutation on change. Every page must also declare its unsaved work. The exceptions and
the full wiring are in [docs/conventions.md](docs/conventions.md) - read it before
touching a detail page.

**Dashboard UI conventions:** all lists go through the shared TanStack `DataTable`;
detail pages use `DetailPageLayout` + `DetailPageHeader` / `Body` / `Section`
(`src/components/layout/detail-page.tsx`). No Card components
- flat `rounded-md` bordered blocks and `divide-y` lists. Never use the `primary` /
`accent` theme tokens for semantic state, because admins can recolour them.

**Storefront:** Next.js 16, which differs from most training data - read
`apps/channels/<name>/node_modules/next/dist/docs/` before writing App Router code.
GraphQL is server-only, channel and locale are resolved server-side, and routing is
backend-driven through the `[...slug]` catch-all and the URL-rewrite resolver.

**Plugins** are compiled in from `plugins/plugins.json`, which is the decision file.
Anything writing it must MERGE, never rewrite - `./skelet cli plugin install|enable`
and `bin/setup` all touch it. A slug enabled with no `plugins/<slug>` directory fails
codegen outright.

## 8. Environment traps

- `rg <pattern> apps/backend` silently skips `cmd/server` - use `grep -r` for sweeps
- New dashboard dependencies need `SKELET_FORCE_DOCKER=1 bin/dash install` plus a
  container restart; a host-only install crashes the container's vite
- `core.fileMode` is off here, so `chmod +x` on a `bin/` script needs
  `git update-index --chmod=+x <file>` to stick
- Air's hot reload misses changes on WSL2 - `docker compose restart app`
- Containers run as your uid via `SKELET_UID`, which only `bin/_lib` exports. A script
  that runs `docker compose run` or `up` must source it, and `bin/repo-check` enforces that

## 9. Before you finish

Run `./skelet check`. If you touched schema, SQL or GraphQL documents, run
`./skelet gen` and commit the regenerated output with your change. If you touched Go,
run `./skelet lint` and `./skelet test`. Do not commit or push unless asked.

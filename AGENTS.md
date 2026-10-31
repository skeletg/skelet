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
| `apps/backend/` | Everything server-side: three Go modules side by side, plus the shared dev Dockerfile |
| `apps/backend/core/` | Core's `go.mod`. gqlgen + sqlc. Two GraphQL surfaces, the Signals wire contract's client and generated types, and `lib/`, the infrastructure the suite compiles against |
| `apps/backend/signals/` | The Signals suite, its own `go.mod` requiring core's by relative path: the contract's server, the beacon surface, the spine and its engines |
| `apps/backend/sdk/` | The plugin SDK, its own `go.mod`. Plugin repositories build against it, plus core's gqlgen model packages for the resolvers they add |
| `apps/dashboard/` | React 19 + Vite admin SPA, strict TypeScript |
| `apps/channels/<name>/` | Next.js 16 storefronts, App Router, RSC |
| `cmd/skelet/` | The `skelet` CLI: the front door here, every dev verb, and the released binary that starts projects. Its own `go.mod`, standard library only, imports nothing from the backend |
| `docs/` | One file per question, indexed in `docs/README.md` |

Full tree: [docs/architecture.md](docs/architecture.md). What each root directory is
for: [docs/layout.md](docs/layout.md).

## 2. Commands

**Use `skelet`, not raw `docker compose` / `pnpm` / `go`.** It resolves the right
container, the right database URL and the right node_modules. `skelet` with no
arguments lists every verb; `skelet help <verb>` explains one. Each verb is Go under
`cmd/skelet/internal/`: read it when you need the detail, but drive the repo through the
verbs so there's a single vocabulary. `skelet` is the installed CLI, which runs this
checkout's own `cmd/skelet` - so a verb always matches the code you're on. Without it,
`go -C cmd/skelet run . <verb>` reaches the same build with only Go, the way CI runs the
verbs - but it reports any failure as exit 1, and relative paths in arguments resolve from
`cmd/skelet`.

```bash
skelet setup            # first run: picks apps, writes env, migrates, boots
skelet up               # start the dev stack     (--lean skips OpenSearch, the dev tools and the Signals process with its ClickHouse)
skelet check            # every gate that can fail a commit, in about a second
skelet test             # go test + frontend typechecks + unit tests
skelet lint             # gofmt + golangci-lint (hard) + eslint (reports only)
skelet gen              # ALL codegen: buf, sqlc, gqlgen, plugin bridge, docs, TS types
skelet go test ./...    # go, inside the app container
skelet go --signals test ./...   # the same, in the Signals module
skelet cli <args>       # the ops CLI, inside the app container
skelet psql             # psql on the dev commerce database
skelet logs app         # tail one service
```

`skelet test e2e` runs the money-path suite against throwaway databases - it is excluded
from `skelet test` on purpose, because it writes real orders and never cleans up.

## 3. Generated code - never hand-edit

Editing any of these is always a mistake; regenerate instead.

| Generated | From | Regenerate with |
|---|---|---|
| `graph/{admin,front}/generated.go` | `graph/schema/**/*.graphqls` | `skelet gen` |
| `graph/{admin,front}/model/models_gen.go` | same | `skelet gen` |
| `internal/db/sqlc/**` | `internal/db/query/*.sql` | `skelet gen` |
| `apps/backend/signals/internal/db/sqlc/**` | `apps/backend/signals/internal/db/query/*.sql` | `skelet gen` |
| `signalsapi/v1/**` | `proto/skelet/signals/v1/*.proto` | `skelet gen` |
| `**/*.gql.ts`, `graphql/schema.types.ts` | `*.graphql` docs + the live schema | `skelet gen` |
| `plugins/firstparty/`, `plugin_resolvers.go` | `plugins/plugins.json` | `skelet gen` |
| `docs/{permissions,webhooks,settings}.md` | the code-defined registries | `skelet gen` |
| `apps/dashboard/nginx.conf.template`, `deploy/caddy/Caddyfile` | `httpx/securityheaders.go` | `skelet gen` |

`skelet check codegen` fails when committed output has drifted. The TS half needs a
running backend; `--no-frontend` is the half CI can run.

Write resolver bodies in `*.resolvers.go` only. gqlgen strips unrecognised methods from
those files on regeneration, so private helpers go in `helpers.go` or
`<domain>_helpers.go` in the same directory.

## 4. Database

Three independent migration streams, each applied by the processes that use it as
they boot - adding a migration means restarting the app, not remembering to run
anything.

| Stream | Files |
|---|---|
| Commerce | `apps/backend/core/internal/db/migrations/` |
| Signals | `apps/backend/signals/internal/db/migrations/` |
| ClickHouse spine | `apps/backend/signals/internal/analytics/spinech/migrations/` |

The baseline migration in each stream is generated rather than hand-written, so don't
edit one. A schema change is a new migration on top, made with `skelet db migrate new <name>`:
one dbmate file, timestamped so parallel branches never collide, with its `-- migrate:up` and
`-- migrate:down` halves. Rows every install needs belong in the reference-data migration rather
than in a data patch. Layout and the regeneration procedure:
[docs/database.md](docs/database.md).

There are no hand-written database calls. Write SQL in `internal/db/query/*.sql` and
let sqlc generate the Go.

## 5. Testing

- Go: standard `testing`, `_test.go` beside the code. `skelet test`
- Database-backed Go tests gate on `testdb` and refuse to run against a
  database that is not marked disposable. `skelet test` brings its own
  (`skelet_test` / `skelet_test_signals`, fresh each run, `--keep` to reuse), and
  every test binary clones them (`CREATE DATABASE ... TEMPLATE`), so packages
  never share rows - a harness that opens its own pool must take its DSN from
  `testdb.URL`. ClickHouse tests take theirs from `chtest.DSN` (Signals module), the
  same gate with the marker in the database comment. It ends with a pass/fail/skip
  tally: a skip count that isn't near zero means a database went missing, not that
  the tests are fine. One run at a time - a second `skelet test` refuses while the
  first holds the databases
- `skelet test --changed` runs only what a change reaches - Go's test cache keyed on the code and the
  migrations, plus `vitest related` per frontend. It's for iterating; the full run is still the gate before you finish
- Dashboard: Vitest, `*.test.tsx` beside the component. `skelet pnpm dashboard run test`
- Storefront: Vitest. `skelet pnpm <name> run test`
- E2E money paths: `skelet test e2e`, own databases, dropped and recreated each run (`--keep` to reuse), ~40 seconds
- Storefront browser tests: `skelet test storefront [name]`, Playwright against the running dev container,
  ~1.5 minutes. Writes throwaway customers to the dev database. Needs `skelet up` first

**Dashboard typecheck trap:** `tsc --noEmit` on its own is a no-op here. Use
`skelet pnpm dashboard run typecheck:all`, or `pnpm exec tsc -p tsconfig.app.json --noEmit`.

## 6. Formatting

`gofmt` and `golangci-lint` are hard gates on the Go side. On the TypeScript side eslint
reports but does not block, and **there is no prettier in the dashboard** - do not add
one, it rewrites whole files.

## 7. Patterns worth knowing

**Two GraphQL surfaces.** `/api/admin/graphql` and `/api/front/graphql`, both in the
server process, with entirely separate schemas, generated executables and resolver trees.
The admin middleware is non-blocking, so an ungated admin resolver answers anonymous
callers - every admin resolver must gate explicitly, and `authz_coverage_test.go` enforces
it. The gRPC contract works the same way: the interceptor injects, the handler refuses.

**The dashboard talks to core and nothing else.** Signals runs as its own service and
answers only over the wire contract, so an admin resolver that needs it calls
`r.SignalsAPI` and maps the answer - see `graph/admin/signals_*.go`.

**Signals is its own Go module, and the dependency points one way.** `apps/backend/signals` requires
core; core never imports it, and cannot - Go's `internal/` rule refuses a package under
`apps/backend/core/internal/` from another module. Anything the suite needs of core therefore
sits beside `internal/` (`config`, `auth`, `dbconn`, `objstore` and the rest), and anything
the suite needs *from* core arrives over the contract: channels and customers are pushed
across, settings are administered through it, and every moment a flow can start on is
raised through `Enroll`. Core never opens the signals database - Pulse (campaigns, flows,
the email builder) runs in the suite, and the dashboard reaches it through core's relaying
resolvers like the rest of Signals.

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
`accent` theme tokens for semantic state, because admins can recolour them. State goes
through the `success` / `warning` / `info` / `destructive` tokens as plain utilities
(`text-success`, `bg-warning-soft`, `border-info/40`), never a raw hue like
`bg-emerald-500` - see [docs/conventions.md](docs/conventions.md).

**Storefront:** Next.js 16, which differs from most training data - read
`apps/channels/<name>/node_modules/next/dist/docs/` before writing App Router code.
GraphQL is server-only, channel and locale are resolved server-side, and routing is
backend-driven through the `[...slug]` catch-all and the URL-rewrite resolver.

**Plugins** are compiled in from `plugins/plugins.json`, which is the decision file.
Anything writing it must MERGE, never rewrite - `skelet cli plugin install|enable`
and `skelet setup` all touch it. A slug enabled with no `plugins/<slug>` directory fails
codegen outright.

## 8. Environment traps

- `rg <pattern> apps/backend` silently skips `cmd/server` - use `grep -r` for sweeps
- A Docker frontend's dev server runs under `watchexec` (the CMD of its dev Dockerfile), which reinstalls
  and restarts it within a couple of seconds of `package.json` or the lockfile changing - no manual
  install into the container. A Dockerfile change still needs `skelet up`, which rebuilds
- The frontends use pnpm, one project per app (`pnpm-workspace.yaml` beside each
  `package.json` holds its settings - pnpm ignores `package.json`'s `pnpm` field). Three
  differences from npm bite: an import of a package the app doesn't declare fails, `--` reaches
  the script verbatim (`pnpm run dev --host`, not `-- --host`), and a version published less
  than a day ago won't install (`minimumReleaseAge`)
- `core.fileMode` is off here, so `chmod +x` on a script needs
  `git update-index --chmod=+x <file>` to stick
- Air's hot reload misses changes on WSL2 - `docker compose restart app`
- The first `skelet` after a change under `cmd/skelet` rebuilds the CLI - host Go if there is one, else the
  `golang` image - and every later run starts the cached binary. Its verbs are one line each in
  `cmd/skelet/internal/route/table.go`
- Containers run as your uid via `SKELET_UID`, which the CLI sets for every compose command that
  creates one. A raw `docker compose up` leaves it at 1000 - another reason to go through the verbs

## 9. Before you finish

Run `skelet check`. If you touched schema, SQL or GraphQL documents, run
`skelet gen` and commit the regenerated output with your change. If you touched Go,
run `skelet lint` and `skelet test`. A change worth a changelog line gets a fragment
in `changelog/unreleased/` (see its README), never an edit under `[Unreleased]`. Do not commit or
push unless asked.

# Development workflow

Running the stack, the commands you'll use daily, regenerating what's
generated, and the cross-app contracts a change has to keep.

## Commands

`./skelet` is the front door - bare for a grouped list of every command,
`./skelet help <command>` for one. It routes onto the scripts below, so
`./skelet up` and `bin/dev` are the same thing. The verb→script map is in
[scripts.md](scripts.md).

| Command | Action |
|---|---|
| `bin/setup` | Guided first-run: asks what you want, writes the env files and secrets, boots, migrates. `--yes` for defaults, `--profile FILE` to replay answers |
| `bin/check [--codegen] [--list]` | **Every gate that can fail a commit, in one command** - version pins, repo structure, cross-app contracts. Under a second, no Docker, no backend. What the pre-commit hook and CI both run; `--list` explains each gate, `--codegen` adds the generated-file check |
| `bin/install-hooks` | Point `.git/hooks/pre-commit` at `bin/check`. Opt-in, and deliberately not the `--codegen` variant: needing a running backend to make a commit is a bad trade |
| `bin/dev` | Start the full dev stack (ports exposed, hot reload, `APP_ENV=development`, all sidecars) |
| `bin/stop` | `docker compose stop` - halt without removing |
| `bin/down` | `docker compose down` - remove containers (volumes preserved) |
| `bin/restart` | `docker compose restart` - restart in place |
| `bin/ps` | List running containers |
| `bin/logs [service] [flags]` | Tail container logs (forwards `--tail`, `--since`, etc.) |
| `bin/bash` | Shell into the Go app container |
| `bin/go <args>` | Run go inside the app container (e.g. `bin/go build ./...`) |
| `bin/gqlgen <args>` | Run gqlgen inside the app container |
| `bin/sqlc <args>` | Run sqlc against `apps/backend/` (one-shot container) |
| `bin/migrate <up\|down> [N]` | Run DB migrations |
| `bin/cli <args>` | Run the Cobra CLI inside the app container (`-it` only when stdin is a tty, so CI works) |
| `bin/psql` | psql shell into the dev database |
| `bin/dash <args>` | Run an npm command inside `apps/dashboard/` (e.g. `bin/dash run dev`, `bin/dash install`) |
| `bin/channel [name] <args>` | Run an npm command inside a storefront container under `apps/channels/` (e.g. `bin/channel run build`, `bin/channel vetrina-demo run codegen`). The name is optional when exactly one storefront exists. Exec's into the running container when `bin/dev` is up; otherwise spins up a one-shot. |
| `bin/codegen [--check]` | Regenerate everything: sqlc → gqlgen admin → gqlgen front → dashboard codegen. Pre-flights `/health` before the dashboard step. `--check` exits non-zero if any generated file changed (used by the pre-commit hook). Storefront codegen IS wired in - every storefront under `apps/channels/` gets its TS docs regenerated (skipped for apps deselected via bin/setup). |
| `bin/test [pkgs] [--no-frontend]` | Backend `go test` + dashboard/storefront typechecks + both apps' vitest suites. Args after the script forward to `go test`. |
| `bin/lint` | gofmt + golangci-lint + dashboard ESLint |

Full per-script documentation: [scripts.md](scripts.md).

### CLI examples

| Command | Action |
|---|---|
| `bin/cli admin create-superuser` | Interactive: create superuser |
| `bin/cli admin create` | Interactive: create admin + assign roles |
| `bin/cli admin list` | List admins with roles |
| `bin/cli role list` | List roles with member counts + permissions |
| `bin/cli integration list` | List API integrations |
| `bin/cli config list/get/set/reset/orphans/prune` | Config registry management |
| `bin/cli customer group list` | List customer groups |
| `bin/cli db backup` / `db backups` | Stream a pg_dump to the configured dedicated storage (Settings → System → Database backups) / list bucket contents |

## Docker setup

Three compose files - `docker-compose.yml` is the dev stack (`docker compose
up` is enough, no `-f` needed), `docker-compose.small.yml` the one-box tenant and
`docker-compose.prod.yml` the split-tier shape. Both deployment files carry a Caddy
edge behind `--profile edge`; leave the profile off to put your own proxy in front.

**Core services**:
- `app` - Go API, exposes :8080, mounts `./apps/backend` → `/var/www/app`.
- `vetrina-demo` - Next.js on `:3000`, `depends_on: app`. Image built from
  `apps/channels/vetrina-demo/Dockerfile`. Each storefront is its own service,
  named after its directory under `apps/channels/`.
- `db` - Postgres 18 (alpine).
- `valkey` - Redis-compatible KV; JWT refresh tokens + caches.

**Dev sidecars**:
- `vetrina-demo` - `next dev`, host bind-mount on
  `./apps/channels/vetrina-demo` with named volumes shadowing `node_modules` + `.next`
  (so host-built Windows/macOS artifacts can't poison the Linux/musl
  install or the SWC native binary). `CHOKIDAR_USEPOLLING=true` +
  `WATCHPACK_POLLING=true` because inotify is unreliable over bind mounts
  on WSL2/macOS.
- `mailpit` - SMTP catcher; SMTP at `:1025`, web UI at <http://localhost:8025>.
- `webhook-tester` - webhook listener UI at <http://localhost:8081>.
- `garage` + `garage-init` - S3-compatible storage (Garage); S3 API at
  `:9900`, public reads via the web endpoint at `:9902`
  (vhost-addressed: `http://media.localhost:9902/<key>`), bucket
  `media` + `skeletdev` key auto-provisioned, CORS + website
  access applied by `garage-init`. Web console (`garage-webui`) at
  <http://localhost:9901>; CLI ops via
  `docker compose exec garage /garage <cmd>`.

### Looking at another channel

A storefront instance is pinned to exactly one channel by `STOREFRONT_CHANNEL_ID`,
in dev as much as in production. To see a second channel, run a second instance
of the same app pinned to it - same source, same image, its own port and `.next`
volume - published at that channel's `public_url` so the dashboard's preview links
and the backend's Host-based channel resolution both line up. Put it in your
git-ignored `docker-compose.override.yml`; there's a worked example there if this
clone has one, and `bin/new channel` writes services in the same shape.

There is deliberately no cookie or query-param channel switcher. Anything
request-scoped can't decide the channel, because the routes that serve most of the
site render without a request - on a `force-static` route, which is every pretty
URL, a switcher would do nothing at all, so previewing a channel would appear to
work while actually rendering the pinned one.

The storefront container reads two URLs to bridge SSR vs CSR:
`BACKEND_INTERNAL_URL=http://app:8080` (server-side fetches from RSC /
route handlers - Docker-network hostname) and
`NEXT_PUBLIC_BACKEND_URL=http://localhost:8080` (client-side fallback -
host port). Mixing these up gets you "fetch failed" on the server or
CORS in the browser.

`bin/dev` runs `docker compose up -d` - the default file is the dev stack.

Where the frontends run is your choice, recorded in `.skelet-apps`. Listed
there, `bin/dev` starts them in Docker on <http://localhost:5173> and
<http://localhost:3000>; left out, they stay on the host and `./skelet serve
dashboard` runs one - the default, because the dev server and the gates then
share one `node_modules`. Either way `bin/dash` / `bin/channel` run npm against
whichever tree is live. Every script is documented in [scripts.md](scripts.md).

## Codegen flow

The generated artifacts must always agree with the source-of-truth
schemas. A mismatch surfaces as an import error that white-screens the
dashboard, so treat every schema/query change as "must regenerate":

1. **`*.sql` in `apps/backend/internal/db/query/`** → `bin/sqlc generate`.
   Manually delete stale `apps/backend/internal/db/sqlc/<old>.sql.go`
   when tables are dropped.
2. **`*.graphqls` in `apps/backend/graph/schema/<surface>/`** →
   `bin/gqlgen generate --config gqlgen.<surface>.yml` (or
   `bin/codegen` to do both surfaces). Schema files are paired with
   resolver files by name - `cart.graphqls` ↔ `cart.resolvers.go`.
3. **`*.graphql` in `apps/dashboard/src/features/<domain>/`** →
   `bin/dash run codegen` (backend must be running - codegen fetches
   the live admin schema over HTTP).
4. **`*.graphql` in `apps/channels/vetrina-demo/app/`** →
   `bin/channel run codegen` (backend must be running - codegen
   fetches the live front schema at `app:8080/api/front/graphql`).

`bin/codegen` runs all four, discovering storefronts from
`apps/channels/` rather than naming them. Steps 3 and 4 introspect a
live server, so `--no-frontend` stops after step 2 when the stack isn't up.

`bin/codegen --check` regenerates and fails if the output would differ
from what's committed, which is how a stale generated file gets caught
before it white-screens the dashboard. It is
**not** in the pre-commit hook - it needs the container, and steps 3 and 4
need the backend, so gating every commit on a running stack is the wrong
trade. Run it yourself before a PR that touched a `.sql`, `.graphqls` or
`.graphql` file. The hook runs `bin/check`, which is greps only.

## Cross-app contracts

Some invariants span two or three apps: the CMS block-type list (backend
validator, dashboard palette, storefront renderer), the
anonymous-cacheable operation list (backend allowlist, storefront
token-stripper), the lexical node types the dashboard authors and the
storefront renders, the conditional-option evaluator (ported into both
frontends from `internal/cart/options_visibility.go`), and the security
headers the backend and the dashboard's nginx both set.

Each end carries a `SYNC` or `SYNC SENTINEL` comment, and
`bin/contract-check` enforces the mechanical part - including that the
paths those comments name still exist, since a `SYNC` comment pointing at
a moved file is worth nothing. It runs on the host, because the app
containers each mount only their own tree and no single test runner can
see all three sides.

Adding a contract means a `SYNC` comment at both ends **and** a check in
that script. Every check there fails when its extractor matches nothing,
so a moved source is a red build instead of a quiet pass.

## First run

`bin/setup` asks a short set of questions and writes the answers out, so nothing has to be
found in a template first:

| Question | Writes |
| --- | --- |
| Which apps | `.skelet-apps`, plus sparse-checkout of the rest |
| Deployment target - dev / small / prod | materializes only that target's env files |
| Suites - Signals, B2B, AI | `SIGNALS_ENABLED` / `B2B_ENABLED` / `AI_ENABLED` |
| Search tier, analytics spine | `OPENSEARCH_URL`, `SIGNALS_ENGINE` in the root `.env` |
| Payment gateways | `apps/backend/plugins/plugins.json`, then `bin/codegen` if it changed |
| Hide the admin API | `ADMIN_API_SLUG` + the matching dashboard URLs |

Secrets are generated, never prompted, and an existing non-placeholder value is left alone -
so re-running is safe and never rotates a key. The answers land in git-ignored
`.skelet-profile`, which becomes the defaults next time and which `bin/doctor` reads back to
tell you what this clone is meant to be.

Non-interactive: `bin/setup --yes` takes the defaults; `bin/setup --profile <file>` replays a
saved profile, which is what CI and a second clone want.

## Which apps run

The backend (GraphQL APIs + workers) always runs. The admin dashboard and every
storefront under `apps/channels/` are optional, so a clone that ships its own
frontend can run headless. Deselected apps are removed from your working tree via
`git sparse-checkout` - they stay in history, and `--keep-code` keeps the source
without running it.

Non-interactive variants: `bin/setup --all`, `--backend-only`, or `bin/setup
--apps dashboard,vetrina-demo` (comma list). The choice is saved per-clone in
`.skelet-apps` (git-ignored); re-run `bin/setup --apps ...` then `bin/down &&
bin/dev` to change it. Selecting an app again restores its source and containers.

For the smallest possible backend-only clone, skipping even the *download* of the
frontends' file history:

```bash
git clone --filter=blob:none --no-checkout <this repo> skelet
cd skelet
git sparse-checkout set --no-cone '/*' '!/apps/dashboard/' '!/apps/channels/'
git checkout main
bin/setup --backend-only
```

### Adding your own storefront

One directory per shop under `apps/channels/<name>/`; the directory name
doubles as the compose service and profile name.

`skelet new channel <your-shop>` does steps 1 and 2 for you - it forks a
storefront, gives it the first free ports, copies the compose service out of the
existing one (so it inherits changes to the demo's wiring rather than a snapshot
taken once), and writes nothing until the result parses. `--tracked` puts the
service in the tracked compose files instead, which is for adding a storefront to
skelet itself and gives up the conflict-free property below. `skelet rm channel
<your-shop>` reverses it, keeping the Docker volumes unless you pass `--volumes`.
By hand:

1. Create `apps/channels/<your-shop>/` - fork of the vetrina-demo template or
   from scratch.
2. Declare its compose service in `docker-compose.override.yml` (git-ignored,
   auto-merged by docker compose) with `profiles: ["<your-shop>"]`. Never edit
   the tracked compose files, so `git pull` from upstream skelet stays
   conflict-free. Anchors don't cross compose files, so a block copied from
   `docker-compose.yml` needs its own `x-logging: &logging` beside it.
3. `bin/setup --apps dashboard,<your-shop>` - deselecting `vetrina-demo` removes
   it from your working tree via sparse-checkout *without committing a deletion*,
   so upstream updates to the demo never conflict with your clone.
That's the whole list. Everything else follows the directory: `bin/dev`,
`bin/channel`, `bin/codegen` and `bin/push-images` all discover storefronts from
`apps/channels/`, CI derives its typecheck, test, audit and image-build matrices
the same way, and Renovate finds your `package.json` and `Dockerfile` on its own.
A shop that lands in the tree is gated like the demo without touching a workflow
or a config file.

## Working with WSL + Windows

If you're editing the repo from VSCode running on Windows against a WSL
working tree, install the **WSL** extension (`ms-vscode-remote.remote-wsl`)
and open via `code .` from inside WSL. Without it, Windows-side git
strips POSIX exec bits on `bin/*` and adds CRLF, polluting `git status`.

`.gitattributes` enforces LF normalization regardless of platform.
`core.fileMode = false` in local `.git/config` is the workaround if
you can't switch to WSL Remote.

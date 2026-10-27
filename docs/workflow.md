# Development workflow

Running the stack, the commands you'll use daily, regenerating what's
generated, and the cross-app contracts a change has to keep.

## Commands

`./skelet` is the front door - bare for a grouped list of every command,
`./skelet help <command>` for one. The scripts under `bin/` are what it runs,
and [scripts.md](scripts.md) has the detail on each command plus the map
between the two.

| Command | Action |
|---|---|
| `./skelet setup` | Guided first-run: asks what you want, writes the env files and secrets, boots, migrates. `--yes` for defaults, `--profile FILE` to replay answers |
| `./skelet check [--codegen] [--list]` | **Every gate that can fail a commit, in one command** - version pins, repo structure, cross-app contracts. Under a second, no Docker, no backend. What the pre-commit hook and CI both run; `--list` explains each gate, `--codegen` adds the generated-file check |
| `./skelet hooks` | Point `.git/hooks/pre-commit` at `bin/check`. Opt-in, and deliberately not the `--codegen` variant: needing a running backend to make a commit is a bad trade |
| `./skelet up` | Start the full dev stack (ports exposed, hot reload, `APP_ENV=development`, all sidecars) |
| `./skelet stop` | `docker compose stop` - halt without removing |
| `./skelet down` | `docker compose down` - remove containers (volumes preserved) |
| `./skelet restart` | `docker compose restart` - restart in place |
| `./skelet ps` | List running containers |
| `./skelet logs [service] [flags]` | Tail container logs (forwards `--tail`, `--since`, etc.) |
| `./skelet sh` | Shell into the Go app container |
| `./skelet go <args>` | Run go inside the app container (e.g. `./skelet go build ./...`); `./skelet go --signals ...` and `./skelet go --sdk ...` target the other two modules |
| `./skelet gen gqlgen <args>` | Run gqlgen inside the app container |
| `./skelet gen sqlc <args>` | Run sqlc for both Go modules, core and signals (one-shot container) |
| `./skelet db migrate <up\|down> [N]` | Run DB migrations |
| `./skelet cli <args>` | Run the Cobra CLI inside the app container (`-it` only when stdin is a tty, so CI works) |
| `./skelet psql` | psql shell into the dev database |
| `./skelet pnpm dashboard <args>` | Run a pnpm command inside `apps/dashboard/` (e.g. `./skelet pnpm dashboard run dev`, `./skelet pnpm dashboard install`) |
| `./skelet pnpm <name> <args>` | Run a pnpm command inside a storefront container under `apps/channels/` (e.g. `./skelet pnpm vetrina-demo run build`, `./skelet pnpm vetrina-demo run codegen`). Exec's into the running container when the stack is up; otherwise spins up a one-shot. |
| `./skelet gen [--check]` | Regenerate everything: sqlc → gqlgen admin → gqlgen front → dashboard codegen. Pre-flights `/health` before the dashboard step. `--check` exits non-zero if any generated file changed (used by the pre-commit hook). Storefront codegen IS wired in - every storefront under `apps/channels/` gets its TS docs regenerated (skipped for apps deselected via ./skelet setup). |
| `./skelet test [pkgs] [--keep] [--no-frontend]` | Backend `go test` against disposable `skelet_test` / `skelet_test_signals` databases (fresh each run, `--keep` reuses them) + dashboard/storefront typechecks + both apps' vitest suites. Each Go module ends with a pass/fail/skip tally. Args after the flags forward to `go test`. |
| `./skelet lint` | gofmt + golangci-lint + dashboard ESLint |

Full documentation for every command: [scripts.md](scripts.md).

### CLI examples

| Command | Action |
|---|---|
| `./skelet cli admin create-superuser` | Interactive: create superuser |
| `./skelet cli admin create` | Interactive: create admin + assign roles |
| `./skelet cli admin list` | List admins with roles |
| `./skelet cli role list` | List roles with member counts + permissions |
| `./skelet cli integration list` | List API integrations |
| `./skelet cli config list/get/set/reset/orphans/prune` | Config registry management |
| `./skelet cli customer group list` | List customer groups |

## Docker setup

Three compose files - `docker-compose.yml` is the dev stack (`docker compose
up` is enough, no `-f` needed), `docker-compose.small.yml` the one-box tenant and
`docker-compose.prod.yml` the split-tier shape. Both deployment files carry a Caddy
edge behind `--profile edge`; leave the profile off to put your own proxy in front.

**Core services**:
- `app` - Go API, exposes :8080, mounts `./apps/backend` → `/var/www/backend` and works from `core/` under it.
- `vetrina-demo` - Next.js on `:3000`, `depends_on: app`. Image built from
  `apps/channels/vetrina-demo/Dockerfile`. Each storefront is its own service,
  named after its directory under `apps/channels/`.
- `db` - Postgres 18 (alpine).
- `valkey` - Redis-compatible KV; JWT refresh tokens + caches.

**Dev sidecars**:
- `signals` - the Signals process, HTTP on `:8090` and the gRPC contract on `:8092`,
  started whenever `SIGNALS_ENABLED` is on in `env/go.env`. Core proxies `/t` and
  `/e` to it; the separate `ingest` tier is a scale-out option, off in dev.
- `vetrina-demo` - `next dev`, host bind-mount on
  `./apps/channels/vetrina-demo` with named volumes shadowing `node_modules` + `.next`
  (so host-built Windows/macOS artifacts can't poison the Linux/musl
  install or the SWC native binary). `CHOKIDAR_USEPOLLING=true` +
  `WATCHPACK_POLLING=true` because inotify is unreliable over bind mounts
  on WSL2/macOS.
- `garage` + `garage-init` - S3-compatible storage (Garage); S3 API at
  `:9900`, public reads via the web endpoint at `:9902`
  (vhost-addressed: `http://media.localhost:9902/<key>`), bucket
  `media` + `skeletdev` key auto-provisioned, CORS + website
  access applied by `garage-init`. CLI ops via
  `docker compose exec garage /garage <cmd>`.

### Optional dev tools

None of these are needed, and nothing in the apps knows about them. Each is a compose
profile of its own name: pick them in `./skelet setup` (or `--tools LIST|none`), which
saves the list to git-ignored `.skelet-tools`, or start one for a single session with
`./skelet up <name>`.

- `mailpit` - SMTP catcher; SMTP at `:1025`, web UI at <http://localhost:8025>.
  Without it, mail prints to the app log.
- `webhook-tester` - webhook listener UI at <http://localhost:8081>; point
  webhooks at `http://webhook-tester:8080/<any-path>`.
- `grafana` - Grafana with Tempo, Prometheus and Loki behind it, UI at
  <http://localhost:3030> (admin / admin). Takes the backends' OTLP traces and
  metrics on `grafana:4318`, and `grafana-alloy` beside it ships every skelet
  container's stdout into Loki.
- `garage-webui` - Garage's web console at <http://localhost:9901>.

`./skelet up` wires the env files to match `.skelet-tools`: picking `mailpit` points
`SMTP_HOST` at it, picking `grafana` points the `OTEL_*` vars at it, and dropping either
blanks them again. Values you set to something else (a real relay, your own collector)
are left alone. A tool started ad hoc with `./skelet up <name>` is not wired - add it to
`.skelet-tools` for that.

### Looking at another channel

A storefront instance is pinned to exactly one channel by `STOREFRONT_CHANNEL_ID`,
in dev as much as in production. To see a second channel, run a second instance
of the same app pinned to it - same source, same image, its own port and `.next`
volume - published at that channel's `public_url` so the dashboard's preview links
and the backend's Host-based channel resolution both line up. Put it in your
git-ignored `docker-compose.override.yml`; there's a worked example there if this
clone has one, and `./skelet new channel` writes services in the same shape.

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

`./skelet up` runs `docker compose up -d --build` - the default file is the dev stack.
Source edits hot-reload through the bind mounts - air for the app and the Signals process, Vite
and Next for the frontends. A frontend container also watches its own `package.json` and
lockfile, reinstalling and restarting its dev server when either changes (`watchexec` in its dev
Dockerfile's CMD). What still needs you is a Dockerfile change - `./skelet up` rebuilds the
images. A new migration doesn't: the app applies it the next time it rebuilds or restarts.

Where the frontends run is your choice, recorded in `.skelet-apps`. Listed
there, `./skelet up` starts them in Docker on <http://localhost:5173> and
<http://localhost:3000>; left out, they stay on the host and `./skelet serve
dashboard` runs one - the default, because the dev server and the gates then
share one `node_modules`. Either way `./skelet pnpm dashboard` / `./skelet pnpm` run pnpm against
whichever tree is live. Every command is documented in [scripts.md](scripts.md).

## Codegen flow

The generated artifacts must always agree with the source-of-truth
schemas. A mismatch surfaces as an import error that white-screens the
dashboard, so treat every schema/query change as "must regenerate":

1. **`*.sql` in `apps/backend/core/internal/db/query/`** → `./skelet gen sqlc generate`.
   Manually delete stale `apps/backend/core/internal/db/sqlc/<old>.sql.go`
   when tables are dropped.
2. **`*.graphqls` in `apps/backend/core/graph/schema/<surface>/`** →
   `./skelet gen gqlgen generate --config gqlgen.<surface>.yml` (or
   `./skelet gen` to do both surfaces). Schema files are paired with
   resolver files by name - `cart.graphqls` ↔ `cart.resolvers.go`.
3. **`*.graphql` in `apps/dashboard/src/features/<domain>/`** →
   `./skelet pnpm dashboard run codegen` (backend must be running - codegen fetches
   the live admin schema over HTTP).
4. **`*.graphql` in `apps/channels/vetrina-demo/app/`** →
   `./skelet pnpm <name> run codegen` (backend must be running - codegen
   fetches the live front schema at `app:8080/api/front/graphql`).

`./skelet gen` runs all four, discovering storefronts from
`apps/channels/` rather than naming them. Steps 3 and 4 introspect a
live server, so `--no-frontend` stops after step 2 when the stack isn't up.

`./skelet gen --check` regenerates and fails if the output would differ
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

`./skelet setup` asks a short set of questions and writes the answers out, so nothing has to be
found in a template first:

| Question | Writes |
| --- | --- |
| Which apps | `SKELET_APPS_SELECTED` in the profile, plus sparse-checkout of the rest |
| Where the frontends run | `.skelet-apps` - empty means the host, which is the default |
| Deployment target - dev / small / prod | materializes only that target's env files |
| Suites - Signals, AI | `SIGNALS_ENABLED` / `AI_ENABLED` |
| Search tier | `OPENSEARCH_URL` in the root `.env` |
| Payment gateways | `apps/backend/core/plugins/plugins.json`, then `./skelet gen` if it changed |
| Hide the admin API | `ADMIN_API_SLUG` + the matching dashboard URLs |

Secrets are generated, never prompted, and an existing non-placeholder value is left alone -
so re-running is safe and never rotates a key. The answers land in git-ignored
`.skelet-profile`, which becomes the defaults next time and which `./skelet doctor` reads back to
tell you what this clone is meant to be.

Non-interactive: `./skelet setup --yes` takes the defaults; `./skelet setup --profile <file>` replays a
saved profile, which is what CI and a second clone want.

## Which apps run

The backend (GraphQL APIs + workers) always runs. The admin dashboard and every
storefront under `apps/channels/` are optional, so a clone that ships its own
frontend can run headless. Deselected apps are removed from your working tree via
`git sparse-checkout` - they stay in history, and `--keep-code` keeps the source
without running it.

Where they run is a second, independent answer, and the default is the host - there
the dev server and the gates share one `node_modules`, so a lint run checks the tree
vite is actually serving. `.skelet-apps` lists only the ones you asked to containerize,
so an empty file means every frontend runs on the host with `./skelet serve <app>`.

Non-interactive variants: `./skelet setup --all`, `--backend-only`, or `./skelet setup --apps
dashboard,vetrina-demo` (comma list), with `--docker-apps LIST|none` for the placement and
`--tools LIST|none` for the optional dev tools.
Both answers are saved per-clone in the git-ignored `.skelet-profile`; re-run
`./skelet setup --apps ...` then `./skelet down && ./skelet up` to change them. Selecting an app
again restores its source.

### pnpm

The frontends install with pnpm, one project per app - each has its own
`pnpm-lock.yaml` and a `pnpm-workspace.yaml` holding its settings (overrides, and which
dependency install scripts may run). A host app needs the pnpm version its
`packageManager` field pins: `corepack enable`, or `npm install -g pnpm@<version>`, or
`pnpm self-update <version>` over an older one. `./skelet setup` refuses to start when a host
app has no pnpm, or not the pinned one, before it writes anything - offering to run the
frontends in Docker instead when it is interactive. `./skelet doctor` checks it from inside an
app, because that is where pnpm switches itself to the pinned version - and on Windows a
10.x switching itself to 12 has been seen to leave a pnpm that won't start, which
`self-update` fixes.

CI's `pnpm audit` checks each lockfile against the live advisory database, so it can start
failing with no commit at all. Most findings are a transitive dep the lockfile resolved before
the fix shipped, inside a range that already allows it - Renovate's weekly lockfile
maintenance picks those up, and to fix one sooner, trigger that PR from the dependency
dashboard or delete the app's `pnpm-lock.yaml` and run `./skelet pnpm <app> install`. An
override in `pnpm-workspace.yaml` is only for a fix the parent's range excludes - check with
`pnpm why <pkg>` - and its comment names the parent, so it can go once that parent widens.

A clone from before the switch to pnpm needs two things done once:

- **Host apps**: the old npm `node_modules` has to go - `./skelet setup` replaces it, and
  `./skelet pnpm` and `./skelet serve` refuse to run on it with the command to use.
- **Docker apps**: Docker fills a named volume from the image only while the volume is
  empty, so the container keeps serving the old npm tree after a rebuild. Drop the volume
  and rebuild:

  ```bash
  docker compose rm -sf dashboard vetrina-demo
  docker volume rm skelet_dashboard-node-modules skelet_vetrina-demo-node-modules
  ./skelet up --build
  ```

For the smallest possible backend-only clone, skipping even the *download* of the
frontends' file history:

```bash
git clone --filter=blob:none --no-checkout <this repo> skelet
cd skelet
git sparse-checkout set --no-cone '/*' '!/apps/dashboard/' '!/apps/channels/'
git checkout main
./skelet setup --backend-only
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
3. `./skelet setup --apps dashboard,<your-shop>` - deselecting `vetrina-demo` removes
   it from your working tree via sparse-checkout *without committing a deletion*,
   so upstream updates to the demo never conflict with your clone.
That's the whole list. Everything else follows the directory: `./skelet up`,
`./skelet pnpm`, `./skelet gen` and `./skelet release` all discover storefronts from
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

# Development workflow

Running the stack, the commands you'll use daily, regenerating what's
generated, and the cross-app contracts a change has to keep.

## Commands

`skelet` is the front door - bare for a grouped list of every command,
`skelet help <command>` for one, and [scripts.md](scripts.md) has the detail
on each. In a project the installed CLI runs that project's own Go program under
`cmd/skelet`, built on first use - a couple of seconds with host Go, a few more
through Docker.

| Command | Action |
|---|---|
| `skelet setup` | Guided first-run: asks what you want, writes the env files and secrets, boots, migrates. `--yes` for defaults, `--profile FILE` to replay answers |
| `skelet check [--codegen] [--list]` | **Every gate that can fail a commit, in one command** - version pins, repo structure, cross-app contracts. Under a second, no Docker, no backend. What the pre-commit hook and CI both run; `--list` explains each gate, `--codegen` adds the generated-file check |
| `skelet hooks` | Install a pre-commit hook that runs `skelet check`. Opt-in, and deliberately not the `--codegen` variant: needing a running backend to make a commit is a bad trade |
| `skelet up` | Start the full dev stack (ports exposed, hot reload, `APP_ENV=development`, all sidecars) |
| `skelet stop` | `docker compose stop` - halt without removing |
| `skelet down` | `docker compose down` - remove containers (volumes preserved) |
| `skelet restart` | `docker compose restart` - restart in place |
| `skelet ps` | List running containers |
| `skelet logs [service] [flags]` | Tail container logs (forwards `--tail`, `--since`, etc.) |
| `skelet sh` | Shell into the Go app container |
| `skelet go <args>` | Run go inside the app container (e.g. `skelet go build ./...`); `skelet go --signals ...` and `skelet go --sdk ...` target the other two modules |
| `skelet gen gqlgen <args>` | Run gqlgen inside the app container |
| `skelet gen sqlc <args>` | Run sqlc for both Go modules, core and signals (one-shot container) |
| `skelet db migrate <up\|down> [N]` | Run DB migrations |
| `skelet cli <args>` | Run the Cobra CLI inside the app container (`-it` only when stdin is a tty, so CI works) |
| `skelet psql` | psql shell into the dev database |
| `skelet pnpm dashboard <args>` | Run a pnpm command inside `apps/dashboard/` (e.g. `skelet pnpm dashboard run dev`, `skelet pnpm dashboard install`) |
| `skelet pnpm <name> <args>` | Run a pnpm command for a storefront under `apps/channels/` or `templates/` (e.g. `skelet pnpm nextjs-vetrina run build`, `skelet pnpm nextjs-vetrina run test`), or for a package under `packages/` by its directory name (`skelet pnpm storefront-core test`), which runs in a storefront's container - a running one, else one this clone runs. Exec's into the running container when the stack is up; otherwise spins up a one-shot. In Docker an `add`, `remove` or `update` runs over the workspace's manifests in a throwaway container, since the storefront's own can't write the lockfile |
| `skelet pnpm --lockfile` | Bring `pnpm-lock.yaml` up to date with the workspace's manifests, in a throwaway container - for a shop `skelet add channel` couldn't finish, or a manifest edited by hand |
| `skelet gen [--check]` | Regenerate everything: sqlc → gqlgen admin → gqlgen front → dashboard codegen. Pre-flights `/health` before the dashboard step. `--check` exits non-zero if any generated file changed (used by the pre-commit hook). Storefront codegen IS wired in - every storefront under `apps/channels/` and `templates/`, and every package under `packages/` with a codegen script, gets its TS docs regenerated (skipped for apps deselected via skelet setup). |
| `skelet test [pkgs] [--changed[=REF]] [--keep] [--no-frontend]` | Backend `go test` against disposable `skelet_test` / `skelet_test_signals` databases (fresh each run, `--keep` reuses them) + dashboard/storefront typechecks + both apps' vitest suites. Each Go module ends with a pass/fail/skip tally. Args after the flags forward to `go test`. `--changed` runs only what a change since REF (the merge base with develop by default) can reach - see below. |
| `skelet lint` | gofmt + golangci-lint + dashboard ESLint |

Full documentation for every command: [scripts.md](scripts.md).

### Testing only what changed

`skelet test --changed` is for the loop while you work. The plain `skelet test` is still the one to run before you push.

- Go keeps its test cache, so a package reruns only when its own code, something it imports, or the migrations changed. The migrations reach the cache through `SKELET_TEST_SCHEMA`, a hash of the migration files that `skelet test` passes in and `testdb` reads.
- Each frontend runs its typecheck, then only the vitest files that import a changed file (`vitest related`). An app nothing touched is skipped, and a change to its `package.json`, lockfile, tsconfig or vite/vitest config runs its whole suite. A change under `packages/`, or to the root `pnpm-workspace.yaml` or `pnpm-lock.yaml`, can reach every storefront, so each of them and each package runs whole.
- "Changed" is everything since REF plus whatever isn't committed yet. REF defaults to where the branch left develop, so `--changed=HEAD` is just the uncommitted work.

It can only see what Go and vitest can trace. A test that depends on the date, or a backend change that breaks a frontend through the API, still needs the full run.

### CLI examples

| Command | Action |
|---|---|
| `skelet cli admin create-superuser` | Interactive: create superuser |
| `skelet cli admin create` | Interactive: create admin + assign roles |
| `skelet cli admin list` | List admins with roles |
| `skelet cli role list` | List roles with member counts + permissions |
| `skelet cli integration list` | List API integrations |
| `skelet cli config list/get/set/reset/orphans/prune` | Config registry management |
| `skelet cli customer group list` | List customer groups |

## Docker setup

Three compose files - `docker-compose.yml` is the dev stack (`docker compose
up` is enough, no `-f` needed), `docker-compose.small.yml` the one-box tenant and
`docker-compose.prod.yml` the split-tier shape. Both deployment files carry a Caddy
edge behind `--profile edge`; leave the profile off to put your own proxy in front.

**Core services**:
- `app` - Go API, exposes :8080, mounts `./apps/backend` → `/var/www/backend` and works from `core/` under it.
- `nextjs-vetrina` - Next.js on `:3000`, `depends_on: app`. Image built from
  `templates/nextjs-vetrina/Dockerfile`. Each storefront is its own service,
  named after its directory under `apps/channels/` or `templates/`.
- `db` - Postgres 18 (alpine).
- `valkey` - Redis-compatible KV; JWT refresh tokens + caches.

**Dev sidecars**:
- `signals` - the Signals process, HTTP on `:8090` and the gRPC contract on `:8092`,
  started whenever `SIGNALS_ENABLED` is on in `env/go.env`. Core proxies `/t` and
  `/e` to it; the separate `ingest` tier is a scale-out option, off in dev.
- `nextjs-vetrina` - `next dev`, with the repo bind-mounted read-only at `/var/www/skelet`
  (the storefronts and `packages/` are one pnpm workspace). Only its own directory and
  `packages/` are writable, so no dependency the dev server runs can change what the host
  runs next - the CLI, git hooks, compose files - and `env/`, the root `.env` and the rest of
  `apps/` (the dashboard's e2e session) are masked, so none can read the backend's secrets.
  The container installs frozen: a manifest the lockfile doesn't match yet waits for
  `skelet pnpm --lockfile`, and `skelet pnpm <name> add|remove|update` runs over the
  workspace's manifests in a throwaway container instead of the read-only one. Named volumes
  shadow the workspace's `node_modules` and the app's `.next` (so host-built
  Windows/macOS artifacts can't poison the Linux/musl install or the SWC native
  binary) - an app's or a package's own `node_modules` is only links into the root one. `CHOKIDAR_USEPOLLING=true` +
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
profile of its own name: pick them in `skelet setup` (or `--tools LIST|none`), which
saves the list to git-ignored `.skelet-tools`, or start one for a single session with
`skelet up <name>`.

- `mailpit` - SMTP catcher; SMTP at `:1025`, web UI at <http://localhost:8025>.
  Without it, mail prints to the app log.
- `webhook-tester` - webhook listener UI at <http://localhost:8081>; point
  webhooks at `http://webhook-tester:8080/<any-path>`.
- `grafana` - Grafana with Tempo, Prometheus and Loki behind it, UI at
  <http://localhost:3030> (admin / admin). Takes the backends' OTLP traces and
  metrics on `grafana:4318`, and `grafana-alloy` beside it ships every skelet
  container's stdout into Loki.
- `garage-webui` - Garage's web console at <http://localhost:9901>.

`skelet up` wires the env files to match `.skelet-tools`: picking `mailpit` points
`SMTP_HOST` at it, picking `grafana` points the `OTEL_*` vars at it, and dropping either
blanks them again. Values you set to something else (a real relay, your own collector)
are left alone. A tool started ad hoc with `skelet up <name>` is not wired - add it to
`.skelet-tools` for that.

### Looking at another channel

A storefront instance is pinned to exactly one channel by `STOREFRONT_CHANNEL_ID`,
in dev as much as in production. To see a second channel, run a second instance
of the same app pinned to it - same source, same image, its own port and `.next`
volume - published at that channel's `public_url` so the dashboard's preview links
and the backend's Host-based channel resolution both line up. Put it in your
git-ignored `docker-compose.override.yml`; there's a worked example there if this
clone has one, and `skelet add channel` writes services in the same shape.

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

`skelet up` runs `docker compose up -d --build` - the default file is the dev stack.
Source edits hot-reload through the bind mounts - air for the app and the Signals process, Vite
and Next for the frontends. A frontend container also watches its own `package.json` and
lockfile, reinstalling and restarting its dev server when either changes (`watchexec` in its dev
Dockerfile's CMD). What still needs you is a Dockerfile change - `skelet up` rebuilds the
images. A new migration doesn't: the app applies it the next time it rebuilds or restarts.

Where the frontends run is your choice, recorded in `.skelet-apps`. Listed
there, `skelet up` starts them in Docker on <http://localhost:5173> and
<http://localhost:3000>; left out, they stay on the host and `skelet serve
dashboard` runs one - the default, because the dev server and the gates then
share one `node_modules`. Either way `skelet pnpm dashboard` / `skelet pnpm` run pnpm against
whichever tree is live. Every command is documented in [scripts.md](scripts.md).

## Codegen flow

The generated artifacts must always agree with the source-of-truth
schemas. A mismatch surfaces as an import error that white-screens the
dashboard, so treat every schema/query change as "must regenerate":

1. **`*.sql` in `apps/backend/core/internal/db/query/`** → `skelet gen sqlc generate`.
   Manually delete stale `apps/backend/core/internal/db/sqlc/<old>.sql.go`
   when tables are dropped.
2. **`*.graphqls` in `apps/backend/core/graph/schema/<surface>/`** →
   `skelet gen gqlgen generate --config gqlgen.<surface>.yml` (or
   `skelet gen` to do both surfaces). Schema files are paired with
   resolver files by name - `cart.graphqls` ↔ `cart.resolvers.go`.
3. **`*.graphql` in `apps/dashboard/src/features/<domain>/`** →
   `skelet pnpm dashboard run codegen` (backend must be running - codegen fetches
   the live admin schema over HTTP).
4. **`*.graphql` in `packages/storefront-core/`**, beside the module that runs them → `skelet gen`,
   which runs the package's codegen in a storefront's container (backend must be running - codegen
   fetches the live front schema at `app:8080/api/front/graphql`). A storefront with operations of its
   own gives itself a `codegen` script, and `skelet gen` runs that too.

`skelet gen` runs all four, discovering storefronts from
`apps/channels/` and `templates/` rather than naming them. Steps 3 and 4 introspect a
live server, so `--no-frontend` stops after step 2 when the stack isn't up.

`skelet gen --check` regenerates and fails if the output would differ
from what's committed, which is how a stale generated file gets caught
before it white-screens the dashboard. It is
**not** in the pre-commit hook - it needs the container, and steps 3 and 4
need the backend, so gating every commit on a running stack is the wrong
trade. Run it yourself before a PR that touched a `.sql`, `.graphqls` or
`.graphql` file. The hook runs `skelet check`, which needs no backend.

## Cross-app contracts

Some invariants span two or three apps: the CMS block-type list (backend
validator, dashboard palette, storefront renderer), the
anonymous-cacheable operation list (backend allowlist, storefront
token-stripper), the lexical node types the dashboard authors and the
storefront renders, the conditional-option evaluator (ported into both
frontends from `internal/cart/options_visibility.go`), and the security
headers the backend and the dashboard's nginx both set.

Each end carries a `SYNC` or `SYNC SENTINEL` comment, and
`skelet check contracts` enforces the mechanical part - including that the
paths those comments name still exist, since a `SYNC` comment pointing at
a moved file is worth nothing. It runs on the host, because the app
containers each mount only their own tree and no single test runner can
see all three sides.

Adding a contract means a `SYNC` comment at both ends **and** a check in
that script. Every check there fails when its extractor matches nothing,
so a moved source is a red build instead of a quiet pass.

## First run

`skelet setup` asks a short set of questions and writes the answers out, so nothing has to be
found in a template first:

| Question | Writes |
| --- | --- |
| Which apps | `SKELET_APPS_SELECTED` in the profile, plus sparse-checkout of the rest |
| Where the frontends run | `.skelet-apps` - empty means the host, which is the default |
| Deployment target - dev / small / prod | materializes only that target's env files |
| Suites - Signals, AI | `SIGNALS_ENABLED` / `AI_ENABLED` |
| Search tier | `OPENSEARCH_URL` in the root `.env` |
| Payment gateways | `apps/backend/core/plugins/plugins.json`, then `skelet gen` if it changed |
| Hide the admin API | `ADMIN_API_SLUG` + the matching dashboard URLs |

Secrets are generated, never prompted, and an existing non-placeholder value is left alone -
so re-running is safe and never rotates a key. Setup is part of the Go CLI, so generating them
needs nothing on the host - no OpenSSL. The env files it creates are readable by you alone. The answers land in git-ignored
`.skelet-profile`, which becomes the defaults next time and which `skelet doctor` reads back to
tell you what this clone is meant to be.

Non-interactive: `skelet setup --yes` takes the defaults; `skelet setup --profile <file>` replays a
saved profile, which is what CI and a second clone want. The file is only read - the answers are
saved to this clone's `.skelet-profile` like any other run's. Where the profile has no answer for
where the frontends run, `.skelet-apps` keeps what it holds rather than being emptied.

## Which apps run

The backend (GraphQL APIs + workers) always runs. The admin dashboard and your
storefronts under `apps/channels/` are optional, and selected unless you say no, so a
clone that ships its own frontend can run headless. The templates under `templates/` are
left out unless you ask for one: setup is for working on your own store, and a template
is what that store is copied from - run one only to work on the template itself.
Deselected apps are removed from your working tree via `git sparse-checkout` - they stay
in history, where `skelet add channel` still copies a template from, and `--keep-code`
keeps the source without running it.

Where they run is a second, independent answer, and the default is the host - there
the dev server and the gates share one `node_modules`, so a lint run checks the tree
vite is actually serving. `.skelet-apps` lists only the ones you asked to containerize,
so an empty file means every frontend runs on the host with `skelet serve <app>`.

When the project has no storefront of its own yet, setup offers one: pick a template and a
name, and it's copied into `apps/channels/<name>` the way `skelet add channel` copies one,
selected and run where your other frontends run. `--template <name>` and `--shop <name>`
make it without asking - and since `skelet new` hands setup its flags, `skelet new my-store
--template astro-shop` starts a project with its own Astro shop.

Non-interactive variants: `skelet setup --all` (templates too), `--backend-only`, or
`skelet setup --apps dashboard,nextjs-vetrina` (comma list), with `--docker-apps LIST|none` for the placement and
`--tools LIST|none` for the optional dev tools.
Both answers are saved per-clone in the git-ignored `.skelet-profile`; re-run
`skelet setup --apps ...` then `skelet down && skelet up` to change them. Selecting an app
again restores its source.

### pnpm

The frontends install with pnpm. The storefronts, the templates and `packages/` are one
workspace: the root `pnpm-workspace.yaml` lists them and holds its settings (overrides,
which dependency install scripts may run, and the catalog of versions they share, so a
package and the app using it resolve the same React), and the root `pnpm-lock.yaml` is
their one lockfile. The dashboard is a project of its own, with both files beside its
`package.json`. A host app needs the pnpm version its
`packageManager` field pins: `corepack enable`, or `npm install -g pnpm@<version>`, or
`pnpm self-update <version>` over an older one. `skelet setup` refuses to start when a host
app has no pnpm, or not the pinned one, before it writes anything - offering to run the
frontends in Docker instead when it is interactive. `skelet doctor` checks it from inside an
app, because that is where pnpm switches itself to the pinned version - and on Windows a
10.x switching itself to 12 has been seen to leave a pnpm that won't start, which
`self-update` fixes.

CI's `pnpm audit` checks each lockfile against the live advisory database, so it can start
failing with no commit at all. Most findings are a transitive dep the lockfile resolved before
the fix shipped, inside a range that already allows it - Renovate's weekly lockfile
maintenance picks those up, and to fix one sooner, trigger that PR from the dependency
dashboard or delete the lockfile and run `skelet pnpm <app> install` (the dashboard's, or
the root one for the storefronts). An override in `pnpm-workspace.yaml` is only for a fix the parent's range excludes - check with
`pnpm why <pkg>` - and its comment names the parent, so it can go once that parent widens.

A clone from before the switch to pnpm needs two things done once:

- **Host apps**: the old npm `node_modules` has to go - `skelet setup` replaces it, and
  `skelet pnpm` and `skelet serve` refuse to run on it with the command to use.
- **Docker apps**: Docker fills a named volume from the image only while the volume is
  empty, so the container keeps serving the old npm tree after a rebuild. Drop the volume
  and rebuild:

  ```bash
  docker compose rm -sf dashboard nextjs-vetrina
  docker volume rm skelet_dashboard-node-modules skelet_nextjs-vetrina-node-modules
  skelet up --build
  ```

For the smallest possible backend-only clone, skipping even the *download* of the
frontends' file history:

```bash
git clone --filter=blob:none --no-checkout <this repo> skelet
cd skelet
git sparse-checkout set --no-cone '/*' '!/apps/dashboard/' '!/apps/channels/' '!/templates/' '!/packages/'
git checkout main
skelet setup --backend-only
```

### Taking a new release

A project from `skelet new` has Skelet as its `upstream` remote and your work on
`main`, so a release is a merge like any other:

```bash
git fetch upstream --tags
git merge v0.4.0
```

Read that release's changelog first - while Skelet is `0.x`, a minor version can
break things. Migrations run when the app boots, so `skelet up` afterwards is the
whole upgrade. Your own remote, once you add one, is `origin`; `upstream` stays
Skelet's.

Leave the core's dependency bumps to the releases. The `.github/renovate.json5` you
inherit covers the whole tree, so Renovate turned on in a project would bump the backend,
the dashboard and the shared packages that the next release bumps too, and each merge
would conflict in their manifests and the lockfile. Keep it to your own shops with
`includePaths: ["apps/channels/**"]`, or leave it off. The release workflow you inherit
does nothing outside Skelet's own repository, so tagging your project publishes nothing.

### Adding your own storefront

One directory per shop under `apps/channels/<name>/`; the directory name
doubles as the compose service and profile name. A shop starts as a copy of a
template under `templates/`, and the copy is yours to change. The templates and
the `packages/` they build on stay upstream's - leave them as they are, and
merging a skelet release brings their fixes without touching your shop.

`skelet add channel <your-shop>` does steps 1 and 2 for you - it copies a
template (`--template <name>`, by default `nextjs-vetrina`, the full one) or a
shop of your own (`--from <name>`), points every path in it at its new home, gives
it the first free ports, copies the compose service out of the source's (so it
inherits changes to the template's wiring rather than a snapshot taken once), adds
it to the workspace lockfile, and writes nothing until the result parses - the copy is made aside and moved into
place last, so a failure leaves nothing half-made. A template setup left out of your working tree is copied from git,
so it never has to be checked out. The lockfile refresh runs in a throwaway container over the workspace's manifests -
one setup left out comes from git too, so the lockfile keeps it - and if it can't run, `skelet pnpm --lockfile`
finishes the job once the cause is fixed. `--tracked` puts the
service in the tracked compose files instead, which is for adding a storefront to
skelet itself and gives up the conflict-free property below. `skelet rm channel
<your-shop>` reverses it, keeping the Docker volumes unless you pass `--volumes`.
By hand:

1. Create `apps/channels/<your-shop>/` - a copy of `templates/nextjs-vetrina` with its
   `templates/nextjs-vetrina` paths (the Dockerfiles, their `.dockerignore`) pointed at
   `apps/channels/<your-shop>`, or from scratch - then `skelet pnpm --lockfile`, so the
   lockfile knows it.
2. Declare its compose service in `docker-compose.override.yml` (git-ignored,
   auto-merged by docker compose) with `profiles: ["<your-shop>"]`. Never edit
   the tracked compose files, so `git pull` from upstream skelet stays
   conflict-free. Anchors don't cross compose files, so a block copied from
   `docker-compose.yml` needs its own `x-logging: &logging` beside it.
3. `skelet setup --apps dashboard,<your-shop>` - leaving `nextjs-vetrina` out removes
   it from your working tree via sparse-checkout *without committing a deletion*,
   so upstream updates to the template never conflict with your clone.
That's the whole list. Everything else follows the directory: `skelet up`,
`skelet pnpm`, `skelet gen` and `skelet release` all discover storefronts from
`apps/channels/` and `templates/`, CI installs and checks the whole workspace in one job and derives its image
builds, each smoke-run, the same way, and Renovate - kept to `apps/channels/` as above - finds your `package.json`
and `Dockerfile` on its own.
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

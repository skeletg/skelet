# Commands

Every dev command, in detail. `./skelet` is the front door: run it bare for a
grouped list, `./skelet help <command>` for one. The scripts under `bin/` are
what each verb runs, and you shouldn't need to call one by name - the map
between them is at the bottom, for when you're reading the source.

They all `cd` to the repo root on their own, so you can run them from anywhere.

**Running it without the `./`.** Symlink it into a directory already on your PATH:

```bash
ln -s "$PWD/skelet" ~/.local/bin/skelet
```

It resolves the symlink itself (by hand, since macOS has no `readlink -f`) to
find its own `bin/`. Two clones don't collide: it walks up from your working
directory first and uses the checkout you're in, falling back to the symlinked
one when you're outside a checkout entirely.

Three things they share (living in `_lib`, which is sourced, not run):

- **One compose file set** - `docker-compose.yml` is the whole dev stack
  including sidecars, and compose auto-merges your git-ignored
  `docker-compose.override.yml` on its own, so `_lib` sets no
  `COMPOSE_FILE` at all. `stop`, `down`, `ps` and friends therefore cover
  Garage, Mailpit and co. as well as the apps.
- **Profiles follow your app selection** - `_lib` exports
  `COMPOSE_PROFILES` built from the sidecars (`search,tracing`, plus
  `signals` unless `env/go.env` switches `SIGNALS_ENABLED` off - that profile
  carries the Signals process and its ClickHouse) and
  whatever `.skelet-apps` lists, so the apps you picked in `setup` come up
  and the ones you skipped stay down (see `dev --lean` below to drop the
  sidecars).
- **Files stay yours** - containers run as your uid, so generated code
  doesn't end up root-owned and break `git pull`. Set
  `SKELET_NO_USER_MAP=1` if you actually need root inside a container.

## Starting and stopping the stack

| Command | What it does |
|---|---|
| `./skelet up` | Start the full dev stack: backend with hot reload, dashboard on :5173, storefront on :3000, plus the sidecars - Garage (S3 :9900, console :9901), Mailpit (:8025), Jaeger (:16686), webhook-tester (:8081). Idempotent - re-run any time, it only touches what changed. `./skelet up --lean` skips OpenSearch, Jaeger and the Signals process with its ClickHouse (~1GB+ less RAM; search returns empty, traces go nowhere, and the dashboard's Signals pages have no suite to ask). |
| `./skelet stop` | Halt everything (sidecars too) without removing containers. |
| `./skelet down` | Remove the containers. Volumes (your data) survive. |
| `./skelet restart` | Restart containers in place. For a full recreate: `down` then `dev`. |
| `./skelet ps` | List running containers. |
| `./skelet serve` | Run one frontend on the HOST instead of in its container - `./skelet serve dashboard`, `./skelet serve vetrina-demo`. It exports the root `.env` first, because Next only reads `.env` files from its own directory and would otherwise need a second copy of `STOREFRONT_CHANNEL_ID` that drifts from the one compose reads. Trailing args go to the dev server, with no `--` in front - pnpm passes that through verbatim. |
| `./skelet logs` | Tail logs - `./skelet logs app`, `./skelet logs --tail 50 garage`. |
| `./skelet doctor` | See "Other tools" below - worth running right after `setup`, it's what catches a datastore the config expects but the stack isn't running. |
| `./skelet setup` | Guided first-run. Asks which apps you want (backend always does; deselected apps also leave the working tree via sparse-checkout unless `--keep-code`) and, separately, whether to run the frontends in Docker rather than on the host - the host is the default, and `.skelet-apps` lists only the containerized ones, the deployment target, which suites and engines you want, which payment gateways compile in, and whether to hide the admin API. Writes the target's env files with generated secrets - an existing non-placeholder value is never overwritten - plus the root `.env`, `plugins.json` and git-ignored `.skelet-profile`. Then boots the stack with `--lean`, migrates, and regenerates when the plugin set changed. Safe to re-run. `--yes` takes defaults, `--profile FILE` replays answers, `--all` / `--backend-only` / `--apps LIST` set the selection directly, and `--docker-apps LIST|none` sets the placement. |

## Running things inside containers

| Command | What it does |
|---|---|
| `./skelet sh` | Shell into the app container. |
| `./skelet go` | Run go in the app container - `./skelet go build ./...`, `./skelet go test ./internal/...`. `--signals` or `--sdk` first runs it in that module instead. |
| `./skelet cli` | The skelet ops CLI - `./skelet cli admin list`, `./skelet cli media-migrate ...`, `./skelet cli job run customer-roster-push` to run a scheduled job now. Works with pipes and CI (only grabs a TTY when there is one). |
| `./skelet pnpm dashboard` | pnpm in the dashboard container - `./skelet pnpm dashboard run codegen`, `./skelet pnpm dashboard run lint`. Vite is already running via `./skelet up`, so no need for `run dev`. |
| `./skelet pnpm <name>` | Same for storefronts (`apps/channels/<name>/`) - `./skelet pnpm <name> [name] run codegen`, `run build`; the name is optional when only one storefront exists. Note: storefront codegen/tsc only works right in the container; host runs show phantom errors (no `.next/types`). |
| `./skelet psql` | psql shell into the dev database. |

If the stack is up, these exec into the running container (instant). If
not, they spin up a one-off. Works either way.

## The gates

**`./skelet check` is the one you need to know.** It runs every gate below, takes
about a second, needs no running backend, and is exactly what the pre-commit
hook and CI run. The one step that wants Docker is the proto compatibility check
against `main`, which runs in the app container when the stack is up and says it
skipped when it isn't - CI runs it on the host toolchain instead. `./skelet check --list` prints what each gate is
for. The individual scripts exist so you can rerun one while fixing it.

| Script | What it protects |
|---|---|
| `./skelet check` | All three gates, in one command. `--codegen` adds the generated-file check (needs the container, and the backend for its TS half). `--list` explains each gate. |
| `./skelet check pins` | Version pins that live in several files at once: postgres, clickhouse, dbmate, golangci-lint, node (plus `.nvmrc`), alpine, and the `golang:` images against `go.mod`. Prints every site when they disagree. Add a pin here when you introduce one more than one file has to know. |
| `./skelet check structure` | Structural invariants a fresh clone or a forked storefront leans on: every `bin/*` script parses and is executable, no doc or workflow names a script that doesn't exist, every compose `env_file` has a committed `.example`. |
| `./skelet check contracts` | The cross-app contracts the `SYNC` comments describe: every path a SYNC comment names still exists, CMS block types agree across backend / dashboard / storefront, the anonymous-cacheable operation lists match, every lexical node the dashboard authors has a renderer, the two conditional-option evaluators are still the same code, the security-header copies match, and the Signals proto has no breaking change against `main` (`origin/main` in CI, where a missing ref fails rather than passing), or against `develop` while `main` has no proto yet. Each check fails if its extractor matches nothing, so it can't rot into a no-op. |
| `./skelet hooks` | Points `.git/hooks/pre-commit` at `./skelet check`. Opt-in; bypass once with `git commit --no-verify`. |

## Codegen, lint and tests

| Command | What it does |
|---|---|
| `./skelet gen` | Regenerate everything in order: the Signals wire contract (buf), sqlc, gqlgen (both surfaces), the plugin bridge, the remote-provider contract, the generated reference tables in `docs/` (permissions, webhook events, settings), dashboard TS types, storefront TS types. Needs the backend up for the last two steps, or pass `--no-frontend` to stop before them. `--check` fails on stale output instead of writing it. |
| `./skelet gen sqlc` | sqlc for both Go modules, each from its own `sqlc.yaml` - pinned to 1.31.1 so regens don't rewrite every version header. |
| `./skelet gen buf` | buf against `proto/`, for one command at a time - `./skelet gen buf lint /var/www/proto`. It runs from the Go module, since that is where `go tool` finds buf and its two protoc plugins; nothing is installed outside `go.mod`. |
| `./skelet gen gqlgen` | One gqlgen surface at a time. Usually you want `codegen` instead. |
| `./skelet lint` | gofmt + golangci-lint are hard gates, run over both Go modules; dashboard + storefront ESLint run after but don't fail the script (matches CI, which keeps frontend lint non-blocking until it's clean). |
| `./skelet test` | `go test` in both Go modules + dashboard/storefront typechecks + dashboard and storefront unit tests (vitest) - the same hard gates CI runs. Args are forwarded to core's `go test`; `--no-frontend` skips the frontend steps. The default sweep excludes each module's `internal/e2e` (they write real rows and never clean up) - run those with `test-e2e`. |
| `./skelet test e2e` | Run both e2e suites against dedicated disposable databases (`skelet_e2e` / `skelet_e2e_signals`) so the dev DB never accumulates test orders/customers. Starts a Signals process against the disposable signals database first, since core's scenarios reach the suite over the contract, then runs core's scenarios and the suite's own. Drops and recreates the databases every run - kept, they grow until the store fixture costs seconds instead of milliseconds. `--keep` reuses them while iterating on one scenario. Extra args forward to `go test` (e.g. `-run TestFoo`). |
| `./skelet test storefront [name]` | Browser tests for a storefront (Playwright, `apps/channels/<name>/e2e/`) against its running dev container. Runs in Microsoft's Playwright image inside the storefront container's network namespace, so the host needs no Node or browser and the page is served as `localhost` (the only origin `next dev` hydrates for). Each spec signs up throwaway `e2e-*@skelet.test` customers in the dev database. Extra args forward to Playwright (`-g addresses`, a spec path). Headless - there is no display in the container. |
| `./skelet provider` | **Not a gate** - a conformance tester you point at a remote provider endpoint: signed envelope, forged/unsigned/wrong-key rejection, replay window, timeout budget, 256 KiB response ceiling, cache stability. `./skelet provider --url URL --secret rpsec_... --kind tax_provider`. Posts the generated example for the kind unless you pass `--payload`. Contract: [remote-providers.md](remote-providers.md). |

## Database

| Command | What it does |
|---|---|
| `./skelet db migrate` | `./skelet db migrate up`, `down 1`, `version`, `force <v>`. Runs inside the container, so `DATABASE_URL` is always the right one. `up` is mostly a no-op: the `migrate` one-shot in the compose file applies the stream on every `docker compose up` and gates the backend roles on it finishing, so what you actually need this for is `down`, `force` and `version`. |
| `./skelet db migrate --signals` | The same, against the Signals database (`SIGNALS_DB_URL`) - the `signals-migrate` one-shot gates the same way. |
| `./skelet db export` | Dump the dev DB into the gitignored `backups/` dir (timestamped `.dump`; pass a `.sql` path for plain SQL, or any explicit path). |
| `./skelet db import` | Restore a dump. **Overwrites the dev DB**, so it asks for confirmation first (`-y` to skip in scripts). |

## Deployment

| Command | What it does |
|---|---|
| `./skelet release` | Build the prod images - backend, dashboard, and every storefront under `apps/channels/` - and push them to a container registry (e.g. DigitalOcean Container Registry). `REGISTRY=registry.digitalocean.com/<name> ./skelet release [tag]` - tag defaults to the git short sha, images build for linux/amd64. Prints the `*_IMAGE` vars `docker-compose.prod.yml` reads, one per image, the storefront ones named after the directory. Authenticate first with `doctl registry login`. |

The two shapes those images go into - `docker-compose.small.yml` (one
container, one Postgres) and `docker-compose.prod.yml` (split tiers) - are
in the README under Operations > Deploying. Neither has a `./skelet` verb:
they take a plain `docker compose -f <file> up -d` on the server.

## Other tools

| Command | What it does |
|---|---|
| `./skelet search` | Query the search cluster without writing the DSL by hand: `status`, `products <q>`, `blog <q>`, `show products <id>`, `raw GET /_cluster/health`. Reindexing lives in `./skelet cli search reindex`. |
| `./skelet new` | Scaffold a storefront: `./skelet new channel <name> [--from <storefront>] [--tracked]`. Forks an existing shop under `apps/channels/`, allocates the first free dev and small-tier ports across every compose file, and copies the source's compose service rather than templating one here, so it inherits wiring changes instead of drifting. The service lands in the git-ignored `docker-compose.override.yml` (the convention in [workflow.md](workflow.md) - it keeps upstream pulls conflict-free) along with any YAML anchors it references, since those don't cross compose files. `--tracked` writes the dev, prod and small files instead, for adding a storefront to skelet itself. Nothing is written until every generated file passes `docker compose config`, so a failure leaves the tree untouched. |
| `./skelet rm` | Undo a scaffold: `./skelet rm channel <name> [-y] [--volumes]`. Finds the service wherever `new` put it, strips it and its volume declarations, deletes the directory and env templates, stops the container, and drops `.skelet-apps` back. Prints everything it will delete with sizes and asks first, saying whether the directory is committed (recoverable) or untracked (not). Volumes are **kept** unless you pass `--volumes` - that's the part you can't undo, and a storefront's `.next` cache runs to gigabytes. Validates the edited compose files before writing, same as `new`; an override left with no services is deleted rather than written empty. |
| `./skelet fixperms` | Find and fix root-owned files under `apps/` left behind by container writes. Only asks for sudo when there's something to fix. |
| `./skelet doctor` | Sanity-check the environment: docker up, env files present, containers running, ports answering, backend /health, `*.localhost` resolution, disk space. Run it when something feels off. |

## The scripts behind the verbs

| Verb | Runs |
|---|---|
| `up` / `down` / `stop` / `restart` / `ps` | `dev` / `down` / `stop` / `restart` / `ps` |
| `serve` | `serve` |
| `logs [service]` | `logs` |
| `sh`, `go`, `cli`, `psql` | `bash`, `go`, `cli`, `psql` |
| `pnpm <app> <args>` | `dash` when app is `dashboard`, else `channel <app>` - `npm` is kept as an alias from before the switch |
| `db migrate [--signals]`, `db export`, `db import`, `db shell` | `migrate`, `migrate-signals`, `db-export`, `db-import`, `psql` |
| `gen`, `gen sqlc`, `gen gqlgen`, `gen buf` | `codegen`, `sqlc`, `gqlgen`, `buf` |
| `check`, `check pins\|structure\|contracts`, `check codegen` | `check`, the three gates, `codegen --check` |
| `test`, `test e2e`, `test storefront`, `lint` | `test`, `test-e2e`, `test-storefront`, `lint` |
| `setup`, `doctor`, `hooks`, `fixperms` | `setup`, `doctor`, `install-hooks`, `fixperms` |
| `search`, `release`, `provider` | `opensearch`, `push-images`, `provider-check` |
| `new channel <name>`, `rm channel <name>` | `new`, `remove` |

`bin/repo-check` fails if `./skelet` routes to a script that doesn't exist, so
the table above can't quietly rot when a script is renamed.

---

Adding or removing a script? Update this file too. The table in
[workflow.md](workflow.md) has the short version; this one has the details.

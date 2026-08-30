# Scripts

Cheat sheet for everything in `bin/`. The scripts all `cd` to the repo root on
their own, so you can run them from anywhere.

## The front door

`./skelet` is one entry point for all of it. Run it bare for a grouped list of
every command, `./skelet help <command>` for detail on one. It's a router: each
verb execs the `bin/` script named below, unchanged, so behaviour is identical
either way and every `bin/*` path in these docs still works.

Reach for the verb when you want to find something, the script when you already
know its name.

**Running it without the `./`.** Symlink it into a directory already on your PATH:

```bash
ln -s "$PWD/skelet" ~/.local/bin/skelet
```

It resolves the symlink itself (by hand, since macOS has no `readlink -f`) to
find its own `bin/`. Two clones don't collide: it walks up from your working
directory first and uses the checkout you're in, falling back to the symlinked
one when you're outside a checkout entirely.

| Verb | Runs |
|---|---|
| `up` / `down` / `stop` / `restart` / `ps` | `dev` / `down` / `stop` / `restart` / `ps` |
| `logs [service]`, `logs query "<where>"` | `logs`, `logs-query` |
| `sh`, `go`, `cli`, `psql` | `bash`, `go`, `cli`, `psql` |
| `npm <app> <args>` | `dash` when app is `dashboard`, else `channel <app>` |
| `db migrate [--signals]`, `db export`, `db import`, `db shell` | `migrate`, `migrate-signals`, `db-export`, `db-import`, `psql` |
| `gen`, `gen sqlc`, `gen gqlgen` | `codegen`, `sqlc`, `gqlgen` |
| `check`, `check pins\|structure\|contracts`, `check codegen` | `check`, the three gates, `codegen --check` |
| `test`, `test e2e`, `lint` | `test`, `test-e2e`, `lint` |
| `signals backfill-ch` | `signals-ch-backfill` |
| `setup`, `doctor`, `hooks`, `fixperms` | `setup`, `doctor`, `install-hooks`, `fixperms` |
| `search`, `release`, `provider` | `opensearch`, `push-images`, `provider-check` |
| `new channel <name>`, `rm channel <name>` | `new`, `remove` |

`bin/repo-check` fails if `./skelet` routes to a script that doesn't exist, so
the table above can't quietly rot when a script is renamed.

Three things they share (living in `_lib`, which is sourced, not run):

- **One compose file set** - `docker-compose.yml` is the whole dev stack
  including sidecars, and compose auto-merges your git-ignored
  `docker-compose.override.yml` on its own, so `_lib` sets no
  `COMPOSE_FILE` at all. `stop`, `down`, `ps` and friends therefore cover
  Garage, Mailpit and co. as well as the apps.
- **Profiles follow your app selection** - `_lib` exports
  `COMPOSE_PROFILES` built from the sidecars (`search,tracing`) plus
  whatever `.skelet-apps` lists, so the apps you picked in `setup` come up
  and the ones you skipped stay down (see `dev --lean` below to drop the
  sidecars).
- **Files stay yours** - containers run as your uid, so generated code
  doesn't end up root-owned and break `git pull`. Set
  `SKELET_NO_USER_MAP=1` if you actually need root inside a container.

## Starting and stopping the stack

| Script | What it does |
|---|---|
| `dev` | Start the full dev stack: backend with hot reload, dashboard on :5173, storefront on :3000, plus the sidecars - Garage (S3 :9900, console :9901), Mailpit (:8025), Jaeger (:16686), webhook-tester (:8081). Idempotent - re-run any time, it only touches what changed. `bin/dev --lean` skips OpenSearch and Jaeger (~1GB less RAM; search returns empty, traces go nowhere). |
| `stop` | Halt everything (sidecars too) without removing containers. |
| `down` | Remove the containers. Volumes (your data) survive. |
| `restart` | Restart containers in place. For a full recreate: `down` then `dev`. |
| `ps` | List running containers. |
| `serve` | Run one frontend on the HOST instead of in its container - `bin/serve dashboard`, `bin/serve vetrina-demo`. It exports the root `.env` first, because Next only reads `.env` files from its own directory and would otherwise need a second copy of `STOREFRONT_CHANNEL_ID` that drifts from the one compose reads. Trailing args go to npm. |
| `logs` | Tail logs - `bin/logs app`, `bin/logs --tail 50 garage`. |
| `logs-query` | Search log HISTORY in ClickHouse (needs `--profile clickhouse --profile logs`, which runs the Vector collector). Args are a WHERE clause: `bin/logs-query "level = 'error'"`. Container logs rotate at 10 MB x 3, so this is the only durable copy. |
| `doctor` | See "Other tools" below - worth running right after `setup`, it's what catches a datastore the config expects but the stack isn't running. |
| `setup` | Guided first-run. Asks which apps run (backend always does; selection in git-ignored `.skelet-apps`, deselected apps also leave the working tree via sparse-checkout unless `--keep-code`), the deployment target, which suites and engines you want, which payment gateways compile in, and whether to hide the admin API. Writes the target's env files with generated secrets - an existing non-placeholder value is never overwritten - plus the root `.env`, `plugins.json` and git-ignored `.skelet-profile`. Then boots via `dev --lean`, migrates, and runs `codegen` when the plugin set changed. Safe to re-run. `--yes` takes defaults, `--profile FILE` replays answers, `--all` / `--backend-only` / `--apps LIST` still set the app selection directly. |

## Running things inside containers

| Script | What it does |
|---|---|
| `bash` | Shell into the app container. |
| `go` | Run go in the app container - `bin/go build ./...`, `bin/go test ./internal/...`. |
| `cli` | The skelet ops CLI - `bin/cli admin list`, `bin/cli media-migrate ...`. Works with pipes and CI (only grabs a TTY when there is one). |
| `dash` | npm in the dashboard container - `bin/dash run codegen`, `bin/dash run lint`. Vite is already running via `bin/dev`, so no need for `run dev`. |
| `channel` | Same for storefronts (`apps/channels/<name>/`) - `bin/channel [name] run codegen`, `run build`; the name is optional when only one storefront exists. Note: storefront codegen/tsc only works right in the container; host runs show phantom errors (no `.next/types`). |
| `psql` | psql shell into the dev database. |

If the stack is up, these exec into the running container (instant). If
not, they spin up a one-off. Works either way.

## The gates

**`bin/check` is the one you need to know.** It runs every gate below, takes
under a second, needs no Docker and no running backend, and is exactly what
the pre-commit hook and CI run. `bin/check --list` prints what each gate is
for. The individual scripts exist so you can rerun one while fixing it.

| Script | What it protects |
|---|---|
| `check` | All three gates, in one command. `--codegen` adds the generated-file check (needs the container, and the backend for its TS half). `--list` explains each gate. |
| `pin-check` | Version pins that live in several files at once: postgres, clickhouse, golang-migrate, golangci-lint, node (plus `.nvmrc`), alpine, and the `golang:` images against `go.mod`. Prints every site when they disagree. Add a pin here when you introduce one more than one file has to know. |
| `repo-check` | Structural invariants a fresh clone or a forked storefront leans on: every `bin/*` script parses and is executable, no doc or workflow names a script that doesn't exist, every compose `env_file` has a committed `.example`. |
| `contract-check` | The cross-app contracts the `SYNC` comments describe: every path a SYNC comment names still exists, CMS block types agree across backend / dashboard / storefront, the anonymous-cacheable operation lists match, every lexical node the dashboard authors has a renderer, the two conditional-option evaluators are still the same code, and the security-header copies match. Each check fails if its extractor matches nothing, so it can't rot into a no-op. |
| `install-hooks` | Points `.git/hooks/pre-commit` at `bin/check`. Opt-in; bypass once with `git commit --no-verify`. |

## Codegen, lint and tests

| Script | What it does |
|---|---|
| `codegen` | Regenerate everything in order: sqlc, gqlgen (both surfaces), the plugin bridge, the remote-provider contract, the generated reference tables in `docs/` (permissions, webhook events, settings), dashboard TS types, storefront TS types. Needs the backend up for the last two steps, or pass `--no-frontend` to stop before them. `--check` fails on stale output instead of writing it. |
| `sqlc` | sqlc, pinned to 1.31.1 so regens don't rewrite every version header. |
| `gqlgen` | One gqlgen surface at a time. Usually you want `codegen` instead. |
| `lint` | gofmt + golangci-lint are hard gates; dashboard + storefront ESLint run after but don't fail the script (matches CI, which keeps frontend lint non-blocking until it's clean). |
| `test` | Backend `go test` + dashboard/storefront typechecks + dashboard and storefront unit tests (vitest) - the same hard gates CI runs. Args are forwarded to `go test`; `--no-frontend` skips the frontend steps. The default sweep excludes `internal/e2e` (it writes real rows and never cleans up) - run that with `test-e2e`. |
| `test-e2e` | Run the e2e suite against dedicated disposable databases (`skelet_e2e` / `skelet_e2e_signals`) so the dev DB never accumulates test orders/customers. Creates, marks and migrates them on first run, reuses after; `--fresh` drops and recreates them first. Extra args forward to `go test` (e.g. `-run TestFoo`). |
| `provider-check` | **Not a gate** - a conformance tester you point at a remote provider endpoint: signed envelope, forged/unsigned/wrong-key rejection, replay window, timeout budget, 256 KiB response ceiling, cache stability. `bin/provider-check --url URL --secret rpsec_... --kind tax_provider`. Posts the generated example for the kind unless you pass `--payload`. Contract: [remote-providers.md](remote-providers.md). |

## Database

| Script | What it does |
|---|---|
| `migrate` | `bin/migrate up`, `down 1`, `version`, `force <v>`. Runs inside the container, so `DATABASE_URL` is always the right one. `up` is mostly a no-op: the `migrate` one-shot in the compose file applies the stream on every `docker compose up` and gates the backend roles on it finishing, so what you actually need this for is `down`, `force` and `version`. |
| `migrate-signals` | The same, against the Signals database (`SIGNALS_DB_URL`) - the `signals-migrate` one-shot gates the same way. |
| `signals-ch-backfill` | Backfill the ClickHouse spine from the Signals Postgres via ClickHouse's own `postgresql()` function (no dump files). Run after `clickhouse-migrate`; safe either side of flipping `SIGNALS_ENGINE`, and safe to re-run (ReplacingMergeTree collapses the duplicates). |
| `db-export` | Dump the dev DB into the gitignored `backups/` dir (timestamped `.dump`; pass a `.sql` path for plain SQL, or any explicit path). |
| `db-import` | Restore a dump. **Overwrites the dev DB**, so it asks for confirmation first (`-y` to skip in scripts). |

## Deployment

| Script | What it does |
|---|---|
| `push-images` | Build the prod images - backend, dashboard, and every storefront under `apps/channels/` - and push them to a container registry (e.g. DigitalOcean Container Registry). `REGISTRY=registry.digitalocean.com/<name> bin/push-images [tag]` - tag defaults to the git short sha, images build for linux/amd64. Prints the `*_IMAGE` vars `docker-compose.prod.yml` reads, one per image, the storefront ones named after the directory. Authenticate first with `doctl registry login`. |

The two shapes those images go into - `docker-compose.small.yml` (one
container, one Postgres) and `docker-compose.prod.yml` (split tiers) - are
in the README under Operations > Deploying. Neither has a `bin/*` wrapper:
they take a plain `docker compose -f <file> up -d` on the server.

## Other tools

| Script | What it does |
|---|---|
| `opensearch` | Query the search cluster without writing the DSL by hand: `status`, `products <q>`, `blog <q>`, `show products <id>`, `raw GET /_cluster/health`. Reindexing lives in `bin/cli search reindex`. |
| `new` | Scaffold a storefront: `bin/new channel <name> [--from <storefront>] [--tracked]`. Forks an existing shop under `apps/channels/`, allocates the first free dev and small-tier ports across every compose file, and copies the source's compose service rather than templating one here, so it inherits wiring changes instead of drifting. The service lands in the git-ignored `docker-compose.override.yml` (the convention in [workflow.md](workflow.md) - it keeps upstream pulls conflict-free) along with any YAML anchors it references, since those don't cross compose files. `--tracked` writes the dev, prod and small files instead, for adding a storefront to skelet itself. Nothing is written until every generated file passes `docker compose config`, so a failure leaves the tree untouched. |
| `remove` | Undo a scaffold: `bin/remove channel <name> [-y] [--volumes]`. Finds the service wherever `new` put it, strips it and its volume declarations, deletes the directory and env templates, stops the container, and drops `.skelet-apps` back. Prints everything it will delete with sizes and asks first, saying whether the directory is committed (recoverable) or untracked (not). Volumes are **kept** unless you pass `--volumes` - that's the part you can't undo, and a storefront's `.next` cache runs to gigabytes. Validates the edited compose files before writing, same as `new`; an override left with no services is deleted rather than written empty. |
| `fixperms` | Find and fix root-owned files under `apps/` left behind by container writes. Only asks for sudo when there's something to fix. |
| `doctor` | Sanity-check the environment: docker up, env files present, containers running, ports answering, backend /health, `*.localhost` resolution, disk space. Run it when something feels off. |

---

Adding or removing a script? Update this file too. The table in
[workflow.md](workflow.md) has the short version; this one has the details.

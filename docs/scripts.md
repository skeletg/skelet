# Commands

Every dev command, in detail. `skelet` is the front door: run it bare for a
grouped list, `skelet help <command>` for one. Every verb is Go, under
`cmd/skelet/internal/` - where each one lives is at the bottom, for when you're
reading the source.

**The CLI and the project.** These pages write `skelet`, the CLI the README's one-line
install puts on your PATH. Inside a project it runs that project's own Go program in
`cmd/skelet`, so a command never runs with a different release's code. The first run, and
the first after anything in there changes, builds it - with your own Go if you have it,
otherwise inside the `golang` image, so Docker alone is enough - and every run after that
starts the cached build straight away. A verb that ends by running something - `skelet go`,
`skelet psql` - becomes that program, so Ctrl-C, exit codes and the terminal behave exactly
as if you'd run it yourself. With Go and nothing installed, `go -C cmd/skelet run . <verb>`
does the same, though it reports any failure as exit 1.

**`./skelet`.** The bash front door from before the CLI, still at the root of a project for
one more release. The installed CLI hands a project that has it to it, so nothing changes
until it's removed, and running it yourself says so. A symlink of it onto your PATH (`ln -s
"$PWD/skelet" ~/.local/bin/skelet`) stops working once it's gone - `skelet doctor` warns
about one, and the installer replaces it with `--force`.

Every verb runs from the repo root whatever directory you're in, and three
things hold for all of them:

- **One compose file set** - `docker-compose.yml` is the whole dev stack
  including sidecars, and compose auto-merges your git-ignored
  `docker-compose.override.yml` on its own, so no `COMPOSE_FILE` is set at
  all. `stop`, `down`, `ps` and friends therefore cover
  Garage, the dev tools and co. as well as the apps.
- **Profiles follow your app selection** - every compose command gets
  `COMPOSE_PROFILES` built from the sidecars (`search`, plus
  `signals` unless `env/go.env` switches `SIGNALS_ENABLED` off - that profile
  carries the Signals process and its ClickHouse),
  whatever `.skelet-apps` lists and whatever dev tools `.skelet-tools` lists,
  so what you picked in `setup` comes up and what you skipped stays down
  (see `up --lean` below to drop the sidecars and tools).
- **Files stay yours** - containers run as your uid, so generated code
  doesn't end up root-owned and break `git pull`. Set
  `SKELET_NO_USER_MAP=1` if you actually need root inside a container.

## Starting a project

| Command | What it does |
|---|---|
| `skelet new <dir>` | The installed CLI's own command. Fetches a release into `<dir>` as a git checkout - a partial clone, so old file contents stay on the server until something asks for them - with Skelet as the `upstream` remote and your `main` branch starting at the release tag, then runs that project's `setup`, handing it every flag `new` doesn't know (`skelet new shop --yes`). `--version vX.Y.Z` picks the release, by default the one the CLI came from; `--repo URL` fetches from a fork or a mirror; `--force` allows a directory inside another project, which it otherwise refuses. Taking a later release is a merge - see [workflow.md](workflow.md#taking-a-new-release). |
| `skelet doctor` | Outside a project, the installed CLI checks the machine alone - Docker, compose, git, Go and free disk - so you know `skelet new` will work. Inside one it's the project's full doctor. |
| `skelet version` | The installed CLI's version, inside a project too - every other command there is the project's own. `gh attestation verify $(command -v skelet) --repo skeletg/skelet` checks the binary was built by the release workflow. |

## Starting and stopping the stack

| Command | What it does |
|---|---|
| `skelet up` | Start the full dev stack: backend with hot reload, dashboard on :5173, storefront on :3000, plus the sidecars - Garage (S3 :9900), OpenSearch - and the optional dev tools `.skelet-tools` picks (Mailpit :8025, webhook-tester :8081, Grafana :3030, Garage web UI :9901; see [workflow.md](workflow.md#optional-dev-tools)). `skelet up <name>` starts one tool ad hoc. Idempotent - re-run any time, it only touches what changed. `skelet up --lean` skips OpenSearch, the dev tools and the Signals process with its ClickHouse (~1GB+ less RAM; search returns empty, and the dashboard's Signals pages have no suite to ask). |
| `skelet stop` | Halt everything (sidecars too) without removing containers. |
| `skelet down` | Remove the containers. Volumes (your data) survive. |
| `skelet restart` | Restart containers in place. For a full recreate: `down` then `dev`. |
| `skelet ps` | List running containers. |
| `skelet serve` | Run one frontend on the HOST instead of in its container - `skelet serve dashboard`, `skelet serve vetrina-demo`. It exports the root `.env` first, because Next only reads `.env` files from its own directory and would otherwise need a second copy of `STOREFRONT_CHANNEL_ID` that drifts from the one compose reads. Trailing args go to the dev server, with no `--` in front - pnpm passes that through verbatim. |
| `skelet logs` | Tail logs - `skelet logs app`, `skelet logs --tail 50 garage`. |
| `skelet doctor` | See "Other tools" below - worth running right after `setup`, it's what catches a datastore the config expects but the stack isn't running. |
| `skelet setup` | Guided first-run. Asks which apps you want (backend always does; deselected apps also leave the working tree via sparse-checkout unless `--keep-code`) and, separately, whether to run the frontends in Docker rather than on the host - the host is the default, and `.skelet-apps` lists only the containerized ones, the deployment target, which suites and engines you want, which payment gateways compile in, whether to hide the admin API, and which optional dev tools to run (saved to `.skelet-tools`). Writes the target's env files with generated secrets - an existing non-placeholder value is never overwritten - plus the root `.env`, `plugins.json` (merged, in the form `skelet cli plugin` writes) and git-ignored `.skelet-profile`. It's Go rather than a script, so it needs no OpenSSL or anything else on the host. Then boots the stack with `--lean`, migrates, and regenerates when the plugin set changed. Safe to re-run. `--yes` takes defaults, `--profile FILE` replays answers (saving them to `.skelet-profile`, never back into FILE), `--all` / `--backend-only` / `--apps LIST` set the selection directly, `--docker-apps LIST|none` sets the placement, and `--tools LIST|none` the dev tools. |

## Running things inside containers

| Command | What it does |
|---|---|
| `skelet sh` | Shell into the app container. |
| `skelet go` | Run go in the app container - `skelet go build ./...`, `skelet go test ./internal/...`. `--signals` or `--sdk` first runs it in that module instead. |
| `skelet cli` | The skelet ops CLI - `skelet cli admin list`, `skelet cli media-migrate ...`, `skelet cli job run customer-roster-push` to run a scheduled job now. Works with pipes and CI (only grabs a TTY when there is one). |
| `skelet pnpm dashboard` | pnpm in the dashboard container - `skelet pnpm dashboard run codegen`, `skelet pnpm dashboard run lint`. Vite is already running via `skelet up`, so no need for `run dev`. |
| `skelet pnpm <name>` | Same for storefronts (`apps/channels/<name>/`) - `skelet pnpm <name> [name] run codegen`, `run build`; the name is optional when only one storefront exists. Note: storefront codegen/tsc only works right in the container; host runs show phantom errors (no `.next/types`). |
| `skelet psql` | psql shell into the dev database. |

If the stack is up, these exec into the running container (instant). If
not, they spin up a one-off. Works either way.

## The gates

**`skelet check` is the one you need to know.** It runs every gate below, takes
about a second, needs no running backend, and is exactly what the pre-commit
hook and CI run. The one step that wants Docker is the proto compatibility check
against `main`, which runs in the app container when the stack is up and says it
skipped when it isn't - CI runs it on the host toolchain instead. `skelet check --list` prints what each gate is
for. The individual scripts exist so you can rerun one while fixing it.

| Script | What it protects |
|---|---|
| `skelet check` | All three gates, in one command. `--codegen` adds the generated-file check (needs the container, and the backend for its TS half). `--list` explains each gate. |
| `skelet check pins` | Version pins that live in several files at once: postgres, clickhouse, dbmate, golangci-lint, node (plus `.nvmrc`), alpine, and the `golang:` images against `go.mod`. Prints every site when they disagree. Add a pin here when you introduce one more than one file has to know. |
| `skelet check structure` | Structural invariants a fresh clone or a forked storefront leans on: every `bin/*` script parses and is executable, no doc or workflow names a script that doesn't exist, every compose `env_file` has a committed `.example`. |
| `skelet check contracts` | The cross-app contracts the `SYNC` comments describe: every path a SYNC comment names still exists, CMS block types agree across backend / dashboard / storefront, the anonymous-cacheable operation lists match, every lexical node the dashboard authors has a renderer, the two conditional-option evaluators are still the same code, the security-header copies match, and the Signals proto has no breaking change against `main` (`origin/main` in CI, where a missing ref fails rather than passing), or against `develop` while `main` has no proto yet. Each check fails if its extractor matches nothing, so it can't rot into a no-op. |
| `skelet hooks` | Installs a pre-commit hook that runs `skelet check`. Opt-in; skip it once with `git commit --no-verify`. The hook finds the CLI on its own - `skelet` on the PATH, `~/.local/bin/skelet`, the one you installed it with, then `go run` from the checkout - because git GUIs often run hooks without your shell's PATH. A hook of your own is left alone unless you pass `--force`, and the hook an older `skelet hooks` wrote, which ran `bin/check`, is updated the next time you run any `skelet` command. |

## Codegen, lint and tests

| Command | What it does |
|---|---|
| `skelet gen` | Regenerate everything in order: the Signals wire contract (buf), sqlc, gqlgen (both surfaces), the plugin bridge, the remote-provider contract, the generated reference tables in `docs/` (permissions, webhook events, settings), dashboard TS types, storefront TS types. Needs the backend up for the last two steps, or pass `--no-frontend` to stop before them. `--check` fails on stale output instead of writing it. |
| `skelet gen sqlc` | sqlc for both Go modules, each from its own `sqlc.yaml` - pinned to 1.31.1 so regens don't rewrite every version header. |
| `skelet gen buf` | buf against `proto/`, for one command at a time - `skelet gen buf lint /var/www/proto`. It runs from the Go module, since that is where `go tool` finds buf and its two protoc plugins; nothing is installed outside `go.mod`. |
| `skelet gen gqlgen` | One gqlgen surface at a time. Usually you want `codegen` instead. |
| `skelet lint` | gofmt + golangci-lint are hard gates, run over both Go modules; dashboard + storefront ESLint run after but don't fail the script (matches CI, which keeps frontend lint non-blocking until it's clean). |
| `skelet test` | `go test` in both Go modules + dashboard/storefront typechecks + dashboard and storefront unit tests (vitest) - the same hard gates CI runs. Args are forwarded to core's `go test`; `--no-frontend` skips the frontend steps; `--changed[=REF]` runs only what changed since REF (the merge base with develop by default) can reach. The default sweep excludes each module's `internal/e2e` (they write real rows and never clean up) - run those with `skelet test e2e`. |
| `skelet test e2e` | Run both e2e suites against dedicated disposable databases (`skelet_e2e` / `skelet_e2e_signals`) so the dev DB never accumulates test orders/customers. Starts a Signals process against the disposable signals database first, since core's scenarios reach the suite over the contract, then runs core's scenarios and the suite's own. Drops and recreates the databases every run - kept, they grow until the store fixture costs seconds instead of milliseconds. `--keep` reuses them while iterating on one scenario. Extra args forward to `go test` (e.g. `-run TestFoo`). |
| `skelet test storefront [name]` | Browser tests for a storefront (Playwright, `apps/channels/<name>/e2e/`) against its running dev container. Runs in Microsoft's Playwright image inside the storefront container's network namespace, so the host needs no Node or browser and the page is served as `localhost` (the only origin `next dev` hydrates for). Each spec signs up throwaway `e2e-*@skelet.test` customers in the dev database. Extra args forward to Playwright (`-g addresses`, a spec path). Headless - there is no display in the container. |
| `skelet changelog` | The pending fragments in `changelog/unreleased/`, assembled as the next release would print them. `check` validates their names and format, fails on anything written straight under `[Unreleased]` in `CHANGELOG.md`, and makes sure every `changelog/<version>.md` is listed there and nothing listed is missing - `skelet check` runs it. `release <version> [date]` writes `changelog/<version>.md`, lists it and deletes the fragments; see [releasing.md](releasing.md). |
| `skelet provider` | **Not a gate** - a conformance tester you point at a remote provider endpoint: signed envelope, forged/unsigned/wrong-key rejection, replay window, timeout budget, 256 KiB response ceiling, cache stability. `skelet provider --url URL --secret rpsec_... --kind tax_provider`. Posts the generated example for the kind unless you pass `--payload`. Contract: [remote-providers.md](remote-providers.md). |

## Database

| Command | What it does |
|---|---|
| `skelet db migrate` | `skelet db migrate up`, `new <name>`, `rollback`, `status` - dbmate. Runs inside the container, so `DATABASE_URL` is always the right one. `up` is mostly a no-op: the app applies the stream as it boots, so what you actually need this for is `new`, `rollback` and `status`. |
| `skelet db migrate --signals` | The same, against the Signals database (`SIGNALS_DB_URL`) - the signals process applies it as it boots. |
| `skelet db export` | Dump the dev DB into the gitignored `backups/` dir (timestamped `.dump`; pass a `.sql` path for plain SQL, or any explicit path). |
| `skelet db import` | Restore a dump. **Overwrites the dev DB**, so it asks for confirmation first (`-y` to skip in scripts). |

## Deployment

| Command | What it does |
|---|---|
| `skelet release` | Build the prod images - backend, dashboard, and every storefront under `apps/channels/` - and push them to a container registry (e.g. DigitalOcean Container Registry). `REGISTRY=registry.digitalocean.com/<name> skelet release [tag]` - tag defaults to the git short sha, images build for linux/amd64. Prints the `*_IMAGE` vars `docker-compose.prod.yml` reads, one per image, the storefront ones named after the directory. Authenticate first with `doctl registry login`. |

The two shapes those images go into - `docker-compose.small.yml` (one
container, one Postgres) and `docker-compose.prod.yml` (split tiers) - are
in the README under Operations > Deploying. Neither has a `skelet` verb:
they take a plain `docker compose -f <file> up -d` on the server.

## Other tools

| Command | What it does |
|---|---|
| `skelet search` | Query the search cluster without writing the DSL by hand: `status`, `products <q>`, `blog <q>`, `show products <id>`, `raw GET /_cluster/health`. Reindexing lives in `skelet cli search reindex`. |
| `skelet add` | Scaffold a storefront: `skelet add channel <name> [--from <storefront>] [--tracked]`. Forks an existing shop under `apps/channels/`, allocates the first free dev and small-tier ports across every compose file, and copies the source's compose service rather than templating one here, so it inherits wiring changes instead of drifting. The service lands in the git-ignored `docker-compose.override.yml` (the convention in [workflow.md](workflow.md) - it keeps upstream pulls conflict-free) along with any YAML anchors it references, since those don't cross compose files. `--tracked` writes the dev, prod and small files instead, for adding a storefront to skelet itself. Nothing is written until every generated file passes `docker compose config`, so a failure leaves the tree untouched. `new channel` still works and says it's `add channel` now. |
| `skelet rm` | Undo a scaffold: `skelet rm channel <name> [-y] [--volumes]`. Finds the service wherever `add` put it, strips it and its volume declarations, deletes the directory and env templates, stops the container, and drops `.skelet-apps` back. Prints everything it will delete with sizes and asks first, saying whether the directory is committed (recoverable) or untracked (not). Volumes are **kept** unless you pass `--volumes` - that's the part you can't undo, and a storefront's `.next` cache runs to gigabytes. Validates the edited compose files before writing, same as `add`; an override left with no services is deleted rather than written empty. |
| `skelet fixperms` | Find and fix root-owned files under `apps/` left behind by container writes. Only asks for sudo when there's something to fix. |
| `skelet doctor` | Sanity-check the machine and the checkout: Docker and compose, git (2.35 or newer, for setup's sparse-checkout) and Go on the host, the env files the target needs, where the frontends run and whether pnpm and Node can run the host ones, the profile against the env files, `plugins.json` against the plugin directories (the rules codegen enforces), the running containers and the Signals spine, ports answering, backend /health, `*.localhost` resolution, the pre-commit hook, a WSL clone on the Windows drive, and disk space. Every line is ok, warn or FAIL, and only a FAIL fails it. Run it when something feels off. Outside a project, the installed `skelet doctor` checks the machine alone. |

## Where the verbs live

Each verb is one line in `cmd/skelet/internal/route/table.go`, naming the Go function that
carries it out and the help text `skelet help` prints. `pnpm` takes `npm` as an alias from
before the switch, and `new channel` is `add channel`'s old name.

`bin/` is down to two files while the `./skelet` shim lasts. `bin/check` is for the hooks an
older `skelet hooks` wrote, and hands them to `skelet check` - any `skelet` command rewrites
such a hook to the current one. `bin/_lib` runs nothing any more; beside the shim it tells an
installed `skelet` to hand this checkout to `./skelet`.

---

Adding or removing a verb? Update this file too. The table in
[workflow.md](workflow.md) has the short version; this one has the details.

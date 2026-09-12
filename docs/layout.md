# Repository layout

What every directory at the root is for, including the dotfile ones a tree view
usually hides. If you have just cloned this and want to know where something
lives, start here.

```
skelet/
├── apps/          the three applications - everything else supports these
├── proto/         the core-to-Signals wire contract, owned by neither side
├── bin/           one script per dev operation
├── docs/          you are here
├── env/           environment templates, one file per service
├── deploy/        sidecar config, one directory per service
├── examples/      standalone reference implementations
├── .github/       CI workflows and dependency bumps
```

## The ones that hold the product

### `apps/`

The three applications, and the only place product code lives.

| | |
|---|---|
| `apps/backend/` | The Go GraphQL API. Also the worker, media, signals and ingest processes - one module, several binaries under `cmd/` |
| `apps/dashboard/` | The React admin panel (Vite) |
| `apps/channels/` | One directory per shop, each its own Next.js app. `vetrina-demo` is the template that ships in-tree |

Two of these are optional: `bin/setup` asks which apps you want and removes the
rest from your working tree via sparse-checkout, so a clone that brings its own
frontend can run headless. The backend always runs.

Inside `apps/backend` you will also find untracked build output - `server`,
`signals` and `tmp/` are compiled binaries and air's scratch directory, not
source. What the packages inside each app mean is
[architecture.md](architecture.md).

### `proto/`

The wire contract between core and Signals, as protobuf. It sits at the root
rather than inside `apps/backend/` because neither side owns it - core calls it
and Signals serves it, and when Signals becomes its own module both will import
the code generated from here.

`buf.yaml` holds the lint and compatibility rules, `buf.gen.yaml` says which
plugins run and where the output lands (`apps/backend/internal/signalspb/`,
never hand-edited). `./skelet gen` regenerates; `./skelet check contracts`
refuses a change that would break a deployed peer, since the two processes
deploy separately and an old core will meet a new Signals.

### `bin/`

A wrapper for every operation you would otherwise run by hand: starting the
stack, opening a shell in a container, migrations, codegen, the commit gates.
They all `cd` to the repo root themselves, so they work from anywhere.

`bin/_lib` is the odd one out - it is sourced by the others rather than run, and
holds the shared compose and uid setup. Full per-script reference:
[scripts.md](scripts.md).

## The ones that hold configuration

### `env/`

Environment variables, split per service rather than one giant file. The rule is
visible in the directory listing: `*.env.example` is committed, the matching
`*.env` is git-ignored and is what actually gets read. `bin/setup` copies the
templates on first run.

`garage.toml` is the exception - object storage config that carries no secrets,
so it is committed as-is. Every variable, which file it belongs in, and the
tradeoffs behind the optional ones: [env.md](env.md).

Secrets that apply install-wide (`JWT_SECRET`, `CONFIG_ENCRYPTION_KEY`,
`DB_PASSWORD`) live in the root `.env`, from `.env.example`.

### `deploy/`

Config for sidecar containers, kept out of `apps/` because it belongs to no
application. Mostly production, but not exclusively - the `logs` profile mounts
the Vector config in dev too.

One directory per service configured, never per deployment shape - which stack
mounts a file is a fact about the compose file, and putting it in the path would have
sibling directories answering two different questions.

| | Read by |
|---|---|
| `caddy/` | The bundled reverse proxy: three vhosts, automatic Let's Encrypt. Behind a profile because it takes `:80` and `:443`. Small and prod |
| `vector/` | Ships container stdout into ClickHouse so log history outlives the 10 MB json-file cap. Dev (`logs` profile) and prod |
| `postgres/` | Creates the second database by hand, since the one-container profile collapses both onto a single cluster. Small only |

`bin/contract-check` reads the Caddyfile directly - it holds one of the three
copies of the security headers that have to agree.

The vhost map in the Caddyfile is meant to be edited - it names the demo
storefront rather than deriving yours.

### Root files

`docker-compose.yml` is the dev stack. `docker-compose.small.yml` is the
minimum deployable tenant, and `docker-compose.prod.yml` the full split-tier
shape; both are described under Deploying in the README. `.nvmrc` pins Node and is what CI reads (`node-version-file`),
`.gitattributes` locks every text file to LF so a Windows checkout cannot rewrite
line endings, `.gitignore` and `.dockerignore` keep build contexts and commits
clean. `README.md`, `AGENTS.md` and `CHANGELOG.md` are the only pages up here -
everything else belongs in `docs/`.

Never edit the tracked compose files to add your own service. Docker Compose
auto-merges a git-ignored `docker-compose.override.yml`, which is where a shop
of your own belongs, so pulling from upstream stays conflict-free.

## The ones that support the work

### `docs/`

One file per question you might be asking, and the index that decides which is
which lives in the README rather than here. Some of these are generated rather
than written - `permissions.md`, `settings.md` and `webhooks.md` are rebuilt by
`bin/codegen` from the code that defines them, so edit the source, not the page.

**Paths in these pages.** A backticked path starting with `apps/`, `bin/`,
`deploy/` or `docs/` is from the repo root. A bare `internal/…`, `graph/…` or
`cmd/…` is relative to `apps/backend`, which is where all three of those live -
the short form keeps Go prose readable, and there is no second `internal/` to
confuse it with. Write the repo-root form when a page compares apps, the way
`architecture.md` does for the GraphQL surfaces.

### `examples/`

Standalone programs that demonstrate a published contract, kept outside `apps/`
because nothing in the platform imports them. Today that is a reference remote
tax provider in Go, written against [remote-providers.md](remote-providers.md)
and testable with `bin/provider-check`.

### `.github/`

`workflows/ci.yml` and `workflows/security.yml` run the same gates you run
locally, deriving their app matrices from `apps/channels/` so a new shop is
covered without touching a workflow. `renovate.json5` configures dependency
bumps and needs no per-app entry either - Renovate scans the repo for manifests
itself, so the same rule holds there.

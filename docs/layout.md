# Repository layout

What every directory at the root is for, including the dotfile ones a tree view
usually hides. If you have just cloned this and want to know where something
lives, start here.

```
skelet/
├── apps/          the applications and the Signals module - everything else supports these
├── templates/     the storefronts a shop is forked from
├── packages/      the TypeScript the storefronts share
├── proto/         the core-to-Signals wire contract, owned by neither side
├── cmd/skelet/    the skelet command line - the dev front door, and the binary releases ship
├── docs/          you are here
├── env/           environment templates, one file per service
├── deploy/        sidecar config, one directory per service
├── .github/       CI workflows and dependency bumps
```

## The ones that hold the product

### `apps/`

The applications, and the only place product code lives.

| | |
|---|---|
| `apps/backend/` | Everything server-side, three Go modules side by side and the dev Dockerfile they share |
| `apps/backend/core/` | The Go GraphQL API. Also the worker and media processes - one module, several binaries under `cmd/` |
| `apps/backend/signals/` | The Signals suite as its own Go module - the `signals` and `ingest` processes. It requires core by relative path, never the other way round |
| `apps/backend/sdk/` | The plugin SDK as its own Go module - what plugin repositories build against, beside core's gqlgen model packages for the resolvers they add |
| `apps/dashboard/` | The React admin panel (Vite) |
| `apps/channels/` | One directory per shop of the project's own, each forked from a template with `skelet add channel <name>`. Empty in skelet itself |

Two of these are optional: `skelet setup` asks which apps you want and removes the
rest from your working tree via sparse-checkout, so a clone that brings its own
frontend can run headless. The backend always runs.

Inside `apps/backend/core` you will also find untracked build output - `server` and
`tmp/` are a compiled binary and air's scratch directory, not source; the same
goes for `signals` and `ingest` under `apps/backend/signals`. What the packages inside each app mean is
[architecture.md](architecture.md).

### `templates/`

The storefronts a shop is forked from. `nextjs-vetrina` is the full Next.js one, and the
one the dev stack runs in skelet itself - for the browser tests, CI and demo work.
They're upstream's, like `packages/`: a project leaves them alone and builds its
shop in `apps/channels/`, so merging a skelet release never collides with it.

### `packages/`

TypeScript every storefront shares, as source - Next and Vite compile workspace
packages themselves, so there's no build step. `storefront-core` is a
storefront's logic with no framework in it - the GraphQL client and operations,
auth, cart, checkout, channel and locale - and a template builds its design on
it through an adapter: `storefront-next` for Next. `storefront-react` holds the
client hooks any React framework can use.
The storefronts, the templates and these are one pnpm workspace: the root
`pnpm-workspace.yaml` lists them and pins the versions they share in its catalog,
so a package and the app using it resolve the same React, and the one root
`pnpm-lock.yaml` covers them all. The dashboard stays a project of its own.

### `proto/`

The wire contract between core and Signals, as protobuf. It sits at the root
rather than inside either module because neither side owns it - core calls it
and Signals serves it, and both import the code generated from here, which lands
in core's `signalsapi/v1`, under the hand-written vocabulary, so the contract is
one directory the Signals module can reach.

`buf.yaml` holds the lint and compatibility rules, `buf.gen.yaml` says which
plugins run and where the output lands (`apps/backend/core/signalsapi/v1/`,
never hand-edited). `skelet gen` regenerates; `skelet check contracts`
refuses a change that would break a deployed peer, since the two processes
deploy separately and an old core will meet a new Signals.

### `cmd/skelet/`

The `skelet` command line: every dev verb, and the binary each release ships,
which installs and starts projects - inside one it builds and runs that project's
own copy of this directory. It's the tooling that drives the repo rather than part
of the product, so it sits at the root, not under `apps/`. Its own Go module, standard library only, and it
imports nothing from the backend - `cmd/skelet` because Go names a binary after
its directory, so `go install` gives you `skelet`.

## The ones that hold configuration

### `env/`

Environment variables, split per service rather than one giant file. The rule is
visible in the directory listing: `*.env.example` is committed, the matching
`*.env` is git-ignored and is what actually gets read. `skelet setup` copies the
templates on first run.

`garage.toml` is the exception - object storage config that carries no secrets,
so it is committed as-is. Every variable, which file it belongs in, and the
tradeoffs behind the optional ones: [env.md](env.md).

Secrets that apply install-wide (the session and link keys, `SIGNALS_SERVICE_SECRET`,
`CONFIG_ENCRYPTION_KEY`, `DB_PASSWORD`) live in the root `.env`, from `.env.example`.

### `deploy/`

Config for sidecar containers, kept out of `apps/` because it belongs to no
application. Production shapes only - the dev stack mounts nothing from here.

One directory per service configured, never per deployment shape - which stack
mounts a file is a fact about the compose file, and putting it in the path would have
sibling directories answering two different questions.

| | Read by |
|---|---|
| `caddy/` | The bundled reverse proxy: three vhosts, automatic Let's Encrypt. Behind a profile because it takes `:80` and `:443`. Small and prod |

`skelet check contracts` reads the Caddyfile directly - it holds one of the three
copies of the security headers that have to agree.

The vhost map in the Caddyfile is meant to be edited - it names the demo
storefront rather than deriving yours.

### `changelog/`

One file per release, `changelog/0.2.0.md` and on, with `CHANGELOG.md` at the root as
their index. Pending entries wait in `changelog/unreleased/` as one small file each, so
branches never edit the same lines, and a release gathers them into the next version's
file - the naming rule is in that folder's README.

### Root files

`docker-compose.yml` is the dev stack. `docker-compose.small.yml` is the
minimum deployable tenant, and `docker-compose.prod.yml` the full split-tier
shape; both are described under Deploying in the README. `pnpm-workspace.yaml` and
`pnpm-lock.yaml` are the storefront workspace's (see `packages/` above). `.nvmrc` pins Node and is what CI reads (`node-version-file`),
`.gitattributes` locks every text file to LF so a Windows checkout cannot rewrite
line endings, `.gitignore` and `.dockerignore` keep build contexts and commits
clean. `.skelet/` is git-ignored scratch for the CLI - the build cache it falls back to
when your user cache directory isn't writable, among other things - delete it whenever
you like. `README.md`, `AGENTS.md`
and `CHANGELOG.md` are the only pages up here - everything else belongs in `docs/`.

Never edit the tracked compose files to add your own service. Docker Compose
auto-merges a git-ignored `docker-compose.override.yml`, which is where a shop
of your own belongs, so pulling from upstream stays conflict-free.

## The ones that support the work

### `docs/`

One file per question you might be asking, and the index that decides which is
which lives in the README rather than here. Some of these are generated rather
than written - `permissions.md`, `settings.md` and `webhooks.md` are rebuilt by
`skelet gen` from the code that defines them, so edit the source, not the page.

**Paths in these pages.** A backticked path starting with `apps/`, `templates/`,
`packages/`, `deploy/` or `docs/` is from the repo root. A bare `internal/…`, `graph/…` or
`cmd/…` is relative to `apps/backend/core`, which is where all three of those live -
the short form keeps Go prose readable, and there is no second `internal/` to
confuse it with. Write the repo-root form when a page compares apps, the way
`architecture.md` does for the GraphQL surfaces.

### `.github/`

`workflows/ci.yml` and `workflows/security.yml` run the same gates you run
locally, deriving their app matrices from `apps/channels/`, `templates/` and
`packages/` so a new shop or package is covered without touching a workflow. `workflows/release.yml` publishes a release,
CLI binaries included, when its tag is pushed. `renovate.json5` configures dependency
bumps and needs no per-app entry either - Renovate scans the repo for manifests
itself, so the same rule holds there.

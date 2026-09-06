# Changelog

Notable changes per release. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/), with the `0.x` caveat
that breaking changes ride the MINOR until `1.0.0`.

The release process, and which version number means what, is in
[docs/releasing.md](docs/releasing.md).

## [Unreleased]

### Changed

- **The email send log now ages out by default, and core owns the job.** `email_sends` moved to
  the commerce database, but its retention still defaulted to keep-forever and its prune lived in
  the Signals `analytics-prune` job. Every transactional mail writes a row - receipts, verification,
  password resets, shipment notices - so a store that never opens Signals still grew the table
  forever in its primary database, and the maintenance stopped entirely with
  `SIGNALS_JOBS_EXTERNAL=true` and no signals container. A new core `send-log-retention` job
  pre-creates the upcoming month partitions and drops whole months past the window, registered
  unconditionally so it runs with Signals off. `pulse/analytics/email_sends_retention_months`
  now defaults to `24`; set it to `0` to keep the old keep-forever behaviour. Existing installs
  that never set it will start dropping months older than two years on the next daily run.


### Added

- **Resend an order email from the dashboard.** `resendOrderEmail` sends an order's receipt,
  shipment notice, cancellation or refund again, which is the recovery path for mail a paused
  flow swallowed or a customer never received - previously there was none, and the automatic
  sends are event-driven with no way to replay them. It deliberately skips the flow engine:
  the operator wants that mail now, and enrolling would either restart a multi-step journey or
  hand delivery back to the same flow that lost the original. `SHIPMENT` resends the newest
  shipment's notice unless a `fulfillmentId` names one, which is checked against the order's
  own shipments so the channel permission gate can't be sidestepped with a fulfillment id from
  another order. Recorded and pixel-tracked like any other transactional send. Requires
  `orders.update`. The order detail page carries a **Resend email** menu that offers each
  notice only once the order has something to say in it, so a shipment notice never appears
  on an unshipped order.

### Fixed

- **Frontend codegen was silently skipped whenever `ADMIN_API_SLUG` was set.** `bin/codegen`
  probed the fixed `/api/admin/graphql` to decide whether the backend was up, but that is
  exactly the path the slug moves, so the probe timed out and both the dashboard and the
  storefront kept their stale generated types with only a note in the output. The probe now
  reads the slug from `.env` and follows the endpoint.

- **Storefront codegen validated its documents against the admin schema.** The dashboard and
  the storefront services both read `GRAPHQL_CODEGEN_URL`, and `bin/setup` sets it to the
  admin endpoint when it moves the admin API, which overrode the storefront's own front-API
  default. The storefront now reads `FRONT_GRAPHQL_CODEGEN_URL` so the two can't collide.

### Changed

- **Every direct transactional send is recorded and carries an open pixel.** Three domains
  send mail directly when no flow takes delivery - order receipts, account links and
  back-in-stock notices - and only the receipts were being logged. A directly-sent
  verification recorded nothing at all, so a store running without auth flows showed zero
  verification sends, indistinguishable from mail that was never sent. A shared
  `internal/mailsend` now records first, points the open pixel at that record, then
  delivers, which is the order the flow engine already used. Without it a logged send could
  never register an open and reported as mail nobody read. Transactional mail gets the pixel
  only, never click-wrapping, so nothing sits in front of a verification or reset link.

- **Core owns the order-lifecycle mail.** The receipt, shipment notice, cancellation and
  refund were delivered entirely by the flow engine, which also owned the subscriptions -
  so a channel whose flow was paused or deleted silently sent nothing, and there is no
  admin resend to recover it. A new `internal/ordermail` owns those four subscriptions and
  calls the flow engine as a dispatcher: a flow still wins whenever one is active, and only
  a zero enrollment falls through to sending directly. The order-to-template mapping moved
  out of the flow engine so both senders share it. Flow-sent order mail now stamps its
  `order_id` on the send log, which it never did, so a receipt is attributable to its order
  whichever sender delivered it.

- **A new channel seeds its own transactional emails and delivery flows.** Seeding was a
  manual `skelet emails seed-transactional` that nothing called, so a channel created
  through the admin API had no flows and silently sent no receipts, no verification and no
  password resets - with no admin resend to recover them. Channel creation now publishes
  `channel.created` and a subscriber seeds both, emails first since a flow points at the
  slot email it sends. Best-effort and idempotent, so the CLI still repairs a channel this
  missed, and it is registered ahead of the `SIGNALS_ENABLED` gate because transactional
  mail is not part of the analytics suite.

- **Account email no longer depends on a flow being active.** Verification and password
  reset were delivered only by the seeded Pulse flows, and nothing guaranteed those exist:
  seeding is a manual `skelet emails seed-transactional`, archived flows are deliberately
  never re-seeded, and a flow can be paused or deleted outright. When none ran, the
  storefront still answered "check your email" and only a log line recorded that nothing
  was sent. Both now fall back to sending directly, through the channel's own template
  where it has one, so a missing flow can no longer lock a customer out of their account.
  The flow still wins whenever one is active, so nothing double-sends.

- **The email send log moved to the commerce database.** `email_sends` lived in the
  Signals database, but the flow engine writes it on every send and the flow engine is
  what delivers transactional mail - order receipts, shipping notices, email verification,
  password reset. That put `signals-db` on the password-reset path. Nothing joined the
  table across the two databases, so it moved without a schema change: commerce migration
  `000003` creates it, signals `000002` drops it, and `skelet db move-email-sends` copies
  existing rows between the two (it goes between the two `db migrate` calls). GDPR's send
  scrub is now a local query inside the anonymize transaction instead of a best-effort
  call before it, and the campaign package no longer opens a Signals handle at all.

- **The Go module is now `github.com/skeletg/skelet/apps/backend`.** It was declaring a
  repo-root path while its `go.mod` sat in a subdirectory, which Go cannot resolve - so
  nothing could ever `go get` it. Imports gain `apps/backend/`; module tags become
  `apps/backend/vX.Y.Z`.
- **SDK 0.2.1.** 0.2.0 was the import-path move: no declaration changed, but every plugin
  rewrites its imports, which is breaking for anyone installing one, and the surface gate
  caught it. 0.2.1 adds the seams below.
- **B2B is a plugin, not part of core.** Company accounts, credit lines, contract
  pricing, quotes and on-account payment moved out of `internal/b2b` into a standalone
  Tier 1 plugin with its own repository. Core no longer contains the word B2B outside
  a few comments: the couplings that used to be core code are registered seams now -
  an `order.tendering` subscriber that reserves credit inside the placement
  transaction, an `order.cancelled` subscriber that releases the unshipped share, and
  a price resolver for negotiated rates. The `purchase-order` payment method stays in
  core next to COD and bank transfer, where it is a plain pay-later tender any store
  can use; the plugin adds the credit line on top by matching its slug.
- **`B2B_ENABLED` is gone.** The plugin's own kill switch replaces it, which is per
  channel rather than server-wide: `plugins/plugins.json` decides whether it compiles
  in, `plugin/b2b/enabled` in Settings switches it off per store. `bin/setup` asks the
  same question and writes the plugin file instead of an env var.
- **`masqueradeCustomer` moved to the core customer surface** and is gated on
  `customers.manage` rather than `b2b.manage`. It was never a B2B feature - it is the
  support tool for "it looks wrong on my account", and it hangs off the customer detail
  page, where it now lives.
- **Six tables renamed.** `companies`, `company_members` and `company_prices` became
  `b2b_companies`, `b2b_company_members` and `b2b_company_prices`; the three that were
  already prefixed kept their names. The plugin's migration adopts existing tables by
  renaming them in place, so an upgrade carries its rows, foreign keys and grants over.

### Added

- **Plugin permission groups register themselves.** A plugin naming a group core does
  not have used to fail boot with "references unknown group" and no way to fix it from
  the plugin side; the host now creates it from the manifest name on first reference.
- **Storefront plugin surface** - `app/plugins/` in vetrina-demo: a slot registry
  (`registerAccountNavItem`, `registerSlot` with per-zone context) and a `loaded.ts` import
  list, so a plugin's buyer-facing UI lives in `app/plugins/<slug>/` instead of threaded
  through the cart and account pages. Routes need no registration, since Next's filesystem
  router already is the glob. B2B was the first consumer and no core storefront page
  mentions it any more.
- **`internal/e2ekit`** - core's end-to-end harness extracted from `internal/e2e`'s test
  files into a normal package, so a plugin can write real end-to-end tests in its own
  directory instead of copying files into a core package. It takes a four-method `TB`
  rather than `*testing.T`, so it imports no `testing`. `bin/test-e2e` now runs
  `./plugins/...` too, and applies enabled plugins' migrations before the suite.

- **`bin/cli plugin add` - plugins as Go module dependencies.** `go get` fetches the
  plugin, `plugins.json` records its module path, and `plugins/firstparty` imports it.
  Nothing is copied into the repo, so you get a version, a `go.sum` checksum, one copy
  instead of two, and `go get -u` to upgrade. `--local` covers a plugin with no remote yet
  via a relative `replace`. `plugin install` stays as the fallback for a plugin that is
  not a module.
- A module plugin's GraphQL SDL is staged out of the read-only module cache into
  `plugins/.modules/` (git-ignored, rebuilt each run) so gqlgen can reach it by a stable
  relative path. `plugins.json` grew an object form recording each plugin's module path,
  and still reads the old `0`/`1` shape.
- **The authz coverage gate follows a plugin into the module cache.** It resolves the
  plugin's source with `go list -m`, parses it, and demands a real
  `sdk.Require*`/`PermittedChannels` guard on the method serving each delegated admin
  field - so a plugin shipping an ungated field fails core's build. Mutation-checked by
  removing a guard from a plugin and confirming the gate fires.

## [0.1.0] - 2026-08-30

First public release.

**This is a 0.x release and is not ready to run a live store on.** The feature
set below is implemented and covered by tests, including an end-to-end suite over
the money paths, but it has not been run in production by anyone. Expect rough
edges, gaps you only find under real traffic, and breaking changes: until `1.0.0`
there is no compatibility promise, and a breaking change rides the MINOR. Treat it
as something to evaluate, build against and report problems from, rather than
something to take payments with.

Two things worth knowing before you try it. Everything runs in containers and a
fresh clone comes up as a working shop, so evaluating it is cheap. And the SDK,
the remote-provider contract and the product version move independently, so a
plugin you write against `0.1.0` states its own compatibility range rather than
tracking the platform.

### Added

- Backend: catalog, cart, checkout, orders, payments, inventory, promotions,
  subscriptions, B2B, loyalty and referrals over a Go + PostgreSQL core, with two
  separate GraphQL surfaces for the admin and the storefront.
- Multi-channel and multi-locale throughout: one catalog serving several
  storefronts in different currencies and languages, with per-channel listings,
  scoped permissions and a scoped settings registry.
- Admin dashboard (React) and a reference storefront (Next.js).
- Tier 1 plugin system with an SDK, and remote providers over a versioned wire
  contract.
- Six payment gateways, each in its own repository.
- Signals: an append-only analytics spine, web analytics, session recording and
  customer profiles, on its own database and pluggable between PostgreSQL and
  ClickHouse.
- Search behind a pluggable engine, with a PostgreSQL tier and OpenSearch.
- Operations: a transactional outbox, HMAC-signed webhooks, a scheduled-jobs
  engine with retries and alerting, encrypted config secrets, and nightly
  encrypted database backups.

[Unreleased]: https://github.com/skeletg/skelet/compare/v0.1.0...develop
[0.1.0]: https://github.com/skeletg/skelet/releases/tag/v0.1.0

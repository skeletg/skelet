# Changelog

Notable changes per release. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/), with the `0.x` caveat
that breaking changes ride the MINOR until `1.0.0`.

The release process, and which version number means what, is in
[docs/releasing.md](docs/releasing.md).

## [Unreleased]


### Fixed

- **A redelivered event could send a second order receipt.** Order confirmation, shipment,
  cancellation and refund mail go out from async event handlers, and the outbox redelivers an event
  whose handler finished but whose delivery was never marked done - a crash in that window sent the
  customer a second receipt, which reads as a second order. The only check in place asked whether a
  marketing flow had taken the send, not whether the mail had already gone. Each of the four now
  records itself in `order_email_log`, keyed the way that table's unique index already is (order,
  kind, and a ref that carries the fulfillment id for shipments and the payment reference for
  refunds), and a replay finds the row and sends nothing. The row is written after the send, so mail
  that never left is not recorded and still goes on a retry; an unreadable log sends anyway, since a
  missing receipt costs more than a duplicate. An operator's manual resend is deliberately outside
  this - asking for it again means asking for it again.

- **The customer roster was never synced, so no customer got a trait row.** `SyncRoster` copies
  every customer into the signals database and the trait recompute is driven off that copy, but
  nothing in either process called it - the roster stayed empty, `RecomputeChannelTraits` had no
  rows to compute over, and every RFM score, lifecycle stage and engagement number came back
  missing. It now runs once at the head of the trait-compute job, ahead of the per-channel
  recompute.

- **A segment's members list was always empty.** Moving segments onto the Signals API left the
  `members` field bound to a model field nothing fills, so the segment detail page showed "No
  members yet" under a header counting them, and `limit`/`offset` were silently ignored. The field
  resolver is back on that surface, still gated on `customers.view` as well as `promotions.view`
  and still narrowed to the caller's channels.

- **A paused marketing flow could stop every transactional email on the server.** The flow engine
  claims the oldest 100 runnable runs per tick, and a run whose flow was paused or deleted, or a
  marketing run under `SIGNALS_ENABLED=false`, was claimed and then left exactly as it was. Those
  runs came back on every tick, so once 100 of them were the oldest in the table nothing behind
  them ever advanced - and this engine delivers order receipts, verification links and password
  resets, so they stopped going out. Enrolling a few hundred customers in a campaign and then
  pausing it was enough to trigger it. The claim now joins `flows` and returns only runs the
  engine will actually advance, so a run it would refuse no longer spends a slot. Parked runs are
  still left untouched and resume when the flow is reactivated.

- **Turning Signals off took the flow builder with it.** Every flow resolver sat behind the
  `SIGNALS_ENABLED` gate, but the engine deliberately keeps running transactional flows when the
  suite is off, because that is what delivers receipts, verification links and password resets. So
  a server with Signals disabled went on sending flow mail while nobody could open, edit or debug
  the flows sending it, and a newly created channel was seeded with transactional flows that could
  not be inspected. Flows are now reachable with the suite off, and appear in the Marketing
  sidebar - the same fallback the Emails hub already uses. Marketing flows are still parked by the
  engine rather than hidden from the list.

- **Promotions and CMS blocks kept matching on segments nothing was recomputing.** Under
  `SIGNALS_ENABLED=false` the segment refresh ran as part of a Signals-only job, so it did not run
  at all: every rules-based segment froze at whatever membership it last had, and both the
  `core.customer_in_segment` promotion condition and segment-gated CMS blocks went on matching
  against that snapshot. A VIP discount could keep applying to customers who no longer qualified.
  Segments whose rules read Synapse traits (`trait`, `lifecycle`), and Synapse-defined behavioral
  segments, now count as empty while the suite is off. The rule kinds that are plain commerce
  facts - order count, total spend, recency, consent and customer group - moved to their own job
  and keep refreshing, so segments that never needed Signals are unaffected either way.

- **Email opens and clicks went unrecorded with Signals off.** The send log moved into core so
  transactional mail would not depend on the analytics stack, but the open pixel still stamped
  `first_open_at` from inside the analytics recorder, which returns early when
  `SIGNALS_ENABLED=false`. Every receipt, shipping notice and password reset therefore reported as
  never opened. The stamp is now written on the core path alongside the spine event rather than
  inside it, and a repeat pixel load skips the write instead of re-running it.

- **A few core screens broke when Signals was switched off.** The CMS page detail rendered an
  "A/B test (Prism)" section that could only error, and the email library, block library and CMS
  block-visibility picker each called a resolver that lived in a Signals file but reads no Signals
  data - the merge-tag vocabulary, the block HTML preview and the segment picker. The Prism
  section is now hidden with the suite off, and those three resolvers answer either way. The merge
  tags and the block preview now check `emails.view` rather than `campaigns.view`, matching the
  screens that actually need them.

### Fixed

- **You could not build a segment for lapsed customers, and one for dormant ones matched nobody.**
  The trait recompute writes five lifecycle stages - prospect, new, active, at_risk and lapsed -
  but the segment rule validator accepted `dormant` instead of `lapsed`. So a rule targeting
  lapsed buyers, which is the win-back audience most stores want first, was rejected at save time,
  while a rule targeting `dormant` saved cleanly and then matched nobody, because no customer is
  ever given that stage. The validator now accepts exactly what the recompute writes, and a test
  pins the two lists together so they cannot drift apart again.

### Changed

- **Prism's experiments live in the signals database.** The `experiments` table was the last suite
  table still in commerce, and every read of it pairs with the exposure and goal events on the
  spine - a results query had to reach across two databases to score one experiment. Definitions
  now sit next to those events, so it reads one. The channel foreign key does not survive the move,
  since a key cannot cross databases; an experiment on a deleted channel is inert, because
  assignment and listing are both channel-scoped and neither will ever name it. Nothing about the
  admin API or the storefront SDK endpoint changes.

- **The Synapse trait cluster lives in the signals database.** `customer_traits` and the two
  projections that feed it sit next to the spine rather than in commerce, and the recompute runs
  there against a synced customer roster instead of joining `customers`. Core reads none of it.
  Three things follow from the split: the segment rule compiler resolves `trait` and `lifecycle`
  conditions against the signals database and injects the matching ids, rather than building one
  query with a subquery it cannot reach; the three Synapse segment panels resolve membership in
  commerce and traits next door, joining in the caller; and because a foreign key cannot cross
  databases, the roster sync prunes a deleted customer's traits where `ON DELETE CASCADE` would
  have.

- **Trait compute reads the event spine instead of joining the orders table.** RFM's inputs -
  recency, frequency and monetary - came from `customers LEFT JOIN orders` inside
  `RecomputeChannelTraits`, which is the join that kept the Signals trait job inside the commerce
  database, on the table that grows fastest. The spine already carries `order_placed` with the
  customer, channel, total and timestamp, so those aggregates are now read there and landed in
  `customer_order_agg`, the same projection shape the 30-day engagement count already used.
  Cancellations are netted out on the spine side rather than filtered at read time, since an
  append-only log cannot retract a row.

### Added

- **Synapse moved to the Signals API.** The overview, insights, funnels, flows, retention,
  engagement, behavioural segments and both profile screens are served from
  `/api/signals/graphql`, along with the Pulse email-engagement rollup. Rows carry customer ids and
  no names, since that service has no customers table - the dashboard labels them from core in one
  batched lookup, shared by every Signals screen that shows people. What stays in core is what
  reads commerce: the profile page's Commerce and Marketing panels, since orders, subscriptions,
  consent and flow enrollments are core's. A customer's audiences come from `customerAudiences` on
  the Signals side, beside them.

- **The segment admin API moved to Signals with them.** Creating, editing, refreshing and
  administering audiences all happens on `/api/signals/graphql` now, along with the segment pickers
  the flow builder, form builder, campaign composer, promotion editor and CMS block canvas use.
  Core keeps nothing segment-shaped. A member row carries the customer id alone, since that service
  has no customers table - the dashboard resolves names from core in one batched follow-up, so
  addresses stay in the commerce database rather than being copied for a list label.

- **Segments belong to Signals.** Audiences are a Signals capability, so `customer_segments`, its
  memberships and the daily snapshots live in the signals database. Core offers segment-gated
  promotions and segment-gated CMS blocks only while Signals is running: with it off, or if the
  signals database is unreachable, `SlugsForCustomer` returns nothing and both simply stop
  offering the feature rather than acting on a frozen membership. Signup forms that assign a
  submitter to a segment no-op the same way. The membership foreign key to `customers` could not
  cross databases, so the roster sync prunes a deleted customer's memberships alongside their
  traits, replacing that cascade.

- **Event destinations moved to Signals, table and all.** The webhook, GA4 and Meta CAPI receivers
  are configured through the Signals API now, and `event_destinations` sits in the signals database
  beside the spine it forwards from - so the forwarding sweep pages events and advances its cursor
  without crossing a database. The audience export still reads segments from commerce, since those
  stay core's, and the channel foreign key does not survive the move: a destination whose channel
  was deleted is inert, and the sweep skips it.

- **Lens is served by the Signals API too.** The whole session-replay, heatmap, web-vitals and
  on-site-search surface moves onto `/api/signals/graphql` alongside Prism, which is where its data
  already lived. Two things needed answers rather than a move: the channel-scoped permission check
  on a customer's recordings now reads the synced roster instead of the customers table, and the
  recordings list returns customer ids without names. The dashboard labels those rows by asking
  core for the ids in one batched follow-up, so customer names and email addresses stay in the
  commerce database rather than being copied into the analytics one for a table label.

- **Prism is served by its own API.** The Signals process (`cmd/signals`) now serves a GraphQL
  surface at `/api/signals/graphql`, and the A/B testing screens talk to it directly rather than
  to the core admin API. Callers authenticate with a short-lived token core mints for that
  audience, carrying only the permissions that surface checks, so the Signals service authorizes
  every request without reaching into the commerce database. Killing a session in core still
  takes effect there: revoked session ids are published to Valkey and checked per request, and an
  unreachable Valkey fails open like the existing admin guard.

  `cmd/signals` has to be running for the Prism screens to work (`docker compose --profile signals
  up`), and it needs `JWT_SECRET` to verify the tokens core mints - without it every request is
  refused.

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

- **The email send log is core's now, not the analytics stack's.** `email_sends` moved to the commerce
  database earlier, but the code that owned it stayed in `internal/analytics` - so a core package
  delivering a password reset reached into the Signals stack to record it. A new `internal/sendlog`
  owns the table: the `Send`/`Spec` types, the three send sources, the reads, the engagement marks,
  partition pre-creation and whole-month retention. `analytics` keeps the spine side and now imports
  `sendlog` to attribute engagement, which is the one-way direction the split needs. The gzip-JSONL
  archive writer both retention paths share moved to `internal/archivefile`, with `analytics` aliasing
  its old names so `spinech` and `signalsboot` are untouched. No behaviour or schema change.

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

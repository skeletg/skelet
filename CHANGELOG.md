# Changelog

Notable changes per release. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versions follow [Semantic Versioning](https://semver.org/), with the `0.x` caveat
that breaking changes ride the MINOR until `1.0.0`.

The release process, and which version number means what, is in
[docs/releasing.md](docs/releasing.md).

## [Unreleased]

### Changed

- **GA4 and Meta destination credentials move from the URL into the destination's secret.** The
  forwarder adds the GA4 `api_secret` or the Meta `access_token` to the URL when it sends, and refuses a
  target URL that still carries one, because the URL is shown to admins and a failed delivery used to
  copy it into `last_error` and the logs. Destination secrets are now sealed at rest under a new
  `SIGNALS_ENCRYPTION_KEY`, which production requires for the `signals` process and which `./skelet setup`
  and `./skelet up` generate. Audience sync now posts to webhook destinations only, signed like every
  forwarded batch.

- **`JWT_SECRET` is retired and split into keys each tier holds only if it needs them.** It signed
  admin sessions, customer sessions, email links and (as a fallback) review votes, and compose handed it
  to core, both Signals processes and the storefront - so any of them could mint a superuser admin
  token. Now:
  - `ADMIN_JWT_SECRET` signs admin sessions and only core holds it.
  - Customer sessions are Ed25519: core signs with `CUSTOMER_JWT_PRIVATE_KEY`, the Signals processes
    verify with `CUSTOMER_JWT_PUBLIC_KEY` and can mint nothing.
  - `EMAIL_LINK_SECRET` signs unsubscribe, open and click links (core and Signals).
  - `REVIEW_VOTER_TOKEN_SECRET` is required, with no fallback; the storefront holds it and no session key.

  Tokens carry an audience and a key id, and every key has a `_PREVIOUS` list so it can be rotated
  without signing anyone out. Every process refuses to boot while `JWT_SECRET` is still set;
  `./skelet setup` generates the new keys and removes it. Deploying signs everyone out once. See
  [docs/env.md](docs/env.md#session-and-link-keys).

- **Inventory location types are a fixed enum, and pickup is set through the location save.** The admin
  API's `InventoryLocation.type` and the `type` of `createInventoryLocation` / `updateInventoryLocation`
  are now `InventoryLocationType` (`RETAIL_STORE`, `WAREHOUSE`, `STORAGE`, `DROPSHIPPER`,
  `FULFILLMENT_PARTNER`, `UNCLASSIFIED`) instead of a free-form string, so a client sending
  `type: "warehouse"` must send `WAREHOUSE`. `setLocationPickup` is gone: pass `pickupEnabled` and
  `pickupInstructions` to `updateInventoryLocation`. Only retail stores may offer pickup, and the upgrade
  withdraws pickup from locations explicitly typed as something else - see
  [docs/store-availability.md](docs/store-availability.md).

- `placeOrder` takes an `acceptTerms` argument. A store whose `sales/checkout/terms_consent` is `checkbox` (the
  default) now refuses an order sent without it as `TERMS_NOT_ACCEPTED`, so a custom storefront must pass it.

- **The Signals suite is its own Go module.** `apps/backend/signals` holds the event spine and its engines,
  Lens, Synapse's traits and segments, Prism, the event destinations, the beacon surface and the wire
  contract's server, with a `go.mod` that requires core's by relative path. Core cannot import it
  back: the dependency points one way, and Go's `internal/` rule now enforces it at compile time.
  What the suite needs of core sits beside `internal/` - `txrunner`, `cache`, `secrets`, `urlguard` and
  `otelx` moved up whole, and two leaves were split out for it: `dbconn` (pool opening, out of
  `internal/db`) and `objstore` (the S3 client, out of `internal/media`); what only the suite uses -
  `geo`, `uaclass` and the `archivefile` writer - moved into the suite with it. The
  suite's settings are declared in core's `config` package beside the sections that already were, so
  both processes compile one definition of each. The suite process opens no commerce database any
  more: `DATABASE_URL` is not read and `/healthz` pings the signals database alone. Deploys get a
  second image, `skelet-signals:prod` (`SIGNALS_IMAGE`), which the `signals`, `ingest` and
  `clickhouse-migrate` services run; the backend image no longer carries those binaries or the
  ClickHouse schema. Two behaviours moved with the boundary. A merchant's CDP audiences are unioned
  into a customer's segments by core, where the provider rows are, so the beacon surface's
  segment-targeted experiments now see the suite's native audiences only. And the GDPR scrub of the
  send log runs in core beside the identity erasure - it had silently stopped running once the suite
  ran as its own process, since that process was never handed the send log. The `segment.refreshed`
  webhook event is gone; nothing had published it since segments moved onto the suite. `bin/go
  --signals`, `bin/test` and `bin/lint` cover the second module, and `bin/test-e2e` starts a Signals
  process beside core's scenarios and runs the suite's own.

- **The email send log and the suppression list are the suite's, and core reaches them over the
  contract.** Campaigns and flows report a send with the id core mints, so the tracking token names a
  record the suite already holds; the open pixel and click redirect hand the suite the id and get the
  record back, classified against when the send went out, so the "human open" decision moved with the
  log. The suppression list is a `SuppressionService` on the contract: the send loops, back-in-stock,
  the bounce webhook and the deliverability screen all call it as core, and a read fails open so a
  suite blip never stalls a send. The send-log retention sweep is the suite's `PruneSendLog`, still
  on core's schedule, and `signals/analytics/send_log_retention_months` moved into the suite's
  settings store with it - a customised value on core's side reads as the default until set again. A
  flow's `email_activity` condition and a campaign's A/B winner read the suite's tallies over the
  contract too. Core opens no query against `email_sends` or `email_suppressions` any more.

- **Pulse - the email builder, automation flows and campaigns - runs in the Signals suite, and core
  opens no signals database at all.** The three engines moved into `apps/backend/signals` whole, with the
  send tracker, the SMS gateway and their queries, and the signals migration stream moved with them
  to `apps/backend/signals/internal/db/migrations`. Core's `SIGNALS_DB_URL` pool, read pool and generated
  `signalsdb` package are gone; the one place core still names that database is the nightly
  `pg_dump`, which is a DSN and not a query. Everything the dashboard does in Pulse now relays over
  three new contract services (`FlowService`, `CampaignService`, `EmailBuilderService`) with the
  caller's own grants, and the GraphQL surface is unchanged. Core raises the moments a flow can start
  on - an order placed or shipped, a password reset, a form submitted, a product back in stock, a cart
  or a browse abandoned, a segment entered - through `Enroll`, and sends its own transactional mail
  when no flow takes one, exactly as before. The engine ticks are two maintenance calls
  (`AdvanceFlowRuns`, `ProcessDueCampaigns`) that core's scheduler still drives, so the jobs panel
  keeps their history. The open pixel and click redirect (`/e/o`, `/e/c`) are served by `cmd/signals`
  beside the send log they name and proxied there by `app` per the new `SIGNALS_URL`; the
  unsubscribe link stays in core, where consent lives, verified with the same shared token. Who a
  customer is and what shop a channel is come off the rosters core pushes - the channel roster now
  carries the customer-facing store name from `general/branding/store_name` - so the engines read no
  settings of core's; the deliverability switches moved into the suite's settings store with them.
  Two seams reshaped with the move. The builder's products block reads live catalog data over core's
  public storefront API (`CORE_FRONT_API_URL`, a new `productsBySkus` query beside `productsByIds`),
  and renders nothing when that is unset. And the `remote_action` flow node is gone, with the
  `flow_action` remote-provider kind: the engine can no longer reach core's
  provider rows, and a node that waits on an external decision belongs in a suite that can. Saved
  flows carrying one skip the node. The `email` and `emailtrack` packages moved beside `internal/` for
  the suite to import, and the `flows`, `campaigns` and `emails` permission groups are registered in
  core's defaults rather than by the packages that left.

- **Each process backs up the database it holds.** The nightly `db-backup` job dumps the commerce
  database from `app`, then asks the `signals` process over the contract (`BackupDatabase`) to dump
  its own under the same timestamp, so the pair still restores together; `skelet db backup` does the
  same by hand, and `skelet db restore` hands a `skelet-signals-*` key to the suite (`RestoreDatabase`)
  rather than opening that database itself. The `backup` package moved beside `internal/` for the
  suite to import, the Signals image carries `postgresql18-client`, and `SIGNALS_DIRECT_URL` is read
  by the `signals` process now - core reads no signals database variable at all, and `app` and
  `worker` no longer carry the suite's environment in the dev compose. Set the `BACKUP_*` variables
  on both processes; the shared prod env file already does.

- **The commerce schema baseline is regenerated without the Pulse tables**, which its first version
  created only for later migrations to drop. The stream is `000001_schema`, `000002_reference_data`
  and `000003_drop_legacy_pulse_tables` - a no-op on a fresh install, and the drop for a database
  built from the earlier baseline. A development database on the old stream needs
  `bin/migrate force 3`; `bin/test-e2e` rebuilds its own. The `graphqlx` error surface for a Signals
  GraphQL API that was never served is gone, and core's roster source queries live in `roster.sql`.

- **The hand-off to the suite survives the suite being slow or down.** Every call core makes on the
  contract carries a deadline - ten seconds unless the caller set one, thirty minutes for the
  maintenance calls - so a hung suite cannot hang a password reset. `Enroll` names the moment it
  raises (`idempotency_key`: the order, the one-time link, the cart's episode, the registration) and
  the suite keeps that key on the run under a unique index, so the same moment asked twice starts one
  run per flow and answers the same count; core asks a second time before reading an unreachable
  suite as "no flow took it", and the suite finishes an account mail's inline send even when the
  caller's deadline has passed. The customer events that keep the suite's roster current -
  registered, updated, profile updated, deleted - and every consent change ride the outbox rather
  than the fire-and-forget bus, so a suite that was down gets the push, and the welcome flow, on the
  relay's next pass. The abandoned-cart and browse-abandonment jobs stamp a customer as told only
  once the suite has answered for them, instead of on the way past. With OpenTelemetry on, a trace
  that began at core's HTTP edge continues into the suite's handler.

- **The engines drain instead of ticking once.** A scheduler tick used to advance one batch of a
  hundred flow runs and send five hundred recipients per campaign, whatever the minute had left -
  thirty thousand marketing mails an hour at best. `AdvanceFlowRuns` and `ProcessDueCampaigns` now
  take a budget, and the suite keeps claiming batches and sending passes until nothing is pending or
  the budget is spent; core's ticks grant forty-five seconds of their minute. Throughput follows the
  relay and the database rather than a batch size.

- **The builder's products block is resolved once per send, not once per recipient.** The suite keeps
  a block's cards for five minutes, so a campaign of fifty thousand with a product row makes a handful
  of storefront requests instead of fifty thousand, and it signs those requests with the service
  secret - core's per-IP limiter on the storefront API no longer counts the suite as a flood.

- **Whoever enters a segment starts their flow in the suite.** The segment refresh used to hand the
  new members back to core so core could enrol them over the contract again. The engine is in the
  suite, so it enrols them there and then; the response's `entered` list is deprecated and empty.

- **Starter content is an admin action.** The emails hub has an "Add starter content" button: it
  creates an editable email for every transactional slot, an active flow per transactional trigger
  that sends it, and the reusable blocks, and leaves anything the channel already has alone. Nothing
  seeds a channel on its own, since an active order_placed flow moves the receipt out of core's
  templates - that is the operator's call. `seedPulseStarterContent` on the admin API,
  `FlowService.SeedStarterContent` on the contract.

- **The ingest tier serves `/e` beside `/t`.** The open pixel and click redirect are mounted on
  `cmd/ingest` too, so once beacons route to a scaled-out ingest the tracking routes can follow.

- **`apps/backend/` is the server-side umbrella, with three Go modules side by side.** Core moved to
  `apps/backend/core` (module `github.com/skeletg/skelet/apps/backend/core`), the suite to
  `apps/backend/signals`, and the plugin SDK is its own module at `apps/backend/sdk`, where it already
  was, so its import path and every plugin repository's `require` are unchanged. Core requires the SDK
  through a relative `replace`, the suite requires both the same way, and each production image builds
  with `apps/backend` as its context. The dev containers mount `apps/backend` at `/var/www/backend` and
  work from `core/` or `signals/` under it; `bin/go`, `bin/lint`, `bin/sqlc`, CI and the plugin
  workspace follow. The SDK's API gate runs from its own module and still compares against tags from
  before the split.
- **The wire contract is one directory.** The code buf generates from `proto/` now lands in
  `core/signalsapi/v1`, under the hand-written vocabulary, and the `signalspb` directory is gone.
- **The event spine lives in ClickHouse, full stop.** The Postgres spine driver is gone with its
  `SIGNALS_ENGINE` switch: `analytics_events`, `analytics_identities` and the Lens heatmap grid
  leave the Signals database, every analytics read has one implementation, and the suite refuses
  to boot without `SIGNALS_CH_URL`. ClickHouse comes with the `signals` compose profile in dev and
  is a fixed part of the prod stack, with `CLICKHOUSE_PASSWORD` generated by `bin/setup`; the small
  stack gains an opt-in `signals` profile that brings the process and its ClickHouse. The nightly
  Signals backup now also runs `BACKUP DATABASE` into the shared bucket, pruned by the same
  `BACKUP_KEEP`, and `skelet db backup` reports the copy. Gone with the second engine: the
  `BackfillLensRollups` RPC and `skelet lens backfill-rollups`, the Postgres-to-ClickHouse backfill
  script, and the setup question about the spine.
- **Review pass over the branch.** A per-customer read on the contract gained the channel gate its
  neighbours have, and a second stopped skipping that gate when the roster lookup failed. The suite's
  retention and segment jobs now register only where there is a suite to ask, instead of failing every
  tick with it switched off, and the `/t` and `/e` proxies follow the same switch rather than their
  URLs alone. Transactional sends carry their recipient, message key and order id to the spine again;
  a consent change and the event carrying it now commit together; a deleted customer no longer poisons
  the roster outbox; suite-backed settings read as their registered defaults when the suite is
  unreachable instead of failing the whole settings page; a disabled suite reports as off rather than
  red on the health page; browse candidates the commerce check excludes are marked told; the abandoned
  roster stream releases its context; and the storefront catalog cache evicts expired entries first.

- **The spine's ClickHouse starts with whatever needs it.** In dev it rides the `signals`, `ingest`
  and `logs` profiles rather than the suite's alone, so a beacon tier or a log search brings its own
  server up; in production it joins the `bundled` datastores beside Postgres and Valkey, so pointing
  `SIGNALS_CH_URL` at a managed instance drops the container the way the others do. The small stack's
  suite profile now also tells core where to dial it.

- **The production app tier carries `JOBS_EXTERNAL` itself.** The stack always runs a worker, so the
  compose file sets it inline on `app` rather than depending on someone remembering it in
  `env/prod.env`. Every background loop leases what it claims, so an app that also polled was wasted
  connections rather than duplicated work, and the template said otherwise.

- **Session replays and page snapshots live in object storage only.** The `signals/lens/blob_storage`
  setting is gone with the `bytea` columns behind it: a replay chunk and a DOM snapshot are an object
  and a key, which is what the retention sweep already deleted them as. Gone with the second home: the
  `MigrateLensBlobs` and `GetLensBlobStatus` contract methods, `skelet lens migrate-blobs` and
  `skelet lens blob-status` (which leaves the `lens` command group empty, so it goes too). Without
  `S3_BUCKET_NAME` the suite still boots and every other Lens surface works, but recordings and
  heatmap snapshots are dropped at ingest and it says so at boot.

- **Core's shared infrastructure lives under `apps/backend/core/lib/`.** The packages core keeps outside
  `internal/` so the Signals module can import them - `apperr`, `auth`, `config`, `applog`, `envcheck`,
  `httpx`, `observability`, `ratelimit`, `testdb`, `tx`, `cache`, `secrets`, `urlguard`, `otelx`,
  `pgconn`, `objstore`, `email`, `emailtrack` and `backup` - moved
  from the module root into one directory, so the root is back to the entrypoints, the domain, the
  surfaces, the plugins, the SDK and the wire contract. `lib/` is plumbing with no commerce knowledge,
  and a lint rule keeps it that way by refusing imports of `internal/` from it, with no exceptions:
  core's settings store became `internal/configstore` behind `config.Repository`, its token and
  refresh persistence and the admin session log became `internal/authstore` behind the `auth`
  interfaces (`RefreshStore` takes a `SessionLog` and a `Publisher` now, and lists `auth.AdminSession`
  rows), the customer guards that read the resolved store moved to `internal/channel`, the Tools-page
  registrations for config and the cache moved to `wiring`, Sentry's wiring folded into
  `observability`, and `graphqlx` stays in `internal/` as the gqlgen surface only core serves. The
  suite has a test that pins its doors into core to `lib/`, `sdk/`
  and the contract. Import paths change accordingly; the SDK's did not, so plugin repositories are
  untouched.

- **The signals migration stream is squashed to a baseline**, the way commerce was: `000001_schema`
  and `000002_reference_data` (the two system email groups), generated from a fully migrated database
  and proven identical to the fourteen-file stream by dumping both. The dated send-log partitions
  are left out of the dump and created relative to the install date instead. A development database
  on the old stream needs `bin/migrate-signals force 2`; `bin/test-e2e` rebuilds its own.

- **The Signals processes have no switch of their own.** `SIGNALS_ENABLED` is core's, deciding whether
  core dials the suite; the suite's copy of it could leave a deployed suite silently dropping every
  write while core kept sending them. The recorder, the flow engine, the segment reads and the
  beacon surface no longer carry a disabled state, and the health probe reports reachability.

- **Deploy Signals before core.** Core now pushes its deactivated channels as well, and a Signals
  older than this release reads every pushed channel as active, so it would go on sending marketing
  mail under a switched-off shop.

### Removed

- **Database backups.** skelet no longer takes them: gone are the `lib/backup` package, the nightly
  `db-backup` scheduled job, the `skelet cli db backup|backups|restore` commands, the backups health
  group, the ClickHouse spine's `BACKUP DATABASE` job, and the contract's `BackupDatabase` and
  `RestoreDatabase` RPCs. A logical `pg_dump` taken from inside the application is right for a small
  store and quietly stops being right as one grows - it slows down, holds a snapshot against vacuum
  for the length of the run, and restores by rebuilding every index - and the job's own 30-minute
  timeout made that a 3am failure rather than a warning. Backups belong to the infrastructure now:
  point-in-time recovery on managed Postgres, or pgBackRest or wal-g against the bundled containers,
  with the spine taking its own `BACKUP DATABASE`. [docs/backups.md](docs/backups.md) says what needs
  covering. `BACKUP_ENCRYPTION_PASSPHRASE` and `BACKUP_HEARTBEAT_URL` are gone; the rest of the
  `BACKUP_S3_*` variables stay, because the analytics retention archive writes to that bucket.
  `./skelet db export` and `db import` are untouched - they were always local dev snapshots.

- Code nothing reached any more, found by a reachability pass over both modules with every entrypoint
  as a root: the suite's pre-contract trigger vocabulary (`flow.IsAuthTrigger`, `TransactionalTriggers`,
  `IsDefaultTransactional`, `WiredTriggerTypes`) and `campaign.CampaignMergeTags`, which `signalsapi`
  replaced; core's `signalsclient.NewSendID`, since the suite mints send ids; the HTTP half of service
  auth (`auth.ServiceMiddleware`, `auth.ServiceSessionGuard`) that the gRPC interceptors replaced;
  `backup.ArchiveStorage`; the `apperr` class predicates other than `IsNotFound`, in favour of
  `ClassOf`; `config.FindSection` and `FindGroup`; the tax rate-table CSV parser and the two tax
  registry helpers; `payment.RegisteredSlugs`; `pluginmigrate.Runner.Up`; `review.SignVoterToken`
  (the storefront mints, core only verifies - the tests keep a copy); and the action argument readers
  `Args.Int` and `Args.Bool`.

### Security

- Lens session recordings and heatmap snapshots were stored under keys built from the session, channel and
  page, in the media bucket that is usually served publicly, so a snapshot's key could be worked out. Every
  object key now carries a 128-bit random part, `LENS_S3_BUCKET` moves Lens to a private bucket of its own,
  and production warns while Lens shares the public one or `PPROF_ENABLED` is on.

- A webhook node could be added to the password-reset and other account flows, and it posted the run's
  context, sign-in and reset links included, to any URL. Account flows now refuse webhook nodes, every
  webhook payload drops the one-time links, and webhooks no longer follow redirects. A flow can only be
  transactional - skipping consent, suppression and the unsubscribe header - on an order or account
  trigger, whatever its config says.

- **Logging out logs out of the suite too.** The wire contract's interceptor now consults the list of
  killed admin sessions that core publishes on logout, "log out other devices" and session deletion, so
  a service token minted just before is refused for the rest of its two minutes instead of honoured.
  The suite reads the list off the primary Valkey (`VALKEY_URL`), where core writes it; without one the
  check is skipped, as it is on core's own surface.

- **The wire contract has its own secret.** Core signs its calls with `SIGNALS_SERVICE_SECRET` and the
  suite verifies with the same value; `JWT_SECRET` signs sessions and nothing else. Until now one
  secret did both, and compose hands `JWT_SECRET` to the storefront too, so a storefront that held it
  could mint a service token and call the contract as core. Production refuses the new secret empty,
  short, or equal to `JWT_SECRET`, and core refuses to boot with a suite address and no secret. The
  root `.env`, both compose files and `bin/test-e2e` carry it.

### Fixed

- A scripted email block ran with no limits, so a large loop or a self-doubling string could hang or
  exhaust the send loop. Saving one now needs a sample render inside 200 ms and 512 KB, literal loops are
  capped at 1000, `append`/`prepend`/`replace` results are capped, and `capture` is refused. Button,
  image, video and social links must be http(s), mailto, tel or a merge tag, including inside rows.

- A flow email that failed to send was dropped and the run moved on, so a passing SMTP error lost an order
  receipt or a reset email for good. The run now waits on its node and retries on a 1m/5m/15m/1h ladder
  before failing; permanent bounces don't retry and, like campaign bounces, feed the suppression list.
  A run batch stops at half its lease instead of running into another replica's claim.

- An A/B campaign in its test window couldn't be cancelled, so the winner job still mailed the held-back
  audience. It can now, a deleted one is never resolved, and a cancel or delete stops a send pass at its
  next recipient instead of after up to 500 more emails. A recipient without an account is skipped rather
  than mailed without a consent check or unsubscribe link, and audiences are read from the roster a page
  at a time.

- Lens enforces its limits on the server instead of trusting the tracker: a recording stops at
  `replay_max_minutes` and 1000 chunks, a retransmitted chunk is written once, and the player reads at
  most 64 MB of a recording. A known heatmap layout's backdrop can no longer be replaced by a later post,
  a page state keeps at most 20 layouts, and tracker fields are length-capped, with page keys required to
  be a path. Heatmap and Lens dashboard reads now report a failed query instead of showing zeros.

- A tracker batch whose events claimed timestamps spread over more than 100 days made ClickHouse refuse the
  shared insert, and the flusher retried that batch forever, so the spine stopped taking anyone's events.
  Browser timestamps more than a day old or five minutes ahead now take the server's clock, and a batch
  ClickHouse keeps refusing is split until only the refused events are dropped.

- GA4 forwarding sent every batch as one JSON array, which the Measurement Protocol accepts with a 2xx
  and then discards, so nothing ever reached GA4. Each visitor now goes as its own request of at most 25
  events.
- Event destinations no longer follow redirects, which re-sent a signed batch to wherever the target
  pointed, and a destination with a backlog now hands over after 20 batches a run instead of holding up
  the rest.
- An over-limit email click still redirects to its link instead of answering 429.
- `/t/config` and `/t/experiments` send `Vary: X-Channel-Id`, so a shared cache can't serve one store's
  answer to another. One `/t/collect` batch is capped at 2000 events, and Lens page keys must be a path
  of at most 512 bytes, which stops a forged crawl beacon from minting unbounded rollup rows.

- **The AI chat offered Signals tools on a server with Signals switched off.** Its sections were
  gated on permission slugs alone and never consulted `SIGNALS_ENABLED`, so with the suite off the
  model was still handed `get_customer_behaviour`, `get_web_traffic`, `get_experiments` and the rest
  - it called them, got empty results back, and had no way to tell "nothing happened" from "this
  store does not collect that". The four suite sections now follow the switch, so those tools are
  never offered and are refused if called anyway, and the opening digest stops reaching for them.
  `get_customer` moves off the Synapse flag onto `customers.view` proper, since it reads the
  commerce record rather than the behavioural profile and has no reason to disappear with the suite.

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

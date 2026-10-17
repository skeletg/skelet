# The commerce model

The engineering side of the multi-channel model: the identifiers you'll code
against, and the per-domain notes that aren't obvious from the code.

For what a channel *is* and how scope, locale and currency behave, read
[concepts.md](concepts.md) then [channels.md](channels.md). This page assumes
both and doesn't repeat them.

## Header-driven scope (dashboard)

The single source of truth for "what scope am I editing in" is the header
dropdowns, implemented in `apps/dashboard/src/lib/config-scope.ts` (state and
hooks) and `config-scope-ui.tsx` (the selector). Mode is **GLOBAL** or
**CHANNEL**. In CHANNEL the user picks a channel and optionally a locale. Pages
read from `useConfigScope()`:

- `effectiveChannelId` - the channel id when CHANNEL, else `undefined`
- `effectiveLocale` - explicit locale > channel default > `''`
- `isGlobal` / `isChannel` flags

**Rules** (wired across products, customers, blog, cms, categories, collections,
currencies, countries, locales, attributes, product types):

- Lists in CHANNEL are filtered to that channel. GLOBAL shows everything.
- Detail pages drop in-form locale pickers; the header is the only locale
  source. Detail forms in GLOBAL show translation fields read-only.
- Per-channel availability cards: GLOBAL renders a multi-channel editor;
  CHANNEL renders a single-row editor for the active channel.
- Truly global entities (categories, collections, attributes, currencies,
  countries, locales, product types) disable mutations in CHANNEL.
- Mutations that *require* a channel gate behind `useScopeRequirement({
  requiresChannel: true })` rather than checking the flag by hand.
- Switching the header with unsaved edits triggers a confirm prompt
  (registered via `useUnsavedChangesGuard(isDirty)`).

## Per-channel availability

Products, categories, CMS pages and blog posts each have a per-(entity, channel)
listing row that gates storefront visibility. Without a row, the storefront hides
the entity entirely. The Go domain is symmetric across these:

| Entity | Go type | Where |
|---|---|---|
| Product | `catalog.ProductChannelListing` | `internal/catalog/listing.go` |
| Category | `catalog.CategoryChannelListing` | `internal/catalog/category.go` |
| Blog post | `blog.PostChannelListing` | `internal/blog/domain.go` |
| CMS page | `db.CmsPageChannelListing` | `internal/cms/repository.go` |

The tables are `product_channel_listings`, `category_channel_listings`,
`blog_post_channel_listings` and `cms_page_channel_listings`, all created in the
schema baseline.

Admin: edit listings via the entity's detail page (single-row in CHANNEL,
multi-channel grid in GLOBAL). The save-confirm modal lists every staged change
before commit.

## Permissions

Code-declared: `permission.RegisterGroup` + `permission.Register` in domain
`init()`, with the core groups in `internal/permission/defaults.go` and the rest
registered by their own domain packages. `permission.Sync()`
(`internal/permission/registry.go`) upserts rows and CROSS JOIN grants all
permissions to `administrator`, so adding a permission needs no migration.

- **Admins**: permissions via roles; `is_superuser = true` bypasses checks.
- **Integrations**: permissions directly via `integration_permissions`.
- **Guards** (`auth/guard.go`, `auth/actor.go`): `RequireActor` (an admin or a token),
  `RequireAdmin` (a person only), `RequireCustomerAuth`,
  `RequirePermission(ctx, checker, slug)`, `RequireTokenPermission(ctx, slug)`,
  `RequireChannelPermission(ctx, checker, slug, channelID, locale)` - the last one
  keeps a pre-rename name, but the argument is a channel id.

Seeded roles: `administrator` (all), `store_manager` (catalog/orders/customers),
`support_agent` (read-only), seeded in the reference-data baseline.

**The slug catalogue is generated, not listed here** - see
[permissions.md](permissions.md) for the current reference and
[permissions-enforcement.md](permissions-enforcement.md) for the
who/what/scope contract to read before writing a resolver gate. A hand-copied
list in this file went stale; don't reintroduce one.

## Key domain notes

**Attribute value storage (catalog)**: the attribute's input type picks
the store. Scalars (plain_text/numeric/boolean/date/date_time) live in
`catalog_products.attributes` JSONB keyed by attribute slug; selectables
(dropdown/multiselect/swatch) are junction rows in
`catalog_product_attribute_values` pointing at the per-attribute value
dictionary. The JSONB value is canonical and locale-independent - it is
what the admin edits, the indexer's facet slugs, URL `{attr:}` tokens,
and CSV import/export all read. Locale is a sparse overlay resolved at
the front GraphQL edge only (`graph/front/attribute_localization.go`):
plain_text values can get per-locale override rows
(`catalog_product_attribute_locale_values`, empty/equal-to-canonical =
no row, falls back to canonical); selectable value names translate at
the dictionary level (`catalog_attribute_value_content`). Channels never
scope values - only URL slugs (`catalog_attribute_value_channel_slugs`,
`catalog_product_attribute_slugs` global row + per-channel override).

**Admin users**: `roleSlugs: []` clears all roles; nil leaves unchanged.
`deleteRole` refuses (`ErrRoleInUse`) if role has members.

**Customer**: `adminCreateCustomer` - admin supplies password,
`isEmailVerified` defaults true, no verification email fired.
`deleteCustomer` refuses (`ErrCustomerHasOrders`) if orders exist.
`Customer.group` is non-nullable - every return path must load it;
`toCustomerModel` panics on nil in dev to catch missed loads.
`setDefaultCustomerGroup` atomically swaps via tx, no window with zero
defaults. `Customer.note` is admin-only internal text (not exposed to
the customer-facing API).

**Consent** (`customer_consents`): nil row falls back to
`Definition.DefaultOptIn`. **Load-bearing rule: any marketing send MUST
go through `consent.Service.IsOptedIn`** - raw `repo.Get` treats nil as
"no opinion" and will send to non-consenting customers.

**Email**: `LogSender` (dev, no SMTP) / `SMTPSender` (env:
`SMTP_HOST/PORT/USERNAME/PASSWORD`). `SMTP_USERNAME` empty = skip auth.
Non-dev with no SMTP → error-sender that fails every Send loudly.
Mailpit sidecar via `./skelet up`: catches mail at `:1025`, UI at
<http://localhost:8025>. Token stores: `VerificationStore` (24h, prefix
`verify:customer`) + `PasswordResetStore` (30m, prefix `reset:customer`).
`resetPassword` calls `RefreshStore.RevokeAll`. `STOREFRONT_URL` env
builds link base.

**Cart**: guest cart - knowing UUID is the capability. Customer cart -
JWT required. `mergeGuestCart` adopts guest cart if no active customer
cart (no copy). `ConvertCart` is the concurrent placement serialization
point - concurrent callers get `ErrCartAlreadyConverted`. Statuses:
`active → converted` (placeOrder) or `merged` (mergeGuestCart).

**Checkout** lives on the cart, in three steps: how the order arrives,
who it is for, then payment. `setCartDestination` saves a country and
postcode, which is all delivery and pickup need to be quoted, so no
address is typed until the second step. `setCartShippingMethod` picks
one of the quotes, delivery or pickup. `setCartDetails` saves contact,
the business details, and either the delivery address with its billing
address or, for pickup, who collects it, in one write. A pickup order
needs no shipping address: placement ships it to the pickup location,
addressed to the collector. Then `setCartPaymentMethod`,
`applyGiftCard` and `setCartStoreCredit`, plus `setCartEmail` and
`setCartNote`. Each is validated in the resolver and saved on the cart,
so a reload resumes where the shopper was. A step is computed from the
cart as loaded, with no transaction open, since its quotes and checks
may wait on a carrier or a remote validator; the write is a short
transaction that takes the row lock and saves only if
`carts.checkout_version` hasn't moved, recomputing the step otherwise
(`CHECKOUT_CONFLICT` after three tries). The steps and `Cart.checkout`
share a per-IP budget, `CHECKOUT_RATE_LIMIT_PER_MIN`. What the
details step asks for and requires follows the `sales/checkout/*`
settings (phone, business purchase, PO number, someone else
collecting, a billing address on pickup orders), and placement
re-checks them. The company identifiers a business purchase asks for
are defined per country (`countries.business_id_fields`: label, key,
required, an RE2 pattern, a hint) and follow the company's country - its
billing address, else the destination - with one generic tax ID, per
`sales/checkout/tax_id_field`, where a country defines none. The order
keeps them with their labels (`orders.business_ids`) for the invoice. Addresses are snapshots with the saved address they
came from. Moving the destination, or saving an address in another
postcode, drops a shipping or payment method it no longer supports, and
`Cart.checkout` re-checks both on read, so a line change that alters the
quotes shows up as an unchosen method. A carrier that fails to answer
isn't one that stopped offering the method: the choice is kept, and
choosing or placing answers `SHIPPING_QUOTE_FAILED`. Purchase order is
offered only on a business purchase whose details step asks for the PO
number (`sales/checkout/po_number_field`), and payment eligibility is
checked for the destination's country at every step and at placement,
pickup included. A pickup with no billing address is billed to the
collector by name, never to the store's address. `Cart.checkout.totals` is priced
by `order.Service.QuoteCheckout`, the placement pricing path without the
transaction, which quotes from the destination alone until there is an
address. `placeOrder(cartId, paymentToken, savedPaymentMethodId,
acceptTerms)` reads everything else from the cart and refuses an
incomplete checkout with a `CHECKOUT_*_REQUIRED` code, a chosen method
the carriers no longer offer with `SHIPPING_METHOD_UNAVAILABLE` rather
than the cheapest one, and an unticked terms box with
`TERMS_NOT_ACCEPTED` when `sales/checkout/terms_consent` is `checkbox`. On merge the guest cart's selections and
email win.

**Orders**: prices in **cents** (BIGINT). Address denormalized on
`orders` (snapshot at placement, never changes). `order_events`
immutable audit log. Number format `ORD-001000`. `PlaceOrder` wraps
create+lines+event+convert-cart in one tx. Status:
`pending → confirmed → processing → fulfilled → cancelled`.

**Config**: paths always `section/group/field`. Scope is `default` or
`channel`; all global values at `default/0`. Resolution:
requested scope → default/0 → registry default. `Set` validates +
normalizes (int `"007"` → `"7"`, bool `"1"` → `"true"`). Unknown paths
rejected.

**Catalog**: `ProductType` slug immutable. `Attribute` slug + inputType
immutable (changing inputType orphans values). Dual attribute storage:
scalar types (`plain_text/numeric/boolean/date/date_time`) →
`products.attributes JSONB`; selectable types
(`dropdown/multiselect/swatch`) → `product_attribute_values` junction.
`setProductAttributes` upsert - only touched attributes change.
`hasVariants` flip blocked while variants exist
(`ErrHasVariantsFlipBlocked`). Known footgun: deleting an attribute
leaves dead JSONB keys on products.

**Catalog product listings**: `ProductChannelListing` ties (product,
channel) with `isPublished` + optional explicit price. `null` price means
inherit from `defaultPrice` - only valid when the channel currency matches
the product's `defaultCurrencyCode`. Currency mismatch on inherit is
rejected (`ErrListingCurrencyMismatch`).

**Catalog category listings**: `CategoryChannelListing` ties (category,
channel) with `isPublished` + optional `sort_order_override`. Without a
row, the category is hidden on that storefront. Storefront resolvers
(`rootCategories`/`categoryChildren`/`category`) branch on the active
channel: with one in scope, only listed-and-published categories are
returned. Admin's `rootCategories(channelId)` and
`categoryChildren(parentId, channelId)` accept the same filter so the
admin tree mirrors what the storefront serves. The dashboard's category
pickers read `categoryOptions(channelId)` instead - every category flat,
with `parentId` and a direct product count, which the client builds into a
tree at any depth (the nested tree queries stop at a fixed depth). Creating a category in
CHANNEL mode auto-fires `setCategoryListing` on the active channel so
new categories show up where the user is working.

**Events** (`internal/events/`): `Publish` (async, background goroutine,
errors logged, panics recovered) · `PublishSync` (blocking, first error
aborts chain). `-ing` events = pre-commit sync (can abort/enrich).
`-ed` events = post-commit async. Bus is in-process only - not durable.
Subscriber wiring in `RegisterDomainSubscribers` in
`internal/wiring/subscribers.go` - registration order there is load-bearing.

**Webhooks** (`internal/webhook/`): `async` (post-commit) · `sync`
(pre-commit). HMAC-SHA256 signature in `X-Webhook-Signature-256:
sha256=<hex>` over `<ts>.<body>`; envelope `{"apiVersion","event",
"timestamp","data"}` (+ `X-Webhook-Api-Version` header). Secrets
(`whsec_<hex>`) shown once, stored plaintext as the outbound signing key -
never returned via GraphQL.
- **Delivery contract (v2):** each webhook carries `failure_policy`
  (`fail_closed` default / `fail_open`), `timeout_ms` (0=kind default,
  ≤30s), `custom_headers` (reserved names rejected), `payload_version`.
  Sync `fail_open` forgives transport/timeout/non-2xx but still honors an
  explicit `{"reject":true}`. A per-process circuit breaker
  (`internal/circuit`, 5 fails → 30s open) guards sync targets; state on
  `Webhook.circuit`.
- **Async delivery** is queue-backed (`webhook_queue`): the
  dispatcher enqueues, `QueueWorker` delivers on the 10s/60s/300s ladder
  (survives restarts), exhausted rows become redeliverable dead letters.
  `redeliverWebhookDelivery` re-enqueues any historical delivery.
- **Subscriptions** accept exact types, `*`, or `<prefix>.*` wildcards
  (matched in SQL + `webhook.MatchesEntry` - keep in sync). Sync events
  implement `SyncResponseApplier`; response mutation coverage includes
  cart price/qty, inventory routing, review status, and order
  discount/risk/tags. Tax/shipping/total are NOT webhook-settable - they're
  owned by the remote tax + shipping providers (one HTTP source per
  number). `checkout.validating` is the pre-transaction veto point (twin of
  `order.placing`, no held tx).

**Remote providers** (`internal/remoteprovider/`): provider-shaped webhooks
- merchant HTTP endpoints that IMPLEMENT a pluggable role instead of observing
events. 16 kinds, `AllKinds` in `domain.go` is the list (no payment kind, on
purpose - PCI scope; gateways are Tier 1 plugins). Integrator-facing contract
is `docs/remote-providers.md`; per-kind JSON Schemas + example payloads are
GENERATED from the adapter structs into `internal/remoteprovider/contract/` by
`cmd/providerschema` (runs in `./skelet gen`, so a renamed payload type fails
the run rather than leaving a stale published schema). `./skelet provider`
conformance-tests an endpoint. The `remote_providers` table carries
kind+slug identity, config JSON, timeout, cache TTL, failure policy,
channel scope and an `rpsec_` secret. The `Invoker` signs a
`{kind,provider,timestamp,config,payload}` envelope (`X-Provider-Signature-256`),
enforces per-invoke timeout + payload-keyed response cache + circuit breaker.
Consumers wire adapters in main.go: shipping `SetRemoteRateSource`, order
`SetRemoteTax`, `address.RegisterRemoteValidator`,
`promotion.RegisterRemoteProviderTypes` (remote.condition/remote.action rule
types). Admin CRUD gated
by `remote_providers.view/manage`; `remoteprovider.MemRepo` for consumer
tests. SSRF guard shared via `urlguard` - its own package so webhook and
remote providers can both use it without importing each other.

**Admin Actions**: handlers are closures - register after services
built, before `action.Validate()`. Default 30s timeout. Args numbers
arrive as `float64` - use `Args.Int(name)`.

**Scheduler** (`internal/scheduler/`): DB-backed scheduled jobs claimed
atomically via `FOR UPDATE SKIP LOCKED` + a lease (multi-replica safe, no
leader election). Code jobs register at boot (`Register`), plugins via
`sdk.RegisterScheduledJob`; webhook jobs are admin-created (SSRF-guarded,
HMAC-signed). Per-job timezone, retries/backoff, failure alerting, and run
history. Global pause + alert URL + history retention live in config
(`system/scheduler/*`). Durable async event delivery is the Postgres
outbox (`internal/outbox/`), not a broker.

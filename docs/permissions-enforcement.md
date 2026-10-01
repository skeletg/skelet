# Permissions: enforcement

Skelet's admin permission model has three axes: **who** (admin user)
holds **what** (permission slug like `orders.view`) **at what scope**
(any combination of channel + locale, or "any"). This doc is the
contract for resolver authors.

Configuring roles rather than writing guards? [`permissions.md`](permissions.md) is the
operator's guide, and carries the generated slug reference.

## Data model

All grants live in `admin_role_permission_scopes`:

```
(role_id, permission_id, channel_id NULL, locale TEXT NULL)
```

Each row is **one** grant. A role with three permissions across two
channels has six rows, and NULL in either scope column means "any".
[`permissions.md`](permissions.md#the-model) has what the scopes mean and
how they stack; what matters when you are reading the SQL:

- A role can grant **different permissions on different channels** -
  e.g. one role granting `catalog.manage` on EU but only `catalog.view`
  on US. This is a single-table redesign; the earlier "all permissions
  inherit all scopes" model forced two separate roles.
- Locale and channel scope are **independent** columns. A translator role
  can grant `cms.manage` with `(channel=NULL, locale='de')`, which works
  on every channel that carries German content.
- There's no `all_channels` boolean to look for. "Global for slug X" is
  derived from "at least one scope row exists for slug X with both
  columns NULL".
- Superusers never reach the table at all - `RequireChannelPermission`
  and friends short-circuit on the claim before consulting a checker.

## The endpoint does not authenticate for you

`auth.Middleware` is **non-blocking on purpose**: it parses the JWT, injects
claims, and calls the next handler either way. `/api/admin/graphql` has no
blanket "must be signed in" gate in front of it - the resolver is the gate.

The consequence is worth stating plainly: **a Query or Mutation resolver with
no guard answers anonymous callers.** Not "any admin" - anyone who can reach
the endpoint. There is no second line of defence behind the resolver.

`TestEveryAdminResolverIsGated` (`graph/admin/authz_coverage_test.go`) walks
the package and fails if any exported `queryResolver` / `mutationResolver`
method neither guards itself nor calls a helper that does. Resolvers that are
genuinely public (login, the password-reset pair, token refresh, the dev-mode
ones, plugin-delegated fields whose gate lives in the plugin, and the
self-scoped chat history) are listed in `publicResolvers` with a
reason each. A second test fails on stale entries, so an allowlisted resolver
that later grows a guard has to leave the list.

If you add a resolver, the test tells you which of the two things you forgot.

## Permission slugs

Resources register their own slugs in their `register.go` (or domain init).
Convention is `<resource>.view`, `<resource>.manage`, plus extras like
`reviews.moderate`. Slugs are global identifiers - the scope (global vs.
channel vs. locale) is attached on the role assignment, not the slug.

See `apps/dashboard/src/features/admins/roles/RoleEditPage.tsx` for the
UI that creates these assignments.

## Resolver patterns

### 1. Mutations that touch a single channel

Always know the `channelID` up-front (from the input). Use
`RequireChannelPermission`:

```go
func (r *mutationResolver) UpdateOrder(ctx context.Context, input model.UpdateOrderInput) (*model.Order, error) {
    channelID, _ := uuid.Parse(input.ChannelID)
    if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.manage", channelID, nil); err != nil {
        return nil, err
    }
    // …mutate…
}
```

For locale-specific mutations (e.g. editing the German translation of a
CMS page), pass the locale too: `RequireChannelPermission(..., &locale)`.

### 2. Single-resource queries (Order, Customer, Promotion, etc.)

Same as mutations - once you've loaded the row from the service, gate on
its channel ID before returning it:

```go
order, err := r.OrderSvc.Get(ctx, id)
if err != nil { return nil, err }
if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", order.ChannelID, nil); err != nil {
    return nil, err
}
return toOrderModel(order), nil
```

A scoped admin asking by ID for a channel they can't access gets
`FORBIDDEN`, not a 404 - keep the distinction.

### 3. List queries (orders, reviews, promotions, etc.)

Load the permitted-channel set once, fetch the full list, filter:

```go
perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "orders.view")
if err != nil { return nil, err }
orders, err := r.OrderSvc.List(ctx)
if err != nil { return nil, err }
visible := auth.FilterByChannel(orders, perm, func(o order.Order) uuid.UUID { return o.ChannelID })
```

The `FilterByChannel` generic is a no-op when `perm.Bypass()` is true
(superuser, global-scoped role) so the cost on the unscoped path is one
slice copy avoided.

When the underlying repo supports `WHERE channel_id = ANY(...)`, prefer
passing `perm.IDs()` into the query - fewer rows scanned. Most repos
don't today; the post-filter is fine until they grow that surface.

**`IDs()` nil vs empty is load-bearing.** It returns `nil` only on bypass,
meaning "no channel filter", and a non-nil empty slice for an admin permitted
on zero channels, meaning "match nothing". Callers key off exactly that (see
`reviews.resolvers.go`). Collapsing the empty case to `nil` - the sort of
thing an innocuous `var out []uuid.UUID` refactor does - would silently show a
zero-channel admin every channel. `TestPermittedChannels_IDsNilOnlyOnBypass`
pins it.

### 4. Nested field resolvers (`Product.listings`, `Category.listings`, etc.)

Per-channel collections inside a parent object leak the same way a list
query would. Filter them in the field resolver:

```go
func (r *productResolver) Listings(ctx context.Context, obj *model.Product) ([]*model.ProductChannelListing, error) {
    perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "catalog.view")
    if err != nil { return nil, err }
    return auth.FilterByChannel(obj.Listings, perm, func(l *model.ProductChannelListing) uuid.UUID {
        id, _ := uuid.Parse(l.ChannelID)
        return id
    }), nil
}
```

If the parent resolver already loaded the array eagerly, the field resolver
just filters in place - no extra DB roundtrip.

### 5. Operations whose scope depends on their target

Some entry points are global or channel-scoped depending on what they were
asked to do - an export of every channel versus one channel's, a promotion
rule with a null channel versus a channel's own. Two helpers cover this so
each caller doesn't re-derive the rule:

- `RequireGlobalOrChannelPermission(ctx, checker, slug, *uuid.UUID)` - a nil
  channel demands the global grant, otherwise it gates on that channel. This
  is the one for nullable-channel columns (promotion rules).
- `RequireScopedOrGlobal(ctx, svc, slug, channelID, global bool)` - same idea
  with an explicit `global` flag, for operations whose blast radius is the
  whole installation when untargeted. Import/export uses it, and the GraphQL
  and HTTP surfaces share the one implementation so they can't drift.

### 6. Global-only resources (currencies, countries, locales, admin users)

These are not channel-scoped - they live above the channel layer. Use
`RequireGlobalPermission` or plain `RequirePermission`. A scoped admin
without any global slug grant cannot manage them, which is the intended
behaviour.

### 7. Product-global rows shared across listing channels

Content translations, attribute values, media, downloads, subscription
terms, product options and their values, bundle options and selections,
grouped members, kit components, variant structure - one row, rendered by every channel that lists
the product. A plain `RequirePermission` here would let a Channel-A-scoped
admin edit data that shows on Channel B. Gate these with
`RequireAllListedChannelsPermission`: the admin needs the slug on *every*
channel currently listing the product (or a global grant). Pass the locale
for locale-bound rows (content); pass `""` for non-localized rows, where a
grant of any locale scope counts per channel. In `graph/admin` the
`requireCatalogManageOnAllListingChannels` helper wraps the listings
lookup for the catalog mutations.

When the mutation only takes a child row's id (an option, a value, a bundle
selection, a grouped member, a kit component), call the matching
`requireCatalogManageFor<Child>` helper in `catalog_product_authz.go`. Each
resolves child to owning product and then calls the gate above. Do it right
after the id parse: a gate that reads the row *after* the write is not a gate.
Watch which id is the owner. `BundleSelection.ProductID` is the product being
selected into the bundle, not the bundle itself (that one comes through
`OptionID`), while `KitComponent.ProductID` *is* the owning kit and
`ComponentProductID` is the reference.

Per-channel option surcharges (`setProductOptionChannelPrice`,
`setProductOptionValueChannelPrice`) take the same gate, and the channel they
name has to be one the product is listed on - so a channel-scoped admin can
only ever price a channel they already hold.

`graph/admin/catalog_product_scope_test.go` enforces this: every mutation in
the product-row resolver files has to reach a channel-aware gate, so a bare
`RequirePermission` fails the build. Product *types* are deliberately out of
scope, being global config with no listings of their own.

## The "no channel at all" admin

An admin whose roles grant `orders.view` only on Channel A will:
- See Channel A in the channel picker (the channels list filters too).
- See orders only from Channel A.
- Get `FORBIDDEN` on any mutation against Channel B.
- See a Product detail page with no Listings rows for channels they can't access (the field filter strips them).

An admin with **zero** assignments granting a slug sees an empty list / no
records / no mutations available. That's correct - they shouldn't be able
to act on anything.

## What's enforced today

| Resource | List/Query | Single get | Mutation | Notes |
| --- | --- | --- | --- | --- |
| Channels | ✅ filtered | - | ✅ via `channels.manage` per id | `channels.resolvers.go` |
| Catalog mutations | n/a | n/a | ✅ via `RequireChannelPermission` | Per-channel listing + product listing setters |
| Product-global mutations (content, attributes, media, downloads, terms, options, bundle/grouped/kit structure, children, variant mode) | n/a | n/a | ✅ all-listing-channels gate | See pattern 7; CSV import and create/update/delete product require a global grant |
| Product.listings (nested) | n/a | ✅ filtered in `renderProductListings` | n/a | Strips listings for channels the admin can't see |
| Customers | ✅ filtered | ✅ load-then-gate | ✅ via `requireCustomerManage` helper | `customer_helpers.go` |
| Orders | ✅ filtered | ✅ load-then-gate | ✅ via `adminTransitionOrderStatus` | Status transitions check order's channel |
| Reviews | ✅ filtered (TotalCount drops to filtered count) | ✅ load-then-gate | ✅ via `requireReviewModerate` | |
| Promotions | ✅ filtered (rules with nil ChannelID are global) | ✅ via `RequireGlobalOrChannelPermission` | ✅ on Create/Update/Delete, both old + new channel checked on update | |
| Blog channel listings | n/a | n/a | ✅ `Set/DeleteBlogPostChannelListing` | The post itself is global; listings are per-channel |
| CMS channel listings | n/a | n/a | ✅ `Set/DeleteCMSPageChannelListing` | Same as blog |
| Menus | n/a | n/a | ✅ `SetMenuStores` checks every assigned channel | Uses `catalog.manage` slug |
| URL rewrites | - | - | ✅ pre-existing per-channel gate | |
| Webhooks | - | - | ✅ pre-existing per-channel gate | |
| Inventory low-stock list | ✅ filters via location's channel assignment (when applicable) | n/a | ✅ `RequireChannelPermission` on per-channel ops | Inventory locations are global; channel↔location bindings are gated |
| Tax rules | ✅ filtered (cross-channel view) / gated (single channel) | ✅ load-then-gate | ✅ on the rule's own channel | Every rule carries a `channel_id`, so nothing here is global |
| Shipping zones, methods, boxes, carriers | ✅ gated per channel | ✅ load-then-gate | ✅ per channel; a method inherits its zone's | `requireZoneScope` / `requireMethodScope` in `shipping_helpers.go` |
| Category.listings (nested) | n/a | ✅ filtered in the `Category.listings` field resolver | n/a | Matches `Product.listings`; wired via `resolver: true` in gqlgen.admin.yml |
| Credit memos | ✅ `orders.view` (per order) | ✅ `orders.view` on the memo's order | ✅ `orders.refund` to issue | `creditableQuantities` needs `orders.refund` - it only feeds the issue form |
| Blog taxonomy (categories, tags, authors) | ✅ `blog.view` | ✅ `blog.view` | ✅ `blog.manage` | Global entities. Authors carry an email, so the reads are gated too |
| Review questions | ✅ `reviews.view` | ✅ `reviews.view` | ✅ `reviews.manage` | Global entities (assigned to product types) |
| Promotion condition/action catalogs | ✅ `promotions.view` | n/a | n/a | Static registry metadata |
| Feature + mode flags (`serverFeatures`, `aiStatus`, `inventoryMode`), `resolveUrl` | ✅ `RequireAuth` | n/a | n/a | Every signed-in admin needs these for nav and page chrome, so authentication rather than a slug |

## Refunds and credit memos

Every refund writes a **credit memo**: a numbered record of what was credited,
which lines, and where the money went. Two ways one is born:

- Settling an RMA. `FinalizeReturnRefund` writes the memo in the same
  transaction as the money, so the two can't disagree. One memo per return,
  enforced by a unique index, which makes a replay a no-op.
- `issueCreditMemo`, with no return behind it. Goodwill, a pricing correction,
  a shipping-only credit. Gated by `orders.refund` on the order's channel.

Three money paths, picked from the order and the requested target:

| Situation | What happens |
| --- | --- |
| Gateway order, refund to original | The gateway's refund API, keyed on the ledger row's idempotency key |
| Any order, refund to gift card | A new gift card is issued - no gateway involved |
| Order with no gateway handle, refund to original | Recorded against the ledger with no provider call - an offline refund |

**The ledger is not gateway-only.** `RecordOfflinePayment` writes a
`capture` row with `provider_slug = 'offline'`, so cash on delivery, bank
transfer, purchase orders and pickup carry the same ledger shape a card order
does. Without that row `Captured` would stay 0 forever on those orders and every
refund guard would read them as "nothing was ever collected", making a refund
impossible - the RMA would park in `received` with no way out.

The refundable ceiling is per-target, not one gateway number (`refundCeiling`
in `internal/order/returns.go`): gateway refunds clamp to captured-minus-
refunded, gift-card refunds clamp to the order total minus refunds, because
issuing store value is not gateway money. Clamping the gift-card path to the
gateway is what made a gift-card-paid order un-refundable even back to a gift
card.

### Cancelling and refunding compensate differently

Both publish an event that other subsystems react to, and they carry **different
sets** of reversals. Getting this backwards pays the customer twice.

| | Tenders consumed (gift card, store credit, B2B credit) | Rewards granted (loyalty, referral) |
| --- | --- | --- |
| `order.cancelled` | Restored - the order is void, nothing was paid for | Revoked |
| `order.refunded` | **Left alone** - the refund itself already returns the money | Revoked |

`OrderRefunded.MerchandiseFraction` is the **cumulative** share of
`subtotal - discount` credited across every memo, so reversals revoke toward a
target rather than by one memo: successive partial refunds each take their
share, and a replayed at-least-once delivery takes nothing. It is merchandise,
not money, because that is what loyalty earns on - a shipping-only or
goodwill-adjustment memo refunds real money and correctly revokes zero points.

Referral keys off `FullyRefunded` instead: a referral reward is all-or-nothing,
so a partial return on a genuine order must not cost the referrer their payout.

## Inventory modes

Inventory has two modes, switched via `sales/inventory/flat_mode`:

- **MULTI_LOCATION** (default): per-product `inventory_items`,
  per-location `inventory_levels`, the `inventory_reservations`
  ledger. Salability = (sum physical at channel's locations) - net
  reservations. When the on-hand quantity drops is a second setting,
  `sales/inventory/deduction_step`: `fulfillment` (default) holds stock
  as a reservation at placement and deducts when a shipment is created,
  `placement` deducts immediately. Salable stock reads the same either
  way - the difference is what a per-location quantity means.
- **FLAT**: an external ERP is the source of truth.
  Each `inventory_items` row has a single `quantity` column; locations,
  levels, reservations are dormant. Salability = `item.quantity`
  (per-product, no channel dimension). Order placement does NOT
  decrement. The ERP pushes updates via the
  `syncProductInventory` / `syncProductInventoryBatch` admin
  mutations.

Both modes gate writes on `inventory.manage` (no separate `.sync`
slug). The flat-mode mutations are permitted in either mode so the
audit trail (`last_synced_at`, `last_synced_source`) stays
consistent. Switching modes does not migrate data - `inventory_levels`
rows survive a flip and become live again on switch-back.

## Plugin / microservice URL rewrites

Skelet's `url_rewrites` table is the single source of truth for
"pretty path → entity" mappings. The eight core entity types
(`product`, `category`, `collection`, `blog_post`, `blog_category`,
`blog_tag`, `blog_author`, `cms_page`) call `urlrewrite.Service`
in-process from their domain services. Tier 1 plugins and external
microservices that own their own URL-addressable entities (e.g. a
loyalty-program service) can hook into the same machinery via the
admin GraphQL mutations:

- `syncCanonicalUrlRewrite(input: { entityType, entityId, requestPath })`
  - upsert the canonical row. Called on create + slug change.
- `createUrlRewriteRedirect(input: { entityType, entityId, fromPath, redirectType })`
  - 301/302 from the old path to the new canonical. Called on slug
  change immediately after the canonical sync.
- `deleteUrlRewritesForEntity(entityType, entityId)` - drops every
  rewrite for the entity. Called on entity delete.

All three are gated by the **`urlrewrite.manage`** permission slug.
The integration that authenticates the microservice (created via the
admin's Integrations page) needs that slug granted to its
auto-generated role. Skelet does not call back into the microservice
- the plugin owns its data; Skelet only owns the path-to-id mapping.

### Storefront routing for plugin entities

When the storefront's catch-all calls `route(path)` and the matched
rewrite has an entity_type Skelet doesn't recognize, the response
returns:

- `kind: PLUGIN`
- `pluginKind: "<the entity_type slug>"`
- `entityId: "<uuid>"`

The storefront switches on `pluginKind` to pick a renderer. The
renderer is expected to fetch entity data itself - either from
Skelet's GraphQL surface (if the plugin extends it) or directly from
the microservice. Unknown `pluginKind` values render as 404.

### Slug naming

Plugin entity_type slugs should be lowercase, snake_case, and
namespaced where collisions are likely (e.g. `loyalty_program`, not
`program`). The DB enforces no closed list - collisions across
plugins are the operator's problem to avoid.

## What's NOT scope-aware (intentionally, today)

- Global resources: currencies, countries, locales, product types, attributes, admin users, API tokens, integrations/apps. They live above the channel layer; permission scope is global.
- Inventory locations themselves (warehouses are global - their *assignments to channels* are gated via `channels.manage`).
- Media library (assets are not per-channel).
- Blog/CMS authoring (creating an author, tag, category, post body) - global. The per-channel publication state is gated separately via the listing setters above.

## Known gaps / deferred

These are spots where channel-aware gating would tighten things further but aren't done yet. Add as needed.

- Order list pagination total: when scope filters out part of a fetched page, the offset can leave gaps. Acceptable until a repo-level channel filter lands.
- `customers.manage` group-level mutations (Create/Update/Delete CustomerGroup, SetDefaultCustomerGroup) are still `RequirePermission` (not channel-scoped) because customer groups are global entities. Confirm this is the intent before changing.
- `hasStore` treats a check with **no** locale as satisfied by a locale-scoped grant, so a `(EU, de)` translator passes a non-locale-specific `(EU, -)` mutation check. That's the SQL's behaviour, mirrored deliberately, and `TestGrantSnapshot_HasStore` pins it - but it means locale scope only narrows resolvers that actually pass a locale. **This is the one open decision in this doc**: tightening it would make locale-scoped roles genuinely narrower, at the cost of breaking any role that relies on the current widening.

When in doubt: if the resource has a `channel_id` column or a per-channel
junction table, it should respect the scope. If it doesn't, it's a global
resource and uses `RequireGlobalPermission` / `RequirePermission`.

## Adding a new resource

1. Register the slugs in your package's `register.go`:
   ```go
   permission.Register(permission.Definition{Group: "myresource", Slug: "myresource.view", ...})
   permission.Register(permission.Definition{Group: "myresource", Slug: "myresource.manage", ...})
   ```
2. In every list resolver: `LoadPermittedChannels` + `FilterByChannel`.
3. In every single-fetch resolver: load the row, then `RequireChannelPermission` against its `channel_id`.
4. In every mutation: `RequireChannelPermission` against the input's `channel_id`.
5. In nested field resolvers that iterate per-channel children: filter
   with `FilterByChannel`.
6. Add a row to the "What's enforced today" list in this doc.
7. Run `./skelet go test ./graph/admin/ -run TestEveryAdminResolverIsGated`. It fails
   on any entry point you left ungated, which is the whole check in one line.

# Plugins

Two ways to extend the backend, and the choice between them is mostly about
where the code runs.

## Licensing: your plugin is yours

Skelet itself is GPL-3.0, but that does not reach your plugin. The
[Skelet Plugin Exception](../PLUGIN-EXCEPTION.txt), an additional permission under
GPL section 7, lets you license an extension under any terms you choose -
proprietary included - with no obligation to publish its source.

That covers everything on this page: Tier 1 plugins compiled into the binary,
remote providers, storefronts, dashboard extensions, language packs, themes. A
merchant's private customisation and an agency's client work are equally exempt.

The line is modification of Skelet's own source. Change Skelet and convey the
result and the GPL governs that; write something that extends it and the exception
applies. Importing the SDK does not by itself make your code a derivative work.

## Tier 1 plugins - Go, compiled in

A plugin is a Go package compiled into the binary, written against the public SDK
in `apps/backend/sdk`. It registers itself from `init()` and gets a typed handle
for everything else: payment providers, shipping carriers, HTTP routes, event
subscriptions, scheduled jobs, config fields, GraphQL schema extensions, and its
own migrations.

The SDK never exposes `internal/*` types. Its API surface is pinned in
`sdk/api.json` so a breaking change is a visible diff, and a second gate runs
`apidiff` against the last SDK tag to classify that diff by Go's own
compatibility rules - an added interface method reads as an addition to a text
diff but breaks every plugin implementing it, and apidiff says so. `sdk/sdktest`
ships a fake host so a plugin can be tested without booting the app, and it is
covered by the same gates.

`apps/backend/core/plugins/example` exercises every registration verb - start there.
`apps/backend/core/plugins/plugins.json` decides which plugin directories compile in.

A plugin in its own repository requires `github.com/skeletg/skelet/apps/backend/sdk`, plus
`.../apps/backend/core` if it adds GraphQL resolvers. Both are published as tags named after
their directory, like `apps/backend/sdk/v0.1.0`. Until the first of those is pushed, `go get`
can't fetch them, so build a plugin inside this tree under `apps/backend/core/plugins/`.

Payment gateways are plugins too, each in its own repository named after its slug - naming the
repo after the slug is what makes the install need no flags. A deployment that takes card
payment installs the gateway it needs:

```
skelet plugin add github.com/acme/payment-gateway@<version>
```

A fresh build therefore ships with NO gateway compiled in - offline payment
methods (COD, bank transfer, pay on pickup, purchase order) work out of the box, card payment
is an install away. Gateways are deliberately Tier 1 rather than remote
providers - PCI scope argues for in-process code over an HTTP hop.

### Installing a plugin from outside the tree

A plugin doesn't have to live in this repository. Two ways in, and the first is the one
to reach for.

**`plugin add` - a Go module dependency.** Nothing is copied into this repo:

```
skelet plugin add github.com/acme/skelet-plugin-fraud@v1.2.0
skelet plugin add github.com/acme/skelet-plugin-fraud        # latest
```

`go get` fetches it, `plugins/plugins.json` records the module path, and
`plugins/firstparty` is regenerated to import it. You get a version, a checksum in
`go.sum`, one copy instead of two, and `go get -u` as the upgrade path.

A module plugin's GraphQL SDL lives read-only in the module cache at a path that carries
its version and differs per machine, so `skelet gen` copies it into `plugins/.modules/`
first (git-ignored, rebuilt every run) and points gqlgen there. That directory is derived
output - delete it whenever.

A plugin with no published module is written in `plugins/<slug>/` instead - `plugin new`
scaffolds one. Those are the only two layouts, so a plugin's source is either in the
module cache or in-tree, never behind a `replace` pointing at a checkout elsewhere.

**`plugin install` - vendored source.** The fallback, for a plugin that is not a module:
no `go.mod`, or no repo you can fetch from.

```
skelet plugin install https://github.com/acme/skelet-plugin-fraud@v1.2.0
skelet plugin install ../my-plugin --slug my-plugin
```

The source is copied to `plugins/<slug>/` with its `.git`, `go.mod`, `go.sum` and
`vendor/` stripped - from then on it compiles as part of this module, its own dependencies
pulled in by `go mod tidy` and any self-imports rewritten onto the vendored path. A build
failure parks the plugin (`plugins/_<slug>`) so the tree keeps compiling while you fix it.

Either way, installing compiles the code into core with full privileges - read it before
you install it. The manifest's capabilities are a disclosure, not a sandbox.

A plugin's SDL can extend `Query`, `Mutation`, and core's own object types - see "Your data on
core's records" below. One thing to know about graph-extending plugins: their resolvers reference types in core's
*generated* `graph/*/model` package, which core produces from that plugin's own SDL. So the
order is always fetch, then codegen, then compile - and such a plugin never builds
standalone in its own repo. A plugin with no GraphQL does.

### Building with your plugins

`skelet build` makes what a deployment runs, with the plugins `plugins.json` enables. It regenerates
first, so a plugin just added or switched off is in or out, then builds core's image as
`skelet-core:prod` - the name its compose service runs when `CORE_IMAGE` is unset - and the
dashboard's static files. Name what you want instead: `skelet build core signals dashboard shop
--tag v1.2`. `skelet release` pushes the backends' images to your registry.

## Remote providers - HTTP, out of process

Provider-shaped webhooks: a merchant HTTP endpoint that *implements* a role the
platform needs (quoting tax, rating a shipment) rather than observing an event.
No Go, no rebuild, any language.

Full contract, the signed envelope and the per-kind schemas:
[remote-providers.md](remote-providers.md). `skelet provider` conformance-tests
an endpoint against it.

## Configure payment plugins in the dashboard

**Settings → Payment providers** lists installed gateway plugins, their versions and
runtime status. Select a provider to edit its registered credentials and test/live
settings using the page-header Save. Secret values stay masked and are not stored in
browser drafts. The page shows the backend webhook path; with a channel selected it
includes the channel routing parameter. Plugin runtime enablement follows its registered
scope, so the built-in plugin switch is edited at Global scope.

**Settings → Checkout → Payment** controls which registered methods a channel offers,
their country rules, checkout labels, display order and manual-payment instructions.
A disabled plugin is excluded from eligible methods and cannot be enabled for checkout.
Restarting the backend preserves merchant labels and ordering.

Installing still requires adding a compatible plugin module and rebuilding the backend;
the dashboard does not install code. Backend installation does not install card-entry
or authentication UI in a storefront. Each gateway needs its matching storefront integration.
Runtime status reports whether the plugin can run; it is not a credential or connectivity test.

## What the SDK gives a gateway author

- **`sdk/httpx`** - the outbound HTTP client: auth hook, 1MiB body cap, torn-read
  propagation, and the gateway's dedupe header driven by a positional idempotency-key
  parameter, so forgetting it is a compile error. Hand-rolling this loop per gateway is
  how the same money bugs get written six times over - use the package.
- **`sdk.NamespacedConfig("payment/<slug>")`** - the default-scope reader for health
  probes and resolvers. Both it and the host's bound reader refuse full paths (keys
  starting `payment/` or `plugin/`), because a double prefix resolves nothing and leaves
  the gateway silently inert.
- **Attributed registrations** - config fields, permissions, events, migrations and data
  patches register through the `*Plugin` handle, so `plugin uninstall` sweeps a plugin's
  exact footprint and a second plugin claiming the same key fails at boot naming both.
- **`Manifest.SecurityOrigins`** - the browser-side origins your integration needs
  (script/connect/frame/form-action/payment). `cmd/secheadgen` composes them into the
  CSP and Permissions-Policy during `skelet gen`, so installing a gateway never means
  editing core's security headers.
- **Channel-scoped webhooks** - register the gateway push URL with `?channel=<uuid>`
  and `WebhookRequest.ChannelID` carries it; config reads and the kill switch then
  resolve per channel. Without the parameter, default scope, as before.

## What the SDK gives a plugin that owns a domain

A gateway is the small case. A plugin that owns tables, a graph surface and a place in the
money path needs more - say company accounts with credit lines and their own checkout tender,
all out of tree. This is what the SDK gives it.

- **`sdk.Database()`** - the application pool, for a plugin whose own migrations created
  tables to read on the request path. The same pool core uses, not a sandbox: scope your SQL
  to tables you created, prefix them with your slug, and reach for an event or a registration
  seam wherever core has one. Resolve the handle on first use, never in `init()`.
- **`order.tendering`** - a sync event inside the placement transaction, carrying the balance
  still owed after core's gift-card and store-credit tenders. Type-assert to `sdk.Tendering`,
  match on `TenderProvider()`, and write through `PlacementTx()` so a reservation commits or
  rolls back with the order. Returning an error refuses the placement; return a `UserError` and
  the shopper reads why.
- **`RegisterPriceResolver`** - what a specific customer pays for a product, resolved ahead of
  list and tier pricing. Fail-soft by contract: an error reads as "no plugin price" rather than
  breaking add-to-cart.
- **`sdk.RequireChannelPermission` / `sdk.PermittedChannels`** - the channel-scoped half of the
  admin permission model. Where your object has a channel, gate on these rather than the bare
  `RequirePermission`, or an admin scoped to one store can act on another's records. Core's
  authz coverage tests FOLLOW the generated delegation into your source and fail the build if a
  plugin field has no guard, so this is enforced, not advisory.
- **`sdk.ReadCart` / `sdk.CreateDraftOrder`** - reading a shopper's cart (scope-checked host-side)
  and turning agreed lines into a draft order for review. The pair a quote-shaped feature needs.
- **`sdk.CustomerID` / `sdk.AdminID` / `sdk.RequestChannelID`** - who is asking, for scoping a
  storefront answer and for audit attribution. `AdminID` is attribution only, never a check.
- **`RegisterCheckoutField`** - one more thing the checkout asks, on every channel your plugin is
  on: a gift message, a delivery date, a consent of your own. See below.
- **`RegisterOrderNumberer`** - names the orders of every channel your plugin is on, when the
  store's own format (`sales/orders/number_format`, with a count per store or per channel and an
  optional yearly restart) isn't enough. `req.Next(ctx, scope, period)` draws from core's count, kept
  apart from the store's under your slug; return letters, digits and `- _ . / #`, at most 64,
  which core upper-cases. An error, anything else, or a number already given out, and the store's
  format names the order instead, so placement never fails on a number. With two numberers on one channel, the one registered first wins, and boot logs which one never runs.
- **`RegisterOrderStage`** - a step of your own inside core's statuses, like "awaiting stock"
  inside confirmed and processing. The status still drives payment and stock; the stage says
  where the work stands, and the dashboard shows it, filters by it and lets an admin set it.
  `p.SetOrderStage(ctx, orderID, key)` moves an order into one of your stages, or out with `""`,
  and leaves another plugin's alone. The order keeps the key as `p.Key(key)` - `<slug>:<key>`, so
  two plugins can both have an "awaiting_stock" - and that's what `order.transitioning` and
  `order.stage_changed` carry. Moving the order to a status the stage doesn't list clears it,
  with a line on the order's timeline, and `order.stage_changed` fires either way. Switching
  your plugin off on a channel stops orders entering your stages there; ones already in a stage
  keep it, and it stays yours.
- **`order.transitioning`** - a sync event before every status or stage move someone asks for:
  an admin's transition, a customer's cancel, a shipment, a hold or release, a stage move.
  Placement's own moves don't fire it. Type-assert to `sdk.OrderTransitioning` and return an
  error to refuse, say a shipment while the order still sits in your awaiting-stock stage. Don't
  move the same order from inside the answer: that move is refused, and a stage that changes
  while a move is being decided refuses the move too (`ORDER_STAGE_RACED`).
- **`loyalty.earning`** - a sync event before a paid-for order's points are written, with the
  channel's rate already applied. Type-assert to `sdk.LoyaltyEarning`, read the lines - each
  one's SKU, quantity and total, and through `EarningLineProduct`, `EarningLineUnitCents` and
  `EarningLineGift` its product, categories, unit price and whether it's a free gift - and set
  the points for a rule of your own: double points on a category, none on free gifts. A note
  you add shows in the customer's points history beside the order number. It can arrive more
  than once for an order, so compute rather than act. To refuse, set the points to 0 or return
  an `sdk.UserError` - the order earns nothing, as a webhook's `reject` does. Any other error is
  a fault: the points stay unearned and the earn is tried again.
- **`RegisterDocumentRenderer`** - makes the invoices, packing slips or credit memos of every
  channel your plugin is on, for a format core's doesn't cover. The request carries the order as
  the document prints it, the seller's details, footer and country note from the channel's
  settings as they read when the order was placed, and `Default()`, core's own PDF, for a renderer
  that only stamps or wraps it. Return a PDF; an error or a panic reaches the admin who asked,
  since quietly swapping in core's would hand out the wrong paperwork. Two renderers of one kind
  on a channel: the first registered wins, and boot logs the other.
- **`subscription.renewing`** - a sync event as each subscription period is invoiced. Type-assert
  to `sdk.SubscriptionRenewing` and set what the period costs (0 bills nothing), skip it, or delay
  it to a later date, after which later periods count from that date. It runs before the
  subscription is locked for billing, and if the subscription changed meanwhile - paused, billed
  by another run - nothing is minted and the next run asks again, so decide from the event alone. An
  error leaves the period for later too: a minute, then twice as long each time up to six hours,
  while the store's other subscriptions bill as usual. One period costs at most
  10,000,000,000 in minor units.
- **Your data on core's records** - a warranty on a product, an ERP status on an order, as fields
  on core's own GraphQL types, saved with the record in its own transaction. See below.
- **`RegisterCustomerData`** - what you keep about a customer leaves with their data export and goes
  with an erasure. See below.
- **`RegisterFee`** - a charge of your own on the order: cash-on-delivery handling, a payment
  surcharge, an eco fee, a deposit. See below.
- **`RegisterDataResource`** - your own data in the dashboard's import and export hub. Declare
  the columns, a view permission and an import permission you registered, and give an `Import`
  per row and an `Export` that streams records. Core handles the CSV or JSON Lines file, the
  column reference and template, dry runs, per-row errors and progress; honour `run.DryRun` by
  writing nothing. A run with no channel needs the permission unscoped, like core's own, and a
  resource marked `Global` - rows the whole installation shares - always does. Left
  channel-scoped, a resource has to keep to `run.ChannelID` itself: core checks the admin's grant
  on that channel, not what your code reads or writes. `row.JSON` reads a JSON column and
  `row.Require` insists on values - a `Required` field is yours to enforce when a row creates a
  record. A `json` field's exported value is marshalled for you, and read-only columns are
  ignored on import, however a spreadsheet reformatted them. See [import-export.md](import-export.md).

### Checkout fields

```go
p.RegisterCheckoutField(sdk.CheckoutField{
    Key: "gift_message", Label: "Gift message", Type: sdk.CheckoutFieldTextarea, MaxLength: 200,
})
```

A field is `text` (the default), `textarea`, `checkbox`, `select` with `Options` - each an
`sdk.CheckoutFieldOption{Value, Label}`, the value kept on the order and the label shown - or `date`,
optionally `Required`, with a `Hint` and a `MaxLength` that defaults to 500 and stops at 5000. Core renders
nothing: the storefront reads the declared fields off `CartCheckout.fields` and sends the
answers with the details step, both templates draw them, and core holds each answer to its
type. Add a `Validate` func for a rule of your own - it gets an `sdk.CheckoutFieldCheck` with the
channel and the answer, and an `sdk.UserError` from it reaches the shopper. A required field
without an answer keeps `ready` false, and `placeOrder` refuses. `placeOrder` runs every saved
answer through the field's rules again, `Validate` included, since either may have changed
since the shopper saved it.

The answers ride the placement onto the order with the label the shopper saw, so the dashboard
shows them under **At checkout** even after the field changes. Carts and orders key an answer
`<slug>:<key>`, so two plugins can ask a "gift_message" each. On `checkout.validating` and
`order.placing`, type-assert the event to `sdk.CheckoutFieldAnswers` and read yours with
`p.CheckoutAnswer(answers, "gift_message")`, or another plugin's by its full key. On
`order.placing`, `p.SetOrderMetadata(meta, "delivery_slot", v)` records a value of your own in
the order's metadata under `<slug>:delivery_slot`, so no other plugin overwrites it; the admin API
returns it as `Order.metadata`. Both go with a customer's data when they're erased.

### Your data on core's records

Keep your data in your own tables, with a foreign key to the core table it belongs to - core's
migrations run before any plugin's, so `customers`, `catalog_products`, `orders` and the rest are
there to point at. Give the key `ON DELETE CASCADE` and your rows go when the record does.

Then put them on core's type, in your SDL:

```graphql
extend type Product {
  acmeWarranty: AcmeWarranty @goField(forceResolver: true, batch: true)
}
```

The field is nullable and batched, or codegen refuses it. Your resolver gets one call per list of
parents, `ProductAcmeWarranty(ctx, objs []*model.Product) ([]*model.AcmeWarranty, error)`, and answers
one value per parent in their order, nil for none - so a listing of 48 products asks you once, and
one `WHERE product_id = ANY($1)` serves it. The method is named after the type and the field, so a
warranty on products and one on orders never clash. Where your plugin is off on a channel the field
answers null without calling you. On the admin API you gate it like a root field, with an
`sdk.Require*` the authz tests follow into your method. Prefix the field with your slug, as for root
fields, so it never collides with a field core adds later.

When the field is just your rows for the record, `sdk.Link` and `sdk.LoadOne` / `sdk.LoadMany` write
that query for you - the resolver becomes a line:

```go
var warranties = sdk.Link{Table: "acme_warranties", Key: "product_id"}

func (adminResolver) ProductAcmeWarranty(ctx context.Context, objs []*model.Product) ([]*model.AcmeWarranty, error) {
    if err := sdk.RequirePermission(ctx, "catalog.view"); err != nil {
        return nil, err
    }
    return sdk.LoadOne[model.AcmeWarranty](ctx, warranties, sdk.IDs(objs, func(p *model.Product) string { return p.ID }))
}
```

The columns are your model's fields - each field with a JSON tag reads its column in snake case
(`monthsLeft` reads `months_left`) - and `LoadMany` returns every row per record, ordered by
`Link.Order`. Write your own query when the answer needs more than that.

The storefront caches product, category and collection pages, and your field with them, so when your
data on a record changes, call `sdk.InvalidateStorefront(ctx, sdk.StorefrontProduct, id)`: the
record's pages are purged the way core's own change to it purges them, following the store's cache
settings. Core sends no event about your table, so nothing else will.

### Saving your data with the record

`createProduct`, `updateProduct`, `adminCreateCustomer` and `adminUpdateCustomer` take `extensions`, an
object with each plugin's part under its slug - `{"acme": {"months": 24}}`. Core hands them to
`product.saving` or `customer.saving`, a sync event inside the save's own transaction, after the
record's row is written:

```go
p.SubscribeSync(sdk.EventProductSaving, 100, func(ctx context.Context, e any) error {
    save, ok := e.(sdk.RecordSaving)
    var warranty struct{ Months int `json:"months"` }
    if !ok {
        return nil
    }
    if given, err := p.Extension(save, &warranty); !given || err != nil {
        return err
    }
    _, err := save.SavingTx().ExecContext(ctx,
        `INSERT INTO acme_warranties (product_id, months) VALUES ($1, $2)
         ON CONFLICT (product_id) DO UPDATE SET months = $2`, save.SavingRecordID(), warranty.Months)
    return err
})
```

Your rows commit with the record's, and an error refuses the whole save - an `sdk.UserError` reads
to the admin. A sync webhook on the same events can refuse too. A save carries at most 64 KB of
extensions, each keyed by a slug.

In the dashboard, a section on the product or customer page stages its part with
`useExtensionSection` from `@skelet/dashboard-sdk`, and it goes out with the page's own Save - in the
record's mutation when the record changed too, alone through the same mutation when it didn't. The
confirm dialog lists it under your title. Refetch your data in `onSaved`, so your section stops
reporting a change once it's saved:

```tsx
useExtensionSection('acme', {
  title: 'Warranty',
  diff: months === saved ? [] : [{ label: 'Months', before: String(saved), after: String(months) }],
  value: { months },
  onSaved: refetch,
})
```

### Customer data

A customer's data export and their erasure reach what core keeps; `RegisterCustomerData` makes them
reach what you keep too:

```go
p.RegisterCustomerData(sdk.CustomerData{
    Key:    "warranties",
    Export: func(ctx context.Context, who sdk.CustomerDataSubject) (any, error) { ... },
    Erase:  func(ctx context.Context, who sdk.CustomerDataSubject) error { ... },
})
```

`Export` returns anything that marshals to JSON, and the export carries it under `<slug>:<key>`; nil
leaves nothing. An error fails the export, since one quietly missing a section answers the request
wrongly. `Erase` runs after the customer is erased - core blanks the account rather than deleting
it, so a cascading key never fires - and again until it returns nil, so make it safe to repeat. Both
run whether or not your plugin is on, since what you kept outlives that. A customer's deletion is
your foreign key's to handle.

### Fees

```go
p.RegisterFee(sdk.Fee{
    Key: "cod_handling", Label: "Cash on delivery",
    Charge: func(ctx context.Context, req sdk.FeeRequest) (int64, error) {
        if req.PaymentMethod != "cod" {
            return 0, nil
        }
        return 250, nil
    },
})
```

A fee goes on after shipping and before tax. `Charge` gets the checkout as it stands - the lines,
subtotal, discount, the shipping picked and what it costs, the payment method, the shopper's country,
group and checkout-field answers (`p.FeeAnswer(req, key)` reads yours) - and returns what the fee comes
to in minor units, holding its tax when `req.TaxInclusive`. Return 0 and the order carries no line for
it. Core taxes it under `TaxClass`, empty being the class of a product that has none, unless you mark
it `Untaxed`; a remote tax provider gets the fees in its request and taxes them its own way.

The checkout quote runs it the same way placement does, so the shopper's summary shows the fee before
they pay, and both storefront templates draw it as a row of its own under its label. It runs whenever
the checkout is priced, often several times per checkout, so compute rather than act, and keep it quick:
it's on every checkout of every channel your plugin is on. An `sdk.UserError` stops the order with your
message ("cash on delivery takes orders up to 100.00"); so does any other error, since charging less
than the store asked is worse than not selling. A fee is zero or more and at most `sdk.MaxFeeCents` -
a discount of your own is a promotion action or `CheckoutReview.AddCheckoutDiscount`.

The order keeps each fee as it was charged, `<slug>:<key>` with its label, amount and tax, so an edit
re-taxes it with the order but never asks your plugin again, and the invoice, the order emails, the
dashboard and both APIs (`Order.fees`, `CheckoutTotals.fees`) show the same rows. A credit memo
refunds a fee through its adjustment refund. Drafts an admin builds carry no fees - they're priced by hand.

## The frontend halves

A plugin's Go arrives through `go get`, its UI through npm. A `skelet-plugin.json` at the plugin's
root names the packages holding its dashboard and storefront halves:

```json
{ "dashboard": "@acme/fraud-dashboard@^1.2.0", "storefront": "@acme/fraud-storefront@^1.2.0" }
```

`skelet plugin add github.com/acme/fraud@v1.2.0` then brings both halves. The ops CLI fetches the
module and records the packages in `plugins.json`, pnpm adds them to the dashboard and to every
storefront that loads plugins, and `skelet gen` writes each app's import list from `plugins.json` -
`apps/dashboard/src/plugins/packages.ts`, and `app/plugins/packages.ts` in each storefront - so
switching the plugin off unloads its UI too. `skelet plugin install` does the same for a plugin
vendored from source. A package published less than a day ago won't install yet (pnpm's
`minimumReleaseAge`).

A dashboard package imports what it needs from `@skelet/dashboard-sdk` - the registries below, the
detail-page layout, `useSaveSection` and the UI kit. The dashboard's build points that name at its
own source, so the plugin registers into the same registries and draws with the same components;
the npm package of that name holds the types alone. Ship your source
(`"exports": { ".": "./src/plugin.tsx" }`), take `react` and `@apollo/client` as peers, and query
through the dashboard's Apollo client with `@apollo/client/react`'s hooks. A storefront package
imports from `@skelet/storefront-react/plugins/registry`.

A plugin's UI can still live in the tree instead - `apps/dashboard/src/plugins/<slug>/`, which the
dashboard's loader globs, or a storefront's `app/plugins/<slug>/`, imported by hand in its
`loaded.ts`. Either way it reaches core's pages only through the registries, never by editing one.

**Dashboard.** The import-time `extendGroup` / `registerNavNode` / `registerDetailRoute` calls are
the whole wiring of a plugin's own pages.

Inside core's own pages a plugin reaches in through registries keyed by a target name:

- **`registerDetailSection`** adds a whole section to a record's page, with `useSaveSection` to
  join the page's Save. `placement` puts it at the top of the page's body (`before`), at the end of
  it (`after`, the default) or in the side panel (`side`), where a body section is a
  `<DetailPageSection>` and a side one a small block of its own. The side panel shows on wide
  screens; below that a side section renders at the end of the body, so it's never lost. Every
  record page takes sections: `product`, `order`, `customer`, `customer-group`, `category`,
  `collection`, `attribute`, `product-type`, `option-template`, `cart`, `promotion`, `review`,
  `review-question`, `segment`, `campaign`, `experiment`, `cms-page`, `cms-page-type`,
  `cms-block-type`, `cms-attribute`, `blog-post`, `blog-author`, `blog-term`, `menu`, `web-form`,
  `webhook`, `integration`, `channel`, `country`, `currency`, `locale`, `location`,
  `shipping-zone`, `tax-rule`, `admin` and `role`. On a page whose side is something else - the
  category tree, a live preview, the post's own panel - side sections stay in the body.
- **`registerDetailField`** puts a field on a record's page - `product`, `customer`,
  `collection`, `country`, `currency`, `location` or `locale` - stored under a metadata key of
  that record. It edits the page's own metadata, so the page's Save, its before-and-after dialog
  and the unsaved-work guard cover it, and the dialog names it by your label. Text, textarea,
  number, checkbox, select or date; the key leaves the free-form metadata rows so nothing edits it
  twice.
- **`registerListColumn`**, `registerListFilter`, `registerListRowAction` and
  `registerListMassAction` extend a list. Every top-level list has a target: `catalog.products`,
  `catalog.collections`, `catalog.attributes`, `catalog.product-types`, `catalog.option-templates`,
  `catalog.url-rewrites`, `orders`, `carts`, `customers`, `customer-groups`, `segments`,
  `subscriptions`, `gift-cards`, `reviews`, `questions`, `conversations`, `promotions`, `campaigns`,
  `flows`, `signup-forms`, `web-forms`, `experiments`, `blog.posts`, `cms.pages`, `menus`,
  `inventory.locations`, `inventory.low-stock`, `shipping.zones`, `tax-rules`, `countries`,
  `currencies`, `locales`, `channels`, `admins`, `roles`, `integrations`, `remote-providers`,
  `webhooks` and `scheduled-jobs`. `metadataColumn(key, header)` shows one of your metadata
  fields as a column on the products and customers lists, whose rows carry metadata.
- **`registerSlot`** renders into a zone: `list-header:<target>`, `list-toolbar:<target>` and
  `list-footer:<target>` on every list above, each handed `{table}`, and `home:widgets` at the
  end of Home.
- **`registerHomeWidget`** adds a section of your own to Home, with an id like
  `<slug>:sales`, and an optional permission and feature switch. An admin arranges it, hides it or
  finds it in the library, like core's own. `defaultHidden` leaves it in the library until they
  switch it on.

**Storefront.** Routes need no registration - Next's filesystem router is already the glob, so a
page copied to its real path simply exists. The registry lives in `@skelet/storefront-react/plugins/registry` and covers
the places a plugin appears INSIDE a core page: `registerAccountNavItem` for the account sidebar,
and `registerSlot` for a zone. The template reaches it through `app/plugins/registry.ts` and
`app/plugins/Slots.tsx`, which load `loaded.ts` first - a slot renders in the browser too, and the
browser keeps its own copy of the registry, so the plugins register there as well as on the
server. Vetrina draws these zones, each with the ctx it hands over:

| Zone | Where | ctx |
|---|---|---|
| `header:actions` | the header's icon row | - |
| `footer:end` | above the footer's last line | - |
| `listing:top` | over every category, collection, catalog and search listing's results | `{path, total, query}` |
| `product-card` | beside each product card's price | `{productId, sku}` |
| `product:buy-box` | under the product page's buy box | `{productId, sku, name}` |
| `product:details` | under the product, above its reviews | `{productId, sku, name}` |
| `mini-cart:footer` | under the mini cart's buttons | `{subtotal, currency}` |
| `cart:summary` | under the cart's totals | `{subtotal, currency}` |
| `cart:actions` | beside Proceed to checkout | `{subtotal, currency}` |
| `checkout:summary` | under checkout's order summary | `{cartId, totalCents, currency}` |
| `checkout:payment` | above Pay, inside the payment form - give a button `type="button"` | `{provider, amountDueCents, currency}` |
| `order` | on every order the shopper reads | `{orderId, number, status, total, currencyCode}` |
| `order:placed` | once, as the order is placed | `{orderId, number, total, currencyCode}` |
| `account:overview` | under the account overview's cards | `{customerId}` |

ctx crosses from server to client components, so it's plain data - a slot that needs more calls
its own server action. The Astro template draws no slots: it renders on the server with no
client components to mount one in.

**A gateway's card fields.** `registerPaymentUI({ provider, Fields })` puts a payment
plugin's fields into the Next storefront's checkout while its method is picked and no saved
card stands in. `Fields` mounts the gateway's browser SDK and hands checkout a handle through
`register`, called from an effect once the SDK is ready (and with null on unmount). Pay stays
disabled until it has been:

- `tokenize()` runs when the shopper presses Pay, before anything is placed, and returns the
  single-use token `placeOrder` takes as `paymentToken` - a Stripe payment-method id, a
  Braintree nonce. Throwing keeps the shopper on the step with the message shown. Returning
  null suits a gateway that confirms after placing instead.
- `challenge(clientSecret)` runs when the order comes back with an in-page step to do (3DS,
  `paymentActionKind: "challenge"`). Checkout then calls `finalizeOrderPayment`, so the
  gateway - never the page - says whether the payment went through, and lands on the order.
  A `"redirect"` action sends the browser to the gateway instead, as long as it's an https URL.

A saved card mounts no fields, so a gateway whose saved cards can be challenged also registers a
`challenge` of its own next to `Fields`, and checkout runs that one. A card the gateway refuses at
placement comes back with `paymentFailed`, and the shopper lands on the order marked unpaid
rather than on a thank-you.

The gateway's hosts also have to be in the storefront's CSP - see `PAYMENT_ORIGINS` in
[env.md](env.md).

Everything in the storefront registry is client-safe on purpose. A slot renders in a client
component; a plugin needing server data calls its own `"use server"` action from it, which is how
a plugin shows its own data at checkout without core fetching it on the plugin's behalf.

`packages.ts` is generated from `plugins.json` and `loaded.ts` kept by hand, rather than a glob, for
the same reason `plugins/firstparty` is generated: the enabled set decides what loads, not whatever
is sitting in a directory.

Note that a storefront is a template you fork, not an app every install shares. Treat a
plugin's storefront half as reference markup to restyle rather than a drop-in.

## Testing a plugin

Two levels, and they answer different questions.

**`sdk/sdktest`** is the in-memory fake host. A plugin registers through the real SDK and
the test drives events, config, the kill switch and permissions against the fake, with no
database and no boot. Right for registration wiring and handler logic.

**`internal/e2ekit`** is the real thing: one assembled fx graph, the real services, the real
outbox. A plugin that owns part of the money path writes ordinary `*_test.go` files in its
own directory and drives it the way core's own scenarios do:

```go
package myplugin_test

import "github.com/skeletg/skelet/apps/backend/core/internal/e2ekit"

// Embedded, so the plugin's own helpers can hang off it
type harness struct{ *e2ekit.Harness }

func newHarness(t *testing.T) *harness { return &harness{e2ekit.New(t)} }

func TestCreditIsReservedInThePlacementTx(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2000)
	o, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1))
	// ... h.Settle(t), h.Cancel(t, o.ID), h.RearmOutbox(t, ...), h.D.<any service>
}
```

`skelet test e2e` runs `./plugins/...` alongside `./internal/e2e/...`, so these go green with
core's suite and need no copying into it. The depguard rule that bars `plugins/**` from
importing `internal/` carries `"!$test"`, which is what makes the import above legal.

A credit-line tender is the case this was built for: credit reserved inside the placement
transaction, released on cancel, prorated to the unshipped share, idempotent under outbox
redelivery.

## Known limitations

- **sdktest fidelity.** The fake host covers every registration verb but does not model
  `Instrument` (timeouts, rate limits, kill-switch translation) or `fail_closed`, so
  those behaviors are testable only against the real host - reach for `internal/e2ekit`
  when that is what you need to prove.

## Which one

| | Tier 1 plugin | Remote provider |
|---|---|---|
| Language | Go | anything that speaks HTTP |
| Deploys | with the backend | independently |
| Latency | in-process | a network hop, with a timeout and cache |
| Reach | the whole SDK surface | one declared role |
| Use when | it needs the event bus, migrations, or PCI scope | it is someone else's service, or not written in Go |

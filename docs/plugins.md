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
`apidiff` against the last release tag to classify that diff by Go's own
compatibility rules - an added interface method reads as an addition to a text
diff but breaks every plugin implementing it, and apidiff says so. `sdk/sdktest`
ships a fake host so a plugin can be tested without booting the app, and it is
covered by the same gates.

`apps/backend/core/plugins/example` exercises every registration verb - start there.
`apps/backend/core/plugins/plugins.json` decides which plugin directories compile in.

The six first-party payment gateways - Stripe, Braintree, PayPal, Klarna, Mollie
and Affirm - each live in their own repository, named after the plugin slug
(`payment-stripe`, `payment-braintree`, ...). Naming the repo after the slug is
what makes the install need no flags. A deployment that takes card payment
installs the ones it needs:

```
./skelet cli plugin add github.com/skeletg/payment-stripe@<version>
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
./skelet cli plugin add github.com/acme/skelet-plugin-fraud@v1.2.0
./skelet cli plugin add github.com/acme/skelet-plugin-fraud        # latest
```

`go get` fetches it, `plugins/plugins.json` records the module path, and
`plugins/firstparty` is regenerated to import it. You get a version, a checksum in
`go.sum`, one copy instead of two, and `go get -u` as the upgrade path.

A module plugin's GraphQL SDL lives read-only in the module cache at a path that carries
its version and differs per machine, so `./skelet gen` copies it into `plugins/.modules/`
first (git-ignored, rebuilt every run) and points gqlgen there. That directory is derived
output - delete it whenever.

A plugin with no published module is written in `plugins/<slug>/` instead - `plugin new`
scaffolds one. Those are the only two layouts, so a plugin's source is either in the
module cache or in-tree, never behind a `replace` pointing at a checkout elsewhere.

**`plugin install` - vendored source.** The fallback, for a plugin that is not a module:
no `go.mod`, or no repo you can fetch from.

```
./skelet cli plugin install https://github.com/acme/skelet-plugin-fraud@v1.2.0
./skelet cli plugin install ../my-plugin --slug my-plugin
```

The source is copied to `plugins/<slug>/` with its `.git`, `go.mod`, `go.sum` and
`vendor/` stripped - from then on it compiles as part of this module, its own dependencies
pulled in by `go mod tidy` and any self-imports rewritten onto the vendored path. A build
failure parks the plugin (`plugins/_<slug>`) so the tree keeps compiling while you fix it.

Either way, installing compiles the code into core with full privileges - read it before
you install it. The manifest's capabilities are a disclosure, not a sandbox.

One thing to know about graph-extending plugins: their resolvers reference types in core's
*generated* `graph/*/model` package, which core produces from that plugin's own SDL. So the
order is always fetch, then codegen, then compile - and such a plugin never builds
standalone in its own repo. A plugin with no GraphQL does.

## Remote providers - HTTP, out of process

Provider-shaped webhooks: a merchant HTTP endpoint that *implements* a role the
platform needs (quoting tax, rating a shipment) rather than observing an event.
No Go, no rebuild, any language.

Full contract, the signed envelope and the per-kind schemas:
[remote-providers.md](remote-providers.md). `./skelet provider` conformance-tests
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
  CSP and Permissions-Policy during `./skelet gen`, so installing a gateway never means
  editing core's security headers.
- **Channel-scoped webhooks** - register the gateway push URL with `?channel=<uuid>`
  and `WebhookRequest.ChannelID` carries it; config reads and the kill switch then
  resolve per channel. Without the parameter, default scope, as before.

## What the SDK gives a plugin that owns a domain

A gateway is the small case. A plugin that owns tables, a graph surface and a place in the
money path needs more, and the `b2b` plugin is the reference for all of it - company accounts,
credit lines, quotes and on-account payment, all out of tree.

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

## The frontend halves

A plugin's Go arrives through `go get`. Its UI does not, and shipping TypeScript through a
Go module buys nothing - so dashboard and storefront code lives in its own repository,
destined for npm once there are SDK packages for the two apps to depend on.

Until then it is copied into place by hand. What core provides either way is the registry
each half hooks into, so a plugin's UI never edits a core page.

**Dashboard.** `apps/dashboard/src/plugins/<slug>/` with a `plugin.tsx` in it. The app's
`src/plugins/loader.ts` glob imports every one eagerly, and the import-time
`extendGroup` / `registerNavNode` / `registerDetailRoute` calls are the whole wiring.

**Storefront.** `app/plugins/<slug>/` plus one line in `app/plugins/loaded.ts`. Routes need
no registration - Next's filesystem router is already the glob, so a page copied to its real
path simply exists. What `app/plugins/registry.ts` covers is the places a plugin appears
INSIDE a core page: `registerAccountNavItem` for the account sidebar, and `registerSlot` for
a zone like `cart:summary` or `cart:actions`.

Everything in the storefront registry is client-safe on purpose. The host components are all
`"use client"`, so a server-rendered slot could not mount in them; a plugin needing server
data calls its own `"use server"` action from the slot, which is how `b2b` shows a credit
hint without core ever fetching a company on its behalf.

`loaded.ts` is a hand-maintained import list rather than a glob, for the same reason
`plugins/firstparty` is generated from `plugins.json`: the enabled set decides what loads,
not whatever is sitting in a directory.

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

`./skelet test e2e` runs `./plugins/...` alongside `./internal/e2e/...`, so these go green with
core's suite and need no copying into it. The depguard rule that bars `plugins/**` from
importing `internal/` carries `"!$test"`, which is what makes the import above legal.

The `b2b` plugin is the worked example: credit reserved inside the placement transaction,
released on cancel, prorated to the unshipped share, idempotent under outbox redelivery.

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

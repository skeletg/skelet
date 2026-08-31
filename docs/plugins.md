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

`apps/backend/plugins/example` exercises every registration verb - start there.
`apps/backend/plugins/plugins.json` decides which plugin directories compile in.

The six first-party payment gateways - Stripe, Braintree, PayPal, Klarna, Mollie
and Affirm - each live in their own repository, named after the plugin slug
(`payment-stripe`, `payment-braintree`, ...). Naming the repo after the slug is
what makes the install need no flags. A deployment that takes card payment
installs the ones it needs:

```
bin/cli plugin install https://github.com/skeletg/payment-stripe@<ref>
```

A fresh build therefore ships with NO gateway compiled in - offline payment
methods (COD, bank transfer, purchase order) work out of the box, card payment
is an install away. Gateways are deliberately Tier 1 rather than remote
providers - PCI scope argues for in-process code over an HTTP hop.

### Installing a plugin from outside the tree

A plugin doesn't have to live in this repository. Two ways in, and the first is the one
to reach for.

**`plugin add` - a Go module dependency.** Nothing is copied into this repo:

```
bin/cli plugin add github.com/acme/skelet-plugin-fraud@v1.2.0
bin/cli plugin add github.com/acme/skelet-plugin-fraud        # latest
bin/cli plugin add --local ../my-plugin                       # unpublished, in development
```

`go get` fetches it, `plugins/plugins.json` records the module path, and
`plugins/firstparty` is regenerated to import it. You get a version, a checksum in
`go.sum`, one copy instead of two, and `go get -u` as the upgrade path.

A module plugin's GraphQL SDL lives read-only in the module cache at a path that carries
its version and differs per machine, so `bin/codegen` copies it into `plugins/.modules/`
first (git-ignored, rebuilt every run) and points gqlgen there. That directory is derived
output - delete it whenever.

`--local` is the development shortcut for a plugin with no remote yet: it writes a
`require` plus a **relative** `replace`. Relative because the dev stack compiles inside a
container that mounts `apps/backend` elsewhere, so an absolute host path resolves to
nothing in there. Every service that builds the backend needs the checkout mounted at the
matching path - `docker-compose.override.yml` is where that goes. Take the `replace` out
before releasing; consumers ignore it anyway.

**`plugin install` - vendored source.** The fallback, for a plugin that is not a module:
no `go.mod`, or no repo you can fetch from.

```
bin/cli plugin install https://github.com/acme/skelet-plugin-fraud@v1.2.0
bin/cli plugin install ../my-plugin --slug my-plugin
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
[remote-providers.md](remote-providers.md). `bin/provider-check` conformance-tests
an endpoint against it.

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
  CSP and Permissions-Policy during `bin/codegen`, so installing a gateway never means
  editing core's security headers.
- **Channel-scoped webhooks** - register the gateway push URL with `?channel=<uuid>`
  and `WebhookRequest.ChannelID` carries it; config reads and the kill switch then
  resolve per channel. Without the parameter, default scope, as before.

## Known limitations

- **sdktest fidelity.** The fake host covers every registration verb but does not model
  `Instrument` (timeouts, rate limits, kill-switch translation) or `fail_closed`, so
  those behaviors are testable only against the real host.

## Which one

| | Tier 1 plugin | Remote provider |
|---|---|---|
| Language | Go | anything that speaks HTTP |
| Deploys | with the backend | independently |
| Latency | in-process | a network hop, with a timeout and cache |
| Reach | the whole SDK surface | one declared role |
| Use when | it needs the event bus, migrations, or PCI scope | it is someone else's service, or not written in Go |

// Package sdk is the public contract for Tier 1 plugins - Go packages listed in
// plugins/firstparty that extend Skelet at build time via typed registries
//
// The SDK never re-exports internal/* types - internal/plugin/host.go adapts every
// registration onto the matching internal registry, so internal refactors don't become SDK
// breaking changes
//
// Plugin contract:
//   - RegisterPlugin from a package-level init(), then everything identity-bearing through
//     the *Plugin handle it returns - the handle carries the slug, so nothing is inferred
//     from init() order, which Go leaves undefined across sibling blank-imports
//   - No importing internal/*, which Go refuses across modules anyway - a sync handler type-asserts
//     its event to the SDK interface for it (CartLineAdding, OrderPlacing, Tendering...)
//   - Manifest.SDKRange gates the plugin against Version and the host refuses to boot a mismatch -
//     the surface is unfrozen so Version sits on 0.x, where a breaking change bumps the MINOR and
//     caret pins it: ^0.1.0 admits 0.1.9 but not 0.2.0 - the apidiff gate enforces the bump, so the
//     number is a real promise now rather than a formality
//
// The shape every plugin follows:
//
//	package paymentacme
//
//	import "github.com/skeletg/skelet/apps/backend/sdk"
//
//	func init() {
//	    p := sdk.RegisterPlugin(sdk.Manifest{
//	        Slug:     "payment-acme",
//	        Name:     "Acme Payments",
//	        Version:  "0.1.0",
//	        SDKRange: "^0.1.0",
//	    })
//
//	    // identity-bearing - off the handle
//	    p.RegisterPaymentProvider("acme", &provider{}, sdk.WithDisplayName("Acme"))
//	    p.RegisterHealthCheck("api", pingAcme)
//	    p.SubscribeAsync("order.placed", onOrderPlaced)
//
//	    // keyed by their own path, attributed to this plugin - uninstall sweeps by that attribution
//	    p.RegisterConfigField("payment/acme/api_key", sdk.ConfigField{Type: sdk.TypeSecret})
//	    p.RegisterEvent("payment-acme.settled", sdk.EventKindAsync)
//	}
//
// Several roles - shipping carriers, promotion conditions and actions - can ALSO be filled by a
// remote provider (internal/remoteprovider): a merchant-registered HTTP endpoint the core invokes
// mid-request - Neither replaces the other, they're different integration modes: a plugin is
// compiled in, sees full in-process context and costs a function call, while a remote provider
// needs no Go and no rebuild but costs a network hop and gets a serialized, thinner view
// Tier 2 (external apps) uses neither - apps talk to Skelet over HTTP with scoped API tokens
package sdk

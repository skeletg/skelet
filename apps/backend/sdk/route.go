package sdk

import "net/http"

// A plugin's own HTTP surface, mounted under /api/v1/plugins/<slug>/ - the storefront-facing seam
// for things a payment flow needs before an order exists, like minting a gateway order for a
// client-side approval widget
// Public and unauthenticated, same as the storefront GraphQL endpoint, and behind the same rate
// limiter - so treat every input as hostile and never return anything the caller shouldn't see
// The host wraps each route in the plugin's kill switch, rate limit, hook timeout and panic
// isolation, so a handler that dies takes its own request down and nothing else
type RouteHandler func(w http.ResponseWriter, r *http.Request)

// pattern is a net/http ServeMux pattern RELATIVE to the plugin's namespace, so "client-config" on plugin "payment-paypal" serves
// /api/v1/plugins/payment-paypal/client-config Include the method to restrict it ("POST create-order"); omit it to accept any
func (pl *Plugin) RegisterRoute(pattern string, h RouteHandler) {
	withHost(func(host Host) error { return host.RegisterRoute(pl.slug, pattern, h) })
}

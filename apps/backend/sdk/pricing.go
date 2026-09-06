package sdk

import (
	"context"

	"github.com/google/uuid"
)

// Resolves what a SPECIFIC customer pays for a product, ahead of list price and group-tier pricing -
// the in-process twin of the remoteprovider pricing kind, for negotiated contract prices, wholesale
// rates, and anything else keyed on who is buying rather than on what is bought
//
// It runs on the cart's hot path, once per line per pricing pass. Cache, or query one indexed row
type PriceResolver interface {
	// ok=false falls through to core pricing, which is what an anonymous shopper or a non-contract
	// product gets - reserve the error for a genuine failure, and expect it to be treated as a
	// fall-through too: pricing is not allowed to break add-to-cart
	ResolveUnitPrice(ctx context.Context, req PriceRequest) (cents int64, ok bool, err error)
}

// CustomerID is never uuid.Nil - core skips resolvers entirely for guests, since a per-customer price
// has nothing to key on
type PriceRequest struct {
	CustomerID uuid.UUID
	ProductID  uuid.UUID
	ChannelID  uuid.UUID
	SKU        string
}

// Registers a per-customer pricing override
//
// FIRST non-fall-through answer wins and the rest are skipped, in plugin registration order, so two
// plugins pricing the same product is resolved by install order rather than by a rule anyone can read.
// Don't ship two - if you need to compose them, compose inside one resolver
//
// The call is wrapped in the plugin's kill switch, rate limit, hook timeout and panic isolation, so a
// resolver that dies prices at catalog rather than failing the request
//
//	p.RegisterPriceResolver(contractPrices{})
func (pl *Plugin) RegisterPriceResolver(r PriceResolver) {
	if r == nil {
		panic("sdk.RegisterPriceResolver: resolver is nil")
	}
	withHost(func(h Host) error { return h.RegisterPriceResolver(pl.slug, r) })
}

package wiring

import (
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/address"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/segment"
	"github.com/skeletg/skelet/apps/backend/internal/shipping"
)

// One adapter per role a merchant's HTTP endpoint can fulfil - the full list of what a remote provider can take over, every one inert until a channel opts in, so an install with no providers configured behaves as before
type remoteProviderDeps struct {
	fx.In
	Provider  *remoteprovider.Service
	Invoker   *remoteprovider.Invoker
	Shipping  *shipping.Service
	Order     *order.Service
	Segment   *segment.Service
	Currency  *currency.Service
	Catalog   *catalog.Service
	Cart      *cart.Service
	Inventory *inventory.Service
	Review    *review.Service
	Flow      *flow.Service
}

func WireRemoteProviders(d remoteProviderDeps) {
	prov, invoke := d.Provider, d.Invoker

	d.Shipping.SetRemoteRateSource(shipping.NewRemoteRateSource(prov, invoke))              // quote alongside registry carriers
	d.Shipping.SetRemoteTracking(shipping.NewRemoteTrackingSource(prov, invoke))            // universal tracker, any carrier
	d.Order.SetRemoteTax(order.NewRemoteTaxSource(prov, invoke))                            // order tax via the TaxCents seam
	d.Order.SetRemoteFraud(order.NewRemoteFraudSource(prov, invoke))                        // screens at the pre-tx gate
	d.Order.SetRemoteReturns(order.NewRemoteReturnsPolicy(prov, invoke))                    // external returns policy: windows, exclusions
	d.Segment.SetRemoteSegments(segment.NewRemoteSegmentSource(prov, invoke))               // CDP audiences union into membership
	d.Currency.SetRemoteRateSource(currency.NewRemoteRateSource(prov, invoke))              // alternate FX source for the rate-refresh job
	d.Catalog.SetRemoteRecommendations(catalog.NewRemoteRecommendationSource(prov, invoke)) // supplies the "also bought" rail
	d.Cart.SetRemotePricing(cart.NewRemotePricingSource(prov, invoke))                      // authoritative cart price (B2B contract / dynamic pricing) over the catalog price
	d.Inventory.SetRemoteAvailability(inventory.NewRemoteAvailabilitySource(prov, invoke))  // salable stock from an external WMS/ERP - feeds product cards, cart and search
	d.Review.SetRemoteAggregates(review.NewRemoteAggregateSource(prov, invoke))             // rating aggregates from an external reviews platform (Yotpo/Bazaarvoice/...)
	d.Flow.SetRemoteActions(prov, invoke)                                                   // remote_action nodes merge the response back into the run

	address.RegisterRemoteValidator(prov, invoke)       // the "remote" validator slug - a channel opts in via sales/checkout/address_validator=remote
	promotion.RegisterRemoteProviderTypes(prov, invoke) // remote.condition + remote.action delegate eligibility / discount computation
}

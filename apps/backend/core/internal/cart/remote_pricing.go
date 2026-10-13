package cart

import (
	"context"

	"github.com/google/uuid"
)

// A merchant endpoint resolving a SKU's unit price per customer and channel - the escape hatch for B2B contract or dynamic pricing without a Go plugin, consulted on cart ops and checkout only so catalog display keeps the hot read paths off the network (the wiring package adapts remoteprovider to it)
type RemotePricingSource interface {
	Price(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID, basePriceCents int64) (cents int64, ok bool, err error) // ok=false keeps the catalog price, whether from no provider, no override, or a fail_open error - a non-nil error means a fail_closed provider failed, so abort the cart op
}

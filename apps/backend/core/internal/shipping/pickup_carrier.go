package shipping

import (
	"context"
	"fmt"
)

const PickupCarrierSlug = "pickup"

// One collectable store or warehouse - declared locally so shipping doesn't import inventory, with main adapting inventory.Location
type PickupLocation struct {
	Slug         string
	Name         string
	Line1        string
	City         string
	Instructions string
}

type PickupLocationLister interface {
	PickupLocations(ctx context.Context) ([]PickupLocation, error)
}

// PickupMethodCode is the pickup carrier's one method - which store is the delivery point the shopper picks, not a method of its own
const PickupMethodCode = "collect"

// Prices collection at $0 for the store_pickup kind - the destination is ignored, since the shopper travels and the parcel doesn't, and the store itself is the point the shopper picks. No method is offered while no location takes pickups
type PickupCarrier struct {
	locations PickupLocationLister
}

func NewPickupCarrier(l PickupLocationLister) *PickupCarrier {
	return &PickupCarrier{locations: l}
}

func (c *PickupCarrier) Quote(ctx context.Context, _ RateRequest) ([]RatedMethod, error) {
	locs, err := c.locations.PickupLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pickup locations: %w", err)
	}
	if len(locs) == 0 {
		return nil, nil
	}
	return []RatedMethod{{Code: PickupMethodCode, Name: "Collect in store", PriceCents: 0, Kind: "store_pickup"}}, nil
}

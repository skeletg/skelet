package shipping

import (
	"context"
	"fmt"
	"strings"
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

// Surfaces every pickup location as a $0 option - destination is ignored, since the shopper travels and the parcel doesn't
type PickupCarrier struct {
	locations PickupLocationLister
}

func NewPickupCarrier(l PickupLocationLister) *PickupCarrier {
	return &PickupCarrier{locations: l}
}

// One free option per pickup point - codes are "loc-<slug>", stable across re-quoting so placement can re-resolve the price, always 0
func (c *PickupCarrier) Quote(ctx context.Context, _ RateRequest) ([]RatedMethod, error) {
	locs, err := c.locations.PickupLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pickup locations: %w", err)
	}
	out := make([]RatedMethod, 0, len(locs))
	for _, l := range locs {
		label := "Pickup - " + l.Name
		if city := strings.TrimSpace(l.City); city != "" {
			label += " (" + city + ")"
		}
		out = append(out, RatedMethod{
			Code:       "loc-" + l.Slug,
			Name:       label,
			PriceCents: 0,
		})
	}
	return out, nil
}

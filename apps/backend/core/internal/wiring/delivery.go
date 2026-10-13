package wiring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/delivery"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
)

type deliveryDeps struct {
	fx.In
	Delivery  *delivery.Service
	Shipping  *shipping.Service
	Inventory *inventory.Service
	Provider  *remoteprovider.Service
	Invoker   *remoteprovider.Invoker
}

// The delivery options' seams - what readiness asks of shipping, remote point sources, and the store-pickup point source core ships
func wireDelivery(d deliveryDeps) {
	d.Delivery.SetCarrierKinds(carrierKinds{d.Shipping})
	d.Delivery.SetRemoteProviders(d.Provider, d.Invoker)
	registerInventoryPointSource(d.Inventory)
}

type carrierKinds struct{ s *shipping.Service }

func (c carrierKinds) CarrierKinds(ctx context.Context, channelID uuid.UUID) (map[string][]string, error) {
	return c.s.EnabledCarrierKinds(ctx, channelID)
}

// registerInventoryPointSource offers a channel's pickup-enabled locations as store-pickup points - registered once per process, so a second fx app in tests reuses it
func registerInventoryPointSource(inv *inventory.Service) {
	inventorySource.svc = inv
	inventorySourceOnce.Do(func() {
		delivery.RegisterPointSource(delivery.PointSourceRegistration{
			Slug: delivery.InventoryPointSource, Label: "Pickup-enabled inventory locations",
			Kinds: []string{delivery.KindStorePickup}, Source: inventorySource,
		})
	})
}

var (
	inventorySource     = &inventoryPointSource{}
	inventorySourceOnce sync.Once
)

type inventoryPointSource struct{ svc *inventory.Service }

func (s *inventoryPointSource) points(ctx context.Context, channelID uuid.UUID) ([]delivery.Point, error) {
	if s.svc == nil {
		return nil, errors.New("inventory not wired")
	}
	locs, err := s.svc.ListChannelLocations(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var out []delivery.Point
	for _, l := range locs {
		if !l.PickupEnabled || !l.IsActive {
			continue
		}
		out = append(out, delivery.Point{
			ID: l.Slug, Source: delivery.InventoryPointSource, Name: l.Name, Lat: l.Latitude, Lng: l.Longitude,
			Instructions: l.PickupInstructions, Phone: l.Phone, Hours: pickupHoursSummary(l.Hours, l.Timezone),
			Details: map[string]string{"locationId": l.ID.String(), "timezone": l.Timezone},
			Address: delivery.Address{Name: l.Name, Street1: l.Line1, Street2: l.Line2, City: l.City, Region: l.Region, PostalCode: l.PostalCode, CountryCode: l.CountryCode},
		})
	}
	return out, nil
}

func (s *inventoryPointSource) Search(ctx context.Context, q delivery.PointQuery) ([]delivery.Point, error) {
	pts, err := s.points(ctx, q.ChannelID)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	if needle == "" {
		return pts, nil
	}
	var out []delivery.Point
	for _, p := range pts {
		if strings.Contains(strings.ToLower(p.Name+" "+p.Address.City+" "+p.Address.Street1), needle) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *inventoryPointSource) Get(ctx context.Context, channelID uuid.UUID, id string) (delivery.Point, error) {
	pts, err := s.points(ctx, channelID)
	if err != nil {
		return delivery.Point{}, err
	}
	for _, p := range pts {
		if p.ID == id {
			return p, nil
		}
	}
	return delivery.Point{}, delivery.ErrPointNotFound
}

// InventoryOriginPlanner answers where each line ships from, for the front resolver's shipments
type InventoryOriginPlanner struct{ Inv *inventory.Service }

func (p InventoryOriginPlanner) PlanOrigins(ctx context.Context, channelID uuid.UUID, lines []delivery.Line, profiles map[string]delivery.LineProfile) (map[uuid.UUID]delivery.Origin, error) {
	in := make([]inventory.OriginLine, 0, len(lines))
	for _, l := range lines {
		prof, ok := profiles[l.SKU]
		if !ok {
			continue
		}
		in = append(in, inventory.OriginLine{LineID: l.ID, ProductID: prof.ProductID, Quantity: l.Quantity})
	}
	locs, err := p.Inv.PlanOrigins(ctx, channelID, in)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]delivery.Origin, len(locs))
	for lineID, loc := range locs {
		id := loc.ID
		out[lineID] = delivery.Origin{LocationID: &id, Name: loc.Name, City: loc.City}
	}
	return out, nil
}

// dayHours is one day of a location's opening hours as inventory stores them
type dayHours struct {
	Open   string `json:"open"`
	Close  string `json:"close"`
	Closed bool   `json:"closed"`
}

func pickupHoursSummary(raw json.RawMessage, timezone string) string {
	var hours map[string]dayHours
	if json.Unmarshal(raw, &hours) != nil {
		return ""
	}
	rows := []string{}
	for _, day := range []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"} {
		if h, ok := hours[day]; ok && !h.Closed && h.Open != "" && h.Close != "" {
			rows = append(rows, strings.ToUpper(day[:1])+day[1:]+" "+h.Open+"–"+h.Close)
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return strings.Join(rows, " · ") + " (" + timezone + ")"
}

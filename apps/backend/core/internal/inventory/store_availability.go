package inventory

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// StoreStock deliberately differs from online purchasability: backorders and the
// missing-item unlimited sentinel are never evidence of physical store stock.
type StoreStock string

const (
	StoreInStock    StoreStock = "IN_STOCK"
	StoreLowStock   StoreStock = "LOW_STOCK"
	StoreOutOfStock StoreStock = "OUT_OF_STOCK"
	StoreUnknown    StoreStock = "UNKNOWN"
)

type StoreProduct struct {
	ID           uuid.UUID
	QuantityStep float64
	MinOrderQty  *float64
}

type StoreQuery struct {
	ChannelID           uuid.UUID
	Product             *StoreProduct
	City, Search        string
	InStockOnly         bool
	PickupOnly          bool
	Latitude, Longitude *float64
	PreferredStoreID    *uuid.UUID
	Limit, Offset       int
}

type PublicLocation struct {
	Location   Location
	Status     *StoreStock
	DistanceKm *float64
}

type StorePage struct {
	StockCheckingAvailable bool
	ProductEligible        bool
	HasPickupStores        bool
	Cities                 []string
	PreferredStore         *PublicLocation
	Items                  []PublicLocation
	TotalCount             int
}

// PublicStores uses the channel junction and native levels only. The API adapter
// is responsible for validating product publication before providing a product.
func (s *Service) PublicStores(ctx context.Context, q StoreQuery) (StorePage, error) {
	out := StorePage{StockCheckingAvailable: !s.flatModeEnabled(ctx), Cities: []string{}, Items: []PublicLocation{}}
	if (q.Latitude == nil) != (q.Longitude == nil) || (q.Latitude != nil && (!validCoordinate(*q.Latitude, 90) || !validCoordinate(*q.Longitude, 180))) {
		return out, ErrInvalidCoordinates
	}
	locations, err := s.repo.ListPublicLocations(ctx, q.ChannelID)
	if err != nil {
		return out, err
	}
	statuses := map[uuid.UUID]StoreStock{}
	if q.Product != nil && out.StockCheckingAvailable {
		item, e := s.repo.GetItemByProductID(ctx, q.Product.ID)
		if e != nil && !errors.Is(e, ErrItemNotFound) {
			return out, e
		}
		if e == nil {
			levels, e := s.repo.ListLevelsByItems(ctx, []uuid.UUID{item.ID})
			if e != nil {
				return out, e
			}
			public := map[uuid.UUID]bool{}
			for _, l := range locations {
				public[l.ID] = true
			}
			for _, level := range levels {
				if !public[level.LocationID] {
					continue
				}
				out.ProductEligible = true
				statuses[level.LocationID] = storeStockStatus(item, level, *q.Product)
			}
		}
	}
	cities := map[string]bool{}
	search := strings.ToLower(strings.TrimSpace(q.Search))
	for _, l := range locations {
		if l.PickupEnabled {
			out.HasPickupStores = true
		}
		if l.City != "" {
			cities[l.City] = true
		}
		row := PublicLocation{Location: l}
		if q.Product != nil && out.StockCheckingAvailable {
			status, ok := statuses[l.ID]
			if !ok {
				status = StoreUnknown
			}
			row.Status = &status
		}
		if q.Latitude != nil && l.Latitude != nil && l.Longitude != nil {
			d := storeDistance(*q.Latitude, *q.Longitude, *l.Latitude, *l.Longitude)
			row.DistanceKm = &d
		}
		if q.PreferredStoreID != nil && l.ID == *q.PreferredStoreID {
			copyRow := row
			out.PreferredStore = &copyRow
		}
		if q.PickupOnly && !l.PickupEnabled {
			continue
		}
		if q.City != "" && !strings.EqualFold(strings.TrimSpace(q.City), l.City) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(strings.Join([]string{l.Name, l.Line1, l.Line2, l.City, l.Region, l.PostalCode}, " ")), search) {
			continue
		}
		if q.InStockOnly && (row.Status == nil || (*row.Status != StoreInStock && *row.Status != StoreLowStock)) {
			continue
		}
		out.Items = append(out.Items, row)
	}
	for city := range cities {
		out.Cities = append(out.Cities, city)
	}
	sort.Strings(out.Cities)
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if q.Latitude != nil {
			if (a.DistanceKm == nil) != (b.DistanceKm == nil) {
				return a.DistanceKm != nil
			}
			if a.DistanceKm != nil && *a.DistanceKm != *b.DistanceKm {
				return *a.DistanceKm < *b.DistanceKm
			}
		}
		an, bn := strings.ToLower(a.Location.Name), strings.ToLower(b.Location.Name)
		if an != bn {
			return an < bn
		}
		return a.Location.ID.String() < b.Location.ID.String()
	})
	out.TotalCount = len(out.Items)
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := max(0, min(q.Offset, len(out.Items)))
	out.Items = out.Items[offset:min(offset+limit, len(out.Items))]
	return out, nil
}

func validCoordinate(v, limit float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= -limit && v <= limit
}

func storeStockStatus(item Item, level Level, p StoreProduct) StoreStock {
	qty := level.Quantity
	if item.Tracked {
		qty -= clampReserved(level.ReservedQuantity)
	}
	qty = max(0, qty)
	step := p.QuantityStep
	if step <= 0 {
		step = 1
	}
	minimum := step
	if p.MinOrderQty != nil && *p.MinOrderQty > step {
		minimum = math.Ceil(*p.MinOrderQty/step-1e-9) * step
	}
	if qty+1e-9 < minimum {
		return StoreOutOfStock
	}
	if item.LowStockThreshold > 0 && qty <= item.LowStockThreshold {
		return StoreLowStock
	}
	return StoreInStock
}

func storeDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const rad = math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*rad/2), 2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Pow(math.Sin((lon2-lon1)*rad/2), 2)
	return 6371 * 2 * math.Asin(math.Sqrt(min(1, max(0, a))))
}

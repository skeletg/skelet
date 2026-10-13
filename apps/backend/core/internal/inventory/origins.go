package inventory

import (
	"context"

	"github.com/google/uuid"
)

// OriginLine is a cart line checkout wants an origin for
type OriginLine struct {
	LineID    uuid.UUID
	ProductID uuid.UUID
	Quantity  float64
}

// PlanOrigins says which location each line would leave from - a read-only preview of ReserveTx's allocation, so checkout can show the packages an order splits into before anything is reserved. A line goes to the first channel location in priority order that can cover it whole, else the first holding any of it, else the top location, the same place a backorder parks. Untracked products ship from the top location. Placement still allocates for real, so a shelf emptied in between can move a line
func (s *Service) PlanOrigins(ctx context.Context, channelID uuid.UUID, lines []OriginLine) (map[uuid.UUID]Location, error) {
	out := make(map[uuid.UUID]Location, len(lines))
	if len(lines) == 0 {
		return out, nil
	}
	locations, err := s.repo.ListChannelLocations(ctx, channelID)
	if err != nil || len(locations) == 0 {
		return out, err
	}
	ids := make([]uuid.UUID, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.ProductID)
	}
	items, err := s.repo.ListItemsByProductIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	itemIDs := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		itemIDs = append(itemIDs, it.ID)
	}
	levels, err := s.repo.ListLevelsByItems(ctx, itemIDs)
	if err != nil {
		return nil, err
	}
	free := make(map[allocKey]float64, len(levels))
	for _, lvl := range levels {
		free[allocKey{item: lvl.InventoryItemID, loc: lvl.LocationID}] = max(lvl.Quantity-lvl.ReservedQuantity, 0)
	}
	for _, l := range lines {
		item, ok := items[l.ProductID]
		if !ok || !item.Tracked || s.flatModeEnabled(ctx) {
			out[l.LineID] = locations[0]
			continue
		}
		pick, partial := -1, -1
		for i, loc := range locations {
			avail := free[allocKey{item: item.ID, loc: loc.ID}]
			if avail >= l.Quantity {
				pick = i
				break
			}
			if avail > 0 && partial < 0 {
				partial = i
			}
		}
		switch {
		case pick >= 0:
		case partial >= 0:
			pick = partial
		default:
			pick = 0
		}
		loc := locations[pick]
		free[allocKey{item: item.ID, loc: loc.ID}] -= l.Quantity
		out[l.LineID] = loc
	}
	return out, nil
}

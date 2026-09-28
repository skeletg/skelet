package inventory

import (
	"fmt"

	"github.com/google/uuid"
)

// Accessors for sdk.InventoryAllocating, pinned in internal/plugin - routing follows the webhook's rules: a line id
// outside this allocation is refused, later routes win, and a location outside the channel fails the allocation
// further down, where the channel's locations are known

func (e *Allocating) AllocatingOrderID() uuid.UUID   { return e.OrderID }
func (e *Allocating) AllocatingChannelID() uuid.UUID { return e.ChannelID }

func (e *Allocating) AllocatingLineIDs() []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(e.Lines))
	for _, l := range e.Lines {
		ids = append(ids, l.OrderLineID)
	}
	return ids
}

func (e *Allocating) AllocatingLineProductID(lineID uuid.UUID) uuid.UUID {
	if l, ok := e.line(lineID); ok {
		return l.ProductID
	}
	return uuid.Nil
}

func (e *Allocating) AllocatingLineQuantity(lineID uuid.UUID) float64 {
	if l, ok := e.line(lineID); ok {
		return l.Quantity
	}
	return 0
}

func (e *Allocating) AllocatingRoute(lineID uuid.UUID) (uuid.UUID, bool) {
	if e.Routing == nil || *e.Routing == nil {
		return uuid.Nil, false
	}
	loc, ok := (*e.Routing)[lineID]
	return loc, ok
}

func (e *Allocating) RouteAllocatingLine(lineID, locationID uuid.UUID) error {
	if _, ok := e.line(lineID); !ok {
		return fmt.Errorf("routing line id %s is not part of this allocation", lineID)
	}
	if e.Routing == nil {
		e.Routing = new(map[uuid.UUID]uuid.UUID)
	}
	if *e.Routing == nil {
		*e.Routing = map[uuid.UUID]uuid.UUID{}
	}
	(*e.Routing)[lineID] = locationID
	return nil
}

func (e *Allocating) line(id uuid.UUID) (ReserveLine, bool) {
	for _, l := range e.Lines {
		if l.OrderLineID == id {
			return l, true
		}
	}
	return ReserveLine{}, false
}

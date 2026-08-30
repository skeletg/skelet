package download

import (
	"context"
	"database/sql"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/order"
)

type GrantableDownload struct {
	ProductID    uuid.UUID
	DownloadID   uuid.UUID
	Title        string
	URL          string
	MaxDownloads *int32
}

// The downloads to grant for a placed order's digital product lines - declared locally so download doesn't import catalog, with the adapter in main.go resolving each line's product downloads
type OrderDownloadLookup interface {
	DownloadsForOrder(ctx context.Context, orderID uuid.UUID) ([]GrantableDownload, error)
}

const EventAccessed = "download.accessed"

// Fires AFTER a grant's token is redeemed and the download served - async and best-effort, with the Synapse activity tracker resolving the channel through OrderID
type Accessed struct {
	GrantID    uuid.UUID
	CustomerID *uuid.UUID
	OrderID    *uuid.UUID
	ProductID  uuid.UUID
	Title      string
}

func (*Accessed) Type() string { return EventAccessed }

// Wires grant-on-purchase - on OrderPlaced, durable through the outbox and at-least-once, create a grant per downloadable line
// Idempotent under replay, skipping orders that already have grants, and it retains the bus so Consume can publish download.accessed
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	s.bus = bus
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
}

func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	placed, ok := e.(*order.OrderPlaced)
	if !ok || s.orders == nil {
		return nil
	}
	// Idempotency - OrderPlaced is at-least-once, so existing grants for this order mean a replay
	if existing, err := s.repo.ListByOrder(ctx, placed.OrderID); err != nil {
		log.Printf("[download] check grants for order %s: %v", placed.OrderID, err)
		return err
	} else if len(existing) > 0 {
		return nil
	}
	items, err := s.orders.DownloadsForOrder(ctx, placed.OrderID)
	if err != nil {
		log.Printf("[download] resolve downloads for order %s: %v", placed.OrderID, err)
		return err
	}
	if len(items) == 0 {
		return nil
	}
	orderID := placed.OrderID
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for _, d := range items {
			token, err := newToken()
			if err != nil {
				return err
			}
			if _, err := r.CreateGrant(ctx, GrantInput{
				Token:        token,
				OrderID:      &orderID,
				CustomerID:   placed.CustomerID,
				Email:        placed.Email,
				ProductID:    d.ProductID,
				DownloadID:   d.DownloadID,
				Title:        d.Title,
				URL:          d.URL,
				MaxDownloads: d.MaxDownloads,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("[download] grant for order %s failed (will retry on replay): %v", placed.OrderID, err)
		return err
	}
	return nil
}

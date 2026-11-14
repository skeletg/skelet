package download

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type GrantableDownload struct {
	ProductID    uuid.UUID
	DownloadID   uuid.UUID
	Title        string
	URL          string
	MaxDownloads *int32
}

// What granting needs of an order - the files its digital lines carry, and enough of its state to know whether it's been paid for
type OrderDownloads struct {
	Status        string
	PaymentStatus string
	CustomerID    *uuid.UUID
	Email         string
	Items         []GrantableDownload
}

// Declared locally so download doesn't import catalog, with the adapter in wiring resolving each line's product downloads
type OrderDownloadLookup interface {
	DownloadsForOrder(ctx context.Context, orderID uuid.UUID) (OrderDownloads, error)
}

const EventAccessed = "download.accessed"

// Fires AFTER a grant's token is redeemed and the download served - async and best-effort, with the Synapse activity tracker resolving the channel through OrderID
type Accessed struct {
	GrantID    uuid.UUID
	CustomerID *uuid.UUID
	OrderID    *uuid.UUID
	// One or the other: the product a bought file belongs to, or the library file the shop shared
	ProductID *uuid.UUID
	AssetID   *uuid.UUID
	Title     string
}

func (*Accessed) Type() string { return EventAccessed }

// What leaves the process - a webhook body. A shared file's title is the shop's own name for a private file, so it stays in
func (a Accessed) MarshalJSON() ([]byte, error) {
	type sent Accessed
	out := sent(a)
	if a.AssetID != nil {
		out.Title = ""
	}
	return json.Marshal(out)
}

// Wires grants to the order lifecycle, durable through the outbox and at-least-once: placement or a payment advance grants a paid-for
// order its files, and cancellation, a void or a full refund expires them - it retains the bus so Consume can publish download.accessed
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	s.bus = bus
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
	bus.Subscribe(order.EventPaymentStatusChanged, s.onPaymentStatusChanged)
	bus.Subscribe(order.EventCancelled, s.onOrderCancelled)
}

// A shared file was handed to an account, so its link stops working when the account is deleted or erased
func (s *Service) SubscribeToCustomerEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(customer.EventDeleted, s.onCustomerGone)
	bus.Subscribe(customer.EventErased, s.onCustomerGone)
}

func (s *Service) onCustomerGone(ctx context.Context, e events.Event) error {
	var gone uuid.UUID
	switch ev := e.(type) {
	case *customer.CustomerDeleted:
		gone = ev.CustomerID
	case *customer.CustomerErased:
		gone = ev.CustomerID
	default:
		return nil
	}
	return s.repo.DeleteSharesWith(ctx, gone)
}

func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	placed, ok := e.(*order.OrderPlaced)
	if !ok {
		return nil
	}
	return s.grantForOrder(ctx, placed.OrderID)
}

func (s *Service) onPaymentStatusChanged(ctx context.Context, e events.Event) error {
	changed, ok := e.(*order.OrderPaymentStatusChanged)
	if !ok {
		return nil
	}
	switch {
	case order.PaidFor(changed.Status):
		return s.grantForOrder(ctx, changed.OrderID)
	case changed.Status == order.PaymentVoided || changed.Status == order.PaymentRefunded:
		return s.revoke(ctx, changed.OrderID)
	}
	return nil
}

func (s *Service) onOrderCancelled(ctx context.Context, e events.Event) error {
	cancelled, ok := e.(*order.OrderCancelled)
	if !ok {
		return nil
	}
	return s.revoke(ctx, cancelled.OrderID)
}

// Grants each of the order's files once it's paid for and still standing - idempotent, since the unique (order, file) pair absorbs replays and races
func (s *Service) grantForOrder(ctx context.Context, orderID uuid.UUID) error {
	if s.orders == nil {
		return nil
	}
	o, err := s.orders.DownloadsForOrder(ctx, orderID)
	if err != nil {
		log.Printf("[download] resolve downloads for order %s: %v", orderID, err)
		return err
	}
	if len(o.Items) == 0 || o.Status == order.OrderStatusCancelled || !order.PaidFor(o.PaymentStatus) {
		return nil
	}
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		for _, d := range o.Items {
			token, err := newToken()
			if err != nil {
				return err
			}
			if _, err := r.CreateGrant(ctx, GrantInput{
				Token:        token,
				OrderID:      &orderID,
				CustomerID:   o.CustomerID,
				Email:        o.Email,
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
		log.Printf("[download] grant for order %s failed (will retry on replay): %v", orderID, err)
	}
	return err
}

func (s *Service) revoke(ctx context.Context, orderID uuid.UUID) error {
	_, err := s.repo.RevokeForOrder(ctx, orderID)
	return err
}

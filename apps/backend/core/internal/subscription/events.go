package subscription

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// One subscription line to start for a placed order - the product's recurring terms plus the snapshotted line price, sku and title
type StartableSubscription struct {
	ProductID     uuid.UUID
	Price         int64
	CurrencyCode  string
	Quantity      float64
	SKU           string
	Title         string
	IntervalUnit  string
	IntervalCount int32
	TrialDays     int32
}

// The subscription lines to start for a placed order - declared locally so subscription doesn't import catalog, with the adapter in main.go resolving each line's product terms
type OrderSubscriptionLookup interface {
	SubscriptionsForOrder(ctx context.Context, orderID uuid.UUID) ([]StartableSubscription, error)
}

// Subscription lifecycle event types, published for the Synapse activity tracker and any plugin - async and best-effort, not durable
const (
	EventCreated       = "subscription.created"
	EventCanceled      = "subscription.cancelled"
	EventPaused        = "subscription.paused"
	EventResumed       = "subscription.resumed"
	EventInvoiced      = "subscription.invoiced"
	EventPaymentFailed = "subscription.payment_failed"
)

// The identity of a state change - Kind discriminates created, cancelled, paused and resumed, and ChannelID may be uuid.Nil when no channel resolves
type Lifecycle struct {
	Kind           string
	SubscriptionID uuid.UUID
	CustomerID     *uuid.UUID
	ChannelID      uuid.UUID
	ProductID      uuid.UUID
}

func (l *Lifecycle) Type() string              { return l.Kind }
func (l *Lifecycle) EventChannelID() uuid.UUID { return l.ChannelID }

// Fires AFTER a billing-period invoice is minted, whether bill-in-advance or catch-up, one event per newly created invoice - Sequence is the invoice's per-subscription ordinal, a stable dedupe key for the activity tracker
type Invoiced struct {
	SubscriptionID uuid.UUID
	CustomerID     *uuid.UUID
	ChannelID      uuid.UUID
	Sequence       int32
	AmountCents    int64
	Currency       string
}

func (*Invoiced) Type() string                { return EventInvoiced }
func (i *Invoiced) EventChannelID() uuid.UUID { return i.ChannelID }

// Fires after a renewal charge fails, one event per dunning attempt - WillCancel marks the terminal failure where the ladder is exhausted and the subscription was cancelled, and NextRetryAt is nil in that case
type PaymentFailed struct {
	SubscriptionID uuid.UUID
	CustomerID     *uuid.UUID
	ChannelID      uuid.UUID
	InvoiceID      uuid.UUID
	Sequence       int32
	AmountCents    int64
	Currency       string
	Attempt        int
	NextRetryAt    *time.Time
	WillCancel     bool
	Reason         string
}

func (*PaymentFailed) Type() string                { return EventPaymentFailed }
func (p *PaymentFailed) EventChannelID() uuid.UUID { return p.ChannelID }

func chanOrNil(c *uuid.UUID) uuid.UUID {
	if c == nil {
		return uuid.Nil
	}
	return *c
}

func (s *Service) publishLifecycle(ctx context.Context, kind string, sub Subscription) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &Lifecycle{
		Kind:           kind,
		SubscriptionID: sub.ID,
		CustomerID:     sub.CustomerID,
		ChannelID:      chanOrNil(sub.ChannelID),
		ProductID:      sub.ProductID,
	})
}

func (s *Service) publishInvoiced(ctx context.Context, sub Subscription, p plannedPeriod) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(ctx, &Invoiced{
		SubscriptionID: sub.ID,
		CustomerID:     sub.CustomerID,
		ChannelID:      chanOrNil(sub.ChannelID),
		Sequence:       p.Sequence,
		AmountCents:    sub.Price,
		Currency:       sub.CurrencyCode,
	})
}

// Wires start-on-purchase - on OrderPlaced, which is durable and at-least-once, start a subscription per line, idempotent under replay - It retains the bus so the lifecycle methods can publish their own events
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
	// Idempotency - OrderPlaced is at-least-once, so subscriptions that already exist mean a replay
	if existing, err := s.repo.ListByOrder(ctx, placed.OrderID); err != nil {
		log.Printf("[subscription] check subs for order %s: %v", placed.OrderID, err)
		return err
	} else if len(existing) > 0 {
		return nil
	}
	lines, err := s.orders.SubscriptionsForOrder(ctx, placed.OrderID)
	if err != nil {
		log.Printf("[subscription] resolve subs for order %s: %v", placed.OrderID, err)
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	orderID := placed.OrderID
	now := s.now()
	var createdSubs []Subscription
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		createdSubs = nil // reset so a tx retry doesn't double-publish
		r := s.repo.WithTx(t)
		for _, ln := range lines {
			// The originating order covers the first period - TrialDays delays the first renewal, otherwise it's one interval out
			start := now
			var end time.Time
			if ln.TrialDays > 0 {
				end = start.AddDate(0, 0, int(ln.TrialDays))
			} else {
				end = nextBillingTime(start, ln.IntervalUnit, ln.IntervalCount)
			}
			channelID := placed.ChannelID
			sub, err := r.Create(ctx, StartInput{
				CustomerID:         placed.CustomerID,
				ChannelID:          &channelID,
				ProductID:          ln.ProductID,
				OrderID:            &orderID,
				IntervalUnit:       ln.IntervalUnit,
				IntervalCount:      ln.IntervalCount,
				Price:              ln.Price,
				CurrencyCode:       ln.CurrencyCode,
				Quantity:           ln.Quantity,
				SKU:                ln.SKU,
				Title:              ln.Title,
				CurrentPeriodStart: start,
				CurrentPeriodEnd:   end,
				NextBillingAt:      end,
			})
			if err != nil {
				return err
			}
			createdSubs = append(createdSubs, sub)
		}
		return nil
	})
	if err != nil {
		log.Printf("[subscription] start for order %s failed (will retry on replay): %v", placed.OrderID, err)
		return err
	}
	for _, sub := range createdSubs {
		s.publishLifecycle(ctx, EventCreated, sub)
	}
	return nil
}

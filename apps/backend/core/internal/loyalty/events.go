package loyalty

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// The earn subscriber computes merchandise value as subtotal minus discounts - shipping and tax don't earn points
type OrderLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (order.Order, error)
}

// Optional - without it, earning on purchase is disabled
func (s *Service) SetOrderLookup(o OrderLookup) { s.orders = o }

// Durable via the outbox, at-least-once, idempotent under replay via order-linked rows - placement or a payment advance earns a paid-for order's points, a void takes them back
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventPlaced, s.onOrderPlaced)
	bus.Subscribe(order.EventPaymentStatusChanged, s.onPaymentStatusChanged)
	bus.Subscribe(order.EventCancelled, s.onOrderCancelled)
	bus.Subscribe(order.EventRefunded, s.onOrderRefunded)
}

func (s *Service) onOrderPlaced(ctx context.Context, e events.Event) error {
	placed, ok := e.(*order.OrderPlaced)
	if !ok {
		return nil
	}
	return s.earn(ctx, placed.OrderID)
}

func (s *Service) onPaymentStatusChanged(ctx context.Context, e events.Event) error {
	changed, ok := e.(*order.OrderPaymentStatusChanged)
	if !ok {
		return nil
	}
	switch {
	case paidFor(changed.Status):
		return s.earn(ctx, changed.OrderID)
	case changed.Status == order.PaymentVoided:
		// The authorization that earned the points never turned into money
		return s.revoke(ctx, changed.OrderID, 1, "payment voided - points revoked")
	}
	return nil
}

// An authorization is the gateway's promise to pay the whole order - unpaid (cash on delivery, bank transfer) isn't, and neither is a partial capture, which is how a cash deposit lands
func paidFor(paymentStatus string) bool {
	return paymentStatus == order.PaymentAuthorized || paymentStatus == order.PaymentPaid
}

func (s *Service) earn(ctx context.Context, orderID uuid.UUID) error {
	if err := s.earnForOrder(ctx, orderID); err != nil {
		log.Printf("[loyalty] earn for order %s: %v", orderID, err)
		return err // points owed - replay is safe, the order-linked entry dedupes
	}
	return nil
}

// Points earned on placement were redeemable into store credit before the order was paid - spend the credit, never pay, and the cancel clawback could only take back what was left
func (s *Service) earnForOrder(ctx context.Context, orderID uuid.UUID) error {
	if s.orders == nil {
		return nil
	}
	o, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("load order: %w", err)
	}
	if o.CustomerID == nil || o.Status == order.OrderStatusCancelled || !paidFor(o.PaymentStatus) {
		return nil
	}
	prog, err := s.ProgramFor(ctx, o.ChannelID)
	if err != nil {
		return err
	}
	if !prog.Enabled || prog.EarnPointsPerUnit <= 0 {
		return nil
	}
	merch := o.SubtotalAmount - o.DiscountAmount
	if merch <= 0 {
		return nil
	}
	// Per whole currency unit of the order's own currency - a flat /100 earned yen at a hundredth of the rate and dinars at ten times it
	unit := int64(math.Pow10(money.ISODecimals(o.CurrencyCode)))
	points := merch / unit * prog.EarnPointsPerUnit
	if points <= 0 {
		return nil
	}
	customerID := *o.CustomerID
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		// Locked before the dedupe read - concurrent deliveries each saw no row and both earned
		if err := q.LockCustomerForLoyalty(ctx, customerID); err != nil {
			return fmt.Errorf("lock customer: %w", err)
		}
		rows, err := q.LoyaltyEntriesForOrder(ctx, uuid.NullUUID{UUID: orderID, Valid: true})
		if err != nil {
			return fmt.Errorf("check existing entries: %w", err)
		}
		if len(rows) > 0 {
			return nil
		}
		if _, err := q.InsertLoyaltyEntry(ctx, db.InsertLoyaltyEntryParams{
			CustomerID: customerID,
			OrderID:    uuid.NullUUID{UUID: orderID, Valid: true},
			Points:     points,
			Reason:     fmt.Sprintf("order %s", o.Number),
		}); err != nil {
			return fmt.Errorf("insert earn entry: %w", err)
		}
		return nil
	})
}

func (s *Service) onOrderCancelled(ctx context.Context, e events.Event) error {
	cancelled, ok := e.(*order.OrderCancelled)
	if !ok {
		return nil
	}
	return s.revoke(ctx, cancelled.OrderID, cancelled.RefundableFraction, "order cancelled - points revoked")
}

func (s *Service) revoke(ctx context.Context, orderID uuid.UUID, fraction float64, reason string) error {
	if err := s.revokeForOrder(ctx, orderID, fraction, reason); err != nil {
		log.Printf("[loyalty] %s for order %s: %v", reason, orderID, err)
		return err
	}
	return nil
}

// Without this, buy → earn → goodwill credit memo kept the points, and points redeem into store credit, so the loop minted spendable money
func (s *Service) onOrderRefunded(ctx context.Context, e events.Event) error {
	refunded, ok := e.(*order.OrderRefunded)
	if !ok {
		return nil
	}
	return s.revoke(ctx, refunded.OrderID, refunded.MerchandiseFraction, "order refunded - points revoked")
}

// Idempotent - a negative order-linked row means the revoke already ran A negative balance after the revoke means those points already became store credit, so it's clawed back too - else place unpaid, redeem, cancel mints free money on every loop fraction revokes only the points attributable to the UNSHIPPED share - points earned on goods the customer kept stay earned
func (s *Service) revokeForOrder(ctx context.Context, orderID uuid.UUID, fraction float64, reason string) error {
	// Read before the tx for the currency and program - a failed lookup returns so the outbox retries, since skipping the clawback is how spent points turned into free credit
	var currency string
	var prog Program
	if s.orders != nil {
		o, err := s.orders.GetByID(ctx, orderID)
		if err != nil {
			return fmt.Errorf("load order: %w", err)
		}
		currency = o.CurrencyCode
		if prog, err = s.ProgramFor(ctx, o.ChannelID); err != nil {
			return err
		}
	}
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		orderKey := uuid.NullUUID{UUID: orderID, Valid: true}
		rows, err := q.LoyaltyEntriesForOrder(ctx, orderKey)
		if err != nil {
			return fmt.Errorf("list entries for order: %w", err)
		}
		var customerID uuid.UUID
		for _, r := range rows {
			customerID = r.CustomerID
		}
		if customerID == uuid.Nil {
			return nil // no points were earned on this order
		}
		// Lock BEFORE summing what's already revoked - revoking toward a target only converges when the reads serialize, else two deliveries each insert the full delta
		if err := q.LockCustomerForLoyalty(ctx, customerID); err != nil {
			return fmt.Errorf("lock customer: %w", err)
		}
		rows, err = q.LoyaltyEntriesForOrder(ctx, orderKey) // re-read under the lock - whoever we queued behind has committed by now
		if err != nil {
			return fmt.Errorf("re-read entries for order: %w", err)
		}
		var earned, alreadyRevoked int64
		for _, r := range rows {
			if r.Points < 0 {
				alreadyRevoked += -r.Points
				continue
			}
			earned += r.Points
		}
		// Toward a TARGET, not bailing on the first negative row - bailing meant only the first partial refund ever deducted
		target := money.ScaleFraction(earned, fraction)
		delta := target - alreadyRevoked
		if delta <= 0 {
			return nil
		}
		if _, err := q.InsertLoyaltyEntry(ctx, db.InsertLoyaltyEntryParams{
			CustomerID: customerID,
			OrderID:    uuid.NullUUID{UUID: orderID, Valid: true},
			Points:     -delta,
			Reason:     reason,
		}); err != nil {
			return fmt.Errorf("insert revoke entry: %w", err)
		}
		// A negative balance means those points were already spent, and the only way to spend them is Redeem, which mints store credit - take that credit back, clamped at what's left so we never create hidden debt
		if s.credit == nil || currency == "" || prog.RedeemCentsPerPoint <= 0 {
			return nil
		}
		bal, err := q.LoyaltyBalance(ctx, customerID)
		if err != nil {
			return fmt.Errorf("loyalty balance: %w", err)
		}
		if bal >= 0 {
			return nil
		}
		cents := -bal * prog.RedeemCentsPerPoint
		if _, err := s.credit.ClawbackTx(ctx, t, customerID, cents, currency, reason); err != nil {
			return fmt.Errorf("claw back redeemed credit: %w", err)
		}
		return nil
	})
}

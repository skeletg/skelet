package storecredit

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

// SubscribeToOrderEvents wires cancel-restore: when an order that spent store credit is cancelled (durable via the outbox, at-least-once),
// the redeemed amount is credited back - Idempotent under replay - a positive order-linked ledger row means the restore already ran
func (s *Service) SubscribeToOrderEvents(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventCancelled, s.onOrderCancelled)
}

func (s *Service) onOrderCancelled(ctx context.Context, e events.Event) error {
	cancelled, ok := e.(*order.OrderCancelled)
	if !ok {
		return nil
	}
	if err := s.restoreForOrder(ctx, cancelled.OrderID, cancelled.RefundableFraction); err != nil {
		log.Printf("[storecredit] restore for cancelled order %s: %v", cancelled.OrderID, err)
		return err
	}
	return nil
}

// fraction scales the restore to the UNSHIPPED share - cancelling a partly-shipped order must not hand back every cent while the customer keeps the goods
func (s *Service) restoreForOrder(ctx context.Context, orderID uuid.UUID, fraction float64) error {
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		orderKey := uuid.NullUUID{UUID: orderID, Valid: true}
		rows, err := q.StoreCreditEntriesForOrder(ctx, orderKey)
		if err != nil {
			return fmt.Errorf("list entries for order: %w", err)
		}
		var customerID uuid.UUID
		for _, r := range rows {
			customerID = r.CustomerID
		}
		if customerID == uuid.Nil {
			return nil // no store credit paid for this order
		}
		// Lock BEFORE deciding whether the restore already ran - the outbox dispatches a claimed batch concurrently, so two deliveries both reading "not restored" would both credit
		if err := q.LockCustomerForStoreCredit(ctx, customerID); err != nil {
			return fmt.Errorf("lock customer: %w", err)
		}
		rows, err = q.StoreCreditEntriesForOrder(ctx, orderKey) // re-read under the lock - whoever we queued behind has committed by now
		if err != nil {
			return fmt.Errorf("re-read entries for order: %w", err)
		}
		var redeemed int64
		var currency string
		for _, r := range rows {
			if r.AmountCents > 0 {
				return nil // already restored
			}
			redeemed += -r.AmountCents
			currency = r.CurrencyCode
		}
		redeemed = money.ScaleFraction(redeemed, fraction)
		if redeemed <= 0 {
			return nil
		}
		if _, err := q.InsertStoreCreditEntry(ctx, db.InsertStoreCreditEntryParams{
			CustomerID:   customerID,
			OrderID:      uuid.NullUUID{UUID: orderID, Valid: true},
			AmountCents:  redeemed,
			CurrencyCode: currency,
			Reason:       "order cancelled - credit restored",
		}); err != nil {
			return fmt.Errorf("insert restore entry: %w", err)
		}
		return nil
	})
}

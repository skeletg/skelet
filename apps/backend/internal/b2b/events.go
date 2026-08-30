package b2b

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/money"
	"github.com/skeletg/skelet/internal/order"
)

func (s *Service) SubscribeToOrderEvents(bus *events.Bus) { // automatic credit release - a cancelled on-account order returns its reserved credit to the company line, durably via the outbox and at-least-once, so a negative order-linked ledger row is the idempotence marker
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
	if err := s.releaseForOrder(ctx, cancelled.OrderID, cancelled.RefundableFraction); err != nil {
		log.Printf("[b2b] release credit for cancelled order %s: %v", cancelled.OrderID, err)
		return err
	}
	return nil
}

func (s *Service) releaseForOrder(ctx context.Context, orderID uuid.UUID, fraction float64) error { // fraction releases only the UNSHIPPED share of the reserved credit - the shipped part is still owed
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := s.queries.WithTx(t)
		orderKey := uuid.NullUUID{UUID: orderID, Valid: true}
		rows, err := q.B2BCreditEntriesForOrder(ctx, orderKey)
		if err != nil {
			return fmt.Errorf("list entries for order: %w", err)
		}
		var companyID uuid.UUID
		for _, r := range rows {
			companyID = r.CompanyID
		}
		if companyID == uuid.Nil {
			return nil // not an on-account order
		}
		if _, err := q.GetCompanyForUpdate(ctx, companyID); err != nil { // lock BEFORE deciding whether the release already ran - the outbox dispatches a claimed batch concurrently, so two deliveries both reading "not released" would both release
			return ErrNotFound
		}
		rows, err = q.B2BCreditEntriesForOrder(ctx, orderKey) // re-read under the lock - whoever we queued behind has committed by now
		if err != nil {
			return fmt.Errorf("re-read entries for order: %w", err)
		}
		var reserved int64
		for _, r := range rows {
			if r.AmountCents < 0 {
				return nil // already released
			}
			reserved += r.AmountCents
		}
		reserved = money.ScaleFraction(reserved, fraction)
		if reserved <= 0 {
			return nil // nothing unshipped to release
		}
		if _, err := q.AdjustCompanyCreditUsed(ctx, db.AdjustCompanyCreditUsedParams{
			ID:              companyID,
			CreditUsedCents: -reserved,
		}); err != nil {
			return fmt.Errorf("release credit: %w", err)
		}
		if _, err := q.InsertB2BCreditEntry(ctx, db.InsertB2BCreditEntryParams{
			CompanyID:   companyID,
			OrderID:     uuid.NullUUID{UUID: orderID, Valid: true},
			AmountCents: -reserved,
			Reason:      "order cancelled - credit released",
		}); err != nil {
			return fmt.Errorf("record credit release: %w", err)
		}
		if _, err := q.VoidOpenB2BInvoiceForOrder(ctx, orderID); err != nil { // close the invoice in the same tx - leaving it 'open' keeps SettleInvoice's status guard satisfied, so voiding it later would release this amount a second time and hand the company phantom headroom
			return fmt.Errorf("void invoice for cancelled order: %w", err)
		}
		return nil
	})
}

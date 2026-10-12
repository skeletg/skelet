package order

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Payment status values for orders.payment_status - free-form TEXT, so the lifecycle lives here: unpaid to authorized to paid to refunded, with unpaid or authorized able to go to voided
const (
	PaymentUnpaid            = "unpaid"
	PaymentAuthorized        = "authorized"
	PaymentPartiallyCaptured = "partially_captured"
	PaymentPaid              = "paid"
	PaymentPartiallyRefunded = "partially_refunded"
	PaymentRefunded          = "refunded"
	PaymentVoided            = "voided"
)

// The allowed forward moves, with anything unlisted rejected - that gives idempotency, since a repeated event no-ops, and protects against out-of-order webhooks, since an "authorized" after "paid" can't downgrade
// The partially_* states support the ledger's partial captures and refunds
var paymentTransitions = map[string]map[string]bool{
	PaymentUnpaid:            {PaymentAuthorized: true, PaymentPartiallyCaptured: true, PaymentPaid: true, PaymentVoided: true},
	PaymentAuthorized:        {PaymentPartiallyCaptured: true, PaymentPaid: true, PaymentVoided: true},
	PaymentPartiallyCaptured: {PaymentPartiallyCaptured: true, PaymentPaid: true, PaymentPartiallyRefunded: true, PaymentRefunded: true},
	PaymentPaid:              {PaymentPartiallyRefunded: true, PaymentRefunded: true},
	PaymentPartiallyRefunded: {PaymentPartiallyRefunded: true, PaymentRefunded: true},
	PaymentRefunded:          {},
	PaymentVoided:            {},
}

// Projects a status label from the ledger totals and amount owed - pure, so table-testable - The ledger is the truth for amounts and this is a fast read label
// amountOwed is what the instruments owe, totalTendered the total across every tender - the only thing a refund of non-instrument money measures against
func derivePaymentStatus(t PaymentTotals, amountOwed, totalTendered int64) string {
	switch {
	case t.Captured == 0 && t.Voided > 0:
		return PaymentVoided
	// A gift-card order refunded to a new gift card moves real value with no capture behind it - against amountOwed (zero) that reads as "paid" and hides the refund
	case t.Captured == 0 && t.Refunded > 0:
		if totalTendered > 0 && t.Refunded < totalTendered {
			return PaymentPartiallyRefunded
		}
		return PaymentRefunded
	case t.Captured == 0 && t.Authorized == 0 && amountOwed <= 0:
		return PaymentPaid
	case t.Captured == 0 && t.Authorized == 0:
		return PaymentUnpaid
	case t.Captured == 0:
		return PaymentAuthorized
	case t.Refunded >= t.Captured:
		return PaymentRefunded
	case t.Refunded > 0:
		return PaymentPartiallyRefunded
	case t.Captured >= amountOwed:
		return PaymentPaid
	default:
		return PaymentPartiallyCaptured
	}
}

// Resolves the status after a gateway event and whether it changed - pure, with unknown events and disallowed transitions no-oping
func nextPaymentStatus(current, event string) (string, bool) {
	if event == "" || event == current {
		return current, false
	}
	if current == "" {
		current = PaymentUnpaid
	}
	if paymentTransitions[current][event] {
		return event, true
	}
	return current, false
}

// Applies a gateway event - advances payment_status idempotently and persists the provider reference when supplied, with no-op events returning the order unchanged
// ListAwaitingPaymentSync feeds the payment-sync sweep - see the job in wiring/jobs.go
func (s *Service) ListAwaitingPaymentSync(ctx context.Context, notBefore, olderThan time.Time, maxRows int32) ([]Order, error) {
	return s.repo.ListOrdersAwaitingPaymentSync(ctx, notBefore, olderThan, maxRows)
}

// The write is conditional on the status read - the sync sweep and a webhook can settle the same order at once, and the slower one used to write authorized over paid - so a lost race re-reads and re-applies the event to what's there now
func (s *Service) SettlePayment(ctx context.Context, orderID uuid.UUID, event, providerReference string) (Order, error) {
	for range settleAttempts {
		o, err := s.repo.GetOrderByID(ctx, orderID)
		if err != nil {
			return Order{}, err
		}
		next, changed := nextPaymentStatus(o.PaymentStatus, event)
		ref := o.PaymentReference
		if providerReference != "" {
			ref = providerReference
		}
		if !changed && ref == o.PaymentReference {
			return o, nil
		}
		updated, applied, err := s.repo.SetPaymentStateFrom(ctx, orderID, o.PaymentStatus, next, ref)
		if err != nil {
			return Order{}, err
		}
		if !applied {
			continue
		}
		if s.bus != nil && changed {
			s.publishDurable(ctx, &OrderPaymentStatusChanged{
				OrderID:   orderID,
				ChannelID: o.ChannelID,
				Status:    next,
				Reference: ref,
			})
		}
		return updated, nil
	}
	return Order{}, apperr.New("PAYMENT_STATUS_RACED", apperr.ClassConflict, "the order's payment status kept changing - try again")
}

const settleAttempts = 3

// Settles a pending ledger row - moved is false when the row was already final with this same outcome, so a repeat finalize changes nothing, and a different final outcome is refused rather than flipped
func finalizeTxn(ctx context.Context, r Repository, txnID uuid.UUID, ok bool, providerReference string) (PaymentTransaction, bool, error) {
	status := PaymentTxnFailed
	if ok {
		status = PaymentTxnSucceeded
	}
	txn, moved, err := r.MarkPaymentTransaction(ctx, txnID, status, providerReference)
	if err != nil || moved {
		return txn, moved, err
	}
	if txn.Status == status {
		return txn, false, nil
	}
	return txn, false, apperr.New("PAYMENT_ALREADY_SETTLED", apperr.ClassConflict,
		fmt.Sprintf("this payment transaction is already %s", txn.Status))
}

const OfflineProviderSlug = "offline" // the ledger was gateway-only, so COD / bank transfer / PO / pickup sat at Captured=0 forever and every refund guard read "nothing collected"

type RecordOfflinePaymentInput struct {
	OrderID uuid.UUID
	// Zero means the whole remaining balance
	AmountCents int64
	// A payment provider slug (cod, bank-transfer, pay-on-pickup, purchase-order, or any registered offline provider) -
	// free-form so an operator can record a tender the customer didn't originally choose
	Method    string
	Reference string
	// When the money landed, defaulting to now - a transfer reconciled Thursday may have cleared Monday
	ReceivedAt *time.Time
	Note       string
}

// Records money taken outside a gateway as a real capture row, then lets the status derive from the ledger
// Partial payments are the point - demanding UNPAID and recording the whole balance makes a deposit then the rest impossible
func (s *Service) RecordOfflinePayment(ctx context.Context, in RecordOfflinePaymentInput) (Order, error) {
	if in.AmountCents < 0 {
		return Order{}, apperr.New("PAYMENT_INVALID_AMOUNT", apperr.ClassValidation, "payment amount cannot be negative")
	}

	// The slug from the payment registry - order doesn't import payment, and the slug
	// is the stable thing to store anyway; the UI resolves display names
	method := strings.TrimSpace(in.Method)
	if method == "" {
		method = "other"
	}
	ref := strings.TrimSpace(in.Reference)
	if ref == "" {
		ref = "manual"
	}

	var updated Order
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		// Two operators recording the same transfer would otherwise both see the full balance owed
		if err := r.LockOrderForPayment(ctx, in.OrderID); err != nil {
			return fmt.Errorf("lock order for payment: %w", err)
		}
		// Re-read under the lock - an edit committed since the first read changed what's owed, and a cancelled order takes no cash
		o, err := r.GetOrderByID(ctx, in.OrderID)
		if err != nil {
			return err
		}
		if err := collectable(o); err != nil {
			return err
		}
		totals, err := r.PaymentTotalsActiveByKind(ctx, in.OrderID)
		if err != nil {
			return err
		}
		remaining := o.AmountDue() - totals.Captured
		if remaining <= 0 {
			return apperr.New("PAYMENT_NOTHING_OWED", apperr.ClassConflict,
				"this order has nothing left to collect")
		}
		amount := in.AmountCents
		if amount == 0 {
			amount = remaining
		}
		if amount > remaining {
			return apperr.New("PAYMENT_OVER_COLLECT", apperr.ClassConflict,
				fmt.Sprintf("payment exceeds the %d cents still owed", remaining))
		}

		// A fresh key per payment - a per-order key makes a second instalment a no-op
		key, err := uuid.NewV7()
		if err != nil {
			return err
		}
		reason := "offline payment - " + method
		if note := strings.TrimSpace(in.Note); note != "" {
			reason += " - " + note
		}
		if _, err := r.InsertPaymentTransaction(ctx, PaymentTransaction{
			OrderID:           in.OrderID,
			Kind:              PaymentTxnCapture,
			AmountCents:       amount,
			Status:            PaymentTxnSucceeded,
			ProviderSlug:      OfflineProviderSlug,
			ProviderReference: ref,
			IdempotencyKey:    key.String(),
			Reason:            reason,
			OccurredAt:        in.ReceivedAt,
		}); err != nil {
			return fmt.Errorf("record offline capture: %w", err)
		}

		msg := fmt.Sprintf("Recorded %d cents by %s", amount, method)
		if in.Reference != "" {
			msg += " (ref: " + strings.TrimSpace(in.Reference) + ")"
		}
		if amount < remaining {
			msg += fmt.Sprintf(" - %d cents still owed", remaining-amount)
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{OrderID: in.OrderID, Type: "payment_recorded", Message: msg}); err != nil {
			return err
		}
		// Derived, not forced to paid - a part payment has to land on partially_captured
		fresh, err := r.PaymentTotalsByKind(ctx, in.OrderID)
		if err != nil {
			return err
		}
		updated, err = r.SetPaymentState(ctx, in.OrderID, derivePaymentStatus(fresh, o.AmountDue(), o.TotalAmount), ref)
		return err
	})
	if err != nil {
		return Order{}, err
	}
	if s.bus != nil {
		s.publishDurable(ctx, &OrderPaymentStatusChanged{
			OrderID:   updated.ID,
			ChannelID: updated.ChannelID,
			Status:    updated.PaymentStatus,
			Reference: updated.PaymentReference,
		})
	}
	return updated, nil
}

func collectable(o Order) error {
	if o.Status == OrderStatusCancelled || o.Status == OrderStatusDraft {
		return apperr.New("PAYMENT_NOT_COLLECTABLE", apperr.ClassConflict,
			fmt.Sprintf("a %s order cannot take a payment", o.Status))
	}
	switch o.PaymentStatus {
	case PaymentUnpaid, PaymentPartiallyCaptured, PaymentAuthorized, "":
		return nil
	}
	return apperr.New("PAYMENT_NOT_COLLECTABLE", apperr.ClassConflict,
		fmt.Sprintf("an order that is %s cannot take another payment", o.PaymentStatus))
}

func (s *Service) TaxLines(ctx context.Context, orderID uuid.UUID) ([]TaxLine, error) {
	return s.repo.ListTaxLines(ctx, orderID)
}

func (s *Service) PaymentLedger(ctx context.Context, orderID uuid.UUID) ([]PaymentTransaction, error) {
	return s.repo.ListPaymentTransactions(ctx, orderID)
}

// RecordAuthorization writes the authorize row every capture and refund guard reads. Without one PaymentTotals.Authorized stays zero and the FIRST capture on every order is rejected as an over-capture - goods shipped, the authorization expired, and the only trace a log line. Written as succeeded because the gateway has already confirmed the hold by the time we get here. Idempotent on a deterministic key, so a retried authorize or a replayed 3DS finalize reuses the row rather than doubling the authorized total.
func (s *Service) RecordAuthorization(ctx context.Context, orderID uuid.UUID, amountCents int64, providerSlug, providerReference string) (PaymentTransaction, error) {
	if amountCents <= 0 {
		return PaymentTransaction{}, apperr.New("PAYMENT_INVALID_AMOUNT", apperr.ClassValidation, "authorization amount must be positive")
	}
	return s.recordSettledTxn(ctx, orderID, PaymentTxnAuthorize, amountCents, providerSlug, providerReference, "authorized at checkout")
}

// authorizationValidFor bounds how long a gateway hold is assumed good. Most card networks expire an authorization around 7 days; capturing after that is declined. Nothing modelled expiry before, so a backordered line that shipped two weeks late simply failed to capture with only a log line to show for it.
const authorizationValidFor = 7 * 24 * time.Hour

// AuthorizationExpired reports whether the order's authorize row is old enough that the gateway is likely to refuse a capture - advisory, so callers warn rather than block
func (s *Service) AuthorizationExpired(ctx context.Context, orderID uuid.UUID) (bool, time.Time, error) {
	rows, err := s.repo.ListPaymentTransactions(ctx, orderID)
	if err != nil {
		return false, time.Time{}, err
	}
	for _, t := range rows {
		if t.Kind == PaymentTxnAuthorize && t.Status == PaymentTxnSucceeded {
			expiresAt := t.CreatedAt.Add(authorizationValidFor)
			return time.Now().After(expiresAt), expiresAt, nil
		}
	}
	return false, time.Time{}, nil
}

// RecordCaptureFailure puts a failed capture on the order timeline where an operator can see it. A capture that fails after the goods have shipped is money the store will never collect, and the only previous signal was a log line
func (s *Service) RecordCaptureFailure(ctx context.Context, orderID uuid.UUID, providerSlug string, cause error) {
	msg := fmt.Sprintf("Payment capture failed (%s): %v", providerSlug, cause)
	if expired, expiresAt, err := s.AuthorizationExpired(ctx, orderID); err == nil && expired {
		msg += fmt.Sprintf(" - the authorization expired on %s and the customer must be re-charged", expiresAt.Format("2006-01-02"))
	}
	if _, err := s.repo.AddOrderEvent(ctx, OrderEvent{
		OrderID: orderID,
		Type:    "payment_capture_failed",
		Message: msg,
	}); err != nil {
		log.Printf("[order] record capture-failure event for order %s: %v", orderID, err)
	}
}

// SettleGatewayAmount records an out-of-band gateway movement and re-derives the order's payment status FROM THE LEDGER. Mapping an event name straight onto a status with the amount thrown away would mark a $10 goodwill refund on a $500 order fully "refunded" - a terminal state that blocks every later correction - and read a $50 partial capture as fully paid. Idempotent on the gateway's own reference, so a re-delivered webhook is a no-op.
func (s *Service) SettleGatewayAmount(ctx context.Context, orderID uuid.UUID, kind string, amountCents int64, providerSlug, providerReference string) (Order, error) {
	if amountCents <= 0 || providerReference == "" {
		return Order{}, apperr.New("PAYMENT_INVALID_AMOUNT", apperr.ClassValidation, "gateway settlement needs a positive amount and a reference")
	}
	key := kind + ":gw:" + providerReference
	var memoID uuid.UUID
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		if err := r.LockOrderForPayment(ctx, orderID); err != nil {
			return fmt.Errorf("lock order for payment: %w", err)
		}
		if _, found, err := r.FindPaymentTransactionByKey(ctx, key); err != nil {
			return err
		} else if found {
			return nil // already recorded - re-delivered webhook
		}
		// The same money can arrive under two keys: an orchestrated capture or refund records its own row (uuidv7 / memo key) and the gateway's event echoes the reference back through here - match the reference itself, and treat a pending row of the same kind as the in-flight twin, since the echo can land before finalize stamps the reference on it Without this, Captured doubles - and Captured is what every refund ceiling measures
		existing, err := r.ListPaymentTransactions(ctx, orderID)
		if err != nil {
			return err
		}
		for _, p := range existing {
			if p.Kind != kind || p.Status == PaymentTxnFailed {
				continue
			}
			// A pending row is the twin only when it is the same money - any pending refund used to swallow an unrelated one typed into the gateway's dashboard
			if p.ProviderReference == providerReference || (p.Status == PaymentTxnPending && p.AmountCents == amountCents) {
				return nil
			}
		}
		txn, err := r.InsertPaymentTransaction(ctx, PaymentTransaction{
			OrderID:           orderID,
			Kind:              kind,
			AmountCents:       amountCents,
			Status:            PaymentTxnSucceeded,
			ProviderSlug:      providerSlug,
			ProviderReference: providerReference,
			IdempotencyKey:    key,
			Reason:            "gateway webhook",
		})
		if err != nil {
			return err
		}
		// A refund typed into the gateway's own dashboard still owes a memo and a reward clawback - no line detail exists, so it's line-less and attributed by money share
		if kind != PaymentTxnRefund {
			return nil
		}
		o, err := r.GetOrderByID(ctx, orderID)
		if err != nil {
			return err
		}
		memo, err := r.CreateCreditMemo(ctx, CreditMemo{
			OrderID:               orderID,
			AdjustmentRefundCents: amountCents,
			TotalCents:            amountCents,
			RefundTarget:          RefundTargetOriginal,
			Reason:                "refunded at the payment gateway",
		})
		if err != nil {
			return fmt.Errorf("write credit memo for gateway refund: %w", err)
		}
		if err := r.LinkPaymentTransactionToMemo(ctx, txn.ID, memo.ID); err != nil {
			return err
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: orderID,
			Type:    "credit_memo_issued",
			Message: fmt.Sprintf("Credit memo %s recorded for a refund made at the gateway.", memo.Number),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		memoID = memo.ID
		return s.enqueueRefunded(ctx, sqlTx, r, o, memo.ID)
	})
	if err != nil {
		return Order{}, err
	}
	updated, err := s.RecomputePaymentStatus(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	s.publishRefundedWithoutOutbox(ctx, updated, memoID)
	return updated, nil
}

// RecomputePaymentStatus re-derives payment_status from the ledger and persists any change
func (s *Service) RecomputePaymentStatus(ctx context.Context, orderID uuid.UUID) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	totals, err := s.repo.PaymentTotalsByKind(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	next := derivePaymentStatus(totals, o.AmountDue(), o.TotalAmount)
	if next == o.PaymentStatus {
		return o, nil
	}
	return s.SettlePayment(ctx, orderID, next, o.PaymentReference)
}

// RecordVoid mirrors RecordAuthorization for a released authorization - derivePaymentStatus has a voided branch that was dead because no void row was ever written
func (s *Service) RecordVoid(ctx context.Context, orderID uuid.UUID, amountCents int64, providerSlug, providerReference string) (PaymentTransaction, error) {
	if amountCents <= 0 {
		return PaymentTransaction{}, apperr.New("PAYMENT_INVALID_AMOUNT", apperr.ClassValidation, "void amount must be positive")
	}
	return s.recordSettledTxn(ctx, orderID, PaymentTxnVoid, amountCents, providerSlug, providerReference, "authorization voided")
}

// recordSettledTxn inserts an already-final ledger row under a deterministic key, returning the existing row on replay
// The key carries the gateway's reference when there is one - a per-order key turned a re-authorization after an expired hold into a replay of the first, so the new hold was never recorded and its capture read as an over-capture
func (s *Service) recordSettledTxn(ctx context.Context, orderID uuid.UUID, kind string, amountCents int64, providerSlug, providerReference, reason string) (PaymentTransaction, error) {
	key := kind + ":" + orderID.String()
	if providerReference != "" {
		key += ":" + providerReference
	}
	var txn PaymentTransaction
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		if err := r.LockOrderForPayment(ctx, orderID); err != nil {
			return fmt.Errorf("lock order for payment: %w", err)
		}
		existing, found, err := r.FindPaymentTransactionByKey(ctx, key)
		if err != nil {
			return err
		}
		if found {
			txn = existing
			return nil
		}
		txn, err = r.InsertPaymentTransaction(ctx, PaymentTransaction{
			OrderID:           orderID,
			Kind:              kind,
			AmountCents:       amountCents,
			Status:            PaymentTxnSucceeded,
			ProviderSlug:      providerSlug,
			ProviderReference: providerReference,
			IdempotencyKey:    key,
			Reason:            reason,
		})
		return err
	})
	return txn, err
}

// Reserves a capture before the gateway call - validates the amount against the authorized remaining and writes a pending ledger row, which FinalizePaymentTransaction settles once the provider responds
func (s *Service) RecordCaptureIntent(ctx context.Context, orderID uuid.UUID, amountCents int64, providerSlug, reason string) (PaymentTransaction, error) {
	return s.recordPaymentIntent(ctx, orderID, PaymentTxnCapture, amountCents, providerSlug, reason)
}

func (s *Service) recordPaymentIntent(ctx context.Context, orderID uuid.UUID, kind string, amountCents int64, providerSlug, reason string) (PaymentTransaction, error) {
	if amountCents <= 0 {
		return PaymentTransaction{}, apperr.New("PAYMENT_INVALID_AMOUNT", apperr.ClassValidation, "payment amount must be positive")
	}
	var txn PaymentTransaction
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		// Lock the order and count money already in flight - without it two concurrent intents read identical succeeded-only totals, both pass, and both reach the gateway
		if err := r.LockOrderForPayment(ctx, orderID); err != nil {
			return fmt.Errorf("lock order for payment: %w", err)
		}
		totals, err := r.PaymentTotalsActiveByKind(ctx, orderID)
		if err != nil {
			return err
		}
		switch kind {
		case PaymentTxnCapture:
			if amountCents > totals.Authorized-totals.Captured {
				return apperr.New("PAYMENT_OVER_CAPTURE", apperr.ClassConflict, "capture exceeds the authorized remaining")
			}
		case PaymentTxnRefund:
			if amountCents > totals.Captured-totals.Refunded {
				return apperr.New("PAYMENT_OVER_REFUND", apperr.ClassConflict, "refund exceeds the captured remaining")
			}
		}
		key, err := uuid.NewV7()
		if err != nil {
			return err
		}
		txn, err = r.InsertPaymentTransaction(ctx, PaymentTransaction{
			OrderID:        orderID,
			Kind:           kind,
			AmountCents:    amountCents,
			Status:         PaymentTxnPending,
			ProviderSlug:   providerSlug,
			IdempotencyKey: key.String(),
			Reason:         reason,
		})
		return err
	})
	return txn, err
}

// Marks a pending row succeeded or failed and, on success, recomputes the order's payment status from the ledger totals
func (s *Service) FinalizePaymentTransaction(ctx context.Context, txnID uuid.UUID, ok bool, providerReference string) (Order, error) {
	var updated Order
	var before string
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		txn, moved, err := finalizeTxn(ctx, r, txnID, ok, providerReference)
		if err != nil {
			return err
		}
		o, err := r.GetOrderByID(ctx, txn.OrderID)
		if err != nil {
			return err
		}
		updated, before = o, o.PaymentStatus
		if !ok || !moved {
			return nil
		}
		totals, err := r.PaymentTotalsByKind(ctx, txn.OrderID)
		if err != nil {
			return err
		}
		// The order's PaymentReference, the authorize handle, stays put - the per-action ref lives on the ledger row, so future captures and refunds resolve
		updated, err = r.SetPaymentState(ctx, txn.OrderID, derivePaymentStatus(totals, o.AmountDue(), o.TotalAmount), o.PaymentReference)
		return err
	})
	if err != nil {
		return Order{}, err
	}
	// Only a real change - a failed capture or a repeat finalize published a status change that wasn't one
	if s.bus != nil && updated.PaymentStatus != before {
		s.publishDurable(ctx, &OrderPaymentStatusChanged{
			OrderID:   updated.ID,
			ChannelID: updated.ChannelID,
			Status:    updated.PaymentStatus,
			Reference: updated.PaymentReference,
		})
	}
	return updated, nil
}

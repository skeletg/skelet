package order

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// One refund event with the lines it covered - the ledger stays the money record, the memo explains it to a human and to accounting
// Born two ways: settling an RMA, or issued straight against an order with no return at all
type CreditMemo struct {
	ID       uuid.UUID
	OrderID  uuid.UUID
	Number   string
	ReturnID *uuid.UUID

	// total = lines + shipping + adjustmentRefund - adjustmentFee
	LinesTotalCents       int64
	ShippingRefundCents   int64
	AdjustmentRefundCents int64
	AdjustmentFeeCents    int64
	TotalCents            int64

	RefundTarget string
	Offline      bool
	Reason       string
	AdminID      *uuid.UUID
	CreatedAt    time.Time

	Lines []CreditMemoLine
}

type CreditMemoLine struct {
	ID          uuid.UUID
	OrderLineID uuid.UUID
	Quantity    float64
	UnitCents   int64
	TotalCents  int64
}

// CreditMemoLineInput is one (order line, quantity) to credit
type CreditMemoLineInput struct {
	OrderLineID uuid.UUID
	Quantity    float64
}

// Lines are optional, so a memo can be pure adjustment or shipping-only
type IssueCreditMemoInput struct {
	OrderID               uuid.UUID
	Lines                 []CreditMemoLineInput
	ShippingRefundCents   int64
	AdjustmentRefundCents int64
	AdjustmentFeeCents    int64
	RefundTarget          string // "original" | "gift_card"; empty → original
	Reason                string
	AdminID               *uuid.UUID
}

// The prepared refund - memo, pending ledger row, and enough to drive the gateway call, the gift card, or neither
type CreditMemoIntent struct {
	Txn         PaymentTransaction
	Order       Order
	Memo        CreditMemo
	AmountCents int64
	Target      string
	Offline     bool
}

func (s *Service) ListCreditMemos(ctx context.Context, orderID uuid.UUID) ([]CreditMemo, error) {
	return s.repo.ListCreditMemos(ctx, orderID)
}

func (s *Service) GetCreditMemo(ctx context.Context, id uuid.UUID) (CreditMemo, error) {
	return s.repo.GetCreditMemo(ctx, id)
}

// Ordered minus what earlier memos covered, so the UI can bound its inputs the way ReturnableByOrderLine does for RMAs
func (s *Service) CreditableByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	credited, err := s.repo.CreditedByOrderLine(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]float64, len(o.Lines))
	for _, l := range o.Lines {
		if rem := l.Quantity - credited[l.ID]; rem > 0 {
			out[l.ID] = rem
		} else {
			out[l.ID] = 0
		}
	}
	return out, nil
}

// Writes the memo and reserves its money as a pending ledger row - drive the refund next, then FinalizeCreditMemo
// One transaction under the order lock, so two admins issuing memos at once can't both read the same refundable remainder
func (s *Service) RecordCreditMemoIntent(ctx context.Context, in IssueCreditMemoInput) (CreditMemoIntent, error) {
	o, err := s.repo.GetOrderByID(ctx, in.OrderID)
	if err != nil {
		return CreditMemoIntent{}, fmt.Errorf("order not found")
	}
	if o.Status == OrderStatusDraft {
		return CreditMemoIntent{}, apperr.New("CREDIT_MEMO_DRAFT_ORDER", apperr.ClassConflict,
			"a draft order has taken no money, so there is nothing to credit")
	}
	target := in.RefundTarget
	if target == "" {
		target = RefundTargetOriginal
	}
	if target != RefundTargetOriginal && target != RefundTargetGiftCard {
		return CreditMemoIntent{}, apperr.New("CREDIT_MEMO_INVALID_TARGET", apperr.ClassValidation,
			"refund target must be 'original' or 'gift_card'")
	}
	if in.ShippingRefundCents < 0 || in.AdjustmentRefundCents < 0 || in.AdjustmentFeeCents < 0 {
		return CreditMemoIntent{}, apperr.New("CREDIT_MEMO_NEGATIVE_ADJUSTMENT", apperr.ClassValidation,
			"shipping refund, adjustment refund and adjustment fee must all be zero or positive")
	}
	if in.ShippingRefundCents > o.ShippingRefundable() {
		return CreditMemoIntent{}, apperr.New("CREDIT_MEMO_SHIPPING_EXCEEDED", apperr.ClassValidation,
			"shipping refund exceeds what was charged for shipping")
	}

	refundUnit := refundUnitByLine(o)
	orderedQty := make(map[uuid.UUID]float64, len(o.Lines))
	for _, l := range o.Lines {
		orderedQty[l.ID] = l.Quantity
	}
	// Dedupe first so a repeated line is summed, not double-counted against the guard
	reqByLine := make(map[uuid.UUID]float64, len(in.Lines))
	lineOrder := make([]uuid.UUID, 0, len(in.Lines))
	for _, ml := range in.Lines {
		if _, ok := orderedQty[ml.OrderLineID]; !ok {
			return CreditMemoIntent{}, apperr.New("CREDIT_MEMO_UNKNOWN_LINE", apperr.ClassValidation,
				"credit memo line does not belong to this order")
		}
		if ml.Quantity <= 0 {
			return CreditMemoIntent{}, apperr.New("CREDIT_MEMO_INVALID_QTY", apperr.ClassValidation,
				"credit quantity must be positive")
		}
		if _, seen := reqByLine[ml.OrderLineID]; !seen {
			lineOrder = append(lineOrder, ml.OrderLineID)
		}
		reqByLine[ml.OrderLineID] += ml.Quantity
	}

	offline := target == RefundTargetOriginal && (o.PaymentProvider == "" || o.PaymentReference == "")
	providerSlug := "giftcard"
	switch {
	case target != RefundTargetOriginal:
		// gift card
	case offline:
		providerSlug = OfflineProviderSlug
	default:
		providerSlug = o.PaymentProvider
	}

	var intent CreditMemoIntent
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		if err := r.LockOrderForPayment(ctx, o.ID); err != nil {
			return fmt.Errorf("lock order for payment: %w", err)
		}
		credited, err := r.CreditedByOrderLine(ctx, o.ID)
		if err != nil {
			return err
		}
		memo := CreditMemo{
			OrderID:               o.ID,
			ShippingRefundCents:   in.ShippingRefundCents,
			AdjustmentRefundCents: in.AdjustmentRefundCents,
			AdjustmentFeeCents:    in.AdjustmentFeeCents,
			RefundTarget:          target,
			Offline:               offline,
			Reason:                in.Reason,
			AdminID:               in.AdminID,
		}
		lines := make([]CreditMemoLine, 0, len(lineOrder))
		for _, lineID := range lineOrder {
			qty := reqByLine[lineID]
			if qty > orderedQty[lineID]-credited[lineID] {
				return apperr.New("CREDIT_MEMO_EXCEEDS_ORDERED", apperr.ClassConflict,
					"credit quantity exceeds what remains uncredited on a line")
			}
			unit := refundUnit[lineID]
			total := money.LineTotal(unit, qty)
			memo.LinesTotalCents += total
			lines = append(lines, CreditMemoLine{
				OrderLineID: lineID, Quantity: qty, UnitCents: unit, TotalCents: total,
			})
		}

		gross := memo.LinesTotalCents + memo.ShippingRefundCents + memo.AdjustmentRefundCents - memo.AdjustmentFeeCents
		if gross <= 0 {
			return apperr.New("CREDIT_MEMO_EMPTY", apperr.ClassValidation,
				"a credit memo must refund a positive amount")
		}
		totals, err := r.PaymentTotalsActiveByKind(ctx, o.ID)
		if err != nil {
			return err
		}
		amount := gross
		// Book the clamped shortfall as a fee so the memo's own arithmetic stays true
		if remaining := refundCeiling(target, totals, o.TotalAmount); amount > remaining {
			if remaining <= 0 {
				return apperr.New("CREDIT_MEMO_NOTHING_TO_REFUND", apperr.ClassConflict, nothingToRefundReason(totals, o))
			}
			memo.AdjustmentFeeCents += amount - remaining
			amount = remaining
		}
		memo.TotalCents = amount

		created, err := r.CreateCreditMemo(ctx, memo)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if _, err := r.AddCreditMemoLine(ctx, created.ID, l); err != nil {
				return err
			}
		}
		created.Lines = lines

		txn, err := r.InsertPaymentTransaction(ctx, PaymentTransaction{
			OrderID:        o.ID,
			Kind:           PaymentTxnRefund,
			AmountCents:    amount,
			Status:         PaymentTxnPending,
			ProviderSlug:   providerSlug,
			IdempotencyKey: "memo:" + created.ID.String(),
			Reason:         in.Reason,
		})
		if err != nil {
			return err
		}
		if err := r.LinkPaymentTransactionToMemo(ctx, txn.ID, created.ID); err != nil {
			return err
		}
		txn.CreditMemoID = &created.ID
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "credit_memo_issued",
			Message: fmt.Sprintf("Credit memo %s issued for %d cents.", created.Number, amount),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		intent = CreditMemoIntent{
			Txn: txn, Order: o, Memo: created, AmountCents: amount, Target: target, Offline: offline,
		}
		return nil
	})
	return intent, err
}

// Settles the reserved row and re-derives payment status - the memo is already durable, so a failed refund leaves a failed transaction rather than a phantom credit
func (s *Service) FinalizeCreditMemo(ctx context.Context, txnID uuid.UUID, ok bool, providerReference string) (Order, CreditMemo, error) {
	var updatedOrder Order
	var memo CreditMemo
	var moved bool
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		txn, m, err := finalizeTxn(ctx, r, txnID, ok, providerReference)
		if err != nil {
			return err
		}
		moved = m
		if txn.CreditMemoID == nil {
			return fmt.Errorf("transaction %s is not linked to a credit memo", txnID)
		}
		if memo, err = r.GetCreditMemo(ctx, *txn.CreditMemoID); err != nil {
			return err
		}
		o, err := r.GetOrderByID(ctx, txn.OrderID)
		if err != nil {
			return err
		}
		updatedOrder = o
		if !moved {
			return nil
		}
		if !ok {
			// The memo was written before the money moved - left standing it counted as credited, and the units could never be refunded again
			return r.MarkCreditMemoFailed(ctx, memo.ID)
		}
		totals, err := r.PaymentTotalsByKind(ctx, o.ID)
		if err != nil {
			return err
		}
		updatedOrder, err = r.SetPaymentState(ctx, o.ID, derivePaymentStatus(totals, o.AmountDue(), o.TotalAmount), o.PaymentReference)
		if err != nil {
			return err
		}
		return s.enqueueRefunded(ctx, sqlTx, r, updatedOrder, memo.ID)
	})
	if err != nil {
		return Order{}, CreditMemo{}, err
	}
	if s.bus != nil && ok && moved {
		s.bus.Publish(ctx, &OrderPaymentStatusChanged{
			OrderID:   updatedOrder.ID,
			ChannelID: updatedOrder.ChannelID,
			Status:    updatedOrder.PaymentStatus,
			Reference: updatedOrder.PaymentReference,
		})
		s.publishRefundedWithoutOutbox(ctx, updatedOrder, memo.ID)
	}
	return updatedOrder, memo, nil
}

// Writes the memo documenting an RMA's refund inside the caller's transaction - the unique index on return_id makes a replay a no-op
func creditMemoForReturn(ctx context.Context, r Repository, o Order, ret OrderReturn, txn PaymentTransaction) (CreditMemo, error) {
	if existing, found, err := r.GetCreditMemoByReturn(ctx, ret.ID); err != nil {
		return CreditMemo{}, err
	} else if found {
		return existing, nil
	}
	returnID := ret.ID
	memo := CreditMemo{
		OrderID:      o.ID,
		ReturnID:     &returnID,
		RefundTarget: ret.RefundTarget,
		Offline:      txn.ProviderSlug == OfflineProviderSlug,
		Reason:       fmt.Sprintf("Return %s", ret.RMANumber),
		AdminID:      ret.AdminID,
		TotalCents:   txn.AmountCents,
	}
	lines := make([]CreditMemoLine, 0, len(ret.Lines))
	for _, rl := range ret.Lines {
		total := money.LineTotal(rl.RefundUnitCents, rl.Quantity)
		memo.LinesTotalCents += total
		lines = append(lines, CreditMemoLine{
			OrderLineID: rl.OrderLineID, Quantity: rl.Quantity, UnitCents: rl.RefundUnitCents, TotalCents: total,
		})
	}
	// Clamped by what remained refundable - book the difference as a fee so the document still adds up
	if memo.LinesTotalCents > memo.TotalCents {
		memo.AdjustmentFeeCents = memo.LinesTotalCents - memo.TotalCents
	}
	created, err := r.CreateCreditMemo(ctx, memo)
	if err != nil {
		return CreditMemo{}, err
	}
	for _, l := range lines {
		if _, err := r.AddCreditMemoLine(ctx, created.ID, l); err != nil {
			return CreditMemo{}, err
		}
	}
	if err := r.LinkPaymentTransactionToMemo(ctx, txn.ID, created.ID); err != nil {
		return CreditMemo{}, err
	}
	created.Lines = lines
	return created, nil
}

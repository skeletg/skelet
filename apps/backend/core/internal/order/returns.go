package order

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// returnTransitions is the allowed forward moves of the return lifecycle: requested to approved or rejected, approved to received or rejected, received to refunded
// rejected and refunded are terminal, and anything not listed is refused, which makes a replayed transition a no-op
var returnTransitions = map[string]map[string]bool{
	ReturnRequested: {ReturnApproved: true, ReturnRejected: true},
	ReturnApproved:  {ReturnReceived: true, ReturnRejected: true},
	ReturnReceived:  {ReturnRefunded: true},
	ReturnRejected:  {},
	ReturnRefunded:  {},
}

func validReturnTransition(from, to string) bool {
	return returnTransitions[from][to]
}

// Re-derives each line's discounted per-unit basis, via the placement-tax allocator so the rounding matches what was charged
// Tax belongs in it only when the store priced EXCLUSIVE - inclusive prices already contain it, and orders carry no tax_inclusive column so it's derived from the total
func refundUnitByLine(o Order) map[uuid.UUID]int64 {
	return unitBasisByLine(o, true)
}

// The same basis WITHOUT tax - what loyalty and referral earned on, so reversals scale by goods returned rather than money
func merchandiseUnitByLine(o Order) map[uuid.UUID]int64 {
	return unitBasisByLine(o, false)
}

func unitBasisByLine(o Order, withTax bool) map[uuid.UUID]int64 {
	lineSubs := make([]int64, len(o.Lines))
	var subtotal int64
	for i, l := range o.Lines {
		lineSubs[i] = money.LineTotal(l.UnitPrice, l.Quantity)
		subtotal += lineSubs[i]
	}
	discounts := allocateLineDiscount(lineSubs, subtotal, o.DiscountAmount)
	var taxes []int64
	if withTax && taxAddedOnTop(o) {
		taxes = allocateLineDiscount(lineSubs, subtotal, max(o.TaxAmount-o.ShippingTaxAmount, 0))
	}
	out := make(map[uuid.UUID]int64, len(o.Lines))
	for i, l := range o.Lines {
		gross := lineSubs[i] - discounts[i]
		if taxes != nil {
			gross += taxes[i]
		}
		out[l.ID] = money.PerUnit(gross, l.Quantity)
	}
	return out
}

func taxAddedOnTop(o Order) bool {
	return o.TaxAmount > 0 && o.TotalAmount == o.SubtotalAmount-o.DiscountAmount+o.TaxAmount+o.ShippingAmount
}

// Shipping's tax rides along only where tax was added on top
func (o Order) ShippingRefundable() int64 {
	if taxAddedOnTop(o) {
		return o.ShippingAmount + o.ShippingTaxAmount
	}
	return o.ShippingAmount
}

// Cumulative share of the order's merchandise credited across EVERY memo, 0..1 - cumulative so subsystems revoke toward a target, making partial refunds add up and a replay a no-op
func refundedMerchandiseFraction(ctx context.Context, repo Repository, o Order) (float64, error) {
	orderMerch := o.SubtotalAmount - o.DiscountAmount
	if orderMerch <= 0 {
		return 0, nil
	}
	credited, err := repo.CreditedByOrderLine(ctx, o.ID)
	if err != nil {
		return 0, err
	}
	unit := merchandiseUnitByLine(o)
	var refunded int64
	for lineID, qty := range credited {
		refunded += money.LineTotal(unit[lineID], qty)
	}

	// A line-less memo refunded money without naming goods - attribute its non-shipping part in the order's own proportion, else a full flat refund revokes nothing
	memos, err := repo.ListCreditMemos(ctx, o.ID)
	if err != nil {
		return 0, err
	}
	if nonShipping := o.TotalAmount - o.ShippingRefundable(); nonShipping > 0 {
		for _, m := range memos {
			if len(m.Lines) > 0 {
				continue // exact, already counted through CreditedByOrderLine
			}
			attributable := m.TotalCents - m.ShippingRefundCents
			if attributable <= 0 {
				continue // shipping-only, no goods implied
			}
			refunded += attributable * orderMerch / nonShipping
		}
	}

	f := float64(refunded) / float64(orderMerch)
	if f > 1 {
		f = 1
	}
	return f, nil
}

// ReturnLineInput is one (order line, quantity) requested for return
type ReturnLineInput struct {
	OrderLineID uuid.UUID
	Quantity    float64
	Reason      string
}

// RequestReturnInput opens an RMA against shipped quantities of an order
const (
	ResolutionRefund   = "refund"
	ResolutionExchange = "exchange"
)

type RequestReturnInput struct {
	OrderID      uuid.UUID
	Lines        []ReturnLineInput
	Reason       string
	Restock      bool
	RefundTarget string // "original" | "gift_card"; empty → original
	// Resolution "refund" (default) or "exchange" - exchange opens a zero-priced replacement draft on receipt
	Resolution string
	AdminID    *uuid.UUID
}

// ReturnableByOrderLine returns the remaining returnable quantity (shipped - already returned) per line - an order
// with no fulfillment rows at all has nothing per-shipment to subtract, so its ordered quantities count as shipped
func (s *Service) ReturnableByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}
	return s.returnableForOrder(ctx, o)
}

// returnableForOrder is the order-in-hand variant, so RequestReturn reuses it without reloading
func (s *Service) returnableForOrder(ctx context.Context, o Order) (map[uuid.UUID]float64, error) {
	return s.returnableFor(ctx, s.repo, o)
}

// returnableFor takes the repo explicitly so the authoritative re-check can run on the transaction's repo, under the order lock
func (s *Service) returnableFor(ctx context.Context, repo Repository, o Order) (map[uuid.UUID]float64, error) {
	shipped, err := repo.ShippedByOrderLine(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	returned, err := repo.ReturnedByOrderLine(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	cnt, err := repo.CountFulfillments(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	legacyFulfilled := cnt == 0 && o.Status == OrderStatusFulfilled
	out := make(map[uuid.UUID]float64, len(o.Lines))
	for _, l := range o.Lines {
		shippedQty := shipped[l.ID]
		if legacyFulfilled {
			shippedQty = l.Quantity
		}
		if rem := shippedQty - returned[l.ID]; rem > 0 {
			out[l.ID] = rem
		} else {
			out[l.ID] = 0
		}
	}
	return out, nil
}

// RequestReturn opens a return for shipped quantities - validates the order has shipped, each line belongs to it, and the quantity is within the returnable remainder - Snapshots each line's discounted per-unit basis so the refund survives later edits, then writes the return + lines in one tx
func (s *Service) RequestReturn(ctx context.Context, in RequestReturnInput) (OrderReturn, error) {
	o, err := s.repo.GetOrderByID(ctx, in.OrderID)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("order not found")
	}
	if o.Status != OrderStatusFulfilled && o.Status != OrderStatusPartiallyFulfilled {
		return OrderReturn{}, apperr.New("RETURN_ORDER_NOT_SHIPPED", apperr.ClassConflict,
			"only orders with shipped items can be returned")
	}
	if len(in.Lines) == 0 {
		return OrderReturn{}, apperr.New("RETURN_EMPTY", apperr.ClassValidation, "a return must include at least one line")
	}
	target := in.RefundTarget
	if target == "" {
		target = RefundTargetOriginal
	}
	if target != RefundTargetOriginal && target != RefundTargetGiftCard {
		return OrderReturn{}, apperr.New("RETURN_INVALID_REFUND_TARGET", apperr.ClassValidation, "refund target must be 'original' or 'gift_card'")
	}
	resolution := in.Resolution
	if resolution == "" {
		resolution = ResolutionRefund
	}
	if resolution != ResolutionRefund && resolution != ResolutionExchange {
		return OrderReturn{}, apperr.New("RETURN_INVALID_RESOLUTION", apperr.ClassValidation, "resolution must be 'refund' or 'exchange'")
	}

	refundUnit := refundUnitByLine(o)
	orderedQty := make(map[uuid.UUID]float64, len(o.Lines))
	for _, l := range o.Lines {
		orderedQty[l.ID] = l.Quantity
	}

	returnable, err := s.returnableForOrder(ctx, o)
	if err != nil {
		return OrderReturn{}, err
	}

	// Validate + dedupe the requested lines, guarding the returnable remainder
	reqByLine := make(map[uuid.UUID]float64, len(in.Lines))
	reasonByLine := make(map[uuid.UUID]string, len(in.Lines))
	order := make([]uuid.UUID, 0, len(in.Lines))
	for _, rl := range in.Lines {
		if _, ok := orderedQty[rl.OrderLineID]; !ok {
			return OrderReturn{}, apperr.New("RETURN_UNKNOWN_LINE", apperr.ClassValidation, "return line does not belong to this order")
		}
		if rl.Quantity <= 0 {
			return OrderReturn{}, apperr.New("RETURN_INVALID_QTY", apperr.ClassValidation, "return quantity must be positive")
		}
		if _, seen := reqByLine[rl.OrderLineID]; !seen {
			order = append(order, rl.OrderLineID)
			reasonByLine[rl.OrderLineID] = rl.Reason
		}
		reqByLine[rl.OrderLineID] += rl.Quantity
		if reqByLine[rl.OrderLineID] > returnable[rl.OrderLineID] {
			return OrderReturn{}, apperr.New("RETURN_EXCEEDS_SHIPPED", apperr.ClassConflict,
				"return exceeds the returnable (shipped − already returned) quantity for a line")
		}
	}

	// Remote returns-policy gate - after the native guard, before any write, so a
	// slow call never holds the tx. fail_open allows, fail_closed refuses
	if s.remoteReturns != nil {
		policyLines := make([]RemoteReturnLine, 0, len(order))
		for _, lineID := range order {
			policyLines = append(policyLines, RemoteReturnLine{
				OrderLineID: lineID, Quantity: reqByLine[lineID], Reason: reasonByLine[lineID],
			})
		}
		if allowed, reason := s.remoteReturns.Allowed(ctx, RemoteReturnInput{
			OrderID: o.ID, ChannelID: o.ChannelID, Resolution: string(resolution), RefundTarget: string(target), Lines: policyLines,
		}); !allowed {
			return OrderReturn{}, apperr.New("RETURN_POLICY_DENIED", apperr.ClassConflict, reason)
		}
	}

	var out OrderReturn
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		// The guard above ran unlocked - fine for a good early error, but two admins opening returns on one line both passed it, so re-check under the lock
		if err := r.LockOrderForPayment(ctx, o.ID); err != nil {
			return fmt.Errorf("lock order: %w", err)
		}
		locked, err := s.returnableFor(ctx, r, o)
		if err != nil {
			return err
		}
		for lineID, qty := range reqByLine {
			if qty > locked[lineID] {
				return apperr.New("RETURN_EXCEEDS_SHIPPED", apperr.ClassConflict,
					"return exceeds the returnable (shipped − already returned) quantity for a line")
			}
		}
		ret, err := r.CreateReturn(ctx, OrderReturn{
			OrderID:      o.ID,
			Status:       ReturnRequested,
			Reason:       in.Reason,
			Restock:      in.Restock,
			RefundTarget: target,
			Resolution:   resolution,
			AdminID:      in.AdminID,
		})
		if err != nil {
			return err
		}
		for _, lineID := range order {
			if _, err := r.AddReturnLine(ctx, ret.ID, OrderReturnLine{
				OrderLineID:     lineID,
				Quantity:        reqByLine[lineID],
				Reason:          reasonByLine[lineID],
				RefundUnitCents: refundUnit[lineID],
			}); err != nil {
				return err
			}
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "return_requested",
			Message: fmt.Sprintf("Return %s requested.", ret.RMANumber),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		out, err = r.GetReturn(ctx, ret.ID)
		return err
	})
	if err == nil && s.bus != nil {
		s.bus.Publish(ctx, &OrderReturnRequested{OrderID: o.ID, ChannelID: o.ChannelID, ReturnID: out.ID})
	}
	return out, err
}

// transitionReturn applies a guarded status move + timeline event, shared by Approve/Reject
func (s *Service) transitionReturn(ctx context.Context, returnID uuid.UUID, to, eventType, message string) (OrderReturn, error) {
	ret, err := s.repo.GetReturn(ctx, returnID)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("return not found")
	}
	if !validReturnTransition(ret.Status, to) {
		return OrderReturn{}, apperr.New("RETURN_INVALID_TRANSITION", apperr.ClassConflict,
			fmt.Sprintf("cannot move return from %q to %q", ret.Status, to))
	}
	var out OrderReturn
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		out, err = r.UpdateReturnStatus(ctx, returnID, to)
		if err != nil {
			return err
		}
		_, err = r.AddOrderEvent(ctx, OrderEvent{OrderID: ret.OrderID, Type: eventType, Message: message})
		return err
	})
	return out, err
}

// ApproveReturn moves a requested return to approved
func (s *Service) ApproveReturn(ctx context.Context, returnID uuid.UUID) (OrderReturn, error) {
	ret, err := s.repo.GetReturn(ctx, returnID)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("return not found")
	}
	out, err := s.transitionReturn(ctx, returnID, ReturnApproved, "return_approved",
		fmt.Sprintf("Return %s approved.", ret.RMANumber))
	if err == nil {
		s.publishReturnEvent(ctx, ret.OrderID, returnID, ReturnApproved)
	}
	return out, err
}

// RejectReturn moves a requested/approved return to rejected (terminal)
func (s *Service) RejectReturn(ctx context.Context, returnID uuid.UUID) (OrderReturn, error) {
	ret, err := s.repo.GetReturn(ctx, returnID)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("return not found")
	}
	out, err := s.transitionReturn(ctx, returnID, ReturnRejected, "return_rejected",
		fmt.Sprintf("Return %s rejected.", ret.RMANumber))
	if err == nil {
		s.publishReturnEvent(ctx, ret.OrderID, returnID, ReturnRejected)
	}
	return out, err
}

// publishReturnEvent emits the approved/rejected RMA event (best-effort, timeline only)
func (s *Service) publishReturnEvent(ctx context.Context, orderID, returnID uuid.UUID, toStatus string) {
	if s.bus == nil {
		return
	}
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return
	}
	switch toStatus {
	case ReturnApproved:
		s.bus.Publish(ctx, &OrderReturnApproved{OrderID: orderID, ChannelID: o.ChannelID, ReturnID: returnID})
	case ReturnRejected:
		s.bus.Publish(ctx, &OrderReturnRejected{OrderID: orderID, ChannelID: o.ChannelID, ReturnID: returnID})
	}
}

// ReceiveReturn moves an approved return to received and, when restock-flagged,
// emits OrderReturnReceived so inventory adds the quantities back (post-commit)
func (s *Service) ReceiveReturn(ctx context.Context, returnID uuid.UUID) (OrderReturn, error) {
	ret, err := s.repo.GetReturn(ctx, returnID)
	if err != nil {
		return OrderReturn{}, fmt.Errorf("return not found")
	}
	if !validReturnTransition(ret.Status, ReturnReceived) {
		return OrderReturn{}, apperr.New("RETURN_INVALID_TRANSITION", apperr.ClassConflict,
			fmt.Sprintf("cannot move return from %q to %q", ret.Status, ReturnReceived))
	}
	o, err := s.repo.GetOrderByID(ctx, ret.OrderID)
	if err != nil {
		return OrderReturn{}, err
	}
	restockLines := make([]ReturnLineQty, 0, len(ret.Lines))
	if ret.Restock {
		for _, l := range ret.Lines {
			restockLines = append(restockLines, ReturnLineQty{OrderLineID: l.OrderLineID, Quantity: l.Quantity})
		}
	}

	var out OrderReturn
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		out, err = r.UpdateReturnStatus(ctx, returnID, ReturnReceived)
		if err != nil {
			return err
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "return_received",
			Message: fmt.Sprintf("Return %s received%s.", ret.RMANumber, restockNote(ret.Restock)),
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		if ret.Restock && s.outbox != nil {
			if err := s.outbox.Enqueue(ctx, sqlTx, &OrderReturnReceived{
				OrderID: o.ID, ChannelID: o.ChannelID, ReturnID: returnID, Lines: restockLines,
			}); err != nil {
				return fmt.Errorf("enqueue order.return_received: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return OrderReturn{}, err
	}
	// Without the outbox, fall back to best-effort in-process dispatch
	if ret.Restock && s.bus != nil && s.outbox == nil {
		s.bus.Publish(ctx, &OrderReturnReceived{OrderID: o.ID, ChannelID: o.ChannelID, ReturnID: returnID, Lines: restockLines})
	}
	// Exchange - open the zero-priced replacement draft now the goods are back Post-commit + best-effort, a failure leaves
	// a loud timeline event to retry manually rather than rolling back the receipt of physical goods
	if ret.Resolution == ResolutionExchange && ret.ExchangeOrderID == nil {
		if exch, xerr := s.createExchangeDraft(ctx, o, ret); xerr != nil {
			log.Printf("[order] exchange draft for return %s failed: %v", ret.RMANumber, xerr)
			_, _ = s.repo.AddOrderEvent(ctx, OrderEvent{
				OrderID: o.ID,
				Type:    "exchange_draft_failed",
				Message: fmt.Sprintf("Exchange draft for return %s could not be created: %v. Create it manually.", ret.RMANumber, xerr),
			})
		} else {
			out, _ = s.repo.GetReturn(ctx, returnID)
			_ = exch
		}
	}
	return out, nil
}

// createExchangeDraft opens the replacement draft - returned lines at zero price (already paid), the original
// addresses, linked back onto the return - Admins can swap variants or add price differences before placing
func (s *Service) createExchangeDraft(ctx context.Context, o Order, ret OrderReturn) (Order, error) {
	lineByID := make(map[uuid.UUID]OrderLine, len(o.Lines))
	for _, l := range o.Lines {
		lineByID[l.ID] = l
	}
	zero := int64(0)
	lines := make([]CreateDraftLine, 0, len(ret.Lines))
	for _, rl := range ret.Lines {
		src, ok := lineByID[rl.OrderLineID]
		if !ok {
			continue
		}
		lines = append(lines, CreateDraftLine{ProductSku: src.ProductSku, Quantity: rl.Quantity, UnitPriceCents: &zero})
	}
	if len(lines) == 0 {
		return Order{}, fmt.Errorf("no exchangeable lines resolved")
	}
	draft, err := s.CreateDraftOrder(ctx, CreateDraftOrderInput{
		ChannelID:       o.ChannelID,
		CustomerID:      o.CustomerID,
		Email:           o.Email,
		BillingAddress:  o.BillingAddress,
		ShippingAddress: o.ShippingAddress,
		Lines:           lines,
		CustomerNote:    fmt.Sprintf("Exchange for RMA %s (order %s)", ret.RMANumber, o.Number),
		// The original picks already shipped - the replacement carries them via the note
		SkipRequiredOptionCheck: true,
	})
	if err != nil {
		return Order{}, err
	}
	if _, err := s.repo.SetReturnExchangeOrder(ctx, ret.ID, draft.ID); err != nil {
		return draft, err
	}
	_, _ = s.repo.AddOrderEvent(ctx, OrderEvent{
		OrderID: o.ID,
		Type:    "exchange_draft_created",
		Message: fmt.Sprintf("Exchange draft %s opened for return %s.", draft.Number, ret.RMANumber),
	})
	return draft, nil
}

func restockNote(restock bool) string {
	if restock {
		return " (restocking)"
	}
	return ""
}

// ListReturns returns the RMAs recorded against an order, lines hydrated
func (s *Service) ListReturns(ctx context.Context, orderID uuid.UUID) ([]OrderReturn, error) {
	return s.repo.ListReturns(ctx, orderID)
}

func (s *Service) GetReturn(ctx context.Context, returnID uuid.UUID) (OrderReturn, error) {
	return s.repo.GetReturn(ctx, returnID)
}

// ReturnRefundIntent is the prepared refund - a pending ledger row plus the
// context to drive the gateway refund or gift-card issuance
type ReturnRefundIntent struct {
	Txn         PaymentTransaction
	Order       Order
	Return      OrderReturn
	AmountCents int64
	Target      string
	// Offline means the money never crossed a gateway (cash on delivery, bank transfer, purchase order, pickup), so the caller records the
	// refund against the ledger without a provider call - the operator hands the money back the same way they took it
	Offline bool
}

// "Already refunded in full" is wrong for an order that was never paid - a fulfilled COD order with an RMA hits the
// same zero ceiling, and telling the operator it was refunded sends them hunting for a refund that never happened
func nothingToRefundReason(totals PaymentTotals, o Order) string {
	if totals.Captured == 0 && o.GiftCardAmount == 0 && o.StoreCreditAmount == 0 {
		return "nothing has been collected on this order yet, so there is nothing to refund - record the payment first"
	}
	return "this order has already been refunded in full"
}

// The most still refundable for a target - the gateway ceiling is right for money a gateway holds and wrong for everything else
// A refund issued as a NEW gift card moves store value, so clamping it to the gateway's capture made gift-card and store-credit orders un-refundable
func refundCeiling(target string, totals PaymentTotals, totalTendered int64) int64 {
	if target == RefundTargetGiftCard {
		return totalTendered - totals.Refunded
	}
	return totals.Captured - totals.Refunded
}

// RecordReturnRefundIntent validates a received return and reserves its refund - gross from the per-line snapshots, clamped to the captured remaining, written as a pending row under a deterministic key ("return:<id>") so a retry returns the existing row instead of double-refunding - Drive the action next, then FinalizeReturnRefund
func (s *Service) RecordReturnRefundIntent(ctx context.Context, returnID uuid.UUID, reason string) (ReturnRefundIntent, error) {
	ret, err := s.repo.GetReturn(ctx, returnID)
	if err != nil {
		return ReturnRefundIntent{}, fmt.Errorf("return not found")
	}
	if ret.Status != ReturnReceived {
		return ReturnRefundIntent{}, apperr.New("RETURN_NOT_RECEIVED", apperr.ClassConflict,
			"a return must be received before it can be refunded")
	}
	if ret.Resolution == ResolutionExchange {
		return ReturnRefundIntent{}, apperr.New("RETURN_IS_EXCHANGE", apperr.ClassConflict,
			"an exchange return is resolved by its replacement order, not a refund")
	}
	o, err := s.repo.GetOrderByID(ctx, ret.OrderID)
	if err != nil {
		return ReturnRefundIntent{}, err
	}
	var gross int64
	for _, l := range ret.Lines {
		gross += money.LineTotal(l.RefundUnitCents, l.Quantity)
	}
	// An order with no gateway handle was settled outside one - the refund is recorded, not requested
	offline := ret.RefundTarget == RefundTargetOriginal && (o.PaymentProvider == "" || o.PaymentReference == "")
	providerSlug := "giftcard"
	switch {
	case ret.RefundTarget != RefundTargetOriginal:
		// gift card
	case offline:
		providerSlug = OfflineProviderSlug
	default:
		providerSlug = o.PaymentProvider
	}

	key := "return:" + returnID.String()
	var intent ReturnRefundIntent
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		// Lock before reading the totals - the ordinary refund path does the same, and without it two RMAs on one order both read the same remaining and jointly over-refund it
		if err := r.LockOrderForPayment(ctx, o.ID); err != nil {
			return fmt.Errorf("lock order for payment: %w", err)
		}
		if existing, found, err := r.FindPaymentTransactionByKey(ctx, key); err != nil {
			return err
		} else if found {
			intent = ReturnRefundIntent{Txn: existing, Order: o, Return: ret, AmountCents: existing.AmountCents, Target: ret.RefundTarget, Offline: offline}
			return nil
		}
		// Active, not succeeded-only: a pending refund on a sibling RMA is money already in flight
		totals, err := r.PaymentTotalsActiveByKind(ctx, o.ID)
		if err != nil {
			return err
		}
		amount := gross
		if remaining := refundCeiling(ret.RefundTarget, totals, o.TotalAmount); amount > remaining {
			amount = remaining
		}
		if amount <= 0 {
			return apperr.New("RETURN_NOTHING_TO_REFUND", apperr.ClassConflict, nothingToRefundReason(totals, o))
		}
		txn, err := r.InsertPaymentTransaction(ctx, PaymentTransaction{
			OrderID:        o.ID,
			Kind:           PaymentTxnRefund,
			AmountCents:    amount,
			Status:         PaymentTxnPending,
			ProviderSlug:   providerSlug,
			IdempotencyKey: key,
			Reason:         reason,
			ReturnID:       &returnID,
		})
		if err != nil {
			return err
		}
		intent = ReturnRefundIntent{Txn: txn, Order: o, Return: ret, AmountCents: amount, Target: ret.RefundTarget, Offline: offline}
		return nil
	})
	return intent, err
}

// FinalizeReturnRefund marks the pending refund row succeeded/failed and, on success, stamps the return refunded
// (recording an issued gift card) and re-derives the order's payment status - all in one tx
func (s *Service) FinalizeReturnRefund(ctx context.Context, txnID, returnID uuid.UUID, ok bool, providerReference string, issuedGiftCardID *uuid.UUID) (Order, OrderReturn, error) {
	var updatedOrder Order
	var updatedReturn OrderReturn
	var refundedMemoID uuid.UUID
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		status := PaymentTxnFailed
		if ok {
			status = PaymentTxnSucceeded
		}
		txn, err := r.MarkPaymentTransaction(ctx, txnID, status, providerReference)
		if err != nil {
			return err
		}
		o, err := r.GetOrderByID(ctx, txn.OrderID)
		if err != nil {
			return err
		}
		if !ok {
			updatedOrder = o
			updatedReturn, err = r.GetReturn(ctx, returnID)
			return err
		}
		updatedReturn, err = r.MarkReturnRefunded(ctx, returnID, txn.AmountCents, issuedGiftCardID)
		if err != nil {
			return err
		}
		// Written in the same tx as the money so the two can't disagree
		memo, err := creditMemoForReturn(ctx, r, o, updatedReturn, txn)
		if err != nil {
			return fmt.Errorf("write credit memo: %w", err)
		}
		updatedReturn.CreditMemoNumber = memo.Number
		refundedMemoID = memo.ID
		totals, err := r.PaymentTotalsByKind(ctx, o.ID)
		if err != nil {
			return err
		}
		updatedOrder, err = r.SetPaymentState(ctx, o.ID, derivePaymentStatus(totals, o.AmountDue(), o.TotalAmount), o.PaymentReference)
		if err != nil {
			return err
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: o.ID,
			Type:    "return_refunded",
			Message: fmt.Sprintf("Return %s refunded %d cents to %s (credit memo %s).",
				updatedReturn.RMANumber, txn.AmountCents, updatedReturn.RefundTarget, memo.Number),
		}); err != nil {
			return err
		}
		return s.enqueueRefunded(ctx, sqlTx, r, updatedOrder, memo.ID)
	})
	if err != nil {
		return Order{}, OrderReturn{}, err
	}
	if s.bus != nil && ok {
		s.bus.Publish(ctx, &OrderPaymentStatusChanged{
			OrderID:   updatedOrder.ID,
			ChannelID: updatedOrder.ChannelID,
			Status:    updatedOrder.PaymentStatus,
			Reference: updatedOrder.PaymentReference,
		})
		s.publishRefundedWithoutOutbox(ctx, updatedOrder, refundedMemoID)
	}
	return updatedOrder, updatedReturn, nil
}

// Rides the refund's own transaction, so a crash between the money committing and the compensation running can't lose the reversals
func (s *Service) enqueueRefunded(ctx context.Context, sqlTx *sql.Tx, repo Repository, o Order, memoID uuid.UUID) error {
	if s.outbox == nil {
		return nil // no durable queue wired - publishRefundedWithoutOutbox covers it post-commit
	}
	evt, err := s.buildRefunded(ctx, repo, o, memoID)
	if err != nil {
		return err
	}
	if err := s.outbox.Enqueue(ctx, sqlTx, evt); err != nil {
		return fmt.Errorf("enqueue order.refunded: %w", err)
	}
	return nil
}

// The best-effort fallback when no outbox is wired
func (s *Service) publishRefundedWithoutOutbox(ctx context.Context, o Order, memoID uuid.UUID) {
	if s.outbox != nil || s.bus == nil || memoID == uuid.Nil {
		return
	}
	evt, err := s.buildRefunded(ctx, s.repo, o, memoID)
	if err != nil {
		log.Printf("[order] build order.refunded for %s: %v", o.ID, err)
		return
	}
	s.bus.Publish(ctx, evt)
}

func (s *Service) buildRefunded(ctx context.Context, repo Repository, o Order, memoID uuid.UUID) (*OrderRefunded, error) {
	fraction, err := refundedMerchandiseFraction(ctx, repo, o)
	if err != nil {
		return nil, err
	}
	// From the ledger, not o.PaymentStatus - the webhook path builds this pre-recompute, so the persisted label would read a full refund as partial and skip the referral reversal
	totals, err := repo.PaymentTotalsByKind(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	return &OrderRefunded{
		OrderID:             o.ID,
		ChannelID:           o.ChannelID,
		CreditMemoID:        memoID,
		MerchandiseFraction: fraction,
		FullyRefunded:       derivePaymentStatus(totals, o.AmountDue(), o.TotalAmount) == PaymentRefunded,
	}, nil
}

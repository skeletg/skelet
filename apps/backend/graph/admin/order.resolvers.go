package admin

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/apperr"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/documents"
	"github.com/skeletg/skelet/internal/giftcard"
	"github.com/skeletg/skelet/internal/order"
	"github.com/skeletg/skelet/internal/payment"
	"github.com/skeletg/skelet/internal/ptr"
)

func (r *mutationResolver) CreateDraftOrder(ctx context.Context, input model.CreateDraftOrderInput) (*model.Order, error) {
	channelID, err := parseID(input.ChannelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", channelID, nil); err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.CreateDraftOrder(ctx, toCreateDraftOrderInput(input, channelID))
	if err != nil {
		return nil, err
	}
	return toOrderModel(o), nil
}

func (r *mutationResolver) PlaceDraftOrder(ctx context.Context, id string) (*model.Order, error) {
	orderID, err := parseID(id, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	placed, err := r.OrderSvc.PlaceDraftOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	return toOrderModel(placed), nil
}

func (r *mutationResolver) CancelOrder(ctx context.Context, id string) (*model.Order, error) {
	// legal from any non-terminal status - the transition emits the inventory release and cancellation-email events
	return r.adminTransitionOrderStatus(ctx, id, "cancelled")
}

func (r *mutationResolver) ConfirmOrder(ctx context.Context, id string) (*model.Order, error) {
	return r.adminTransitionOrderStatus(ctx, id, "confirmed")
}

func (r *mutationResolver) MarkProcessing(ctx context.Context, id string) (*model.Order, error) {
	return r.adminTransitionOrderStatus(ctx, id, "processing")
}

func (r *mutationResolver) HoldOrder(ctx context.Context, id string, reason *string) (*model.Order, error) {
	orderID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("order id", nil)
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.HoldOrder(ctx, orderID, ptr.Deref(reason))
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

func (r *mutationResolver) ReleaseOrder(ctx context.Context, id string) (*model.Order, error) {
	orderID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("order id", nil)
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.ReleaseOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

// Records a real fulfillment covering the remainder rather than only flipping the status, so inventory decrements and the first shipment captures payment - a sync OrderFulfilling subscriber can still override the carrier and tracking
func (r *mutationResolver) FulfillOrder(ctx context.Context, id string, carrier *string, trackingNumber *string, insuredValueCents *int32) (*model.Order, error) {
	orderID, err := parseID(id, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	c := ""
	if carrier != nil {
		c = *carrier
	}
	t := ""
	if trackingNumber != nil {
		t = *trackingNumber
	}
	var insured int64
	if insuredValueCents != nil {
		insured = int64(*insuredValueCents)
	}
	updated, err := r.OrderSvc.FulfillRemaining(ctx, orderID, c, t, insured)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

// Pre-fulfillment only - the service recomputes money, re-reserves stock in-tx (a shortage aborts) and signals payment drift
func (r *mutationResolver) EditOrder(ctx context.Context, input model.EditOrderInput) (*model.Order, error) {
	orderID, err := parseID(input.OrderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID) // looked up first so the channel-scoped gate below runs before any write
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	in := order.EditOrderInput{OrderID: orderID}
	for _, u := range input.UpdateLines {
		lineID, err := parseID(u.OrderLineID, "order line")
		if err != nil {
			return nil, err
		}
		in.UpdateQty = append(in.UpdateQty, order.EditLineQty{OrderLineID: lineID, Quantity: u.Quantity})
	}
	for _, id := range input.RemoveLines {
		lineID, err := parseID(id, "order line")
		if err != nil {
			return nil, err
		}
		in.RemoveLine = append(in.RemoveLine, lineID)
	}
	for _, a := range input.AddLines {
		in.AddLine = append(in.AddLine, order.EditAddLine{ProductSku: a.ProductSku, Quantity: a.Quantity})
	}
	updated, err := r.OrderSvc.EditOrder(ctx, in)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

func (r *mutationResolver) EditOrderAddress(ctx context.Context, input model.EditOrderAddressInput) (*model.Order, error) {
	orderID, err := parseID(input.OrderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	in := order.EditOrderAddressInput{OrderID: orderID}
	if input.ShippingAddress != nil {
		a := toOrderAddressInput(input.ShippingAddress)
		in.Shipping = &a
	}
	if input.BillingAddress != nil {
		a := toOrderAddressInput(input.BillingAddress)
		in.Billing = &a
	}
	updated, err := r.OrderSvc.EditOrderAddress(ctx, in)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

// Partial shipment - the service caps each line at the unshipped remainder, derives partially_fulfilled vs fulfilled, and emits the events driving inventory decrement plus first-shipment capture
func (r *mutationResolver) CreateFulfillment(ctx context.Context, input model.CreateFulfillmentInput) (*model.Order, error) {
	orderID, err := parseID(input.OrderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID) // looked up first so the channel-scoped gate below runs before any write
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	lines := make([]order.FulfillmentLineInput, 0, len(input.Lines))
	for _, l := range input.Lines {
		lineID, err := parseID(l.OrderLineID, "order line")
		if err != nil {
			return nil, err
		}
		lines = append(lines, order.FulfillmentLineInput{OrderLineID: lineID, Quantity: l.Quantity})
	}
	in := order.CreateFulfillmentInput{OrderID: orderID, Lines: lines}
	if input.Carrier != nil {
		in.Carrier = *input.Carrier
	}
	if input.TrackingNumber != nil {
		in.TrackingNumber = *input.TrackingNumber
	}
	if input.InsuredValueCents != nil {
		in.InsuredValueCents = int64(*input.InsuredValueCents)
	}
	updated, err := r.OrderSvc.CreateFulfillment(ctx, in)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

func (r *mutationResolver) GenerateShippingLabel(ctx context.Context, fulfillmentID string) (*model.ShippingLabel, error) {
	fID, err := parseID(fulfillmentID, "fulfillment")
	if err != nil {
		return nil, err
	}
	f, err := r.OrderSvc.GetFulfillment(ctx, fID)
	if err != nil {
		return nil, errNotFound("fulfillment")
	}
	o, err := r.OrderSvc.GetByID(ctx, f.OrderID) // loaded first so the channel-scoped gate below runs before any carrier call
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	label, err := r.OrderSvc.GenerateLabel(ctx, fID)
	if err != nil {
		return nil, err
	}
	return toShippingLabelModel(label), nil
}

// Fulfillment auto-captures - this is the manual, early or partial path, full amount when `amount` is omitted
func (r *mutationResolver) CapturePayment(ctx context.Context, orderID string, amount *int32) (*model.Order, error) {
	o, err := r.orderForPaymentAction(ctx, orderID, "orders.update")
	if err != nil {
		return nil, err
	}
	if o.PaymentProvider == "" || o.PaymentReference == "" {
		return nil, fmt.Errorf("order has no authorized payment to capture")
	}
	amt := o.AmountDue()
	if amount != nil {
		amt = int64(*amount)
	}
	txn, err := r.OrderSvc.RecordCaptureIntent(ctx, o.ID, amt, o.PaymentProvider, "") // ledger row first - it guards over-capture and mints the idempotency key the gateway call carries
	if err != nil {
		return nil, err
	}
	res, capErr := r.PaymentSvc.Capture(ctx, o.PaymentProvider, payment.CaptureRequest{
		OrderID:           o.ID,
		ChannelID:         o.ChannelID,
		ProviderReference: o.PaymentReference,
		AmountCents:       amt,
		IdempotencyKey:    txn.IdempotencyKey,
	})
	ref := ""
	if capErr == nil {
		ref = res.ProviderReference
	}
	updated, finErr := r.OrderSvc.FinalizePaymentTransaction(ctx, txn.ID, capErr == nil, ref)
	if capErr != nil {
		return nil, capErr
	}
	if finErr != nil {
		return nil, finErr
	}
	return toOrderModel(updated), nil
}

// Full when amount is omitted, partial when supplied - records a line-less memo like every other refund path, so the money carries a document and the reward reversals fire
// No lines means the clawback is attributed by money share rather than exactly - issueCreditMemo is the precise tool, this is the quick one
func (r *mutationResolver) RefundPayment(ctx context.Context, orderID string, amount *int32, reason *string) (*model.Order, error) {
	o, err := r.orderForPaymentAction(ctx, orderID, "orders.refund")
	if err != nil {
		return nil, err
	}
	amt := o.AmountDue()
	if amount != nil {
		amt = int64(*amount)
	}
	rsn := ""
	if reason != nil {
		rsn = *reason
	}
	if _, err := r.runCreditMemoRefund(ctx, order.IssueCreditMemoInput{
		OrderID:               o.ID,
		AdjustmentRefundCents: amt,
		Reason:                rsn,
		AdminID:               creditMemoAdminID(ctx),
	}); err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.GetByID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

func (r *mutationResolver) VoidPayment(ctx context.Context, orderID string) (*model.Order, error) {
	o, err := r.orderForPaymentAction(ctx, orderID, "orders.refund")
	if err != nil {
		return nil, err
	}
	if o.PaymentProvider == "" || o.PaymentReference == "" {
		return nil, fmt.Errorf("order has no authorization to void")
	}
	// LOCAL state first - Braintree happily voids a submitted-for-settlement transaction, so calling it first reversed the money while paid -> voided was rejected by the transition table, leaving the order reading "paid" behind a successful mutation
	ledger, err := r.OrderSvc.PaymentLedger(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	var authorized, captured int64
	for _, t := range ledger {
		if t.Status != order.PaymentTxnSucceeded {
			continue
		}
		switch t.Kind {
		case order.PaymentTxnAuthorize:
			authorized += t.AmountCents
		case order.PaymentTxnCapture:
			captured += t.AmountCents
		}
	}
	if captured > 0 {
		return nil, apperr.New("PAYMENT_ALREADY_CAPTURED", apperr.ClassConflict,
			"this payment is already captured - refund it instead of voiding")
	}
	if _, err := r.PaymentSvc.Void(ctx, o.PaymentProvider, payment.VoidRequest{
		OrderID:           o.ID,
		ChannelID:         o.ChannelID,
		ProviderReference: o.PaymentReference,
	}); err != nil {
		return nil, err
	}
	// Record the void so the ledger explains the released hold and derivePaymentStatus's voided branch stops being dead code
	if authorized > 0 {
		if _, err := r.OrderSvc.RecordVoid(ctx, o.ID, authorized, o.PaymentProvider, o.PaymentReference); err != nil {
			return nil, err
		}
	}
	updated, err := r.OrderSvc.SettlePayment(ctx, o.ID, order.PaymentVoided, o.PaymentReference)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

// Gated by orders.update rather than orders.refund - this brings money IN
func (r *mutationResolver) RecordOfflinePayment(ctx context.Context, input model.RecordOfflinePaymentInput) (*model.Order, error) {
	o, err := r.orderForPaymentAction(ctx, input.OrderID, "orders.update")
	if err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.RecordOfflinePayment(ctx, order.RecordOfflinePaymentInput{
		OrderID:     o.ID,
		AmountCents: int64(int32Deref(input.Amount)),
		Method:      ptr.Deref(input.Method),
		Reference:   ptr.Deref(input.Reference),
		ReceivedAt:  input.ReceivedAt,
		Note:        ptr.Deref(input.Note),
	})
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

// The service caps each line at the returnable remainder (shipped minus already returned) and snapshots the discounted per-unit refund basis
func (r *mutationResolver) RequestReturn(ctx context.Context, input model.RequestReturnInput) (*model.OrderReturn, error) {
	orderID, err := parseID(input.OrderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID) // looked up first so the channel-scoped gate below runs before any write
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	lines := make([]order.ReturnLineInput, 0, len(input.Lines))
	for _, l := range input.Lines {
		lineID, err := parseID(l.OrderLineID, "order line")
		if err != nil {
			return nil, err
		}
		li := order.ReturnLineInput{OrderLineID: lineID, Quantity: l.Quantity}
		if l.Reason != nil {
			li.Reason = *l.Reason
		}
		lines = append(lines, li)
	}
	in := order.RequestReturnInput{OrderID: orderID, Lines: lines, Restock: true}
	if input.Reason != nil {
		in.Reason = *input.Reason
	}
	if input.Restock != nil {
		in.Restock = *input.Restock
	}
	if input.RefundTarget != nil {
		in.RefundTarget = *input.RefundTarget
	}
	if input.Resolution != nil {
		in.Resolution = *input.Resolution
	}
	if claims, ok := auth.AdminFromContext(ctx); ok {
		in.AdminID = &claims.AdminID
	}
	ret, err := r.OrderSvc.RequestReturn(ctx, in)
	if err != nil {
		return nil, err
	}
	return toOrderReturnModel(ret), nil
}

func (r *mutationResolver) ApproveReturn(ctx context.Context, id string) (*model.OrderReturn, error) {
	ret, _, err := r.returnForAction(ctx, id, "orders.update")
	if err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.ApproveReturn(ctx, ret.ID)
	if err != nil {
		return nil, err
	}
	return toOrderReturnModel(updated), nil
}

func (r *mutationResolver) RejectReturn(ctx context.Context, id string) (*model.OrderReturn, error) {
	ret, _, err := r.returnForAction(ctx, id, "orders.update")
	if err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.RejectReturn(ctx, ret.ID)
	if err != nil {
		return nil, err
	}
	return toOrderReturnModel(updated), nil
}

func (r *mutationResolver) ReceiveReturn(ctx context.Context, id string) (*model.OrderReturn, error) {
	// restock, when flagged, runs async off the emitted OrderReturnReceived event
	ret, _, err := r.returnForAction(ctx, id, "orders.update")
	if err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.ReceiveReturn(ctx, ret.ID)
	if err != nil {
		return nil, err
	}
	return toOrderReturnModel(updated), nil
}

// Refund goes to the original tender or a newly-issued gift card - reserved in the ledger first under a deterministic key so a retry can't double-refund, then the money moves, then return and order are finalized from the ledger
func (r *mutationResolver) IssueReturnRefund(ctx context.Context, id string, reason *string) (*model.OrderReturn, error) {
	ret, _, err := r.returnForAction(ctx, id, "orders.refund")
	if err != nil {
		return nil, err
	}
	rsn := ""
	if reason != nil {
		rsn = *reason
	}
	intent, err := r.OrderSvc.RecordReturnRefundIntent(ctx, ret.ID, rsn)
	if err != nil {
		return nil, err
	}
	if intent.Txn.Status == order.PaymentTxnSucceeded { // replay - the refund already succeeded under this key, so don't move money again
		updated, err := r.OrderSvc.GetReturn(ctx, ret.ID)
		if err != nil {
			return nil, err
		}
		return toOrderReturnModel(updated), nil
	}

	var providerRef string
	var giftCardID *uuid.UUID
	var actionErr error
	switch {
	case intent.Target == order.RefundTargetGiftCard:
		card, gcErr := r.GiftcardSvc.Issue(ctx, giftcard.IssueInput{
			CurrencyCode:  intent.Order.CurrencyCode,
			Amount:        intent.AmountCents,
			IssuedOrderID: &intent.Order.ID,
			CustomerID:    intent.Order.CustomerID,
			Message:       fmt.Sprintf("Refund for return %s", intent.Return.RMANumber),
			IssueRef:      "return:" + intent.Return.ID.String(),
		})
		if gcErr == nil {
			providerRef = card.Code
			id := card.ID
			giftCardID = &id
		}
		actionErr = gcErr
	case intent.Offline: // COD, bank transfer, PO, pickup - no gateway to call, so the ledger row IS the refund and the operator hands the money back the way they took it
		providerRef = intent.Order.PaymentReference
		if providerRef == "" {
			providerRef = "manual"
		}
	default: // original tender, through the gateway that holds the money
		res, refErr := r.PaymentSvc.Refund(ctx, intent.Order.PaymentProvider, payment.RefundRequest{
			OrderID:           intent.Order.ID,
			ChannelID:         intent.Order.ChannelID,
			ProviderReference: intent.Order.PaymentReference,
			AmountCents:       intent.AmountCents,
			Reason:            rsn,
			IdempotencyKey:    intent.Txn.IdempotencyKey,
		})
		if refErr == nil {
			providerRef = res.ProviderReference
		}
		actionErr = refErr
	}

	_, updatedReturn, finErr := r.OrderSvc.FinalizeReturnRefund(ctx, intent.Txn.ID, ret.ID, actionErr == nil, providerRef, giftCardID)
	if actionErr != nil {
		return nil, actionErr
	}
	if finErr != nil {
		return nil, finErr
	}
	return toOrderReturnModel(updatedReturn), nil
}

// Refunds with no RMA behind it, through the same money paths as a return - the order's gateway, a new gift card, or recorded without a provider call when the order was settled outside one
func (r *mutationResolver) IssueCreditMemo(ctx context.Context, input model.IssueCreditMemoInput) (*model.CreditMemo, error) {
	orderID, err := parseID(input.OrderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.refund", o.ChannelID, nil); err != nil {
		return nil, err
	}
	in := order.IssueCreditMemoInput{
		OrderID:               orderID,
		ShippingRefundCents:   int64(int32Deref(input.ShippingRefund)),
		AdjustmentRefundCents: int64(int32Deref(input.AdjustmentRefund)),
		AdjustmentFeeCents:    int64(int32Deref(input.AdjustmentFee)),
		RefundTarget:          ptr.Deref(input.RefundTarget),
		Reason:                ptr.Deref(input.Reason),
	}
	in.AdminID = creditMemoAdminID(ctx)
	for _, l := range input.Lines {
		lineID, perr := parseID(l.OrderLineID, "order line")
		if perr != nil {
			return nil, perr
		}
		in.Lines = append(in.Lines, order.CreditMemoLineInput{OrderLineID: lineID, Quantity: l.Quantity})
	}

	memo, err := r.runCreditMemoRefund(ctx, in)
	if err != nil {
		return nil, err
	}
	return toCreditMemoModel(memo, o), nil
}

// Best-effort - returns "" and never errors when there's no tracking number, no recorded carrier, or the carrier can't track, and the UI just shows the bare number
func (r *orderResolver) TrackingURL(ctx context.Context, obj *model.Order) (string, error) {
	if obj.TrackingNumber == "" {
		return "", nil
	}
	orderID, err := parseID(obj.ID, "order")
	if err != nil {
		return "", nil
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil || o.ShippingCarrierSlug == nil || *o.ShippingCarrierSlug == "" {
		return "", nil
	}
	res, err := r.ShippingSvc.Track(ctx, *o.ShippingCarrierSlug, obj.TrackingNumber)
	if err != nil {
		return "", nil
	}
	return res.URL, nil
}

// Ungated on purpose - the Order/Orders resolver that loaded obj already channel-permission-checked the row, and a field resolver means only the detail page pays the lookup
func (r *orderResolver) Fulfillments(ctx context.Context, obj *model.Order) ([]*model.Fulfillment, error) {
	orderID, err := parseID(obj.ID, "order")
	if err != nil {
		return nil, err
	}
	fs, err := r.OrderSvc.ListFulfillments(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Fulfillment, len(fs))
	for i, f := range fs {
		out[i] = toFulfillmentModel(f)
	}
	return out, nil
}

func (r *orderResolver) PaymentTransactions(ctx context.Context, obj *model.Order) ([]*model.PaymentTransaction, error) {
	orderID, err := parseID(obj.ID, "order")
	if err != nil {
		return nil, err
	}
	txns, err := r.OrderSvc.PaymentLedger(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.PaymentTransaction, len(txns))
	for i, t := range txns {
		out[i] = toPaymentTransactionModel(t)
	}
	return out, nil
}

func (r *orderResolver) Returns(ctx context.Context, obj *model.Order) ([]*model.OrderReturn, error) {
	orderID, err := parseID(obj.ID, "order")
	if err != nil {
		return nil, err
	}
	rets, err := r.OrderSvc.ListReturns(ctx, orderID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.OrderReturn, len(rets))
	for i, ret := range rets {
		out[i] = toOrderReturnModel(ret)
	}
	return out, nil
}

// The order carries only an aggregate discountAmount, so this is the only answer to
// why it was discounted - the parent Order resolver already gated orders.view
func (r *orderResolver) AppliedPromotions(ctx context.Context, obj *model.Order) ([]*model.AppliedPromotion, error) {
	if r.PromotionSvc == nil {
		return []*model.AppliedPromotion{}, nil
	}
	oid, err := parseID(obj.ID, "order")
	if err != nil {
		return nil, err
	}
	rows, err := r.PromotionSvc.RedemptionsForOrder(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AppliedPromotion, len(rows))
	for i, rd := range rows {
		m := &model.AppliedPromotion{
			ID:        rd.ID.String(),
			RuleName:  rd.RuleName,
			RuleSlug:  rd.RuleSlug,
			Code:      rd.Code,
			Discount:  int32(rd.DiscountCents),
			AppliedAt: rd.RedeemedAt,
		}
		if rd.RuleID != nil {
			id := rd.RuleID.String()
			m.RuleID = &id
		}
		out[i] = m
	}
	return out, nil
}

// Empty for orders placed before the breakdown was recorded, which the UI reads as
// "no detail" rather than "no tax" - the parent Order resolver already gated orders.view
func (r *orderResolver) TaxLines(ctx context.Context, obj *model.Order) ([]*model.OrderTaxLine, error) {
	oid, err := parseID(obj.ID, "order")
	if err != nil {
		return nil, err
	}
	rows, err := r.OrderSvc.TaxLines(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.OrderTaxLine, len(rows))
	for i, tl := range rows {
		m := &model.OrderTaxLine{
			ID:      tl.ID.String(),
			Name:    tl.Name,
			RateBps: tl.RateBPS,
			Amount:  int32(tl.AmountCents),
		}
		if tl.RuleID != nil {
			id := tl.RuleID.String()
			m.RuleID = &id
		}
		out[i] = m
	}
	return out, nil
}

// The explicit channelId filter only narrows - it can never widen past the admin's permitted scope
func (r *queryResolver) Orders(ctx context.Context, channelID *string) ([]*model.Order, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "orders.view")
	if err != nil {
		return nil, err
	}
	orderList, err := r.OrderSvc.ListOrders(ctx)
	if err != nil {
		return nil, err
	}
	explicit, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	visible := auth.FilterByChannel(orderList, perm, func(o order.Order) uuid.UUID { return o.ChannelID })
	result := make([]*model.Order, 0, len(visible))
	for _, o := range visible {
		if explicit != nil && o.ChannelID != *explicit {
			continue
		}
		result = append(result, toOrderModel(o))
	}
	return result, nil
}

func (r *queryResolver) OrdersPaged(ctx context.Context, channelID *string, search *string, statuses []string, paymentStatuses []string, limit *int32, offset *int32) (*model.OrderPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.view"); err != nil {
		return nil, err
	}
	// Scope resolves to a concrete id list BEFORE the SQL page, so LIMIT applies to rows the caller may actually see
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "orders.view")
	if err != nil {
		return nil, err
	}
	explicit, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var channels []uuid.UUID
	switch {
	case explicit != nil:
		if !perm.Allows(*explicit) {
			return nil, auth.ErrForbidden
		}
		channels = []uuid.UUID{*explicit}
	case !perm.Bypass():
		channels = perm.IDs()
		if len(channels) == 0 {
			return &model.OrderPage{TotalCount: 0, Items: []*model.Order{}}, nil
		}
	}
	f := order.ListPageFilter{
		ChannelIDs:      channels,
		Statuses:        lowerAll(statuses),
		PaymentStatuses: lowerAll(paymentStatuses),
		Limit:           intDeref(limit, 50),
		Offset:          intDeref(offset, 0),
	}
	if search != nil {
		f.Search = *search
	}
	rows, total, err := r.OrderSvc.ListOrdersPage(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]*model.Order, 0, len(rows))
	for _, o := range rows {
		items = append(items, toOrderModel(o))
	}
	return &model.OrderPage{TotalCount: int32(total), Items: items}, nil
}

// A scoped admin asking by ID for a channel they can't reach gets FORBIDDEN, not the row
func (r *queryResolver) Order(ctx context.Context, id string) (*model.Order, error) {
	orderID, err := parseID(id, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", o.ChannelID, nil); err != nil {
		return nil, err
	}
	return toOrderModel(o), nil
}

func (r *queryResolver) OrderDocumentPDF(ctx context.Context, orderID string, kind model.OrderDocumentKind, creditMemoID *string) (string, error) {
	oid, err := uuid.Parse(orderID)
	if err != nil {
		return "", errInvalidID("order id", nil)
	}
	o, err := r.OrderSvc.GetByID(ctx, oid)
	if err != nil {
		return "", errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", o.ChannelID, nil); err != nil {
		return "", err
	}

	docKind := documents.KindInvoice
	var memo *order.CreditMemo
	switch kind {
	case model.OrderDocumentKindPackingSlip:
		docKind = documents.KindPackingSlip
	case model.OrderDocumentKindCreditMemo:
		docKind = documents.KindCreditMemo
		// One document per memo - a per-order PDF prints all lines, so a partial refund reads like a full one
		if creditMemoID == nil || *creditMemoID == "" {
			return "", fmt.Errorf("creditMemoId is required for a credit memo document")
		}
		mid, perr := parseID(*creditMemoID, "credit memo")
		if perr != nil {
			return "", perr
		}
		loaded, merr := r.OrderSvc.GetCreditMemo(ctx, mid)
		if merr != nil {
			return "", errNotFound("credit memo")
		}
		if loaded.OrderID != oid {
			return "", fmt.Errorf("credit memo does not belong to this order")
		}
		memo = &loaded
	}

	store := documents.StoreInfo{Name: "Store"} // letterhead comes from the order's channel
	if ch, cerr := r.ChannelSvc.GetByID(ctx, o.ChannelID); cerr == nil {
		store.Name = ch.Name
		if ch.PublicURL != "" {
			store.Lines = append(store.Lines, ch.PublicURL)
		}
	}

	pdf, err := documents.Render(docKind, o, store, memo)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pdf), nil
}

func (r *queryResolver) CreditMemos(ctx context.Context, orderID string) ([]*model.CreditMemo, error) {
	oid, err := parseID(orderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, oid)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.view", o.ChannelID, nil); err != nil {
		return nil, err
	}
	memos, err := r.OrderSvc.ListCreditMemos(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.CreditMemo, len(memos))
	for i, m := range memos {
		out[i] = toCreditMemoModel(m, o)
	}
	return out, nil
}

// Ordered minus what earlier memos covered, per line - bounds the issue form
func (r *queryResolver) CreditableQuantities(ctx context.Context, orderID string) ([]*model.OrderLineQuantity, error) {
	oid, err := parseID(orderID, "order")
	if err != nil {
		return nil, err
	}
	o, err := r.OrderSvc.GetByID(ctx, oid)
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.refund", o.ChannelID, nil); err != nil {
		return nil, err
	}
	byLine, err := r.OrderSvc.CreditableByOrderLine(ctx, oid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.OrderLineQuantity, 0, len(o.Lines))
	for _, l := range o.Lines {
		out = append(out, &model.OrderLineQuantity{OrderLineID: l.ID.String(), Quantity: byLine[l.ID]})
	}
	return out, nil
}

func (r *Resolver) Order() OrderResolver { return &orderResolver{r} }

type orderResolver struct{ *Resolver }

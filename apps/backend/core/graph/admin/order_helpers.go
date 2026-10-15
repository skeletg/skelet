package admin

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

func toOrderAddressInput(in *model.AddressInput) order.OrderAddress { // nil yields a zero address, which the service's resolveAddress then rejects
	if in == nil {
		return order.OrderAddress{}
	}
	return order.OrderAddress{
		FirstName:   in.FirstName,
		LastName:    in.LastName,
		Company:     in.Company,
		Street1:     in.Street1,
		Street2:     in.Street2,
		City:        in.City,
		State:       in.State,
		PostalCode:  in.PostalCode,
		CountryCode: in.CountryCode,
		Phone:       in.Phone,
	}
}

func toCreateDraftOrderInput(in model.CreateDraftOrderInput, channelID uuid.UUID) order.CreateDraftOrderInput { // channelID arrives already parsed and permission-checked
	lines := make([]order.CreateDraftLine, 0, len(in.Lines))
	for _, l := range in.Lines {
		if l == nil {
			continue
		}
		dl := order.CreateDraftLine{ProductSku: l.Sku, Quantity: l.Quantity}
		if l.UnitPriceCents != nil {
			v := int64(*l.UnitPriceCents)
			dl.UnitPriceCents = &v
		}
		lines = append(lines, dl)
	}
	var discount int64
	if in.ManualDiscountCents != nil {
		discount = int64(*in.ManualDiscountCents)
	}
	note := ""
	if in.CustomerNote != nil {
		note = *in.CustomerNote
	}
	return order.CreateDraftOrderInput{
		ChannelID:           channelID,
		CustomerID:          parseOptionalUUID(in.CustomerID),
		Email:               in.Email,
		BillingAddress:      toOrderAddressInput(in.BillingAddress),
		ShippingAddress:     toOrderAddressInput(in.ShippingAddress),
		Lines:               lines,
		ShippingCarrierSlug: in.ShippingCarrierSlug,
		ShippingMethodCode:  in.ShippingMethodCode,
		ManualDiscountCents: discount,
		CustomerNote:        note,
		SourceCartID:        parseOptionalUUID(in.SourceCartID),
	}
}

// perm is a parameter because moving money OUT needs orders.refund, not the orders.update every support role holds for editing addresses
func (r *mutationResolver) orderForPaymentAction(ctx context.Context, id, perm string) (order.Order, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, perm); err != nil {
		return order.Order{}, err
	}
	orderID, err := uuid.Parse(id)
	if err != nil {
		return order.Order{}, errInvalidID("order id", nil)
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID) // lookup first so the gate runs against the order's channel before any gateway call
	if err != nil {
		return order.Order{}, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, perm, o.ChannelID, nil); err != nil {
		return order.Order{}, err
	}
	return o, nil
}

func (r *mutationResolver) adminTransitionOrderStatus(ctx context.Context, id, status string) (*model.Order, error) {
	return r.adminTransitionOrderStatusWithTracking(ctx, id, status, "", "")
}

// Transitions that don't stamp shipping pass empty carrier + trackingNumber and the fields are ignored
func (r *mutationResolver) adminTransitionOrderStatusWithTracking(ctx context.Context, id, status, carrier, trackingNumber string) (*model.Order, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.update"); err != nil {
		return nil, err
	}
	orderID, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("order id", nil)
	}
	o, err := r.OrderSvc.GetByID(ctx, orderID) // lookup first - the channel can't be derived from the order id, so the gate needs the row
	if err != nil {
		return nil, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "orders.update", o.ChannelID, nil); err != nil {
		return nil, err
	}
	updated, err := r.OrderSvc.UpdateStatusWithTracking(ctx, orderID, status, carrier, trackingNumber)
	if err != nil {
		return nil, err
	}
	return toOrderModel(updated), nil
}

// lowerAll maps the SCREAMING enum spellings the dashboard sends onto the lowercase status column values
func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.ToLower(v)
	}
	return out
}

func toOrderModel(o order.Order) *model.Order {
	lines := make([]*model.OrderLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = toOrderLineModel(l)
	}
	events := make([]*model.OrderEvent, len(o.Events))
	for i, e := range o.Events {
		events[i] = toOrderEventModel(e)
	}
	m := &model.Order{
		ID:                 o.ID.String(),
		ChannelID:          o.ChannelID.String(),
		Number:             o.Number,
		PoNumber:           strPtrOrNil(o.PONumber),
		BusinessIds:        toOrderBusinessIDs(o.BusinessIDs),
		Status:             model.OrderStatus(strings.ToUpper(o.Status)),
		PaymentStatus:      model.PaymentStatus(strings.ToUpper(o.PaymentStatus)),
		PaymentProvider:    o.PaymentProvider,
		PaymentReference:   o.PaymentReference,
		Email:              o.Email,
		BillingAddress:     toOrderAddressModel(o.BillingAddress),
		ShippingAddress:    toOrderAddressModel(o.ShippingAddress),
		Lines:              lines,
		Events:             events,
		Subtotal:           int32(o.SubtotalAmount),
		ShippingAmount:     int32(o.ShippingAmount),
		DiscountAmount:     int32(o.DiscountAmount),
		TaxAmount:          int32(o.TaxAmount),
		ShippingTaxAmount:  int32(o.ShippingTaxAmount),
		ShippingRefundable: int32(o.ShippingRefundable()),
		Total:              int32(o.TotalAmount),
		GiftCardAmount:     int32(o.GiftCardAmount),
		StoreCreditAmount:  int32(o.StoreCreditAmount),
		CurrencyCode:       o.CurrencyCode,
		Carrier:            o.Carrier,
		TrackingNumber:     o.TrackingNumber,
		CreatedAt:          o.CreatedAt,
	}
	if o.CustomerID != nil {
		id := o.CustomerID.String()
		m.CustomerID = &id
	}
	if o.CustomerNote != "" {
		m.CustomerNote = &o.CustomerNote
	}
	return m
}

func toFulfillmentModel(f order.Fulfillment) *model.Fulfillment {
	lines := make([]*model.FulfillmentLine, len(f.Lines))
	for i, l := range f.Lines {
		lines[i] = &model.FulfillmentLine{
			ID:          l.ID.String(),
			OrderLineID: l.OrderLineID.String(),
			Quantity:    l.Quantity,
		}
	}
	labels := make([]*model.ShippingLabel, len(f.Labels))
	for i, l := range f.Labels {
		labels[i] = toShippingLabelModel(l)
	}
	return &model.Fulfillment{
		ID:                f.ID.String(),
		Status:            f.Status,
		Carrier:           f.Carrier,
		TrackingNumber:    f.TrackingNumber,
		Sequence:          f.Sequence,
		InsuredValueCents: int32(f.InsuredValueCents),
		Lines:             lines,
		Labels:            labels,
		ShippedAt:         f.ShippedAt,
		CreatedAt:         f.CreatedAt,
	}
}

var labelMimeTypes = map[string]string{
	"pdf": "application/pdf",
	"png": "image/png",
	"zpl": "application/octet-stream",
	"txt": "text/plain",
}

func toShippingLabelModel(l order.ShippingLabel) *model.ShippingLabel {
	mime := labelMimeTypes[l.Format]
	if mime == "" {
		mime = "application/octet-stream"
	}
	return &model.ShippingLabel{
		ID:             l.ID.String(),
		CarrierSlug:    l.CarrierSlug,
		Format:         l.Format,
		TrackingNumber: l.TrackingNumber,
		DataURL:        "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(l.Doc),
		CreatedAt:      l.CreatedAt,
	}
}

func toPaymentTransactionModel(t order.PaymentTransaction) *model.PaymentTransaction {
	return &model.PaymentTransaction{
		ID:                t.ID.String(),
		Kind:              t.Kind,
		Amount:            int32(t.AmountCents),
		Status:            t.Status,
		ProviderSlug:      t.ProviderSlug,
		ProviderReference: t.ProviderReference,
		Reason:            t.Reason,
		CreatedAt:         t.CreatedAt,
	}
}

func toOrderReturnModel(ret order.OrderReturn) *model.OrderReturn {
	lines := make([]*model.OrderReturnLine, len(ret.Lines))
	for i, l := range ret.Lines {
		lines[i] = &model.OrderReturnLine{
			ID:              l.ID.String(),
			OrderLineID:     l.OrderLineID.String(),
			Quantity:        l.Quantity,
			Reason:          l.Reason,
			RefundUnitCents: int32(l.RefundUnitCents),
		}
	}
	m := &model.OrderReturn{
		ID:           ret.ID.String(),
		RmaNumber:    ret.RMANumber,
		Status:       ret.Status,
		Reason:       ret.Reason,
		Restock:      ret.Restock,
		RefundTarget: ret.RefundTarget,
		RefundAmount: int32(ret.RefundAmountCents),
		Resolution:   ret.Resolution,
		Lines:        lines,
		CreatedAt:    ret.CreatedAt,
		UpdatedAt:    ret.UpdatedAt,
	}
	if ret.ExchangeOrderID != nil {
		id := ret.ExchangeOrderID.String()
		m.ExchangeOrderID = &id
	}
	return m
}

// Needs the order because memo lines carry the line id and the money, nothing descriptive
func toCreditMemoModel(m order.CreditMemo, o order.Order) *model.CreditMemo {
	byLineID := make(map[uuid.UUID]order.OrderLine, len(o.Lines))
	for _, l := range o.Lines {
		byLineID[l.ID] = l
	}
	lines := make([]*model.CreditMemoLine, len(m.Lines))
	for i, l := range m.Lines {
		src := byLineID[l.OrderLineID]
		lines[i] = &model.CreditMemoLine{
			OrderLineID: l.OrderLineID.String(),
			Sku:         src.ProductSku,
			Name:        src.ProductName,
			Quantity:    l.Quantity,
			UnitAmount:  int32(l.UnitCents),
			TotalAmount: int32(l.TotalCents),
		}
	}
	out := &model.CreditMemo{
		ID:               m.ID.String(),
		Number:           m.Number,
		OrderID:          m.OrderID.String(),
		ItemsTotal:       int32(m.LinesTotalCents),
		ShippingRefund:   int32(m.ShippingRefundCents),
		AdjustmentRefund: int32(m.AdjustmentRefundCents),
		AdjustmentFee:    int32(m.AdjustmentFeeCents),
		Total:            int32(m.TotalCents),
		RefundTarget:     m.RefundTarget,
		Offline:          m.Offline,
		Reason:           m.Reason,
		Lines:            lines,
		CreatedAt:        m.CreatedAt,
	}
	if m.ReturnID != nil {
		id := m.ReturnID.String()
		out.ReturnID = &id
	}
	return out
}

func int32Deref(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

// The ONE path money takes out of an order - reserve the memo and its ledger row, move the money by whichever route the order and target imply, then settle
// Settling is what publishes order.refunded, so a path with its own copy of this wrote a ledger row and silently skipped the loyalty and referral clawback
func (r *mutationResolver) runCreditMemoRefund(ctx context.Context, in order.IssueCreditMemoInput) (order.CreditMemo, error) {
	intent, err := r.OrderSvc.RecordCreditMemoIntent(ctx, in)
	if err != nil {
		return order.CreditMemo{}, err
	}

	var providerRef string
	var actionErr error
	switch {
	case intent.Target == order.RefundTargetGiftCard:
		card, gcErr := r.GiftcardSvc.Issue(ctx, giftcard.IssueInput{
			CurrencyCode:  intent.Order.CurrencyCode,
			Amount:        intent.AmountCents,
			IssuedOrderID: &intent.Order.ID,
			CustomerID:    intent.Order.CustomerID,
			Message:       fmt.Sprintf("Credit memo %s", intent.Memo.Number),
			IssueRef:      "memo:" + intent.Memo.ID.String(), // a retry after a failed finalize must find this card, not mint a second one
		})
		if gcErr == nil {
			providerRef = card.Code
		}
		actionErr = gcErr
	case intent.Offline: // nothing to call - the operator returns the money the way they took it
		providerRef = intent.Order.PaymentReference
		if providerRef == "" {
			providerRef = "manual"
		}
	default:
		res, refErr := r.PaymentSvc.Refund(ctx, intent.Order.PaymentProvider, payment.RefundRequest{
			OrderID:           intent.Order.ID,
			ChannelID:         intent.Order.ChannelID,
			ProviderReference: intent.Order.PaymentReference,
			AmountCents:       intent.AmountCents,
			Reason:            in.Reason,
			IdempotencyKey:    intent.Txn.IdempotencyKey,
		})
		if refErr == nil {
			providerRef = res.ProviderReference
		}
		actionErr = refErr
	}

	_, memo, finErr := r.OrderSvc.FinalizeCreditMemo(ctx, intent.Txn.ID, actionErr == nil, providerRef)
	if actionErr != nil {
		return order.CreditMemo{}, actionErr
	}
	if finErr != nil {
		return order.CreditMemo{}, finErr
	}
	return memo, nil
}

// Nil for system callers
func creditMemoAdminID(ctx context.Context) *uuid.UUID {
	claims, ok := auth.AdminFromContext(ctx)
	if !ok || claims == nil {
		return nil
	}
	id := claims.AdminID
	return &id
}

// Lookup-first preamble the return mutations share - the channel can't be derived from the return id, so the gate needs the order row
func (r *mutationResolver) returnForAction(ctx context.Context, id, permission string) (order.OrderReturn, order.Order, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, permission); err != nil {
		return order.OrderReturn{}, order.Order{}, err
	}
	returnID, err := parseID(id, "return")
	if err != nil {
		return order.OrderReturn{}, order.Order{}, err
	}
	ret, err := r.OrderSvc.GetReturn(ctx, returnID)
	if err != nil {
		return order.OrderReturn{}, order.Order{}, errNotFound("return")
	}
	o, err := r.OrderSvc.GetByID(ctx, ret.OrderID)
	if err != nil {
		return order.OrderReturn{}, order.Order{}, errNotFound("order")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, permission, o.ChannelID, nil); err != nil {
		return order.OrderReturn{}, order.Order{}, err
	}
	return ret, o, nil
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Maps the enum the dashboard sends onto the template slot the mail path renders from
func orderEmailMessageKey(m model.OrderEmailMessage) string {
	switch m {
	case model.OrderEmailMessageShipment:
		return email.KeyOrderShipment
	case model.OrderEmailMessageCancellation:
		return email.KeyOrderCancellation
	case model.OrderEmailMessageRefund:
		return email.KeyOrderRefund
	default:
		return email.KeyOrderConfirmation
	}
}

// The unpaged orders field stops here - ordersPaged is the one to page
const unpagedOrdersCap int32 = 1000

// Resolved before the SQL page so LIMIT only counts rows the caller can see
// nil with none=false is every channel, none=true is no channel at all
func (r *queryResolver) orderChannelScope(ctx context.Context, channelID *string) (channels []uuid.UUID, none bool, err error) {
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "orders.view")
	if err != nil {
		return nil, false, err
	}
	explicit, err := r.scopedChannelID(ctx, channelID)
	if err != nil {
		return nil, false, err
	}
	switch {
	case explicit != nil:
		if !perm.Allows(*explicit) {
			return nil, false, auth.ErrForbidden
		}
		return []uuid.UUID{*explicit}, false, nil
	case !perm.Bypass():
		ids := perm.IDs()
		return ids, len(ids) == 0, nil
	}
	return nil, false, nil
}

func toOrderBusinessIDs(ids []order.BusinessID) []*model.OrderBusinessID {
	out := make([]*model.OrderBusinessID, len(ids))
	for i, id := range ids {
		out[i] = &model.OrderBusinessID{Key: id.Key, Label: id.Label, Value: id.Value}
	}
	return out
}

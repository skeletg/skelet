package order

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type postgresRepository struct {
	queries *db.Queries
}

func NewRepository(queries *db.Queries) Repository {
	return &postgresRepository{queries: queries}
}

func (r *postgresRepository) WithTx(tx *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(tx)}
}

func (r *postgresRepository) InsertPaymentTransaction(ctx context.Context, t PaymentTransaction) (PaymentTransaction, error) {
	var idem sql.NullString
	if t.IdempotencyKey != "" {
		idem = sql.NullString{String: t.IdempotencyKey, Valid: true}
	}
	var returnID uuid.NullUUID
	if t.ReturnID != nil {
		returnID = uuid.NullUUID{UUID: *t.ReturnID, Valid: true}
	}
	var occurred sql.NullTime
	if t.OccurredAt != nil {
		occurred = sql.NullTime{Time: *t.OccurredAt, Valid: true}
	}
	row, err := r.queries.InsertOrderPaymentTransaction(ctx, db.InsertOrderPaymentTransactionParams{
		OccurredAt:        occurred,
		OrderID:           t.OrderID,
		Kind:              t.Kind,
		AmountCents:       t.AmountCents,
		Status:            t.Status,
		ProviderSlug:      t.ProviderSlug,
		ProviderReference: t.ProviderReference,
		IdempotencyKey:    idem,
		Reason:            t.Reason,
		ReturnID:          returnID,
	})
	if err != nil {
		return PaymentTransaction{}, fmt.Errorf("insert payment transaction: %w", err)
	}
	return toPaymentTransaction(row), nil
}

func (r *postgresRepository) MarkPaymentTransaction(ctx context.Context, id uuid.UUID, status, providerReference string) (PaymentTransaction, bool, error) {
	row, err := r.queries.MarkOrderPaymentTransaction(ctx, db.MarkOrderPaymentTransactionParams{
		ID:                id,
		Status:            status,
		ProviderReference: providerReference,
	})
	if errors.Is(err, sql.ErrNoRows) {
		existing, gerr := r.queries.GetOrderPaymentTransaction(ctx, id)
		if gerr != nil {
			return PaymentTransaction{}, false, fmt.Errorf("get payment transaction: %w", gerr)
		}
		return toPaymentTransaction(existing), false, nil
	}
	if err != nil {
		return PaymentTransaction{}, false, fmt.Errorf("mark payment transaction: %w", err)
	}
	return toPaymentTransaction(row), true, nil
}

func (r *postgresRepository) MarkCreditMemoFailed(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.MarkCreditMemoFailed(ctx, id); err != nil {
		return fmt.Errorf("mark credit memo failed: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPaymentTransactions(ctx context.Context, orderID uuid.UUID) ([]PaymentTransaction, error) {
	rows, err := r.queries.ListOrderPaymentTransactions(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list payment transactions: %w", err)
	}
	out := make([]PaymentTransaction, len(rows))
	for i, row := range rows {
		out[i] = toPaymentTransaction(row)
	}
	return out, nil
}

func (r *postgresRepository) LockOrder(ctx context.Context, orderID uuid.UUID) error {
	return r.queries.LockOrderForPayment(ctx, orderID)
}

// LockOrderForPayment takes the order row so concurrent payment intents serialize
func (r *postgresRepository) LockOrderForPayment(ctx context.Context, orderID uuid.UUID) error {
	return r.queries.LockOrderForPayment(ctx, orderID)
}

// PaymentTotalsActiveByKind counts pending rows as well as succeeded - the guards need money already in flight to be visible
func (r *postgresRepository) PaymentTotalsActiveByKind(ctx context.Context, orderID uuid.UUID) (PaymentTotals, error) {
	rows, err := r.queries.SumOrderPaymentByKindActive(ctx, orderID)
	if err != nil {
		return PaymentTotals{}, fmt.Errorf("sum active payment by kind: %w", err)
	}
	var t PaymentTotals
	for _, row := range rows {
		addKindTotal(&t, row.Kind, row.Total)
	}
	return t, nil
}

func addKindTotal(t *PaymentTotals, kind string, total int64) {
	switch kind {
	case PaymentTxnAuthorize:
		t.Authorized = total
	case PaymentTxnCapture:
		t.Captured = total
	case PaymentTxnRefund:
		t.Refunded = total
	case PaymentTxnVoid:
		t.Voided = total
	}
}

func (r *postgresRepository) PaymentTotalsByKind(ctx context.Context, orderID uuid.UUID) (PaymentTotals, error) {
	rows, err := r.queries.SumOrderPaymentByKind(ctx, orderID)
	if err != nil {
		return PaymentTotals{}, fmt.Errorf("sum payment by kind: %w", err)
	}
	var t PaymentTotals
	for _, row := range rows {
		addKindTotal(&t, row.Kind, row.Total)
	}
	return t, nil
}

func (r *postgresRepository) FindPaymentTransactionByKey(ctx context.Context, idempotencyKey string) (PaymentTransaction, bool, error) {
	row, err := r.queries.GetOrderPaymentTransactionByIdempotencyKey(ctx, sql.NullString{String: idempotencyKey, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return PaymentTransaction{}, false, nil
	}
	if err != nil {
		return PaymentTransaction{}, false, fmt.Errorf("find payment transaction by key: %w", err)
	}
	return toPaymentTransaction(row), true, nil
}

func (r *postgresRepository) CreateFulfillment(ctx context.Context, f Fulfillment) (Fulfillment, error) {
	var shipped sql.NullTime
	if f.ShippedAt != nil {
		shipped = sql.NullTime{Time: *f.ShippedAt, Valid: true}
	}
	row, err := r.queries.CreateOrderFulfillment(ctx, db.CreateOrderFulfillmentParams{
		OrderID:           f.OrderID,
		Status:            f.Status,
		Carrier:           f.Carrier,
		TrackingNumber:    f.TrackingNumber,
		Sequence:          f.Sequence,
		ShippedAt:         shipped,
		InsuredValueCents: f.InsuredValueCents,
	})
	if err != nil {
		return Fulfillment{}, fmt.Errorf("create fulfillment: %w", err)
	}
	return toFulfillment(row), nil
}

func (r *postgresRepository) AddFulfillmentLine(ctx context.Context, fulfillmentID uuid.UUID, line FulfillmentLine) (FulfillmentLine, error) {
	row, err := r.queries.CreateOrderFulfillmentLine(ctx, db.CreateOrderFulfillmentLineParams{
		FulfillmentID: fulfillmentID,
		OrderLineID:   line.OrderLineID,
		Quantity:      line.Quantity,
	})
	if err != nil {
		return FulfillmentLine{}, fmt.Errorf("add fulfillment line: %w", err)
	}
	return toFulfillmentLine(row), nil
}

func (r *postgresRepository) ListFulfillments(ctx context.Context, orderID uuid.UUID) ([]Fulfillment, error) {
	rows, err := r.queries.ListOrderFulfillments(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list fulfillments: %w", err)
	}
	out := make([]Fulfillment, len(rows))
	ids := make([]uuid.UUID, len(rows))
	idx := make(map[uuid.UUID]int, len(rows))
	for i, row := range rows {
		out[i] = toFulfillment(row)
		ids[i] = row.ID
		idx[row.ID] = i
	}
	if len(ids) == 0 {
		return out, nil
	}
	lineRows, err := r.queries.ListOrderFulfillmentLinesByFulfillmentIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list fulfillment lines: %w", err)
	}
	for _, lr := range lineRows {
		if i, ok := idx[lr.FulfillmentID]; ok {
			out[i].Lines = append(out[i].Lines, toFulfillmentLine(lr))
		}
	}
	labelRows, err := r.queries.ListShippingLabelsByFulfillmentIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list fulfillment labels: %w", err)
	}
	for _, lr := range labelRows {
		if i, ok := idx[lr.FulfillmentID]; ok {
			out[i].Labels = append(out[i].Labels, toShippingLabel(lr))
		}
	}
	return out, nil
}

func (r *postgresRepository) GetFulfillment(ctx context.Context, id uuid.UUID) (Fulfillment, error) {
	row, err := r.queries.GetOrderFulfillment(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Fulfillment{}, ErrFulfillmentNotFound
	}
	if err != nil {
		return Fulfillment{}, fmt.Errorf("get fulfillment: %w", err)
	}
	return toFulfillment(row), nil
}

func (r *postgresRepository) CreateShippingLabel(ctx context.Context, l ShippingLabel) (ShippingLabel, error) {
	row, err := r.queries.CreateShippingLabel(ctx, db.CreateShippingLabelParams{
		FulfillmentID:  l.FulfillmentID,
		CarrierSlug:    l.CarrierSlug,
		Format:         l.Format,
		TrackingNumber: l.TrackingNumber,
		Doc:            l.Doc,
	})
	if err != nil {
		return ShippingLabel{}, fmt.Errorf("create shipping label: %w", err)
	}
	return toShippingLabel(row), nil
}

func (r *postgresRepository) GetShippingLabel(ctx context.Context, id uuid.UUID) (ShippingLabel, error) {
	row, err := r.queries.GetShippingLabel(ctx, id)
	if err != nil {
		return ShippingLabel{}, fmt.Errorf("get shipping label: %w", err)
	}
	return toShippingLabel(row), nil
}

func toShippingLabel(row db.ShippingLabel) ShippingLabel {
	return ShippingLabel{
		ID:             row.ID,
		FulfillmentID:  row.FulfillmentID,
		CarrierSlug:    row.CarrierSlug,
		Format:         row.Format,
		TrackingNumber: row.TrackingNumber,
		Doc:            row.Doc,
		CreatedAt:      row.CreatedAt,
	}
}

func (r *postgresRepository) ShippedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error) {
	rows, err := r.queries.SumShippedByOrderLine(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("sum shipped by order line: %w", err)
	}
	out := make(map[uuid.UUID]float64, len(rows))
	for _, row := range rows {
		out[row.OrderLineID] = row.Shipped
	}
	return out, nil
}

func (r *postgresRepository) CountFulfillments(ctx context.Context, orderID uuid.UUID) (int64, error) {
	return r.queries.CountOrderFulfillments(ctx, orderID)
}

func toFulfillment(row db.OrderFulfillment) Fulfillment {
	f := Fulfillment{
		ID:                row.ID,
		OrderID:           row.OrderID,
		Status:            row.Status,
		Carrier:           row.Carrier,
		TrackingNumber:    row.TrackingNumber,
		Sequence:          row.Sequence,
		InsuredValueCents: row.InsuredValueCents,
		CreatedAt:         row.CreatedAt,
	}
	if row.ShippedAt.Valid {
		t := row.ShippedAt.Time
		f.ShippedAt = &t
	}
	return f
}

func toFulfillmentLine(row db.OrderFulfillmentLine) FulfillmentLine {
	return FulfillmentLine{
		ID:            row.ID,
		FulfillmentID: row.FulfillmentID,
		OrderLineID:   row.OrderLineID,
		Quantity:      row.Quantity,
	}
}

func toPaymentTransaction(row db.OrderPaymentTransaction) PaymentTransaction {
	t := PaymentTransaction{
		ID:                row.ID,
		OrderID:           row.OrderID,
		Kind:              row.Kind,
		AmountCents:       row.AmountCents,
		Status:            row.Status,
		ProviderSlug:      row.ProviderSlug,
		ProviderReference: row.ProviderReference,
		IdempotencyKey:    row.IdempotencyKey.String,
		Reason:            row.Reason,
		CreatedAt:         row.CreatedAt,
	}
	if row.ReturnID.Valid {
		id := row.ReturnID.UUID
		t.ReturnID = &id
	}
	if row.CreditMemoID.Valid {
		id := row.CreditMemoID.UUID
		t.CreditMemoID = &id
	}
	return t
}

func toCreditMemo(row db.CreditMemo) CreditMemo {
	m := CreditMemo{
		ID:                    row.ID,
		OrderID:               row.OrderID,
		Number:                row.Number,
		LinesTotalCents:       row.LinesTotalCents,
		ShippingRefundCents:   row.ShippingRefundCents,
		AdjustmentRefundCents: row.AdjustmentRefundCents,
		AdjustmentFeeCents:    row.AdjustmentFeeCents,
		TotalCents:            row.TotalCents,
		RefundTarget:          row.RefundTarget,
		Offline:               row.Offline,
		Reason:                row.Reason,
		CreatedAt:             row.CreatedAt,
	}
	if row.ReturnID.Valid {
		id := row.ReturnID.UUID
		m.ReturnID = &id
	}
	if row.AdminID.Valid {
		id := row.AdminID.UUID
		m.AdminID = &id
	}
	return m
}

func toCreditMemoLine(row db.CreditMemoLine) CreditMemoLine {
	return CreditMemoLine{
		ID:          row.ID,
		OrderLineID: row.OrderLineID,
		Quantity:    row.Quantity,
		UnitCents:   row.UnitCents,
		TotalCents:  row.TotalCents,
	}
}

func (r *postgresRepository) CreateCreditMemo(ctx context.Context, m CreditMemo) (CreditMemo, error) {
	var returnID, adminID uuid.NullUUID
	if m.ReturnID != nil {
		returnID = uuid.NullUUID{UUID: *m.ReturnID, Valid: true}
	}
	if m.AdminID != nil {
		adminID = uuid.NullUUID{UUID: *m.AdminID, Valid: true}
	}
	row, err := r.queries.CreateCreditMemo(ctx, db.CreateCreditMemoParams{
		OrderID:               m.OrderID,
		ReturnID:              returnID,
		LinesTotalCents:       m.LinesTotalCents,
		ShippingRefundCents:   m.ShippingRefundCents,
		AdjustmentRefundCents: m.AdjustmentRefundCents,
		AdjustmentFeeCents:    m.AdjustmentFeeCents,
		TotalCents:            m.TotalCents,
		RefundTarget:          m.RefundTarget,
		Offline:               m.Offline,
		Reason:                m.Reason,
		AdminID:               adminID,
	})
	if err != nil {
		return CreditMemo{}, fmt.Errorf("create credit memo: %w", err)
	}
	return toCreditMemo(row), nil
}

func (r *postgresRepository) AddCreditMemoLine(ctx context.Context, memoID uuid.UUID, line CreditMemoLine) (CreditMemoLine, error) {
	row, err := r.queries.AddCreditMemoLine(ctx, db.AddCreditMemoLineParams{
		CreditMemoID: memoID,
		OrderLineID:  line.OrderLineID,
		Quantity:     line.Quantity,
		UnitCents:    line.UnitCents,
		TotalCents:   line.TotalCents,
	})
	if err != nil {
		return CreditMemoLine{}, fmt.Errorf("add credit memo line: %w", err)
	}
	return toCreditMemoLine(row), nil
}

func (r *postgresRepository) GetCreditMemo(ctx context.Context, id uuid.UUID) (CreditMemo, error) {
	row, err := r.queries.GetCreditMemo(ctx, id)
	if err != nil {
		return CreditMemo{}, fmt.Errorf("get credit memo: %w", err)
	}
	memo := toCreditMemo(row)
	lines, err := r.queries.ListCreditMemoLines(ctx, id)
	if err != nil {
		return CreditMemo{}, fmt.Errorf("list credit memo lines: %w", err)
	}
	memo.Lines = make([]CreditMemoLine, len(lines))
	for i, l := range lines {
		memo.Lines[i] = toCreditMemoLine(l)
	}
	return memo, nil
}

func (r *postgresRepository) GetCreditMemoByReturn(ctx context.Context, returnID uuid.UUID) (CreditMemo, bool, error) {
	row, err := r.queries.GetCreditMemoByReturn(ctx, uuid.NullUUID{UUID: returnID, Valid: true})
	if errors.Is(err, sql.ErrNoRows) {
		return CreditMemo{}, false, nil
	}
	if err != nil {
		return CreditMemo{}, false, fmt.Errorf("get credit memo by return: %w", err)
	}
	return toCreditMemo(row), true, nil
}

// Lines are fetched for every memo in one batched query - the order detail page renders them all
func (r *postgresRepository) ListCreditMemos(ctx context.Context, orderID uuid.UUID) ([]CreditMemo, error) {
	rows, err := r.queries.ListCreditMemosByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list credit memos: %w", err)
	}
	memos := make([]CreditMemo, len(rows))
	ids := make([]uuid.UUID, len(rows))
	byID := make(map[uuid.UUID]int, len(rows))
	for i, row := range rows {
		memos[i] = toCreditMemo(row)
		ids[i] = row.ID
		byID[row.ID] = i
	}
	if len(ids) == 0 {
		return memos, nil
	}
	lines, err := r.queries.ListCreditMemoLinesByMemoIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list credit memo lines: %w", err)
	}
	for _, l := range lines {
		if i, ok := byID[l.CreditMemoID]; ok {
			memos[i].Lines = append(memos[i].Lines, toCreditMemoLine(l))
		}
	}
	return memos, nil
}

func (r *postgresRepository) CreditedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error) {
	rows, err := r.queries.SumCreditMemoQtyByOrderLine(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("sum credited by order line: %w", err)
	}
	out := make(map[uuid.UUID]float64, len(rows))
	for _, row := range rows {
		out[row.OrderLineID] = row.Credited
	}
	return out, nil
}

func (r *postgresRepository) LinkPaymentTransactionToMemo(ctx context.Context, txnID, memoID uuid.UUID) error {
	if err := r.queries.SetPaymentTransactionCreditMemo(ctx, db.SetPaymentTransactionCreditMemoParams{
		ID:           txnID,
		CreditMemoID: uuid.NullUUID{UUID: memoID, Valid: true},
	}); err != nil {
		return fmt.Errorf("link payment transaction to credit memo: %w", err)
	}
	return nil
}

func (r *postgresRepository) HoldOrderFrom(ctx context.Context, orderID uuid.UUID, fromStatus string) (Order, error) {
	row, err := r.queries.HoldOrderFrom(ctx, db.HoldOrderFromParams{ID: orderID, FromStatus: fromStatus})
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, errStatusRaced(fromStatus, OrderStatusOnHold)
	}
	if err != nil {
		return Order{}, fmt.Errorf("hold order: %w", err)
	}
	return r.GetOrderByID(ctx, row.ID)
}

func (r *postgresRepository) ReleaseOrderToHeldFrom(ctx context.Context, orderID uuid.UUID) (Order, error) {
	row, err := r.queries.ReleaseOrderToHeldFrom(ctx, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, errStatusRaced(OrderStatusOnHold, "its held-from status")
	}
	if err != nil {
		return Order{}, fmt.Errorf("release order: %w", err)
	}
	return r.GetOrderByID(ctx, row.ID)
}

func (r *postgresRepository) CreateReturn(ctx context.Context, ret OrderReturn) (OrderReturn, error) {
	var adminID uuid.NullUUID
	if ret.AdminID != nil {
		adminID = uuid.NullUUID{UUID: *ret.AdminID, Valid: true}
	}
	row, err := r.queries.CreateOrderReturn(ctx, db.CreateOrderReturnParams{
		OrderID:      ret.OrderID,
		Status:       ret.Status,
		Reason:       ret.Reason,
		Restock:      ret.Restock,
		RefundTarget: ret.RefundTarget,
		AdminID:      adminID,
		Resolution:   ret.Resolution,
	})
	if err != nil {
		return OrderReturn{}, fmt.Errorf("create return: %w", err)
	}
	return toOrderReturn(row), nil
}

func (r *postgresRepository) SetReturnExchangeOrder(ctx context.Context, returnID, orderID uuid.UUID) (OrderReturn, error) {
	row, err := r.queries.SetOrderReturnExchangeOrder(ctx, db.SetOrderReturnExchangeOrderParams{
		ID:              returnID,
		ExchangeOrderID: uuid.NullUUID{UUID: orderID, Valid: true},
	})
	if err != nil {
		return OrderReturn{}, fmt.Errorf("set return exchange order: %w", err)
	}
	return toOrderReturn(row), nil
}

func (r *postgresRepository) AddReturnLine(ctx context.Context, returnID uuid.UUID, line OrderReturnLine) (OrderReturnLine, error) {
	row, err := r.queries.AddOrderReturnLine(ctx, db.AddOrderReturnLineParams{
		ReturnID:        returnID,
		OrderLineID:     line.OrderLineID,
		Quantity:        line.Quantity,
		Reason:          line.Reason,
		RefundUnitCents: line.RefundUnitCents,
	})
	if err != nil {
		return OrderReturnLine{}, fmt.Errorf("add return line: %w", err)
	}
	return toOrderReturnLine(row), nil
}

func (r *postgresRepository) GetReturn(ctx context.Context, id uuid.UUID) (OrderReturn, error) {
	row, err := r.queries.GetOrderReturn(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return OrderReturn{}, ErrReturnNotFound
	}
	if err != nil {
		return OrderReturn{}, fmt.Errorf("get return: %w", err)
	}
	ret := toOrderReturn(row)
	lineRows, err := r.queries.ListOrderReturnLinesByReturnIDs(ctx, []uuid.UUID{id})
	if err != nil {
		return OrderReturn{}, fmt.Errorf("get return lines: %w", err)
	}
	for _, lr := range lineRows {
		ret.Lines = append(ret.Lines, toOrderReturnLine(lr))
	}
	return ret, nil
}

func (r *postgresRepository) ListReturns(ctx context.Context, orderID uuid.UUID) ([]OrderReturn, error) {
	rows, err := r.queries.ListOrderReturns(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list returns: %w", err)
	}
	out := make([]OrderReturn, len(rows))
	ids := make([]uuid.UUID, len(rows))
	idx := make(map[uuid.UUID]int, len(rows))
	for i, row := range rows {
		out[i] = toOrderReturn(row)
		ids[i] = row.ID
		idx[row.ID] = i
	}
	if len(ids) == 0 {
		return out, nil
	}
	lineRows, err := r.queries.ListOrderReturnLinesByReturnIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list return lines: %w", err)
	}
	for _, lr := range lineRows {
		if i, ok := idx[lr.ReturnID]; ok {
			out[i].Lines = append(out[i].Lines, toOrderReturnLine(lr))
		}
	}
	return out, nil
}

func (r *postgresRepository) UpdateReturnStatus(ctx context.Context, id uuid.UUID, from, status string) (OrderReturn, error) {
	row, err := r.queries.UpdateOrderReturnStatus(ctx, db.UpdateOrderReturnStatusParams{ID: id, FromStatus: from, Status: status})
	if errors.Is(err, sql.ErrNoRows) {
		return OrderReturn{}, apperr.New("RETURN_STATUS_RACED", apperr.ClassConflict,
			fmt.Sprintf("the return left %q before it could move to %q - reload it and try again", from, status))
	}
	if err != nil {
		return OrderReturn{}, fmt.Errorf("update return status: %w", err)
	}
	return toOrderReturn(row), nil
}

func (r *postgresRepository) MarkReturnRefunded(ctx context.Context, id uuid.UUID, refundAmount int64, giftCardID *uuid.UUID) (OrderReturn, error) {
	var gc uuid.NullUUID
	if giftCardID != nil {
		gc = uuid.NullUUID{UUID: *giftCardID, Valid: true}
	}
	row, err := r.queries.MarkOrderReturnRefunded(ctx, db.MarkOrderReturnRefundedParams{
		ID:                id,
		RefundAmountCents: refundAmount,
		IssuedGiftCardID:  gc,
	})
	if err != nil {
		return OrderReturn{}, fmt.Errorf("mark return refunded: %w", err)
	}
	return toOrderReturn(row), nil
}

func (r *postgresRepository) InsertTaxLine(ctx context.Context, orderID uuid.UUID, line TaxLine) error {
	var ruleID uuid.NullUUID
	if line.RuleID != nil {
		ruleID = uuid.NullUUID{UUID: *line.RuleID, Valid: true}
	}
	if err := r.queries.InsertOrderTaxLine(ctx, db.InsertOrderTaxLineParams{
		OrderID:     orderID,
		RuleID:      ruleID,
		Name:        line.Name,
		RateBps:     line.RateBPS,
		AmountCents: line.AmountCents,
	}); err != nil {
		return fmt.Errorf("insert order tax line: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListTaxLines(ctx context.Context, orderID uuid.UUID) ([]TaxLine, error) {
	rows, err := r.queries.ListOrderTaxLines(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list order tax lines: %w", err)
	}
	out := make([]TaxLine, len(rows))
	for i, row := range rows {
		out[i] = TaxLine{
			ID:          row.ID,
			Name:        row.Name,
			RateBPS:     row.RateBps,
			AmountCents: row.AmountCents,
		}
		if row.RuleID.Valid {
			id := row.RuleID.UUID
			out[i].RuleID = &id
		}
	}
	return out, nil
}

func (r *postgresRepository) DeleteTaxLines(ctx context.Context, orderID uuid.UUID) error {
	if err := r.queries.DeleteOrderTaxLines(ctx, orderID); err != nil {
		return fmt.Errorf("delete order tax lines: %w", err)
	}
	return nil
}

func (r *postgresRepository) ReturnedByOrderLine(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]float64, error) {
	rows, err := r.queries.SumReturnedQtyByOrderLine(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("sum returned by order line: %w", err)
	}
	out := make(map[uuid.UUID]float64, len(rows))
	for _, row := range rows {
		out[row.OrderLineID] = row.Returned
	}
	return out, nil
}

func toOrderReturn(row db.OrderReturn) OrderReturn {
	ret := OrderReturn{
		ID:                row.ID,
		OrderID:           row.OrderID,
		RMANumber:         row.RmaNumber,
		Status:            row.Status,
		Reason:            row.Reason,
		Restock:           row.Restock,
		RefundTarget:      row.RefundTarget,
		RefundAmountCents: row.RefundAmountCents,
		Resolution:        row.Resolution,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.ExchangeOrderID.Valid {
		id := row.ExchangeOrderID.UUID
		ret.ExchangeOrderID = &id
	}
	if row.IssuedGiftCardID.Valid {
		id := row.IssuedGiftCardID.UUID
		ret.IssuedGiftCardID = &id
	}
	if row.AdminID.Valid {
		id := row.AdminID.UUID
		ret.AdminID = &id
	}
	return ret
}

func toOrderReturnLine(row db.OrderReturnLine) OrderReturnLine {
	return OrderReturnLine{
		ID:              row.ID,
		ReturnID:        row.ReturnID,
		OrderLineID:     row.OrderLineID,
		Quantity:        row.Quantity,
		Reason:          row.Reason,
		RefundUnitCents: row.RefundUnitCents,
	}
}

func (r *postgresRepository) CreateOrder(ctx context.Context, o Order) (Order, error) {
	id := o.ID
	if id == (uuid.UUID{}) {
		v, err := uuid.NewV7()
		if err != nil {
			return Order{}, fmt.Errorf("allocate order id: %w", err)
		}
		id = v
	}
	var customerID uuid.NullUUID
	if o.CustomerID != nil {
		customerID = uuid.NullUUID{UUID: *o.CustomerID, Valid: true}
	}
	row, err := r.queries.CreateOrder(ctx, db.CreateOrderParams{
		ID:                  id,
		CustomerID:          customerID,
		Email:               o.Email,
		ChannelID:           o.ChannelID,
		Status:              o.Status,
		PaymentStatus:       o.PaymentStatus,
		BillingFirstName:    o.BillingAddress.FirstName,
		BillingLastName:     o.BillingAddress.LastName,
		BillingCompany:      sqlnull.String(o.BillingAddress.Company),
		BillingStreet1:      o.BillingAddress.Street1,
		BillingStreet2:      sqlnull.String(o.BillingAddress.Street2),
		BillingCity:         o.BillingAddress.City,
		BillingState:        sqlnull.String(o.BillingAddress.State),
		BillingPostalCode:   o.BillingAddress.PostalCode,
		BillingCountry:      o.BillingAddress.CountryCode,
		BillingPhone:        sqlnull.String(o.BillingAddress.Phone),
		ShippingFirstName:   o.ShippingAddress.FirstName,
		ShippingLastName:    o.ShippingAddress.LastName,
		ShippingCompany:     sqlnull.String(o.ShippingAddress.Company),
		ShippingStreet1:     o.ShippingAddress.Street1,
		ShippingStreet2:     sqlnull.String(o.ShippingAddress.Street2),
		ShippingCity:        o.ShippingAddress.City,
		ShippingState:       sqlnull.String(o.ShippingAddress.State),
		ShippingPostalCode:  o.ShippingAddress.PostalCode,
		ShippingCountry:     o.ShippingAddress.CountryCode,
		ShippingPhone:       sqlnull.String(o.ShippingAddress.Phone),
		SubtotalAmount:      o.SubtotalAmount,
		ShippingAmount:      o.ShippingAmount,
		DiscountAmount:      o.DiscountAmount,
		TaxAmount:           o.TaxAmount,
		ShippingTaxAmount:   o.ShippingTaxAmount,
		TotalAmount:         o.TotalAmount,
		CurrencyCode:        o.CurrencyCode,
		CustomerNote:        sqlnull.StringValueNonEmpty(o.CustomerNote),
		PoNumber:            sqlnull.StringValueNonEmpty(o.PONumber),
		BusinessIds:         businessIDsJSON(o.BusinessIDs),
		PaymentProvider:     o.PaymentProvider,
		ShippingMethodID:    sqlnull.UUID(o.ShippingMethodID),
		ShippingCarrierSlug: sqlnull.String(o.ShippingCarrierSlug),
		ShippingMethodCode:  sqlnull.String(o.ShippingMethodCode),
		AnonID:              o.AnonID,
	})
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) UpdateOrderLineQuantity(ctx context.Context, lineID uuid.UUID, quantity float64, totalPrice int64) (OrderLine, error) {
	row, err := r.queries.UpdateOrderLineQuantity(ctx, db.UpdateOrderLineQuantityParams{
		ID:         lineID,
		Quantity:   quantity,
		TotalPrice: totalPrice,
	})
	if err != nil {
		return OrderLine{}, fmt.Errorf("update order line quantity: %w", err)
	}
	return toOrderLine(row), nil
}

func (r *postgresRepository) SoftDeleteOrderLine(ctx context.Context, lineID uuid.UUID) (OrderLine, error) {
	row, err := r.queries.SoftDeleteOrderLine(ctx, lineID)
	if err != nil {
		return OrderLine{}, fmt.Errorf("soft delete order line: %w", err)
	}
	return toOrderLine(row), nil
}

func (r *postgresRepository) UpdateOrderAmounts(ctx context.Context, id uuid.UUID, a OrderAmounts) (Order, error) {
	row, err := r.queries.UpdateOrderAmounts(ctx, db.UpdateOrderAmountsParams{
		ID:                id,
		SubtotalAmount:    a.Subtotal,
		ShippingAmount:    a.Shipping,
		DiscountAmount:    a.Discount,
		TaxAmount:         a.Tax,
		ShippingTaxAmount: a.ShippingTax,
		TotalAmount:       a.Total,
	})
	if err != nil {
		return Order{}, fmt.Errorf("update order amounts: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) AddOrderLine(ctx context.Context, line OrderLine) (OrderLine, error) {
	costCents := sql.NullInt64{}
	if line.CostCents != nil {
		costCents = sql.NullInt64{Int64: *line.CostCents, Valid: true}
	}
	row, err := r.queries.CreateOrderLine(ctx, db.CreateOrderLineParams{
		ID:          uuid.NullUUID{UUID: line.ID, Valid: line.ID != uuid.Nil},
		OrderID:     line.OrderID,
		ProductName: line.ProductName,
		ProductSku:  line.ProductSku,
		UnitPrice:   line.UnitPrice,
		Quantity:    line.Quantity,
		TotalPrice:  line.TotalPrice,
		CostCents:   costCents,
		IsGift:      line.IsGift,
		IsCustom:    line.IsCustom,
		TaxClass:    line.TaxClass,
	})
	if err != nil {
		return OrderLine{}, fmt.Errorf("add order line: %w", err)
	}
	return toOrderLine(row), nil
}

func (r *postgresRepository) AddOrderLineComponent(ctx context.Context, c OrderLineComponent) (OrderLineComponent, error) {
	row, err := r.queries.InsertOrderLineComponent(ctx, db.InsertOrderLineComponentParams{
		OrderLineID:       c.OrderLineID,
		BundleOptionID:    c.BundleOptionID,
		BundleSelectionID: c.BundleSelectionID,
		ProductID:         c.ProductID,
		ProductSku:        c.ProductSku,
		ProductName:       c.ProductName,
		UnitPrice:         c.UnitPrice,
		Quantity:          c.Quantity,
		SortOrder:         c.SortOrder,
	})
	if err != nil {
		return OrderLineComponent{}, fmt.Errorf("add order line component: %w", err)
	}
	return toOrderLineComponent(row), nil
}

func (r *postgresRepository) AddOrderLineStockParts(ctx context.Context, orderLineID uuid.UUID, parts []OrderLineStockPart) error {
	for i, p := range parts {
		if err := r.queries.InsertOrderLineStockPart(ctx, db.InsertOrderLineStockPartParams{
			OrderLineID: orderLineID,
			ProductID:   p.ProductID,
			Quantity:    p.Quantity,
			Source:      p.Source,
			SortOrder:   int32(i),
		}); err != nil {
			return fmt.Errorf("add order line stock part: %w", err)
		}
	}
	return nil
}

func (r *postgresRepository) AddOrderLineOption(ctx context.Context, o OrderLineOption) (OrderLineOption, error) {
	valueID := uuid.NullUUID{}
	if o.OptionValueID != nil {
		valueID = uuid.NullUUID{UUID: *o.OptionValueID, Valid: true}
	}
	row, err := r.queries.InsertOrderLineOption(ctx, db.InsertOrderLineOptionParams{
		OrderLineID:     o.OrderLineID,
		ProductOptionID: o.ProductOptionID,
		OptionValueID:   valueID,
		InputType:       o.InputType,
		OptionTitle:     o.OptionTitle,
		ValueLabel:      o.ValueLabel,
		TextValue:       o.TextValue,
		PriceDelta:      o.PriceDelta,
		Quantity:        o.Quantity,
		SortOrder:       o.SortOrder,
		FileUrl:         o.FileURL,
	})
	if err != nil {
		return OrderLineOption{}, fmt.Errorf("add order line option: %w", err)
	}
	return toOrderLineOption(row), nil
}

func (r *postgresRepository) UpdateOrderAddresses(ctx context.Context, id uuid.UUID, billing, shipping OrderAddress) (Order, error) {
	row, err := r.queries.UpdateOrderAddresses(ctx, db.UpdateOrderAddressesParams{
		ID:                 id,
		BillingFirstName:   billing.FirstName,
		BillingLastName:    billing.LastName,
		BillingCompany:     sqlnull.String(billing.Company),
		BillingStreet1:     billing.Street1,
		BillingStreet2:     sqlnull.String(billing.Street2),
		BillingCity:        billing.City,
		BillingState:       sqlnull.String(billing.State),
		BillingPostalCode:  billing.PostalCode,
		BillingCountry:     billing.CountryCode,
		BillingPhone:       sqlnull.String(billing.Phone),
		ShippingFirstName:  shipping.FirstName,
		ShippingLastName:   shipping.LastName,
		ShippingCompany:    sqlnull.String(shipping.Company),
		ShippingStreet1:    shipping.Street1,
		ShippingStreet2:    sqlnull.String(shipping.Street2),
		ShippingCity:       shipping.City,
		ShippingState:      sqlnull.String(shipping.State),
		ShippingPostalCode: shipping.PostalCode,
		ShippingCountry:    shipping.CountryCode,
		ShippingPhone:      sqlnull.String(shipping.Phone),
	})
	if err != nil {
		return Order{}, fmt.Errorf("update order addresses: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) AddOrderEvent(ctx context.Context, event OrderEvent) (OrderEvent, error) {
	// Attribute to the acting admin when the call carries admin claims - one stamp covers every call site
	actor := sql.NullString{String: event.ActorEmail, Valid: event.ActorEmail != ""}
	if !actor.Valid {
		if claims, ok := auth.AdminFromContext(ctx); ok && claims.Email != "" {
			actor = sql.NullString{String: claims.Email, Valid: true}
		}
	}
	row, err := r.queries.CreateOrderEvent(ctx, db.CreateOrderEventParams{
		OrderID:    event.OrderID,
		Type:       event.Type,
		Message:    event.Message,
		ActorEmail: actor,
	})
	if err != nil {
		return OrderEvent{}, fmt.Errorf("add order event: %w", err)
	}
	return toOrderEvent(row), nil
}

func (r *postgresRepository) GetOrderByID(ctx context.Context, id uuid.UUID) (Order, error) {
	row, err := r.queries.GetOrderByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("get order: %w", err)
	}
	return r.hydrateOrder(ctx, toOrder(row))
}

// hydrateOrder fills the child collections a bare orders row lacks - lines (with
// options and bundle components) and the event timeline - Every single-order read goes through here, so id and guest (id, email) return the same shape
func (r *postgresRepository) hydrateOrder(ctx context.Context, o Order) (Order, error) {
	lines, err := r.queries.ListOrderLinesByOrder(ctx, o.ID)
	if err != nil {
		return Order{}, fmt.Errorf("load order lines: %w", err)
	}
	for _, l := range lines {
		o.Lines = append(o.Lines, toOrderLine(l))
	}
	if err := r.attachOrderLineOptions(ctx, o.Lines); err != nil {
		return Order{}, err
	}
	if err := r.attachOrderLineComponents(ctx, o.Lines); err != nil {
		return Order{}, err
	}
	if err := r.attachOrderLineStockParts(ctx, o.Lines); err != nil {
		return Order{}, err
	}
	events, err := r.queries.ListOrderEventsByOrder(ctx, o.ID)
	if err != nil {
		return Order{}, fmt.Errorf("load order events: %w", err)
	}
	for _, e := range events {
		o.Events = append(o.Events, toOrderEvent(e))
	}
	return o, nil
}

func (r *postgresRepository) ListOrderLinesByOrder(ctx context.Context, orderID uuid.UUID) ([]OrderLine, error) {
	rows, err := r.queries.ListOrderLinesByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list order lines: %w", err)
	}
	lines := make([]OrderLine, len(rows))
	for i, row := range rows {
		lines[i] = toOrderLine(row)
	}
	return lines, nil
}

func (r *postgresRepository) ListOrderEventsByOrder(ctx context.Context, orderID uuid.UUID) ([]OrderEvent, error) {
	rows, err := r.queries.ListOrderEventsByOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("list order events: %w", err)
	}
	events := make([]OrderEvent, len(rows))
	for i, row := range rows {
		events[i] = toOrderEvent(row)
	}
	return events, nil
}

func (r *postgresRepository) ListOrdersPage(ctx context.Context, f ListPageFilter) ([]Order, int64, error) {
	allStatuses, allPayment := len(f.Statuses) == 0, len(f.PaymentStatuses) == 0
	var (
		rows  []db.Order
		total int64
		err   error
	)
	if len(f.ChannelIDs) == 0 {
		rows, err = r.queries.ListOrdersPaged(ctx, db.ListOrdersPagedParams{
			Search: f.Search, AllStatuses: allStatuses, Statuses: f.Statuses,
			AllPayment: allPayment, PaymentStatuses: f.PaymentStatuses, Lim: f.Limit, Off: f.Offset,
		})
		if err == nil && !f.SkipCount {
			total, err = r.queries.CountOrdersPaged(ctx, db.CountOrdersPagedParams{
				Search: f.Search, AllStatuses: allStatuses, Statuses: f.Statuses,
				AllPayment: allPayment, PaymentStatuses: f.PaymentStatuses,
			})
		}
	} else {
		rows, err = r.queries.ListOrdersPagedInChannels(ctx, db.ListOrdersPagedInChannelsParams{
			ChannelIds: f.ChannelIDs, Search: f.Search, AllStatuses: allStatuses, Statuses: f.Statuses,
			AllPayment: allPayment, PaymentStatuses: f.PaymentStatuses, Lim: f.Limit, Off: f.Offset,
		})
		if err == nil && !f.SkipCount {
			total, err = r.queries.CountOrdersPagedInChannels(ctx, db.CountOrdersPagedInChannelsParams{
				ChannelIds: f.ChannelIDs, Search: f.Search, AllStatuses: allStatuses, Statuses: f.Statuses,
				AllPayment: allPayment, PaymentStatuses: f.PaymentStatuses,
			})
		}
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list orders page: %w", err)
	}
	orders := make([]Order, len(rows))
	for i, row := range rows {
		orders[i] = toOrder(row)
	}
	return orders, total, nil
}

func (r *postgresRepository) ListOrdersAwaitingPaymentSync(ctx context.Context, notBefore, olderThan time.Time, maxRows int32) ([]Order, error) {
	rows, err := r.queries.ListOrdersAwaitingPaymentSync(ctx, db.ListOrdersAwaitingPaymentSyncParams{
		NotBefore: notBefore,
		OlderThan: olderThan,
		MaxRows:   maxRows,
	})
	if err != nil {
		return nil, fmt.Errorf("list orders awaiting payment sync: %w", err)
	}
	orders := make([]Order, len(rows))
	for i, row := range rows {
		orders[i] = toOrder(row)
	}
	return orders, nil
}

func (r *postgresRepository) ListOrdersByCustomer(ctx context.Context, customerID uuid.UUID) ([]Order, error) {
	rows, err := r.queries.ListOrdersByCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list orders by customer: %w", err)
	}
	orders := make([]Order, len(rows))
	for i, row := range rows {
		orders[i] = toOrder(row)
	}
	return orders, nil
}

func (r *postgresRepository) GetGuestOrder(ctx context.Context, id uuid.UUID, email string) (Order, error) {
	row, err := r.queries.GetGuestOrder(ctx, db.GetGuestOrderParams{ID: id, Email: email})
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("get guest order: %w", err)
	}
	return r.hydrateOrder(ctx, toOrder(row))
}

func (r *postgresRepository) UpdateOrderTracking(ctx context.Context, id uuid.UUID, carrier, trackingNumber string) (Order, error) {
	row, err := r.queries.UpdateOrderTracking(ctx, db.UpdateOrderTrackingParams{
		ID:             id,
		Carrier:        carrier,
		TrackingNumber: trackingNumber,
	})
	if err != nil {
		return Order{}, fmt.Errorf("update order tracking: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) SetStoreCreditTender(ctx context.Context, id uuid.UUID, storeCreditAmount int64, paymentStatus string) (Order, error) {
	row, err := r.queries.SetOrderStoreCreditTender(ctx, db.SetOrderStoreCreditTenderParams{
		ID:                id,
		StoreCreditAmount: storeCreditAmount,
		PaymentStatus:     paymentStatus,
	})
	if err != nil {
		return Order{}, fmt.Errorf("set order store credit tender: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) SetGiftCardTender(ctx context.Context, id uuid.UUID, giftCardAmount int64, paymentStatus string) (Order, error) {
	row, err := r.queries.SetOrderGiftCardTender(ctx, db.SetOrderGiftCardTenderParams{
		ID:             id,
		GiftCardAmount: giftCardAmount,
		PaymentStatus:  paymentStatus,
	})
	if err != nil {
		return Order{}, fmt.Errorf("set order gift card tender: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) SetPaymentState(ctx context.Context, id uuid.UUID, paymentStatus, paymentReference string) (Order, error) {
	row, err := r.queries.SetOrderPaymentState(ctx, db.SetOrderPaymentStateParams{
		ID:               id,
		PaymentStatus:    paymentStatus,
		PaymentReference: paymentReference,
	})
	if err != nil {
		return Order{}, fmt.Errorf("set order payment state: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) SetPaymentStateFrom(ctx context.Context, id uuid.UUID, from, paymentStatus, paymentReference string) (Order, bool, error) {
	row, err := r.queries.SetOrderPaymentStateFrom(ctx, db.SetOrderPaymentStateFromParams{
		ID:               id,
		FromStatus:       from,
		PaymentStatus:    paymentStatus,
		PaymentReference: paymentReference,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, fmt.Errorf("set order payment state: %w", err)
	}
	return toOrder(row), true, nil
}

func (r *postgresRepository) SetPaymentReference(ctx context.Context, id uuid.UUID, paymentReference string) (Order, error) {
	row, err := r.queries.SetOrderPaymentReference(ctx, db.SetOrderPaymentReferenceParams{ID: id, PaymentReference: paymentReference})
	if err != nil {
		return Order{}, fmt.Errorf("set order payment reference: %w", err)
	}
	return toOrder(row), nil
}

func (r *postgresRepository) TrackOrderLookup(ctx context.Context, number, email, postalCode string) (TrackedOrder, error) {
	row, err := r.queries.TrackOrderLookup(ctx, db.TrackOrderLookupParams{Number: number, Email: email, PostalCode: postalCode})
	if errors.Is(err, sql.ErrNoRows) {
		return TrackedOrder{}, ErrOrderNotFound
	}
	if err != nil {
		return TrackedOrder{}, fmt.Errorf("track order lookup: %w", err)
	}
	return TrackedOrder{ID: row.ID, ChannelID: row.ChannelID, Email: row.Email, HasAccount: row.HasAccount}, nil
}

func (r *postgresRepository) ClaimGuestOrdersByEmail(ctx context.Context, customerID uuid.UUID, email string, channelID uuid.UUID) (int64, error) {
	n, err := r.queries.ClaimGuestOrdersByEmail(ctx, db.ClaimGuestOrdersByEmailParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
		Email:      email,
		ChannelID:  channelID,
	})
	if err != nil {
		return 0, fmt.Errorf("claim guest orders: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) UpdateOrderStatusFrom(ctx context.Context, id uuid.UUID, fromStatus, status string) (Order, error) {
	row, err := r.queries.UpdateOrderStatusFrom(ctx, db.UpdateOrderStatusFromParams{
		ID: id, FromStatus: fromStatus, NewStatus: status,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, errStatusRaced(fromStatus, status)
	}
	if err != nil {
		return Order{}, fmt.Errorf("update order status: %w", err)
	}
	return r.GetOrderByID(ctx, row.ID)
}

// --- mappers ---

func toOrder(row db.Order) Order {
	var customerID *uuid.UUID
	if row.CustomerID.Valid {
		c := row.CustomerID.UUID
		customerID = &c
	}
	o := Order{
		ID:               row.ID,
		CustomerID:       customerID,
		Email:            row.Email,
		ChannelID:        row.ChannelID,
		Number:           row.Number,
		Status:           row.Status,
		PaymentStatus:    row.PaymentStatus,
		PaymentProvider:  row.PaymentProvider,
		PaymentReference: row.PaymentReference,
		BillingAddress: OrderAddress{
			FirstName:   row.BillingFirstName,
			LastName:    row.BillingLastName,
			Company:     sqlnull.StringPtr(row.BillingCompany),
			Street1:     row.BillingStreet1,
			Street2:     sqlnull.StringPtr(row.BillingStreet2),
			City:        row.BillingCity,
			State:       sqlnull.StringPtr(row.BillingState),
			PostalCode:  row.BillingPostalCode,
			CountryCode: row.BillingCountry,
			Phone:       sqlnull.StringPtr(row.BillingPhone),
		},
		ShippingAddress: OrderAddress{
			FirstName:   row.ShippingFirstName,
			LastName:    row.ShippingLastName,
			Company:     sqlnull.StringPtr(row.ShippingCompany),
			Street1:     row.ShippingStreet1,
			Street2:     sqlnull.StringPtr(row.ShippingStreet2),
			City:        row.ShippingCity,
			State:       sqlnull.StringPtr(row.ShippingState),
			PostalCode:  row.ShippingPostalCode,
			CountryCode: row.ShippingCountry,
			Phone:       sqlnull.StringPtr(row.ShippingPhone),
		},
		SubtotalAmount:    row.SubtotalAmount,
		ShippingAmount:    row.ShippingAmount,
		DiscountAmount:    row.DiscountAmount,
		TaxAmount:         row.TaxAmount,
		ShippingTaxAmount: row.ShippingTaxAmount,
		TotalAmount:       row.TotalAmount,
		AnonID:            row.AnonID,
		GiftCardAmount:    row.GiftCardAmount,
		StoreCreditAmount: row.StoreCreditAmount,
		CurrencyCode:      row.CurrencyCode,
		CustomerNote:      row.CustomerNote.String,
		PONumber:          row.PoNumber.String,
		FulfillmentNeed:   row.FulfillmentNeed,
		BusinessIDs:       decodeBusinessIDs(row.BusinessIds),
		Carrier:           row.Carrier,
		TrackingNumber:    row.TrackingNumber,
		CreatedAt:         row.CreatedAt,
		UpdatedAt:         row.UpdatedAt,
	}
	if row.ShippingMethodID.Valid {
		id := row.ShippingMethodID.UUID
		o.ShippingMethodID = &id
	}
	o.ShippingCarrierSlug = sqlnull.StringPtr(row.ShippingCarrierSlug)
	o.ShippingMethodCode = sqlnull.StringPtr(row.ShippingMethodCode)
	return o
}

func toOrderLine(row db.OrderLine) OrderLine {
	return OrderLine{
		ID:          row.ID,
		OrderID:     row.OrderID,
		ProductName: row.ProductName,
		ProductSku:  row.ProductSku,
		UnitPrice:   row.UnitPrice,
		Quantity:    row.Quantity,
		TotalPrice:  row.TotalPrice,
		IsGift:      row.IsGift,
		IsCustom:    row.IsCustom,
		TaxClass:    row.TaxClass,
		CreatedAt:   row.CreatedAt,
	}
}

func toOrderLineComponent(row db.OrderLineComponent) OrderLineComponent {
	return OrderLineComponent{
		ID:                row.ID,
		OrderLineID:       row.OrderLineID,
		BundleOptionID:    row.BundleOptionID,
		BundleSelectionID: row.BundleSelectionID,
		ProductID:         row.ProductID,
		ProductSku:        row.ProductSku,
		ProductName:       row.ProductName,
		UnitPrice:         row.UnitPrice,
		Quantity:          row.Quantity,
		SortOrder:         row.SortOrder,
		CreatedAt:         row.CreatedAt,
	}
}

// attachOrderLineOptions hydrates each line's option snapshot in one batched query (no-op when there are none)
func (r *postgresRepository) attachOrderLineOptions(ctx context.Context, lines []OrderLine) error {
	if len(lines) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(lines))
	for i := range lines {
		ids[i] = lines[i].ID
	}
	rows, err := r.queries.ListOrderLineOptionsByOrderLineIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("load order line options: %w", err)
	}
	byLine := make(map[uuid.UUID][]OrderLineOption, len(lines))
	for _, row := range rows {
		o := toOrderLineOption(row)
		byLine[o.OrderLineID] = append(byLine[o.OrderLineID], o)
	}
	for i := range lines {
		lines[i].Options = byLine[lines[i].ID]
	}
	return nil
}

// Without this a re-read bundle line looks simple, which made syntheticCart re-reserve nothing for its components and the edit guard that reads len(Components) never fire
func (r *postgresRepository) attachOrderLineComponents(ctx context.Context, lines []OrderLine) error {
	if len(lines) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(lines))
	for i := range lines {
		ids[i] = lines[i].ID
	}
	rows, err := r.queries.ListOrderLineComponentsByOrderLineIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("load order line components: %w", err)
	}
	byLine := make(map[uuid.UUID][]OrderLineComponent, len(lines))
	for _, row := range rows {
		c := toOrderLineComponent(row)
		byLine[c.OrderLineID] = append(byLine[c.OrderLineID], c)
	}
	for i := range lines {
		lines[i].Components = byLine[lines[i].ID]
	}
	return nil
}

func (r *postgresRepository) attachOrderLineStockParts(ctx context.Context, lines []OrderLine) error {
	if len(lines) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(lines))
	for i := range lines {
		ids[i] = lines[i].ID
	}
	rows, err := r.queries.ListOrderLineStockPartsByOrderLineIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("load order line stock parts: %w", err)
	}
	byLine := make(map[uuid.UUID][]OrderLineStockPart, len(lines))
	for _, row := range rows {
		byLine[row.OrderLineID] = append(byLine[row.OrderLineID], OrderLineStockPart{ProductID: row.ProductID, Quantity: row.Quantity, Source: row.Source})
	}
	for i := range lines {
		lines[i].StockParts = byLine[lines[i].ID]
	}
	return nil
}

func toOrderLineOption(row db.OrderLineOption) OrderLineOption {
	o := OrderLineOption{
		ID:              row.ID,
		OrderLineID:     row.OrderLineID,
		ProductOptionID: row.ProductOptionID,
		InputType:       row.InputType,
		OptionTitle:     row.OptionTitle,
		ValueLabel:      row.ValueLabel,
		TextValue:       row.TextValue,
		PriceDelta:      row.PriceDelta,
		Quantity:        row.Quantity,
		SortOrder:       row.SortOrder,
		FileURL:         row.FileUrl,
		CreatedAt:       row.CreatedAt,
	}
	if row.OptionValueID.Valid {
		v := row.OptionValueID.UUID
		o.OptionValueID = &v
	}
	return o
}

func toOrderEvent(row db.OrderEvent) OrderEvent {
	return OrderEvent{
		ID:         row.ID,
		OrderID:    row.OrderID,
		Type:       row.Type,
		Message:    row.Message,
		ActorEmail: row.ActorEmail.String,
		CreatedAt:  row.CreatedAt,
	}
}

func businessIDsJSON(ids []BusinessID) json.RawMessage {
	if ids == nil {
		ids = []BusinessID{}
	}
	b, _ := json.Marshal(ids) // plain strings - can't fail
	return b
}

// Only placement writes the column, so an undecodable one reads as none rather than failing the order
func decodeBusinessIDs(raw json.RawMessage) []BusinessID {
	var ids []BusinessID
	if json.Unmarshal(raw, &ids) != nil {
		return nil
	}
	return ids
}

// Package orderio is the order-lines entry in the data registry - an accounting-shaped export only, since orders are never created from a spreadsheet
package orderio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

const (
	optStatuses  = "statuses"
	optStartDate = "start_date"
	optEndDate   = "end_date"

	dateOnly = "2006-01-02"

	exportPageSize = 500
)

const (
	colOrderNumber     = "order_number"
	colCreatedAt       = "created_at"
	colStatus          = "status"
	colPaymentStatus   = "payment_status"
	colPaymentProvider = "payment_provider"
	colEmail           = "email"
	colCurrency        = "currency"
	colSKU             = "sku"
	colProductName     = "product_name"
	colQuantity        = "quantity"
	colUnitPrice       = "unit_price_cents"
	colLineTotal       = "line_total_cents"
	colLineCost        = "line_cost_cents"
	colLineOptions     = "line_options"
	colOrderSubtotal   = "order_subtotal_cents"
	colOrderDiscount   = "order_discount_cents"
	colOrderShipping   = "order_shipping_cents"
	colOrderTax        = "order_tax_cents"
	colOrderGiftCard   = "order_giftcard_cents"
	colOrderTotal      = "order_total_cents"
)

var orderFields = []dataio.Field{
	{Name: colOrderNumber, Label: "Order", Type: dataio.FieldText, Identifier: true, ReadOnly: true},
	{Name: colCreatedAt, Label: "Placed", Type: dataio.FieldDate, ReadOnly: true},
	{Name: colStatus, Label: "Status", Type: dataio.FieldText, ReadOnly: true},
	{Name: colPaymentStatus, Label: "Payment status", Type: dataio.FieldText, ReadOnly: true},
	{Name: colPaymentProvider, Label: "Payment provider", Type: dataio.FieldText, ReadOnly: true},
	{Name: colEmail, Label: "Customer email", Type: dataio.FieldText, ReadOnly: true},
	{Name: colCurrency, Label: "Currency", Type: dataio.FieldText, ReadOnly: true},
	{Name: colSKU, Label: "SKU", Type: dataio.FieldText, ReadOnly: true},
	{Name: colProductName, Label: "Product", Type: dataio.FieldText, ReadOnly: true},
	{Name: colQuantity, Label: "Quantity", Type: dataio.FieldNumber, ReadOnly: true},
	{Name: colUnitPrice, Label: "Unit price", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colLineTotal, Label: "Line total", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colLineCost, Label: "Line cost", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colLineOptions, Label: "Options", Type: dataio.FieldText, ReadOnly: true},
	{Name: colOrderSubtotal, Label: "Order subtotal", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colOrderDiscount, Label: "Order discount", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colOrderShipping, Label: "Order shipping", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colOrderTax, Label: "Order tax", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colOrderGiftCard, Label: "Order gift card", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: colOrderTotal, Label: "Order total", Type: dataio.FieldMoney, ReadOnly: true},
}

var (
	errBadDate   = errors.New("want a date (2006-01-02) or an RFC 3339 timestamp")
	errBadStatus = errors.New("not an order status")
	// A From after the To exported an empty file with nothing to say why
	errBadRange = errors.New("the From date is after the To date")
)

// OrderLineResource is one row per order line, shaped for an accounting hand-off - export only
// Customer emails ride along on every row, so reading it takes the customer export's grant as well as orders.view
func OrderLineResource(queries *db.Queries) dataio.Resource {
	return dataio.Resource{
		Slug:             "order_lines",
		Label:            "Order lines",
		Group:            "Sales",
		Description:      "One row per line on every order in range, with the order totals repeated on each - the shape an accounting system wants.",
		ViewPermission:   "orders.view",
		ViewAlsoRequires: []string{"customers.manage"},
		Fields:           orderFields,
		Options: []dataio.OptionDef{
			{Name: optStatuses, Label: "Statuses", Type: dataio.OptString, Description: "Comma-separated order statuses, blank for all", Applies: dataio.DirectionExport, Validate: validateStatuses},
			{Name: optStartDate, Label: "From", Type: dataio.OptDate, Description: "Orders placed on or after this date, or this RFC 3339 instant", Applies: dataio.DirectionExport, Validate: validateDate},
			{Name: optEndDate, Label: "To", Type: dataio.OptDate, Description: "Orders placed up to the end of this date, or before this RFC 3339 instant", Applies: dataio.DirectionExport, Validate: validateDate},
		},
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			return newExporter(queries, o, time.Now())
		},
	}
}

type orderExporter struct {
	queries  *db.Queries
	params   db.ExportOrderRowsParams
	pageSize int
}

func newExporter(queries *db.Queries, o dataio.Opts, now time.Time) (*orderExporter, error) {
	statuses, err := parseStatuses(o.Str(optStatuses))
	if err != nil {
		return nil, err
	}
	start, err := parseBound(o.Str(optStartDate), time.Time{}, false)
	if err != nil {
		return nil, err
	}
	// An open-ended range still needs an upper bound the query can compare against
	end, err := parseBound(o.Str(optEndDate), now.Add(24*time.Hour), true)
	if err != nil {
		return nil, err
	}
	if !start.Before(end) {
		return nil, errBadRange
	}
	p := db.ExportOrderRowsParams{
		AllChannels: o.ChannelID == uuid.Nil,
		AllStatuses: len(statuses) == 0,
		Statuses:    statuses,
		StartAt:     start,
		EndAt:       end,
	}
	if !p.AllChannels {
		p.ChannelIds = []uuid.UUID{o.ChannelID}
	}
	return &orderExporter{queries: queries, params: p, pageSize: exportPageSize}, nil
}

func (e *orderExporter) Columns(context.Context) ([]dataio.Field, error) { return nil, nil }

func (e *orderExporter) Stream(ctx context.Context, emit func(*dataio.Row) error) error {
	p := e.params
	p.PageSize = int32(e.pageSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := e.queries.ExportOrderRows(ctx, p)
		if err != nil {
			return err
		}
		for _, src := range rows {
			if err := emit(lineRow(src)); err != nil {
				return err
			}
		}
		if len(rows) < e.pageSize {
			return nil
		}
		last := rows[len(rows)-1]
		p.AfterCreatedAt, p.AfterOrderID, p.AfterLineID = last.CreatedAt, last.OrderID, last.LineID
	}
}

func lineRow(src db.ExportOrderRowsRow) *dataio.Row {
	r := dataio.NewRow()
	r.Ref = src.Number
	r.Set(colOrderNumber, src.Number)
	r.Set(colCreatedAt, src.CreatedAt)
	r.Set(colStatus, src.Status)
	r.Set(colPaymentStatus, src.PaymentStatus)
	r.Set(colPaymentProvider, src.PaymentProvider)
	r.Set(colEmail, src.Email)
	r.Set(colCurrency, src.CurrencyCode)
	r.Set(colSKU, src.ProductSku)
	r.Set(colProductName, src.ProductName)
	r.Set(colQuantity, src.Quantity)
	r.Set(colUnitPrice, src.UnitPrice)
	r.Set(colLineTotal, src.TotalPrice)
	if src.CostCents.Valid {
		r.Set(colLineCost, src.CostCents.Int64)
	}
	r.Set(colLineOptions, src.LineOptions)
	r.Set(colOrderSubtotal, src.SubtotalAmount)
	r.Set(colOrderDiscount, src.DiscountAmount)
	r.Set(colOrderShipping, src.ShippingAmount)
	r.Set(colOrderTax, src.TaxAmount)
	r.Set(colOrderGiftCard, src.GiftCardAmount)
	r.Set(colOrderTotal, src.TotalAmount)
	return r
}

func parseStatuses(raw string) ([]string, error) {
	var out []string
	for s := range strings.SplitSeq(strings.ToLower(raw), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if !order.IsStatus(s) {
			return nil, fmt.Errorf("%w: %q", errBadStatus, s)
		}
		out = append(out, s)
	}
	return out, nil
}

func validateStatuses(raw string) error {
	_, err := parseStatuses(raw)
	return err
}

func validateDate(raw string) error {
	_, err := parseBound(raw, time.Time{}, false)
	return err
}

// A blank value takes the fallback. A date-only upper bound becomes the next midnight, so the bound stays exclusive and still covers that whole day
func parseBound(raw string, fallback time.Time, upper bool) (time.Time, error) {
	if raw == "" {
		return fallback, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	t, err := time.Parse(dateOnly, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q", errBadDate, raw)
	}
	if upper {
		t = t.AddDate(0, 0, 1)
	}
	return t, nil
}

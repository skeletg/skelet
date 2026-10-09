// Package customerio is the customers and order-lines entries in the data registry - mirrors internal/catalogio's role for products
// Customers round-trip both ways (upsert by channel + email); order lines are an accounting-shaped export only, since orders are never created from a spreadsheet
package customerio

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

const (
	colEmail         = "email"
	colFirstName     = "first_name"
	colLastName      = "last_name"
	colPhone         = "phone"
	colGroup         = "group"
	colIsActive      = "is_active"
	colEmailVerified = "email_verified"
	colCreatedAt     = "created_at"
)

var customerFields = []dataio.Field{
	{Name: colEmail, Label: "Email", Type: dataio.FieldText, Identifier: true, Required: true, Description: "The upsert key, together with the channel this run targets", Example: "ada@example.com"},
	{Name: colFirstName, Label: "First name", Type: dataio.FieldText, Example: "Ada"},
	{Name: colLastName, Label: "Last name", Type: dataio.FieldText, Example: "Lovelace"},
	{Name: colPhone, Label: "Phone", Type: dataio.FieldText},
	{Name: colGroup, Label: "Customer group", Type: dataio.FieldText, ReadOnly: true, Description: "Exported for context - group membership is managed in the admin"},
	{Name: colIsActive, Label: "Active", Type: dataio.FieldBool, ReadOnly: true},
	{Name: colEmailVerified, Label: "Email verified", Type: dataio.FieldBool, ReadOnly: true},
	{Name: colCreatedAt, Label: "Created", Type: dataio.FieldDate, ReadOnly: true},
}

// CustomerResource upserts by (channel, email) - a new customer is created with an unusable password and resets to claim the account
func CustomerResource(queries *db.Queries, customers *customer.Service) dataio.Resource {
	return dataio.Resource{
		Slug:             "customers",
		Label:            "Customers",
		Group:            "Customers",
		Description:      "Contact records for one channel. Names and phone numbers update in place; the import never touches passwords or group membership.",
		ViewPermission:   "customers.manage", // PII leaves the system here, so viewing the export needs the manage grant
		ImportPermission: "customers.manage",
		Fields:           customerFields,
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			return &customerExporter{queries: queries, opts: o}, nil
		},
		NewImporter: func(_ context.Context, o dataio.Opts) (dataio.Importer, error) {
			return &customerImporter{customers: customers, opts: o}, nil
		},
	}
}

type customerExporter struct {
	queries *db.Queries
	opts    dataio.Opts
	rows    []db.ExportCustomerRowsRow
}

func (e *customerExporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	var channels []uuid.UUID
	if e.opts.ChannelID != uuid.Nil {
		channels = []uuid.UUID{e.opts.ChannelID}
	}
	rows, err := e.queries.ExportCustomerRows(ctx, db.ExportCustomerRowsParams{
		AllChannels: len(channels) == 0,
		ChannelIds:  channels,
	})
	if err != nil {
		return nil, err
	}
	e.rows = rows
	return nil, nil
}

func (e *customerExporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	for _, src := range e.rows {
		r := dataio.NewRow()
		r.Ref = src.Email
		r.Set(colEmail, src.Email)
		r.Set(colFirstName, src.FirstName)
		r.Set(colLastName, src.LastName)
		r.Set(colPhone, src.Phone.String)
		r.Set(colGroup, src.GroupSlug)
		r.Set(colIsActive, src.IsActive)
		r.Set(colEmailVerified, src.IsEmailVerified)
		r.Set(colCreatedAt, src.CreatedAt)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

type customerImporter struct {
	customers *customer.Service
	opts      dataio.Opts
}

func (im *customerImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	email := strings.ToLower(r.Str(colEmail))
	r.Ref = email
	if email == "" || !strings.Contains(email, "@") {
		return dataio.OutcomeSkipped, errInvalidEmail
	}
	if im.opts.ChannelID == uuid.Nil {
		return dataio.OutcomeSkipped, errChannelRequired
	}
	if im.opts.DryRun {
		if _, exists := im.customers.CustomerIDByEmail(ctx, im.opts.ChannelID, email); exists {
			return dataio.OutcomeUpdated, nil
		}
		return dataio.OutcomeCreated, nil
	}

	c, created, err := im.customers.EnsureContact(ctx, im.opts.ChannelID, email, r.Str(colFirstName), false)
	if err != nil {
		return dataio.OutcomeSkipped, err
	}
	// The file is the operator's source of truth for these three, so they fill in on both paths
	first, last, phone := r.Str(colFirstName), r.Str(colLastName), r.Str(colPhone)
	if first != "" || last != "" || phone != "" {
		upd := customer.UpdateCustomerInput{
			FirstName: firstNonEmpty(first, c.FirstName),
			LastName:  firstNonEmpty(last, c.LastName),
		}
		if phone != "" {
			upd.Phone = &phone
		}
		if _, err := im.customers.UpdateProfile(ctx, c.ID, upd); err != nil {
			return dataio.OutcomeSkipped, err
		}
	}
	if created {
		return dataio.OutcomeCreated, nil
	}
	return dataio.OutcomeUpdated, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

const (
	optStatuses  = "statuses"
	optStartDate = "start_date"
	optEndDate   = "end_date"
)

var orderFields = []dataio.Field{
	{Name: "order_number", Label: "Order", Type: dataio.FieldText, Identifier: true, ReadOnly: true},
	{Name: "created_at", Label: "Placed", Type: dataio.FieldDate, ReadOnly: true},
	{Name: "status", Label: "Status", Type: dataio.FieldText, ReadOnly: true},
	{Name: "payment_status", Label: "Payment status", Type: dataio.FieldText, ReadOnly: true},
	{Name: "payment_provider", Label: "Payment provider", Type: dataio.FieldText, ReadOnly: true},
	{Name: "email", Label: "Customer email", Type: dataio.FieldText, ReadOnly: true},
	{Name: "currency", Label: "Currency", Type: dataio.FieldText, ReadOnly: true},
	{Name: "sku", Label: "SKU", Type: dataio.FieldText, ReadOnly: true},
	{Name: "product_name", Label: "Product", Type: dataio.FieldText, ReadOnly: true},
	{Name: "quantity", Label: "Quantity", Type: dataio.FieldNumber, ReadOnly: true},
	{Name: "unit_price_cents", Label: "Unit price", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "line_total_cents", Label: "Line total", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "line_cost_cents", Label: "Line cost", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "line_options", Label: "Options", Type: dataio.FieldText, ReadOnly: true},
	{Name: "order_subtotal_cents", Label: "Order subtotal", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "order_discount_cents", Label: "Order discount", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "order_shipping_cents", Label: "Order shipping", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "order_tax_cents", Label: "Order tax", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "order_giftcard_cents", Label: "Order gift card", Type: dataio.FieldMoney, ReadOnly: true},
	{Name: "order_total_cents", Label: "Order total", Type: dataio.FieldMoney, ReadOnly: true},
}

// OrderLineResource is one row per order line, shaped for an accounting hand-off - export only
func OrderLineResource(queries *db.Queries) dataio.Resource {
	return dataio.Resource{
		Slug:           "order_lines",
		Label:          "Order lines",
		Group:          "Sales",
		Description:    "One row per line on every order in range, with the order totals repeated on each - the shape an accounting system wants.",
		ViewPermission: "orders.view",
		Fields:         orderFields,
		Options: []dataio.OptionDef{
			{Name: optStatuses, Label: "Statuses", Type: dataio.OptString, Description: "Comma-separated order statuses, blank for all", Applies: dataio.DirectionExport},
			{Name: optStartDate, Label: "From", Type: dataio.OptDate, Applies: dataio.DirectionExport},
			{Name: optEndDate, Label: "To", Type: dataio.OptDate, Applies: dataio.DirectionExport},
		},
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			return &orderExporter{queries: queries, opts: o}, nil
		},
	}
}

type orderExporter struct {
	queries *db.Queries
	opts    dataio.Opts
	rows    []db.ExportOrderRowsRow
}

func (e *orderExporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	var channels []uuid.UUID
	if e.opts.ChannelID != uuid.Nil {
		channels = []uuid.UUID{e.opts.ChannelID}
	}
	var statuses []string
	for s := range strings.SplitSeq(strings.ToLower(e.opts.Str(optStatuses)), ",") {
		if s = strings.TrimSpace(s); s != "" {
			statuses = append(statuses, s)
		}
	}
	start := parseDate(e.opts.Str(optStartDate), time.Time{})
	// An open-ended range still needs an upper bound the query can compare against
	end := parseDate(e.opts.Str(optEndDate), time.Now().Add(24*time.Hour))

	rows, err := e.queries.ExportOrderRows(ctx, db.ExportOrderRowsParams{
		AllChannels: len(channels) == 0,
		ChannelIds:  channels,
		AllStatuses: len(statuses) == 0,
		Statuses:    statuses,
		StartAt:     start,
		EndAt:       end,
	})
	if err != nil {
		return nil, err
	}
	e.rows = rows
	return nil, nil
}

func (e *orderExporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	for _, src := range e.rows {
		r := dataio.NewRow()
		r.Ref = src.Number
		r.Set("order_number", src.Number)
		r.Set("created_at", src.CreatedAt)
		r.Set("status", src.Status)
		r.Set("payment_status", src.PaymentStatus)
		r.Set("payment_provider", src.PaymentProvider)
		r.Set("email", src.Email)
		r.Set("currency", src.CurrencyCode)
		r.Set("sku", src.ProductSku)
		r.Set("product_name", src.ProductName)
		r.Set("quantity", src.Quantity)
		r.Set("unit_price_cents", src.UnitPrice)
		r.Set("line_total_cents", src.TotalPrice)
		if src.CostCents.Valid {
			r.Set("line_cost_cents", src.CostCents.Int64)
		}
		r.Set("line_options", src.LineOptions)
		r.Set("order_subtotal_cents", src.SubtotalAmount)
		r.Set("order_discount_cents", src.DiscountAmount)
		r.Set("order_shipping_cents", src.ShippingAmount)
		r.Set("order_tax_cents", src.TaxAmount)
		r.Set("order_giftcard_cents", src.GiftCardAmount)
		r.Set("order_total_cents", src.TotalAmount)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

func parseDate(raw string, fallback time.Time) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return fallback
}

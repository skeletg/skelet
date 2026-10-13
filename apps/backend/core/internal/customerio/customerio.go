// Package customerio is the customers entry in the data registry - mirrors internal/catalogio's role for products
// Customers round-trip both ways, upserting by channel + email
package customerio

import (
	"context"

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

// Rows per query, so an all-time export holds one page in memory rather than the table
const exportPageSize = 500

type contacts interface {
	CustomerIDByEmail(ctx context.Context, channelID uuid.UUID, email string) (uuid.UUID, bool, error)
	EnsureContactFrom(ctx context.Context, channelID uuid.UUID, email, firstName string, verified bool, source string) (customer.Customer, bool, error)
	UpdateProfileFrom(ctx context.Context, id uuid.UUID, input customer.UpdateCustomerInput, source string) (customer.Customer, error)
	NormalizeProfilePhone(ctx context.Context, channelID uuid.UUID, raw *string) (*string, error)
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
			return &customerExporter{queries: queries, opts: o, pageSize: exportPageSize}, nil
		},
		NewImporter: func(_ context.Context, o dataio.Opts) (dataio.Importer, error) {
			return &customerImporter{customers: customers, opts: o}, nil
		},
	}
}

type customerExporter struct {
	queries  *db.Queries
	opts     dataio.Opts
	pageSize int
}

func (e *customerExporter) Columns(context.Context) ([]dataio.Field, error) { return nil, nil }

func (e *customerExporter) Stream(ctx context.Context, emit func(*dataio.Row) error) error {
	params := db.ExportCustomerRowsParams{AllChannels: e.opts.ChannelID == uuid.Nil, PageSize: int32(e.pageSize)}
	if !params.AllChannels {
		params.ChannelIds = []uuid.UUID{e.opts.ChannelID}
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := e.queries.ExportCustomerRows(ctx, params)
		if err != nil {
			return err
		}
		for _, src := range rows {
			if err := emit(customerRow(src)); err != nil {
				return err
			}
		}
		if len(rows) < e.pageSize {
			return nil
		}
		last := rows[len(rows)-1]
		params.AfterCreatedAt, params.AfterID = last.CreatedAt, last.ID
	}
}

func customerRow(src db.ExportCustomerRowsRow) *dataio.Row {
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
	return r
}

type customerImporter struct {
	customers contacts
	opts      dataio.Opts
}

func (im *customerImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	email := customer.NormalizeEmail(r.Str(colEmail))
	r.Ref = email
	if customer.ValidateEmail(email) != nil {
		return dataio.OutcomeSkipped, errInvalidEmail
	}
	if im.opts.ChannelID == uuid.Nil {
		return dataio.OutcomeSkipped, errChannelRequired
	}
	first, last, raw := r.Str(colFirstName), r.Str(colLastName), r.Str(colPhone)
	upd := customer.UpdateCustomerInput{FirstName: first, LastName: last}
	if raw != "" {
		upd.Phone = &raw
	}
	if err := customer.ValidateProfile(upd); err != nil {
		return dataio.OutcomeSkipped, err
	}
	// Checked before any write, so a bad phone neither leaves a half-imported contact behind nor passes a dry run
	var err error
	if upd.Phone, err = im.customers.NormalizeProfilePhone(ctx, im.opts.ChannelID, upd.Phone); err != nil {
		return dataio.OutcomeSkipped, err
	}
	if im.opts.DryRun {
		_, exists, err := im.customers.CustomerIDByEmail(ctx, im.opts.ChannelID, email)
		if err != nil {
			return dataio.OutcomeSkipped, err
		}
		if exists {
			return dataio.OutcomeUpdated, nil
		}
		return dataio.OutcomeCreated, nil
	}

	c, created, err := im.customers.EnsureContactFrom(ctx, im.opts.ChannelID, email, first, false, customer.SourceImport)
	if err != nil {
		return dataio.OutcomeSkipped, err
	}
	// The file is the operator's source of truth for these three, so they fill in on both paths
	if first != "" || last != "" || upd.Phone != nil {
		upd.FirstName, upd.LastName = firstNonEmpty(first, c.FirstName), firstNonEmpty(last, c.LastName)
		if _, err := im.customers.UpdateProfileFrom(ctx, c.ID, upd, customer.SourceImport); err != nil {
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

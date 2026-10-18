package customer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// SignalsEraser is the Signals half of a GDPR erasure, satisfied by *signalsclient.Client - pass a nil interface, never a nil client, when Signals is off
type SignalsEraser interface {
	DeleteCustomerIdentities(ctx context.Context, customerID uuid.UUID, email string, erasedAt time.Time) error
}

type postgresRepository struct {
	queries *db.Queries
	signals SignalsEraser // GDPR erasure of the identity mappings
	tx      *txrunner.Manager
}

// NewRepository returns a postgres-backed Repository - txMgr is what Service.atomically opens its transactions on
func NewRepository(queries *db.Queries, signals SignalsEraser, txMgr *txrunner.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, signals: signals, tx: txMgr}
}

// The same repository with every query on t - multi-statement writes run on one of these
func (r *postgresRepository) withTx(t *sql.Tx) *postgresRepository {
	return &postgresRepository{queries: r.queries.WithTx(t), signals: r.signals, tx: r.tx}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateCustomerInput) (Customer, error) {
	row, err := r.queries.CreateCustomer(ctx, db.CreateCustomerParams{
		ChannelID:       input.ChannelID,
		Email:           input.Email,
		PasswordHash:    input.PasswordHash,
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		IsEmailVerified: input.EmailVerified,
		CustomerGroupID: sqlnull.UUID(input.GroupID),
	})
	if err != nil {
		// the (channel, email) unique index firing means "already registered", not a server fault
		if isUniqueViolation(err) {
			return Customer{}, ErrEmailTaken
		}
		return Customer{}, fmt.Errorf("create customer: %w", err)
	}
	return toCustomer(row), nil
}

func (r *postgresRepository) GetByStoreAndEmail(ctx context.Context, channelID uuid.UUID, email string) (Customer, error) {
	row, err := r.queries.GetCustomerByChannelAndEmail(ctx, db.GetCustomerByChannelAndEmailParams{
		ChannelID: channelID,
		Email:     email,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return Customer{}, ErrCustomerNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("get customer by store and email: %w", err)
	}
	return toCustomer(row), nil
}

// IsActive is the narrow read behind the session guard - one boolean rather than the whole row, since this runs per authenticated request
func (r *postgresRepository) IsActive(ctx context.Context, id uuid.UUID) (bool, error) {
	active, err := r.queries.IsCustomerActive(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil // deleted mid-session
		}
		return false, fmt.Errorf("customer is_active: %w", err)
	}
	return active, nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return customerRow(r.queries.GetCustomerByID(ctx, id))
}

// Locks the row until the surrounding transaction ends
func (r *postgresRepository) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (Customer, error) {
	return customerRow(r.queries.GetCustomerByIDForUpdate(ctx, id))
}

func customerRow(row db.Customer, err error) (Customer, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return Customer{}, ErrCustomerNotFound
	}
	if err != nil {
		return Customer{}, fmt.Errorf("get customer by id: %w", err)
	}
	return toCustomer(row), nil
}

// GetByIDs is the batched form - Returns rows keyed by id; missing ids are simply absent from the map (no error) - The customer group is not hydrated here - the storefront DataLoader path (CustomerDisplay Name) only needs first / last name; group hydration would force a second query per call
func (r *postgresRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Customer, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Customer{}, nil
	}
	rows, err := r.queries.GetCustomersByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get customers by ids: %w", err)
	}
	out := make(map[uuid.UUID]Customer, len(rows))
	for _, row := range rows {
		out[row.ID] = toCustomer(row)
	}
	return out, nil
}

func (r *postgresRepository) GetByIDWithAddresses(ctx context.Context, id uuid.UUID) (Customer, error) {
	c, err := r.GetByID(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	c.Addresses, err = r.ListAddresses(ctx, id)
	if err != nil {
		return Customer{}, err
	}
	group, err := r.GetGroupByID(ctx, c.CustomerGroupID)
	if err != nil {
		return Customer{}, fmt.Errorf("load customer group: %w", err)
	}
	c.Group = &group
	return c, nil
}

func (r *postgresRepository) ListInChannels(ctx context.Context, channelIDs []uuid.UUID) ([]Customer, error) {
	rows, err := r.queries.ListCustomersInChannels(ctx, db.ListCustomersInChannelsParams{AllChannels: channelIDs == nil, ChannelIds: channelIDs})
	if err != nil {
		return nil, fmt.Errorf("list customers in channels: %w", err)
	}
	out := make([]Customer, len(rows))
	for i, row := range rows {
		out[i] = withGroup(row.Customer, row.CustomerGroup)
	}
	return out, nil
}

func withGroup(row db.Customer, g db.CustomerGroup) Customer {
	c := toCustomer(row)
	group := toCustomerGroup(g)
	c.Group = &group
	return c
}

// ListPageFilter narrows the admin list server-side - nil ChannelIDs is every channel, an empty slice none
type ListPageFilter struct {
	ChannelIDs []uuid.UUID
	// Matched with ILIKE as given - the service escapes what the admin typed
	Search    string
	GroupSlug string
	// IsActive filters only when non-nil, so "any status" stays the default
	IsActive *bool
	Limit    int32
	Offset   int32
}

// ListPage is the paged, filtered admin list - returns the page plus the unpaged total, both from one query via COUNT(*) OVER ()
func (r *postgresRepository) ListPage(ctx context.Context, f ListPageFilter) ([]Customer, int64, error) {
	active := false
	if f.IsActive != nil {
		active = *f.IsActive
	}
	rows, err := r.queries.ListCustomersPaged(ctx, db.ListCustomersPagedParams{
		AllChannels:  f.ChannelIDs == nil,
		ChannelIds:   f.ChannelIDs,
		Search:       f.Search,
		GroupSlug:    f.GroupSlug,
		FilterActive: f.IsActive != nil,
		IsActive:     active,
		Lim:          f.Limit,
		Off:          f.Offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list customers page: %w", err)
	}
	out := make([]Customer, len(rows))
	var total int64
	for i, row := range rows {
		out[i] = withGroup(row.Customer, row.CustomerGroup)
		total = row.TotalCount
	}
	return out, total, nil
}

func (r *postgresRepository) Update(ctx context.Context, id uuid.UUID, input UpdateCustomerInput) (Customer, error) {
	n, err := r.queries.UpdateCustomer(ctx, db.UpdateCustomerParams{
		ID:        id,
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Phone:     sqlnull.StringNonEmpty(input.Phone),
	})
	if err != nil {
		return Customer{}, fmt.Errorf("update customer: %w", err)
	}
	if n == 0 {
		return Customer{}, ErrCustomerNotFound
	}
	return r.GetByIDWithAddresses(ctx, id)
}

func (r *postgresRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	if err := r.queries.UpdateCustomerPassword(ctx, db.UpdateCustomerPasswordParams{
		ID:           id,
		PasswordHash: passwordHash,
	}); err != nil {
		return fmt.Errorf("update customer password: %w", err)
	}
	return nil
}

// ErrVerificationEmailMismatch when the row no longer has that address
func (r *postgresRepository) MarkEmailVerified(ctx context.Context, id uuid.UUID, email string) error {
	n, err := r.queries.MarkCustomerEmailVerified(ctx, db.MarkCustomerEmailVerifiedParams{ID: id, Email: email})
	if err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	if n == 0 {
		return ErrVerificationEmailMismatch
	}
	return nil
}

// --- admin-side CRUD ---

func (r *postgresRepository) AdminCreate(ctx context.Context, input AdminCreateCustomerInput, passwordHash string, groupID uuid.UUID) (Customer, error) {
	row, err := r.queries.CreateCustomerAdmin(ctx, db.CreateCustomerAdminParams{
		ChannelID:       input.ChannelID,
		Email:           input.Email,
		PasswordHash:    passwordHash,
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		Phone:           sqlnull.StringNonEmpty(input.Phone),
		IsActive:        *input.IsActive,
		IsEmailVerified: *input.IsEmailVerified,
		CustomerGroupID: groupID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Customer{}, ErrEmailTaken
		}
		return Customer{}, fmt.Errorf("admin create customer: %w", err)
	}
	return r.GetByIDWithAddresses(ctx, row.ID)
}

// Read, merge and write under the row lock, so two concurrent edits can't each restore the other's stale fields - runs on a
// transaction-scoped repository, and returns the row as it was alongside the result
func (r *postgresRepository) AdminUpdate(ctx context.Context, id uuid.UUID, input AdminUpdateCustomerInput, groupID *uuid.UUID) (Customer, Customer, error) {
	current, err := r.queries.GetCustomerByIDForUpdate(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Customer{}, Customer{}, ErrCustomerNotFound
	}
	if err != nil {
		return Customer{}, Customer{}, fmt.Errorf("lock customer: %w", err)
	}
	p := db.UpdateCustomerAdminParams{
		ID: id, Email: current.Email, FirstName: current.FirstName, LastName: current.LastName, Phone: current.Phone,
		IsActive: current.IsActive, IsEmailVerified: current.IsEmailVerified, Note: current.Note,
		CustomerGroupID: current.CustomerGroupID, DefaultBillingAddressID: current.DefaultBillingAddressID,
		DefaultShippingAddressID: current.DefaultShippingAddressID, Metadata: current.Metadata,
	}
	if input.Email != nil {
		p.Email = *input.Email
	}
	if input.FirstName != nil {
		p.FirstName = *input.FirstName
	}
	if input.LastName != nil {
		p.LastName = *input.LastName
	}
	if input.Phone != nil {
		p.Phone = sqlnull.StringNonEmpty(input.Phone)
	}
	if input.IsActive != nil {
		p.IsActive = *input.IsActive
	}
	if input.IsEmailVerified != nil {
		p.IsEmailVerified = *input.IsEmailVerified
	}
	if input.Note != nil {
		p.Note = sqlnull.String(input.Note)
	}
	if groupID != nil {
		p.CustomerGroupID = *groupID
	}
	for _, d := range []struct {
		id  *uuid.UUID
		dst *uuid.NullUUID
	}{{input.DefaultBillingAddressID, &p.DefaultBillingAddressID}, {input.DefaultShippingAddressID, &p.DefaultShippingAddressID}} {
		if d.id == nil {
			continue
		}
		if _, err := r.GetCustomerAddress(ctx, id, *d.id); err != nil {
			return Customer{}, Customer{}, err
		}
		*d.dst = uuid.NullUUID{UUID: *d.id, Valid: true}
	}
	// nil keeps the current entries, non-nil (even empty) replaces them
	if input.Metadata != nil {
		b, err := json.Marshal(*input.Metadata)
		if err != nil {
			return Customer{}, Customer{}, fmt.Errorf("marshal metadata: %w", err)
		}
		p.Metadata = b
	}
	if err := r.queries.UpdateCustomerAdmin(ctx, p); err != nil {
		switch {
		case isUniqueViolation(err):
			return Customer{}, Customer{}, ErrEmailTaken
		case pgdb.IsForeignKeyViolation(err) && pgdb.ConstraintName(err) == "customers_customer_group_id_fkey":
			return Customer{}, Customer{}, ErrGroupNotFound
		case pgdb.IsForeignKeyViolation(err): // a default address deleted after the ownership read
			return Customer{}, Customer{}, ErrAddressNotFound
		}
		return Customer{}, Customer{}, fmt.Errorf("admin update customer: %w", err)
	}
	after, err := r.GetByIDWithAddresses(ctx, id)
	return toCustomer(current), after, err
}

func (r *postgresRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	n, err := r.queries.SetCustomerActive(ctx, db.SetCustomerActiveParams{ID: id, IsActive: active})
	return affectedCustomer(n, err, "set customer active")
}

func affectedCustomer(n int64, err error, what string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return ErrCustomerNotFound
	}
	return nil
}

func (r *postgresRepository) CountOrders(ctx context.Context, customerID uuid.UUID) (int64, error) {
	n, err := r.queries.CountOrdersByCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("count orders: %w", err)
	}
	return n, nil
}

// The rows keyed by address, or set null by the delete, are scrubbed first, so a deleted account leaves no more behind than an erased one
// Runs on a transaction-scoped repository; an order placed since the caller counted trips the FK and reads as ErrCustomerHasOrders
func (r *postgresRepository) Delete(ctx context.Context, c Customer) error {
	if err := scrubPersonalData(ctx, r.queries, c); err != nil {
		return err
	}
	n, err := r.queries.DeleteCustomer(ctx, c.ID)
	if pgdb.IsForeignKeyViolation(err) {
		return ErrCustomerHasOrders
	}
	return affectedCustomer(n, err, "delete customer")
}

func (r *postgresRepository) SetTaxStatus(ctx context.Context, id uuid.UUID, exempt bool, note string) error {
	n, err := r.queries.SetCustomerTaxStatus(ctx, db.SetCustomerTaxStatusParams{ID: id, TaxExempt: exempt, TaxExemptionNote: note})
	return affectedCustomer(n, err, "set customer tax status")
}

// Stores a normalized VAT id with its latest validation status
func (r *postgresRepository) SetVatID(ctx context.Context, id uuid.UUID, vatID, status string) error {
	n, err := r.queries.SetCustomerVatID(ctx, db.SetCustomerVatIDParams{ID: id, VatID: vatID, VatIDStatus: status})
	return affectedCustomer(n, err, "set customer vat id")
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching

func toCustomer(row db.Customer) Customer {
	var meta Metadata
	if len(row.Metadata) > 0 && string(row.Metadata) != "null" {
		_ = json.Unmarshal(row.Metadata, &meta)
	}
	c := Customer{
		ID:               row.ID,
		ChannelID:        row.ChannelID,
		Email:            row.Email,
		PasswordHash:     row.PasswordHash,
		FirstName:        row.FirstName,
		LastName:         row.LastName,
		IsActive:         row.IsActive,
		IsEmailVerified:  row.IsEmailVerified,
		Metadata:         meta,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
		CustomerGroupID:  row.CustomerGroupID,
		TaxExempt:        row.TaxExempt,
		TaxExemptionNote: row.TaxExemptionNote,
		VatID:            row.VatID,
		VatIDStatus:      row.VatIDStatus,
	}
	if row.VatIDCheckedAt.Valid {
		t := row.VatIDCheckedAt.Time
		c.VatIDCheckedAt = &t
	}
	if row.Phone.Valid {
		c.Phone = &row.Phone.String
	}
	if row.Note.Valid {
		c.Note = &row.Note.String
	}
	if row.DefaultBillingAddressID.Valid {
		id := row.DefaultBillingAddressID.UUID
		c.DefaultBillingAddressID = &id
	}
	if row.DefaultShippingAddressID.Valid {
		id := row.DefaultShippingAddressID.UUID
		c.DefaultShippingAddressID = &id
	}
	return c
}

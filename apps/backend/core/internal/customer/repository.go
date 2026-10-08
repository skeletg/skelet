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

// NewRepository returns a postgres-backed Repository - The txrunner.Manager is used
// for atomic multi-statement mutations (currently only SetDefaultGroup)
func NewRepository(queries *db.Queries, signals SignalsEraser, txMgr *txrunner.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, signals: signals, tx: txMgr}
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
		// the (channel, email) unique index firing means "already registered", not a server fault - the update paths mapped this and registration never did, so every reused email surfaced as a bare INTERNAL to the storefront
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
	row, err := r.queries.GetCustomerByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Customer{}, ErrCustomerNotFound
		}
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
	row, err := r.queries.GetCustomerByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Customer{}, ErrCustomerNotFound
		}
		return Customer{}, fmt.Errorf("get customer by id: %w", err)
	}
	c := toCustomer(row)
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

func (r *postgresRepository) List(ctx context.Context) ([]Customer, error) {
	rows, err := r.queries.ListCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	return r.withGroups(ctx, rows)
}

func (r *postgresRepository) ListInChannels(ctx context.Context, channelIDs []uuid.UUID) ([]Customer, error) {
	rows, err := r.queries.ListCustomersInChannels(ctx, db.ListCustomersInChannelsParams{AllChannels: channelIDs == nil, ChannelIds: channelIDs})
	if err != nil {
		return nil, fmt.Errorf("list customers in channels: %w", err)
	}
	return r.withGroups(ctx, rows)
}

func (r *postgresRepository) withGroups(ctx context.Context, rows []db.Customer) ([]Customer, error) {
	// One-shot group load + in-memory map join, avoiding N+1 per customer
	groups, err := r.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer groups: %w", err)
	}
	byID := make(map[uuid.UUID]*CustomerGroup, len(groups))
	for i := range groups {
		byID[groups[i].ID] = &groups[i]
	}
	customers := make([]Customer, len(rows))
	for i, row := range rows {
		customers[i] = toCustomer(row)
		customers[i].Group = byID[customers[i].CustomerGroupID]
	}
	return customers, nil
}

// ListPageFilter narrows the admin list server-side - channelIDs nil means every channel the caller may see was already resolved to "all"
type ListPageFilter struct {
	ChannelIDs []uuid.UUID
	Search     string
	GroupSlug  string
	// IsActive filters only when non-nil, so "any status" stays the default
	IsActive *bool
	Limit    int32
	Offset   int32
}

// ListPage is the paged, filtered sibling of List - returns the page plus the unpaged total, both from one query via COUNT(*) OVER ()
func (r *postgresRepository) ListPage(ctx context.Context, f ListPageFilter) ([]Customer, int64, error) {
	active := false
	if f.IsActive != nil {
		active = *f.IsActive
	}
	rows, err := r.queries.ListCustomersPaged(ctx, db.ListCustomersPagedParams{
		AllChannels:  len(f.ChannelIDs) == 0,
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
	groups, err := r.ListGroups(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list customer groups: %w", err)
	}
	byID := make(map[uuid.UUID]*CustomerGroup, len(groups))
	for i := range groups {
		byID[groups[i].ID] = &groups[i]
	}
	out := make([]Customer, len(rows))
	var total int64
	for i, row := range rows {
		// The paged row is db.Customer plus the window count, so map every column across - a partial copy here silently blanks fields the list renders
		out[i] = toCustomer(db.Customer{
			ID:                       row.ID,
			Email:                    row.Email,
			PasswordHash:             row.PasswordHash,
			FirstName:                row.FirstName,
			LastName:                 row.LastName,
			Phone:                    row.Phone,
			IsActive:                 row.IsActive,
			IsEmailVerified:          row.IsEmailVerified,
			Note:                     row.Note,
			Metadata:                 row.Metadata,
			CreatedAt:                row.CreatedAt,
			UpdatedAt:                row.UpdatedAt,
			DefaultBillingAddressID:  row.DefaultBillingAddressID,
			DefaultShippingAddressID: row.DefaultShippingAddressID,
			CustomerGroupID:          row.CustomerGroupID,
			ChannelID:                row.ChannelID,
			TaxExempt:                row.TaxExempt,
			TaxExemptionNote:         row.TaxExemptionNote,
			VatID:                    row.VatID,
			VatIDStatus:              row.VatIDStatus,
			VatIDCheckedAt:           row.VatIDCheckedAt,
		})
		out[i].Group = byID[out[i].CustomerGroupID]
		total = row.TotalCount
	}
	return out, total, nil
}

func (r *postgresRepository) Update(ctx context.Context, id uuid.UUID, input UpdateCustomerInput) (Customer, error) {
	if _, err := r.queries.UpdateCustomer(ctx, db.UpdateCustomerParams{
		ID:        id,
		FirstName: input.FirstName,
		LastName:  input.LastName,
		Phone:     sqlnull.String(input.Phone),
	}); err != nil {
		return Customer{}, fmt.Errorf("update customer: %w", err)
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

// ErrVerificationEmailMismatch when the row no longer has that address - enqueue, when set, writes the event in the same tx
func (r *postgresRepository) MarkEmailVerified(ctx context.Context, id uuid.UUID, email string, enqueue func(*sql.Tx) error) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		n, err := r.queries.WithTx(t).MarkCustomerEmailVerified(ctx, db.MarkCustomerEmailVerifiedParams{ID: id, Email: email})
		if err != nil {
			return fmt.Errorf("mark email verified: %w", err)
		}
		if n == 0 {
			return ErrVerificationEmailMismatch
		}
		if enqueue != nil {
			return enqueue(t)
		}
		return nil
	})
}

func (r *postgresRepository) CountAddresses(ctx context.Context, customerID uuid.UUID) (int64, error) {
	n, err := r.queries.CountAddressesByCustomer(ctx, customerID)
	if err != nil {
		return 0, fmt.Errorf("count addresses: %w", err)
	}
	return n, nil
}

// --- admin-side CRUD ---

func (r *postgresRepository) AdminCreate(ctx context.Context, input AdminCreateCustomerInput, passwordHash string, groupID uuid.UUID) (Customer, error) {
	row, err := r.queries.CreateCustomerAdmin(ctx, db.CreateCustomerAdminParams{
		ChannelID:       input.ChannelID,
		Email:           input.Email,
		PasswordHash:    passwordHash,
		FirstName:       input.FirstName,
		LastName:        input.LastName,
		Phone:           sqlnull.String(input.Phone),
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

func (r *postgresRepository) AdminUpdate(ctx context.Context, id uuid.UUID, input AdminUpdateCustomerInput, groupID *uuid.UUID) (Customer, error) {
	current, err := r.queries.GetCustomerByID(ctx, id)
	if err != nil {
		return Customer{}, fmt.Errorf("customer not found")
	}

	// Merge each non-nil input field over the current row
	email := current.Email
	if input.Email != nil {
		email = *input.Email
	}
	firstName := current.FirstName
	if input.FirstName != nil {
		firstName = *input.FirstName
	}
	lastName := current.LastName
	if input.LastName != nil {
		lastName = *input.LastName
	}
	phone := current.Phone
	if input.Phone != nil {
		phone = sqlnull.String(input.Phone)
	}
	isActive := current.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	isEmailVerified := current.IsEmailVerified
	if input.IsEmailVerified != nil {
		isEmailVerified = *input.IsEmailVerified
	}
	note := current.Note
	if input.Note != nil {
		note = sqlnull.String(input.Note)
	}
	resolvedGroup := current.CustomerGroupID
	if groupID != nil {
		resolvedGroup = *groupID
	}

	// Default address IDs: validate ownership, then merge
	billingID := current.DefaultBillingAddressID
	if input.DefaultBillingAddressID != nil {
		addr, err := r.queries.GetAddressByID(ctx, *input.DefaultBillingAddressID)
		if err != nil || addr.CustomerID != id {
			return Customer{}, fmt.Errorf("billing address not found or does not belong to this customer")
		}
		billingID = uuid.NullUUID{UUID: *input.DefaultBillingAddressID, Valid: true}
	}
	shippingID := current.DefaultShippingAddressID
	if input.DefaultShippingAddressID != nil {
		addr, err := r.queries.GetAddressByID(ctx, *input.DefaultShippingAddressID)
		if err != nil || addr.CustomerID != id {
			return Customer{}, fmt.Errorf("shipping address not found or does not belong to this customer")
		}
		shippingID = uuid.NullUUID{UUID: *input.DefaultShippingAddressID, Valid: true}
	}

	// Metadata: nil = keep current bytes; non-nil (even empty map) = replace
	metadataBytes := current.Metadata
	if input.Metadata != nil {
		b, err := json.Marshal(*input.Metadata)
		if err != nil {
			return Customer{}, fmt.Errorf("marshal metadata: %w", err)
		}
		metadataBytes = b
	}

	if _, err := r.queries.UpdateCustomerAdmin(ctx, db.UpdateCustomerAdminParams{
		ID:                       id,
		Email:                    email,
		FirstName:                firstName,
		LastName:                 lastName,
		Phone:                    phone,
		IsActive:                 isActive,
		IsEmailVerified:          isEmailVerified,
		Note:                     note,
		CustomerGroupID:          resolvedGroup,
		DefaultBillingAddressID:  billingID,
		DefaultShippingAddressID: shippingID,
		Metadata:                 metadataBytes,
	}); err != nil {
		if isUniqueViolation(err) {
			return Customer{}, ErrEmailTaken
		}
		return Customer{}, fmt.Errorf("admin update customer: %w", err)
	}
	return r.GetByIDWithAddresses(ctx, id)
}

func (r *postgresRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) (Customer, error) {
	if _, err := r.queries.SetCustomerActive(ctx, db.SetCustomerActiveParams{
		ID:       id,
		IsActive: active,
	}); err != nil {
		return Customer{}, fmt.Errorf("set customer active: %w", err)
	}
	return r.GetByIDWithAddresses(ctx, id)
}

func (r *postgresRepository) CountOrders(ctx context.Context, customerID uuid.UUID) (int64, error) {
	n, err := r.queries.CountOrdersByCustomer(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("count orders: %w", err)
	}
	return n, nil
}

// The rows keyed by address, or set null by the delete, are scrubbed in the same tx, so a deleted account leaves no more behind than an erased one
func (r *postgresRepository) Delete(ctx context.Context, c Customer) error {
	return r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := scrubPersonalData(ctx, q, c); err != nil {
			return err
		}
		if err := q.DeleteCustomer(ctx, c.ID); err != nil {
			return fmt.Errorf("delete customer: %w", err)
		}
		return nil
	})
}

func (r *postgresRepository) SetDefaultBillingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	if _, err := r.queries.SetDefaultBillingAddress(ctx, db.SetDefaultBillingAddressParams{
		ID:                      customerID,
		DefaultBillingAddressID: uuid.NullUUID{UUID: addressID, Valid: true},
	}); err != nil {
		return Customer{}, fmt.Errorf("set default billing address: %w", err)
	}
	return r.GetByIDWithAddresses(ctx, customerID)
}

func (r *postgresRepository) SetDefaultShippingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	if _, err := r.queries.SetDefaultShippingAddress(ctx, db.SetDefaultShippingAddressParams{
		ID:                       customerID,
		DefaultShippingAddressID: uuid.NullUUID{UUID: addressID, Valid: true},
	}); err != nil {
		return Customer{}, fmt.Errorf("set default shipping address: %w", err)
	}
	return r.GetByIDWithAddresses(ctx, customerID)
}

func (r *postgresRepository) CreateAddress(ctx context.Context, customerID uuid.UUID, input AddressInput) (Address, error) {
	row, err := r.queries.CreateAddress(ctx, db.CreateAddressParams{
		CustomerID:  customerID,
		FirstName:   input.FirstName,
		LastName:    input.LastName,
		Company:     sqlnull.String(input.Company),
		Street1:     input.Street1,
		Street2:     sqlnull.String(input.Street2),
		City:        input.City,
		State:       sqlnull.String(input.State),
		PostalCode:  input.PostalCode,
		CountryCode: input.CountryCode,
		Phone:       sqlnull.String(input.Phone),
	})
	if err != nil {
		return Address{}, fmt.Errorf("create address: %w", err)
	}
	return toAddress(row), nil
}

func (r *postgresRepository) GetAddress(ctx context.Context, id uuid.UUID) (Address, error) {
	row, err := r.queries.GetAddressByID(ctx, id)
	if err != nil {
		return Address{}, fmt.Errorf("get address by id: %w", err)
	}
	return toAddress(row), nil
}

func (r *postgresRepository) ListAddresses(ctx context.Context, customerID uuid.UUID) ([]Address, error) {
	rows, err := r.queries.ListAddressesByCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list addresses: %w", err)
	}
	addresses := make([]Address, len(rows))
	for i, row := range rows {
		addresses[i] = toAddress(row)
	}
	return addresses, nil
}

func (r *postgresRepository) UpdateAddress(ctx context.Context, id uuid.UUID, input AddressInput) (Address, error) {
	row, err := r.queries.UpdateAddress(ctx, db.UpdateAddressParams{
		ID:          id,
		FirstName:   input.FirstName,
		LastName:    input.LastName,
		Company:     sqlnull.String(input.Company),
		Street1:     input.Street1,
		Street2:     sqlnull.String(input.Street2),
		City:        input.City,
		State:       sqlnull.String(input.State),
		PostalCode:  input.PostalCode,
		CountryCode: input.CountryCode,
		Phone:       sqlnull.String(input.Phone),
	})
	if err != nil {
		return Address{}, fmt.Errorf("update address: %w", err)
	}
	return toAddress(row), nil
}

func (r *postgresRepository) DeleteAddress(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteAddress(ctx, id); err != nil {
		return fmt.Errorf("delete address: %w", err)
	}
	return nil
}

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

func toAddress(row db.Address) Address {
	a := Address{
		ID:          row.ID,
		CustomerID:  row.CustomerID,
		FirstName:   row.FirstName,
		LastName:    row.LastName,
		Street1:     row.Street1,
		City:        row.City,
		PostalCode:  row.PostalCode,
		CountryCode: row.CountryCode,
		CreatedAt:   row.CreatedAt,
	}
	if row.Company.Valid {
		a.Company = &row.Company.String
	}
	if row.Street2.Valid {
		a.Street2 = &row.Street2.String
	}
	if row.State.Valid {
		a.State = &row.State.String
	}
	if row.Phone.Valid {
		a.Phone = &row.Phone.String
	}
	return a
}

// --- customer groups ---

func (r *postgresRepository) CreateGroup(ctx context.Context, input CreateCustomerGroupInput) (CustomerGroup, error) {
	row, err := r.queries.CreateCustomerGroup(ctx, db.CreateCustomerGroupParams{
		Name:        input.Name,
		Slug:        input.Slug,
		Description: input.Description,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return CustomerGroup{}, ErrGroupSlugTaken
		}
		return CustomerGroup{}, fmt.Errorf("create customer group: %w", err)
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) UpdateGroup(ctx context.Context, id uuid.UUID, input UpdateCustomerGroupInput) (CustomerGroup, error) {
	current, err := r.queries.GetCustomerGroupByID(ctx, id)
	if err != nil {
		return CustomerGroup{}, fmt.Errorf("group not found")
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	description := current.Description
	if input.Description != nil {
		description = *input.Description
	}
	row, err := r.queries.UpdateCustomerGroup(ctx, db.UpdateCustomerGroupParams{
		ID:          id,
		Name:        name,
		Description: description,
	})
	if err != nil {
		return CustomerGroup{}, fmt.Errorf("update customer group: %w", err)
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	row, err := r.queries.GetCustomerGroupByID(ctx, id)
	if err != nil {
		return fmt.Errorf("group not found")
	}
	if row.IsDefault {
		return ErrCannotDeleteDefaultGroup
	}
	n, err := r.queries.CountCustomerGroupMembers(ctx, id)
	if err != nil {
		return fmt.Errorf("count group members: %w", err)
	}
	if n > 0 {
		return ErrGroupInUse
	}
	if err := r.queries.DeleteCustomerGroup(ctx, id); err != nil {
		return fmt.Errorf("delete customer group: %w", err)
	}
	return nil
}

func (r *postgresRepository) GetGroupByID(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	row, err := r.queries.GetCustomerGroupByID(ctx, id)
	if err != nil {
		return CustomerGroup{}, fmt.Errorf("group not found")
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) GetGroupBySlug(ctx context.Context, slug string) (CustomerGroup, error) {
	row, err := r.queries.GetCustomerGroupBySlug(ctx, slug)
	if err != nil {
		return CustomerGroup{}, fmt.Errorf("group not found")
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) GetDefaultGroup(ctx context.Context) (CustomerGroup, error) {
	row, err := r.queries.GetDefaultCustomerGroup(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CustomerGroup{}, ErrNoDefaultGroup
		}
		return CustomerGroup{}, fmt.Errorf("get default group: %w", err)
	}
	return toCustomerGroup(row), nil
}

func (r *postgresRepository) ListGroups(ctx context.Context) ([]CustomerGroup, error) {
	rows, err := r.queries.ListCustomerGroupsWithMemberCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer groups: %w", err)
	}
	out := make([]CustomerGroup, len(rows))
	for i, row := range rows {
		out[i] = CustomerGroup{
			ID:          row.ID,
			Name:        row.Name,
			Slug:        row.Slug,
			Description: row.Description,
			IsDefault:   row.IsDefault,
			CreatedAt:   row.CreatedAt,
			MemberCount: row.MemberCount,
		}
	}
	return out, nil
}

func (r *postgresRepository) SetDefaultGroup(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	// Clear the current default + mark the new default, atomically - Concurrent callers serialize on the row lock the
	// first UPDATE takes on the currently-default row; there is no window with zero defaults
	var updated CustomerGroup
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		if err := q.ClearDefaultCustomerGroup(ctx); err != nil {
			return fmt.Errorf("clear default: %w", err)
		}
		if err := q.MarkCustomerGroupDefault(ctx, id); err != nil {
			return fmt.Errorf("mark default: %w", err)
		}
		row, err := q.GetCustomerGroupByID(ctx, id)
		if err != nil {
			return fmt.Errorf("reload group: %w", err)
		}
		updated = toCustomerGroup(row)
		return nil
	})
	return updated, err
}

func (r *postgresRepository) SetCustomerGroup(ctx context.Context, customerID, groupID uuid.UUID) (Customer, error) {
	if _, err := r.queries.SetCustomerGroup(ctx, db.SetCustomerGroupParams{
		ID:              customerID,
		CustomerGroupID: groupID,
	}); err != nil {
		return Customer{}, fmt.Errorf("set customer group: %w", err)
	}
	return r.GetByIDWithAddresses(ctx, customerID)
}

func toCustomerGroup(row db.CustomerGroup) CustomerGroup {
	return CustomerGroup{
		ID:          row.ID,
		Name:        row.Name,
		Slug:        row.Slug,
		Description: row.Description,
		IsDefault:   row.IsDefault,
		CreatedAt:   row.CreatedAt,
	}
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching

// SetTaxStatus flips the customer's tax-exempt flag + note
func (r *postgresRepository) SetTaxStatus(ctx context.Context, id uuid.UUID, exempt bool, note string) (Customer, error) {
	row, err := r.queries.SetCustomerTaxStatus(ctx, db.SetCustomerTaxStatusParams{
		ID:               id,
		TaxExempt:        exempt,
		TaxExemptionNote: note,
	})
	if err != nil {
		return Customer{}, fmt.Errorf("set customer tax status: %w", err)
	}
	return toCustomer(row), nil
}

// SetVatID stores a (normalized) VAT id + its latest validation status
func (r *postgresRepository) SetVatID(ctx context.Context, id uuid.UUID, vatID, status string) (Customer, error) {
	row, err := r.queries.SetCustomerVatID(ctx, db.SetCustomerVatIDParams{
		ID:          id,
		VatID:       vatID,
		VatIDStatus: status,
	})
	if err != nil {
		return Customer{}, fmt.Errorf("set customer vat id: %w", err)
	}
	return toCustomer(row), nil
}

package customer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

// A country code the countries table doesn't hold, or one too long for char(2), is the caller's mistake
func addressWriteError(err error, what string) error {
	if pgdb.IsForeignKeyViolation(err) || pgdb.IsStringTooLong(err) {
		return ErrUnknownCountry
	}
	return fmt.Errorf("%s: %w", what, err)
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
		return Address{}, addressWriteError(err, "create address")
	}
	return toAddress(row), nil
}

// Owner-blind - see GetCustomerAddress for every path that knows the customer
func (r *postgresRepository) GetAddress(ctx context.Context, id uuid.UUID) (Address, error) {
	return addressRow(r.queries.GetAddressByID(ctx, id))
}

func (r *postgresRepository) GetCustomerAddress(ctx context.Context, customerID, addressID uuid.UUID) (Address, error) {
	return addressRow(r.queries.GetCustomerAddress(ctx, db.GetCustomerAddressParams{ID: addressID, CustomerID: customerID}))
}

func addressRow(row db.Address, err error) (Address, error) {
	if errors.Is(err, sql.ErrNoRows) {
		return Address{}, ErrAddressNotFound
	}
	if err != nil {
		return Address{}, fmt.Errorf("get address: %w", err)
	}
	return toAddress(row), nil
}

func (r *postgresRepository) CountAddresses(ctx context.Context, customerID uuid.UUID) (int64, error) {
	n, err := r.queries.CountAddressesByCustomer(ctx, customerID)
	if err != nil {
		return 0, fmt.Errorf("count addresses: %w", err)
	}
	return n, nil
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

// Scoped to the owner in the statement itself, so another customer's id updates nothing and reads as not found
func (r *postgresRepository) UpdateAddress(ctx context.Context, customerID, id uuid.UUID, input AddressInput) (Address, error) {
	row, err := r.queries.UpdateAddress(ctx, db.UpdateAddressParams{
		ID:          id,
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
	if errors.Is(err, sql.ErrNoRows) {
		return Address{}, ErrAddressNotFound
	}
	if err != nil {
		return Address{}, addressWriteError(err, "update address")
	}
	return toAddress(row), nil
}

func (r *postgresRepository) DeleteAddress(ctx context.Context, customerID, id uuid.UUID) error {
	n, err := r.queries.DeleteAddress(ctx, db.DeleteAddressParams{ID: id, CustomerID: customerID})
	if err != nil {
		return fmt.Errorf("delete address: %w", err)
	}
	if n == 0 {
		return ErrAddressNotFound
	}
	return nil
}

func (r *postgresRepository) SetDefaultBillingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	n, err := r.queries.SetDefaultBillingAddress(ctx, db.SetDefaultBillingAddressParams{
		CustomerID: customerID, AddressID: uuid.NullUUID{UUID: addressID, Valid: true},
	})
	return r.afterDefaultAddress(ctx, customerID, n, err)
}

func (r *postgresRepository) SetDefaultShippingAddress(ctx context.Context, customerID, addressID uuid.UUID) (Customer, error) {
	n, err := r.queries.SetDefaultShippingAddress(ctx, db.SetDefaultShippingAddressParams{
		CustomerID: customerID, AddressID: uuid.NullUUID{UUID: addressID, Valid: true},
	})
	return r.afterDefaultAddress(ctx, customerID, n, err)
}

// Zero rows means the address isn't the customer's, or is gone
func (r *postgresRepository) afterDefaultAddress(ctx context.Context, customerID uuid.UUID, n int64, err error) (Customer, error) {
	if err != nil {
		return Customer{}, fmt.Errorf("set default address: %w", err)
	}
	if n == 0 {
		return Customer{}, ErrAddressNotFound
	}
	return r.GetByIDWithAddresses(ctx, customerID)
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

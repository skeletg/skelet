// Package paymentvault manages customers' saved payment methods - The card itself lives in the payment provider's vault (sdk.Vaulter capability); this service stores only the opaque provider token + display fields and enforces ownership - Consumers: checkout ("pay with saved card"), the account UI, and subscription renewal charging
package paymentvault

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// ErrNotFound is returned when the method doesn't exist or belongs to a
// different customer (indistinguishable on purpose)
var ErrNotFound = apperr.New("PAYMENT_METHOD_NOT_FOUND", apperr.ClassNotFound, "saved payment method not found")

// A GDPR erasure deletes these rows only - the card stays in the provider's vault until removed there
type Method struct {
	ID           uuid.UUID
	CustomerID   uuid.UUID
	ProviderSlug string
	Token        string
	Brand        string
	Last4        string
	ExpMonth     int
	ExpYear      int
	CreatedAt    time.Time
}

type Service struct {
	queries  *db.Queries
	payments *payment.Service
}

func NewService(queries *db.Queries, payments *payment.Service) *Service {
	return &Service{queries: queries, payments: payments}
}

func (s *Service) List(ctx context.Context, customerID uuid.UUID) ([]Method, error) {
	rows, err := s.queries.ListCustomerPaymentMethods(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list payment methods: %w", err)
	}
	out := make([]Method, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMethod(r))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, customerID, id uuid.UUID) (Method, error) {
	row, err := s.queries.GetCustomerPaymentMethod(ctx, db.GetCustomerPaymentMethodParams{
		ID:         id,
		CustomerID: customerID,
	})
	if err != nil {
		return Method{}, ErrNotFound
	}
	return toMethod(row), nil
}

// Vaults the client-tokenized method with the provider and records the returned token
func (s *Service) Save(ctx context.Context, customerID uuid.UUID, email, providerSlug, token string, channelID uuid.UUID) (Method, error) {
	vaulted, err := s.payments.Vault(ctx, providerSlug, payment.VaultRequest{
		CustomerID: customerID,
		Email:      email,
		Token:      token,
		ChannelID:  channelID,
	})
	if err != nil {
		return Method{}, err
	}
	row, err := s.queries.InsertCustomerPaymentMethod(ctx, db.InsertCustomerPaymentMethodParams{
		CustomerID:   customerID,
		ProviderSlug: providerSlug,
		Token:        vaulted.ProviderToken,
		Brand:        vaulted.Brand,
		Last4:        vaulted.Last4,
		ExpMonth:     int32(vaulted.ExpMonth),
		ExpYear:      int32(vaulted.ExpYear),
	})
	if err != nil {
		return Method{}, fmt.Errorf("record payment method: %w", err)
	}
	return toMethod(row), nil
}

// Delete removes the method from the provider vault and the local table The provider delete runs first; its failure
// aborts (so a row never outlives a still-chargeable vault entry silently)
func (s *Service) Delete(ctx context.Context, customerID, id uuid.UUID, channelID uuid.UUID) error {
	m, err := s.Get(ctx, customerID, id)
	if err != nil {
		return err
	}
	if err := s.payments.DeleteVaulted(ctx, m.ProviderSlug, payment.VaultDeleteRequest{
		ProviderToken: m.Token,
		ChannelID:     channelID,
	}); err != nil {
		return err
	}
	if err := s.queries.DeleteCustomerPaymentMethod(ctx, db.DeleteCustomerPaymentMethodParams{
		ID:         id,
		CustomerID: customerID,
	}); err != nil {
		return fmt.Errorf("delete payment method row: %w", err)
	}
	return nil
}

func toMethod(r db.CustomerPaymentMethod) Method {
	return Method{
		ID:           r.ID,
		CustomerID:   r.CustomerID,
		ProviderSlug: r.ProviderSlug,
		Token:        r.Token,
		Brand:        r.Brand,
		Last4:        r.Last4,
		ExpMonth:     int(r.ExpMonth),
		ExpYear:      int(r.ExpYear),
		CreatedAt:    r.CreatedAt,
	}
}

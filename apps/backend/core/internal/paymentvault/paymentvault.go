// Package paymentvault manages customers' saved payment methods - The card itself lives in the payment provider's vault (sdk.Vaulter capability); this service stores only the opaque provider token + display fields and enforces ownership - Consumers: checkout ("pay with saved card"), the account UI, and subscription renewal charging
package paymentvault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// ErrNotFound is returned when the method doesn't exist or belongs to a
// different customer (indistinguishable on purpose)
var ErrNotFound = apperr.New("PAYMENT_METHOD_NOT_FOUND", apperr.ClassNotFound, "saved payment method not found")

// Each save asks the gateway to vault a card, so an unbounded save is how a signed-in account tests stolen card numbers one call at a time
var (
	ErrTooManyMethods = apperr.New("PAYMENT_METHOD_LIMIT", apperr.ClassConflict, "remove a saved payment method before adding another")
	ErrSaveThrottled  = apperr.New("PAYMENT_METHOD_SAVE_THROTTLED", apperr.ClassRateLimited, "too many attempts to save a payment method - try again later")
)

const maxMethodsPerCustomer = 10

// A per-key allowance - the shared ratelimit limiter in production
type Budget interface {
	Allow(ctx context.Context, key string) bool
}

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
	payments Vaulter
	budget   Budget
}

// The gateway surface saving and removing needs - *payment.Service in production
type Vaulter interface {
	Vault(ctx context.Context, providerSlug string, req payment.VaultRequest) (payment.VaultedMethod, error)
	DeleteVaulted(ctx context.Context, providerSlug string, req payment.VaultDeleteRequest) error
}

func NewService(queries *db.Queries, payments Vaulter) *Service {
	return &Service{queries: queries, payments: payments}
}

// Optional - nil leaves only the per-customer cap
func (s *Service) SetSaveBudget(b Budget) { s.budget = b }

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
	if errors.Is(err, sql.ErrNoRows) {
		return Method{}, ErrNotFound
	}
	if err != nil {
		return Method{}, fmt.Errorf("get payment method: %w", err)
	}
	return toMethod(row), nil
}

// Vaults the client-tokenized method with the provider and records the returned token
func (s *Service) Save(ctx context.Context, customerID uuid.UUID, email, providerSlug, token string, channelID uuid.UUID) (Method, error) {
	// Counted before the gateway call, failures included - a declined card is exactly what card testing produces
	if s.budget != nil && !s.budget.Allow(ctx, "pmsave:"+customerID.String()) {
		return Method{}, ErrSaveThrottled
	}
	existing, err := s.queries.ListCustomerPaymentMethods(ctx, customerID)
	if err != nil {
		return Method{}, fmt.Errorf("list payment methods: %w", err)
	}
	if len(existing) >= maxMethodsPerCustomer {
		return Method{}, ErrTooManyMethods
	}
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
		// The gateway already holds the card - left there with no row, it's chargeable and nobody can see or remove it
		if derr := s.payments.DeleteVaulted(ctx, providerSlug, payment.VaultDeleteRequest{ProviderToken: vaulted.ProviderToken, ChannelID: channelID}); derr != nil {
			log.Printf("[paymentvault] remove unrecorded vault token at %s: %v", providerSlug, derr)
		}
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

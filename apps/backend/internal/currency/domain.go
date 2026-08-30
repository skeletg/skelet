// Package currency manages first-class currencies - ISO 4217 codes registered once and referenced by FK from channels, products.default_currency_code and elsewhere - Same shape as the locale package
// The decimals field is load-bearing: it fixes the price_cents trap (USD=2, JPY=0, BHD/JOD/KWD/OMR/TND=3) once the storefront consumes it
package currency

import (
	"context"
	"errors"
	"time"

	"github.com/skeletg/skelet/internal/permission"
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "currencies",
		Name:      "Currencies",
		SortOrder: 66,
	})
	permission.Register(permission.Definition{
		Group: "currencies",
		Slug:  "currencies.view",
		Name:  "View currencies",
	})
	permission.Register(permission.Definition{
		Group: "currencies",
		Slug:  "currencies.manage",
		Name:  "Manage currencies",
	})
}

// Whether the currency symbol prefixes or suffixes the rendered amount - "$10.00" versus "10.00 EUR"
type SymbolPosition string

const (
	SymbolPositionPrefix SymbolPosition = "prefix"
	SymbolPositionSuffix SymbolPosition = "suffix"
)

type Currency struct {
	Code           string
	Name           string
	Symbol         string
	Decimals       int16          // 0 (JPY) … 4 (rare) - typically 2
	SymbolPosition SymbolPosition // "prefix" | "suffix"
	IsActive       bool
	SortOrder      int32
	Metadata       map[string]string
	// Units of THIS currency per one unit of the platform base currency, nil meaning never set - refreshed by the currency-rates job or set manually
	ExchangeRate  *float64
	RateUpdatedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateCurrencyInput struct {
	Code           string
	Name           string
	Symbol         string
	Decimals       int16
	SymbolPosition SymbolPosition
	IsActive       bool
	SortOrder      int32
	Metadata       map[string]string
}

type UpdateCurrencyInput struct {
	Code           string
	Name           *string
	Symbol         *string
	Decimals       *int16
	SymbolPosition *SymbolPosition
	IsActive       *bool
	SortOrder      *int32
	Metadata       *map[string]string
}

var (
	ErrCurrencyNotFound  = errors.New("currency not found")
	ErrCurrencyCodeTaken = errors.New("currency code already exists")
	ErrCurrencyInUse     = errors.New("currency has references; cannot delete")
	ErrInvalidCode       = errors.New("invalid ISO 4217 currency code")
)

type Repository interface {
	Create(ctx context.Context, input CreateCurrencyInput) (Currency, error)
	GetByCode(ctx context.Context, code string) (Currency, error)
	List(ctx context.Context, activeOnly bool) ([]Currency, error)
	Update(ctx context.Context, input UpdateCurrencyInput) (Currency, error)
	Delete(ctx context.Context, code string) error
	CountReferences(ctx context.Context, code string) (int64, error)
	// Stores a refreshed exchange rate and stamps rate_updated_at
	SetRate(ctx context.Context, code string, rate float64) error
}

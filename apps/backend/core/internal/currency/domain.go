// Package currency manages first-class currencies - ISO 4217 codes registered once and referenced by FK from channels, products.default_currency_code and elsewhere - Same shape as the locale package
// The decimals field is load-bearing: it fixes the price_cents trap (USD=2, JPY=0, BHD/JOD/KWD/OMR/TND=3) once the storefront consumes it
package currency

import (
	"context"
	"errors"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
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

const (
	MinDecimals = 0
	MaxDecimals = 4

	maxMetadataKeys        = 50
	maxMetadataValueLength = 1024
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

	ErrNameRequired          = errors.New("currency name is required")
	ErrInvalidSymbolPosition = errors.New(`currency symbol position must be "prefix" or "suffix"`)
	ErrInvalidDecimals       = errors.New("currency decimals out of range")
	ErrInvalidRate           = errors.New("exchange rate must be positive")
	ErrMetadataTooLarge      = errors.New("currency metadata is too large")
	ErrDecimalsLocked        = errors.New("currency decimals can't change once amounts are stored in it")
	ErrCurrencyUsedByChannel = errors.New("currency is a channel's currency; cannot deactivate")
	ErrRateBaseMismatch      = errors.New("rate feed answered for a different base currency")
)

type Repository interface {
	Create(ctx context.Context, input CreateCurrencyInput) (Currency, error)
	GetByCode(ctx context.Context, code string) (Currency, error)
	List(ctx context.Context, activeOnly bool) ([]Currency, error)
	Update(ctx context.Context, input UpdateCurrencyInput) (Currency, error)
	Delete(ctx context.Context, code string) error
	CountReferences(ctx context.Context, code string) (int64, error)
	// Whether any stored amount, historical orders included, is denominated in the currency
	HasAmounts(ctx context.Context, code string) (bool, error)
	CountChannels(ctx context.Context, code string) (int64, error)
	// Stores a refreshed exchange rate and stamps rate_updated_at - ErrCurrencyNotFound for an unknown code
	SetRate(ctx context.Context, code string, rate float64) error
}

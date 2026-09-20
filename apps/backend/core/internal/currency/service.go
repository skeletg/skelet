package currency

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Service handles currency logic - code validation, uppercase normalization,
// decimal-range enforcement, delete-by-reference guard
type Service struct {
	repo Repository
	// httpClient overrides the rate-fetch client (tests), nil = 15s-timeout default
	httpClient *http.Client
	// An alternate FX source consulted before the built-in feed - see remote.go
	remoteRates RemoteRateSource
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetHTTPClient(c *http.Client) { s.httpClient = c }

// Validates the code as 3 ASCII letters, normalizes case, and applies defaults for zero-valued fields
func (s *Service) Create(ctx context.Context, input CreateCurrencyInput) (Currency, error) {
	input.Code = normalizeCode(input.Code)
	if err := validateCode(input.Code); err != nil {
		return Currency{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Currency{}, fmt.Errorf("name is required")
	}
	if input.SymbolPosition == "" {
		input.SymbolPosition = SymbolPositionPrefix
	}
	if input.SymbolPosition != SymbolPositionPrefix && input.SymbolPosition != SymbolPositionSuffix {
		return Currency{}, fmt.Errorf(`symbol_position must be "prefix" or "suffix" (got %q)`, input.SymbolPosition)
	}
	if input.Decimals < 0 || input.Decimals > 4 {
		return Currency{}, fmt.Errorf("decimals must be 0..4 (got %d)", input.Decimals)
	}
	return s.repo.Create(ctx, input)
}

// Uppercases the lookup so callers don't have to normalize
func (s *Service) GetByCode(ctx context.Context, code string) (Currency, error) {
	return s.repo.GetByCode(ctx, normalizeCode(code))
}

func (s *Service) List(ctx context.Context, activeOnly bool) ([]Currency, error) {
	return s.repo.List(ctx, activeOnly)
}

// Pointer-merges non-nil fields - Code is the PK and immutable
func (s *Service) Update(ctx context.Context, input UpdateCurrencyInput) (Currency, error) {
	input.Code = normalizeCode(input.Code)
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Currency{}, fmt.Errorf("name is required")
	}
	if input.Decimals != nil && (*input.Decimals < 0 || *input.Decimals > 4) {
		return Currency{}, fmt.Errorf("decimals must be 0..4 (got %d)", *input.Decimals)
	}
	if input.SymbolPosition != nil {
		sp := *input.SymbolPosition
		if sp != SymbolPositionPrefix && sp != SymbolPositionSuffix {
			return Currency{}, fmt.Errorf(`symbol_position must be "prefix" or "suffix" (got %q)`, sp)
		}
	}
	return s.repo.Update(ctx, input)
}

// Refuses when any live-data column references the code - orders.currency_code is deliberately excluded, since historical orders are snapshots and mustn't block retirement
func (s *Service) Delete(ctx context.Context, code string) error {
	code = normalizeCode(code)
	count, err := s.repo.CountReferences(ctx, code)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %d reference(s)", ErrCurrencyInUse, count)
	}
	return s.repo.Delete(ctx, code)
}

// Stores a manual exchange-rate override - the scheduled refresh overwrites it next run
func (s *Service) SetRate(ctx context.Context, code string, rate float64) error {
	if rate <= 0 {
		return fmt.Errorf("exchange rate must be > 0 (got %v)", rate)
	}
	return s.repo.SetRate(ctx, normalizeCode(code), rate)
}

// Pre-validates a code before an FK insert, ErrCurrencyNotFound on a miss
func (s *Service) EnsureExists(ctx context.Context, code string) error {
	if _, err := s.repo.GetByCode(ctx, normalizeCode(code)); err != nil {
		if errors.Is(err, ErrCurrencyNotFound) {
			return err
		}
		return err
	}
	return nil
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func validateCode(code string) error {
	if len(code) != 3 {
		return fmt.Errorf("%w: code must be exactly 3 letters (got %q)", ErrInvalidCode, code)
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%w: %q must be ASCII uppercase letters", ErrInvalidCode, code)
		}
	}
	return nil
}

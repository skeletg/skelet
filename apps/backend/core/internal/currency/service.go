package currency

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
)

// Service handles currency logic - code validation, uppercase normalization,
// decimal-range enforcement, delete-by-reference guard
type Service struct {
	repo Repository
	// httpClient overrides the rate-fetch client (tests), nil = a ratesFetchTimeout default
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
		return Currency{}, ErrNameRequired
	}
	if input.SymbolPosition == "" {
		input.SymbolPosition = SymbolPositionPrefix
	}
	if err := validateSymbolPosition(input.SymbolPosition); err != nil {
		return Currency{}, err
	}
	if err := validateDecimals(input.Decimals); err != nil {
		return Currency{}, err
	}
	if err := validateMetadata(input.Metadata); err != nil {
		return Currency{}, err
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
		return Currency{}, ErrNameRequired
	}
	if input.Decimals != nil {
		if err := validateDecimals(*input.Decimals); err != nil {
			return Currency{}, err
		}
	}
	if input.SymbolPosition != nil {
		if err := validateSymbolPosition(*input.SymbolPosition); err != nil {
			return Currency{}, err
		}
	}
	if input.Metadata != nil {
		if err := validateMetadata(*input.Metadata); err != nil {
			return Currency{}, err
		}
	}
	if input.Decimals != nil || (input.IsActive != nil && !*input.IsActive) {
		current, err := s.repo.GetByCode(ctx, input.Code)
		if err != nil {
			return Currency{}, err
		}
		if input.Decimals != nil && *input.Decimals != current.Decimals {
			if err := s.guardDecimals(ctx, input.Code); err != nil {
				return Currency{}, err
			}
		}
		if input.IsActive != nil && !*input.IsActive && current.IsActive {
			if err := s.guardDeactivate(ctx, input.Code); err != nil {
				return Currency{}, err
			}
		}
	}
	return s.repo.Update(ctx, input)
}

// Stored amounts are minor units, so a new decimals value would silently rescale every one of them
func (s *Service) guardDecimals(ctx context.Context, code string) error {
	referenced, err := s.repo.HasAmounts(ctx, code)
	if err != nil {
		return err
	}
	if referenced {
		return ErrDecimalsLocked
	}
	return nil
}

// A channel prices and checks out in its currency, so the currency must stay active while one uses it
func (s *Service) guardDeactivate(ctx context.Context, code string) error {
	n, err := s.repo.CountChannels(ctx, code)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %d channel(s)", ErrCurrencyUsedByChannel, n)
	}
	return nil
}

// Refuses when any live-data column references the code - orders and subscription invoices are deliberately excluded, since they're snapshots and mustn't block retirement
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
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return fmt.Errorf("%w (got %v)", ErrInvalidRate, rate)
	}
	return s.repo.SetRate(ctx, normalizeCode(code), rate)
}

// Pre-validates a code before an FK insert, ErrCurrencyNotFound on a miss
func (s *Service) EnsureExists(ctx context.Context, code string) error {
	_, err := s.repo.GetByCode(ctx, normalizeCode(code))
	return err
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

func validateSymbolPosition(sp SymbolPosition) error {
	if sp != SymbolPositionPrefix && sp != SymbolPositionSuffix {
		return fmt.Errorf("%w (got %q)", ErrInvalidSymbolPosition, sp)
	}
	return nil
}

func validateDecimals(d int16) error {
	if d < MinDecimals || d > MaxDecimals {
		return fmt.Errorf("%w: %d..%d (got %d)", ErrInvalidDecimals, MinDecimals, MaxDecimals, d)
	}
	return nil
}

func validateMetadata(m map[string]string) error {
	if len(m) > maxMetadataKeys {
		return fmt.Errorf("%w: at most %d keys", ErrMetadataTooLarge, maxMetadataKeys)
	}
	for _, v := range m {
		if len(v) > maxMetadataValueLength {
			return fmt.Errorf("%w: values are at most %d bytes", ErrMetadataTooLarge, maxMetadataValueLength)
		}
	}
	return nil
}

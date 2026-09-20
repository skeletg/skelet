package locale

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Pre-validates that a country code exists in the countries table - wired post-construction, and it surfaces ErrCountryNotFound instead of a raw FK violation
type CountryValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

// Code normalization, BCP 47 shape validation, and the delete-by-reference-count guard - validation is deliberately loose, since full RFC 5646 grammar is out of scope and the regex only needs to catch obvious typos
type Service struct {
	repo      Repository
	countries CountryValidator
	// Invoked after an Update flips a locale's is_active flag - main.go wires it to nudge the search indexer, since deactivated locales must drop their per-channel docs and reactivated ones need them rebuilt
	// A plain callback rather than the events bus keeps this package dependency-free
	onStatusChange func(ctx context.Context, code string, isActive bool)
}

func (s *Service) SetStatusChangeHook(fn func(ctx context.Context, code string, isActive bool)) {
	s.onStatusChange = fn
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// The country-existence pre-check Create and Update use when default_country_code is supplied - optional
func (s *Service) SetCountryValidator(v CountryValidator) {
	s.countries = v
}

// Loose BCP 47 shape - a 2-3 letter language plus optional script, region and variant subtags - It rejects underscores and obviously bogus strings, and mixed case is handled by lowercasing first
var bcp47Shape = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)

// Validates the code shape, normalizes presentation fields, and inserts - NativeName defaults to Name and separators default to "." and ",", so storefront date and number rendering can't blow up on a half-filled row
func (s *Service) Create(ctx context.Context, input CreateLocaleInput) (Locale, error) {
	input.Code = normalizeCode(input.Code)
	if err := validateCode(input.Code); err != nil {
		return Locale{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Locale{}, fmt.Errorf("name is required")
	}
	if input.NativeName == "" {
		input.NativeName = input.Name
	}
	if input.DecimalSeparator == "" {
		input.DecimalSeparator = "."
	}
	if input.ThousandsSeparator == "" {
		input.ThousandsSeparator = ","
	}
	if input.FirstDayOfWeek == 0 && input.DefaultCountryCode == "" {
		// Treat 0 as "no opinion" and pick Monday - the column DEFAULT 1 would kick in anyway if we left the zero, so this only matters for a caller explicitly setting 0 for en-US
		input.FirstDayOfWeek = 1
	}
	// Normalize before validating, the way Update does - validateOptionalFields only cases its inputs, so a padded " at " off an admin form would fail the length check here and pass on the edit screen
	input.DefaultCountryCode = strings.ToUpper(strings.TrimSpace(input.DefaultCountryCode))
	input.HreflangValue = strings.ToLower(strings.TrimSpace(input.HreflangValue))
	if err := validateOptionalFields(input.FirstDayOfWeek, input.DefaultCountryCode, input.HreflangValue); err != nil {
		return Locale{}, err
	}
	if input.DefaultCountryCode != "" && s.countries != nil {
		if err := s.countries.EnsureExists(ctx, input.DefaultCountryCode); err != nil {
			return Locale{}, err
		}
	}
	return s.repo.Create(ctx, input)
}

// Lower-cases the lookup so callers don't have to normalize
func (s *Service) GetByCode(ctx context.Context, code string) (Locale, error) {
	return s.repo.GetByCode(ctx, normalizeCode(code))
}

// Active-only is what the admin pickers and storefront language switcher want, and the full list backs the admin Languages page
func (s *Service) List(ctx context.Context, activeOnly bool) ([]Locale, error) {
	return s.repo.List(ctx, activeOnly)
}

// A partial update by pointer-merging non-nil fields - Code is the PK and immutable, and any field the caller changes gets the same shape checks as Create
func (s *Service) Update(ctx context.Context, input UpdateLocaleInput) (Locale, error) {
	input.Code = normalizeCode(input.Code)
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Locale{}, fmt.Errorf("name is required")
	}
	if input.FirstDayOfWeek != nil {
		if *input.FirstDayOfWeek < 0 || *input.FirstDayOfWeek > 6 {
			return Locale{}, fmt.Errorf("first_day_of_week must be 0..6 (got %d)", *input.FirstDayOfWeek)
		}
	}
	if input.DefaultCountryCode != nil {
		c := strings.ToUpper(strings.TrimSpace(*input.DefaultCountryCode))
		if c != "" && !isTwoLetterCountry(c) {
			return Locale{}, fmt.Errorf("default_country_code must be empty or ISO 3166-1 alpha-2 (got %q)", *input.DefaultCountryCode)
		}
		if c != "" && s.countries != nil {
			if err := s.countries.EnsureExists(ctx, c); err != nil {
				return Locale{}, err
			}
		}
		input.DefaultCountryCode = &c
	}
	if input.HreflangValue != nil {
		v := strings.ToLower(strings.TrimSpace(*input.HreflangValue))
		if v != "" && !bcp47Shape.MatchString(v) {
			return Locale{}, fmt.Errorf("%w: hreflang_value %q does not look like a BCP 47 code", ErrInvalidCode, *input.HreflangValue)
		}
		input.HreflangValue = &v
	}
	// Detect an is_active flip BEFORE the write, so the hook fires on a real transition rather than every update that carries the field
	var statusFlipped bool
	if input.IsActive != nil {
		if current, err := s.repo.GetByCode(ctx, input.Code); err == nil {
			statusFlipped = current.IsActive != *input.IsActive
		}
	}
	out, err := s.repo.Update(ctx, input)
	if err != nil {
		return Locale{}, err
	}
	if statusFlipped && s.onStatusChange != nil {
		s.onStatusChange(ctx, out.Code, out.IsActive)
	}
	return out, nil
}

// The shape checks reused between Create and the lower-level update paths
func validateOptionalFields(firstDayOfWeek int16, countryCode, hreflang string) error {
	if firstDayOfWeek < 0 || firstDayOfWeek > 6 {
		return fmt.Errorf("first_day_of_week must be 0..6 (got %d)", firstDayOfWeek)
	}
	if countryCode != "" && !isTwoLetterCountry(strings.ToUpper(countryCode)) {
		return fmt.Errorf("default_country_code must be empty or ISO 3166-1 alpha-2 (got %q)", countryCode)
	}
	if hreflang != "" && !bcp47Shape.MatchString(strings.ToLower(hreflang)) {
		return fmt.Errorf("%w: hreflang_value %q does not look like a BCP 47 code", ErrInvalidCode, hreflang)
	}
	return nil
}

// A loose ISO 3166-1 alpha-2 shape, exactly two A-Z characters - it doesn't validate against the published ISO list, which is a follow-up if real validation matters
func isTwoLetterCountry(s string) bool {
	if len(s) != 2 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// Refuses when any locale-bearing column references this code, handing back a count so the admin UI can say "47 references, resolve them first" - mirrors the currency-change guard in channel.Service.Update
func (s *Service) Delete(ctx context.Context, code string) error {
	code = normalizeCode(code)
	count, err := s.repo.CountReferences(ctx, code)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %d reference(s)", ErrLocaleInUse, count)
	}
	return s.repo.Delete(ctx, code)
}

// A friendlier pre-check for callers who'd otherwise hit a raw FK violation, returning ErrLocaleNotFound when the code is missing - used by channel.Service.SetLocales and the catalog content writers
func (s *Service) EnsureExists(ctx context.Context, code string) error {
	if _, err := s.repo.GetByCode(ctx, normalizeCode(code)); err != nil {
		if errors.Is(err, ErrLocaleNotFound) {
			return err
		}
		return err
	}
	return nil
}

// Reports whether a tag is worth carrying - the same loose shape check validateCode applies, exported so the HTTP middleware can reject junk without a database round trip
func LooksLikeCode(code string) bool {
	return bcp47Shape.MatchString(normalizeCode(code))
}

func normalizeCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func validateCode(code string) error {
	if code == "" {
		return fmt.Errorf("%w: empty code", ErrInvalidCode)
	}
	if !bcp47Shape.MatchString(code) {
		return fmt.Errorf("%w: %q does not look like a BCP 47 code", ErrInvalidCode, code)
	}
	return nil
}

package country

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

// Consulted before writes that set default_currency_code - wired post-construction so country to currency can't become a circular import if currencies ever gain a country reference, mirroring the locale-channel pattern
type CurrencyValidator interface {
	EnsureExists(ctx context.Context, code string) error
}

type ConfigReader interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

// What deleting a country needs to take its code out of the country options wherever they were set - optional, and when nil the stale code stays and the pickers ignore it
type ConfigWriter interface {
	ListStored(ctx context.Context, path string) ([]config.StoredValue, error)
	Set(ctx context.Context, path, value string, scope config.Scope) (config.Entry, error)
	Reset(ctx context.Context, path string, scope config.Scope) (config.Entry, error)
}

type Service struct {
	repo       Repository
	currencies CurrencyValidator
	cfg        ConfigReader
	cfgWriter  ConfigWriter
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Wires the scoped config reader the country option helpers use - optional, and when nil they return their unrestricted defaults
func (s *Service) SetConfigService(cfg ConfigReader) {
	s.cfg = cfg
	if w, ok := cfg.(ConfigWriter); ok {
		s.cfgWriter = w
	}
}

// Optional - when nil, FK violations still reject invalid codes, just with uglier messages
func (s *Service) SetCurrencyValidator(v CurrencyValidator) {
	s.currencies = v
}

// Validates the code shape as 2 ASCII letters, normalizes case, and pre-validates default_currency_code when set
func (s *Service) Create(ctx context.Context, input CreateCountryInput) (Country, error) {
	input.Code = normalizeCode(input.Code)
	if err := validateCode(input.Code); err != nil {
		return Country{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return Country{}, ErrNameRequired
	}
	if err := validateMetadata(input.Metadata); err != nil {
		return Country{}, err
	}
	input.PhoneCode = callingCodeFor(input.Code, input.PhoneCode)
	input.DefaultCurrencyCode = strings.ToUpper(strings.TrimSpace(input.DefaultCurrencyCode))
	if input.DefaultCurrencyCode != "" && s.currencies != nil {
		if err := s.currencies.EnsureExists(ctx, input.DefaultCurrencyCode); err != nil {
			return Country{}, err
		}
	}
	return s.repo.Create(ctx, input)
}

// Uppercases the lookup so callers don't need to normalize
func (s *Service) GetByCode(ctx context.Context, code string) (Country, error) {
	return s.repo.GetByCode(ctx, normalizeCode(code))
}

func (s *Service) List(ctx context.Context, activeOnly bool) ([]Country, error) {
	return s.repo.List(ctx, activeOnly)
}

// The ISO 3166-2 subdivisions seeded for a country - empty when it has none, and callers fall back to free text
func (s *Service) Regions(ctx context.Context, code string) ([]Region, error) {
	return s.repo.ListRegions(ctx, normalizeCode(code))
}

func (s *Service) Update(ctx context.Context, input UpdateCountryInput) (Country, error) {
	input.Code = normalizeCode(input.Code)
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return Country{}, ErrNameRequired
	}
	if input.Metadata != nil {
		if err := validateMetadata(*input.Metadata); err != nil {
			return Country{}, err
		}
	}
	// A known country's calling code isn't editable - it's re-derived, so a stored typo heals on the next save
	if cc := phone.CallingCode(input.Code); cc != "" {
		input.PhoneCode = &cc
	}
	if input.DefaultCurrencyCode != nil {
		v := strings.ToUpper(strings.TrimSpace(*input.DefaultCurrencyCode))
		if v != "" && s.currencies != nil {
			if err := s.currencies.EnsureExists(ctx, v); err != nil {
				return Country{}, err
			}
		}
		input.DefaultCurrencyCode = &v
	}
	return s.repo.Update(ctx, input)
}

// Refuses when any live-data column references this code - mirrors the currency and locale delete guards
func (s *Service) Delete(ctx context.Context, code string) error {
	code = normalizeCode(code)
	count, err := s.repo.CountReferences(ctx, code)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %d reference(s)", ErrCountryInUse, count)
	}
	if err := s.repo.Delete(ctx, code); err != nil {
		return err
	}
	return s.dropFromSettings(ctx, code)
}

// Takes a deleted code out of every country option at every scope, through the config service so its cache drops with it
func (s *Service) dropFromSettings(ctx context.Context, code string) error {
	if s.cfgWriter == nil {
		return nil
	}
	for _, path := range codeSettings {
		rows, err := s.cfgWriter.ListStored(ctx, path)
		if err != nil {
			return fmt.Errorf("list %s: %w", path, err)
		}
		for _, row := range rows {
			codes := parseCodes(row.Value)
			kept := slices.DeleteFunc(slices.Clone(codes), func(c string) bool { return c == code })
			if len(kept) == len(codes) {
				continue
			}
			if len(kept) == 0 {
				_, err = s.cfgWriter.Reset(ctx, path, row.Scope)
			} else {
				_, err = s.cfgWriter.Set(ctx, path, strings.Join(kept, ","), row.Scope)
			}
			if err != nil {
				return fmt.Errorf("drop %s from %s: %w", code, path, err)
			}
		}
	}
	return nil
}

// The friendlier check locale and customer services use to pre-validate a country code before the FK insert - ErrCountryNotFound on a miss
func (s *Service) EnsureExists(ctx context.Context, code string) error {
	_, err := s.repo.GetByCode(ctx, normalizeCode(code))
	return err
}

// The allowed ISO alpha-2 codes for the scope, or nil when unrestricted - "all active" is the default and also what happens with no config service wired
func (s *Service) AllowedCountries(ctx context.Context, scope config.Scope) map[string]bool {
	return s.codeSet(ctx, scope, CfgAllowedCountries)
}

// The country codes requiring a region or state at checkout and address save
func (s *Service) RequiredRegionCountries(ctx context.Context, scope config.Scope) map[string]bool {
	return s.codeSet(ctx, scope, CfgRequiredRegion)
}

func (s *Service) EUCountries(ctx context.Context, scope config.Scope) map[string]bool {
	return s.codeSet(ctx, scope, CfgEUCountries)
}

// The ordered country codes to pin to the top of pickers
func (s *Service) TopDestinations(ctx context.Context, scope config.Scope) []string {
	return s.codeList(ctx, scope, CfgTopDestinations)
}

// "" when unset, or when it names a country the shopper couldn't pick - inactive, unknown or outside the allow-list
func (s *Service) DefaultCountry(ctx context.Context, scope config.Scope) string {
	if s.cfg == nil {
		return ""
	}
	v, err := s.cfg.GetString(ctx, CfgDefaultCountry, scope)
	if err != nil {
		return ""
	}
	code := normalizeCode(v)
	if code == "" {
		return ""
	}
	if c, err := s.repo.GetByCode(ctx, code); err != nil || !c.IsActive {
		return ""
	}
	if allowed := s.AllowedCountries(ctx, scope); allowed != nil && !allowed[code] {
		return ""
	}
	return code
}

// Whether a shopper on this scope may pick the country - active and inside the allow-list, if one is set
func (s *Service) Offered(ctx context.Context, scope config.Scope, c Country) bool {
	if !c.IsActive {
		return false
	}
	allowed := s.AllowedCountries(ctx, scope)
	return allowed == nil || allowed[c.Code]
}

// Whether the storefront should offer a non-required region field for countries that have subdivisions
func (s *Service) AllowOptionalState(ctx context.Context, scope config.Scope) bool {
	if s.cfg == nil {
		return false
	}
	v, err := s.cfg.GetBool(ctx, CfgAllowOptionalState, scope)
	if err != nil {
		return false
	}
	return v
}

// For display - a settings read failure shows the unrestricted list, while ValidateAddress reads through strictCodeSet and refuses instead
func (s *Service) codeList(ctx context.Context, scope config.Scope, path string) []string {
	codes, _ := s.readCodes(ctx, scope, path)
	return codes
}

func (s *Service) readCodes(ctx context.Context, scope config.Scope, path string) ([]string, error) {
	if s.cfg == nil {
		return nil, nil
	}
	raw, err := s.cfg.GetString(ctx, path, scope)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return parseCodes(raw), nil
}

func (s *Service) codeSet(ctx context.Context, scope config.Scope, path string) map[string]bool {
	set, _ := s.strictCodeSet(ctx, scope, path)
	return set
}

func (s *Service) strictCodeSet(ctx context.Context, scope config.Scope, path string) (map[string]bool, error) {
	codes, err := s.readCodes(ctx, scope, path)
	if err != nil || len(codes) == 0 {
		return nil, err
	}
	set := make(map[string]bool, len(codes))
	for _, c := range codes {
		set[c] = true
	}
	return set, nil
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

// Splits a comma-separated multiselect value into normalized uppercase codes, dropping blanks
func parseCodes(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if c := strings.ToUpper(strings.TrimSpace(p)); c != "" {
			out = append(out, c)
		}
	}
	return out
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func validateCode(code string) error {
	if len(code) != 2 {
		return fmt.Errorf("%w: code must be exactly 2 letters (got %q)", ErrInvalidCode, code)
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%w: %q must be ASCII uppercase letters", ErrInvalidCode, code)
		}
	}
	return nil
}

// A country's calling code comes from libphonenumber, as every phone check does - the typed one only stands for a
// code the metadata doesn't know
func callingCodeFor(code, typed string) string {
	if cc := phone.CallingCode(code); cc != "" {
		return cc
	}
	return strings.TrimSpace(typed)
}

package country

import (
	"context"
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// fakeRepo implements Repository; only GetByCode + ListRegions are
// exercised by ValidateAddress
type fakeRepo struct {
	countries map[string]Country
	regions   map[string][]Region
}

func (f fakeRepo) GetByCode(_ context.Context, code string) (Country, error) {
	c, ok := f.countries[code]
	if !ok {
		return Country{}, ErrCountryNotFound
	}
	return c, nil
}
func (f fakeRepo) ListRegions(_ context.Context, code string) ([]Region, error) {
	return f.regions[code], nil
}
func (f fakeRepo) Create(context.Context, CreateCountryInput) (Country, error) {
	return Country{}, nil
}
func (f fakeRepo) List(context.Context, bool) ([]Country, error)               { return nil, nil }
func (f fakeRepo) Update(context.Context, UpdateCountryInput) (Country, error) { return Country{}, nil }
func (f fakeRepo) Delete(context.Context, string) error                        { return nil }
func (f fakeRepo) CountReferences(context.Context, string) (int64, error)      { return 0, nil }

// fakeConfig returns canned values per path
type fakeConfig struct {
	strings map[string]string
	bools   map[string]bool
}

func (f fakeConfig) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f.strings[path], nil
}
func (f fakeConfig) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return f.bools[path], nil
}

func newSvc(cfg ConfigReader) *Service {
	s := NewService(fakeRepo{
		countries: map[string]Country{
			"US": {Code: "US", IsActive: true},
			"CA": {Code: "CA", IsActive: true},
			"XX": {Code: "XX", IsActive: false}, // inactive
		},
		regions: map[string][]Region{
			"US": {{CountryCode: "US", Code: "CA", Name: "California"}, {CountryCode: "US", Code: "NY", Name: "New York"}},
			// CA (country) has no seeded regions → free text
		},
	})
	if cfg != nil {
		s.SetConfigService(cfg)
	}
	return s
}

func ptr(s string) *string { return &s }

func TestRegionMatches(t *testing.T) {
	regions := []Region{
		{CountryCode: "US", Code: "CA", Name: "California"},
		{CountryCode: "US", Code: "NY", Name: "New York"},
	}
	cases := []struct {
		name    string
		regions []Region
		val     string
		want    bool
	}{
		{"exact match", regions, "CA", true},
		{"case-insensitive lower", regions, "ny", true},
		{"case-insensitive mixed", regions, "Ny", true},
		{"no match", regions, "TX", false},
		{"empty value never matches", regions, "", false},
		{"empty region list", nil, "CA", false},
		{"matches against name not allowed", regions, "California", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := regionMatches(tc.regions, tc.val); got != tc.want {
				t.Errorf("regionMatches(%v, %q) = %v, want %v", tc.regions, tc.val, got, tc.want)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ConfigReader
		country string
		region  *string
		wantErr error // nil = ok
	}{
		{name: "active, unrestricted, no region", country: "CA", region: nil},
		{name: "bad code shape", country: "USA", region: nil, wantErr: ErrInvalidCode},
		{name: "unknown country", country: "ZZ", region: nil, wantErr: ErrCountryNotFound},
		{name: "inactive country rejected", country: "XX", region: nil, wantErr: ErrCountryNotAllowed},
		{
			name:    "not in allow-list rejected",
			cfg:     fakeConfig{strings: map[string]string{CfgAllowedCountries: "CA"}},
			country: "US", region: nil, wantErr: ErrCountryNotAllowed,
		},
		{
			name:    "in allow-list ok",
			cfg:     fakeConfig{strings: map[string]string{CfgAllowedCountries: "US,CA"}},
			country: "US", region: ptr("CA"),
		},
		{
			name:    "required region missing",
			cfg:     fakeConfig{strings: map[string]string{CfgRequiredRegion: "US"}},
			country: "US", region: nil, wantErr: ErrRegionRequired,
		},
		{
			name:    "required region present + valid",
			cfg:     fakeConfig{strings: map[string]string{CfgRequiredRegion: "US"}},
			country: "US", region: ptr("ny"), // case-insensitive
		},
		{
			name:    "invalid region for country with a canonical list",
			country: "US", region: ptr("ZZ"), wantErr: ErrInvalidRegion,
		},
		{
			name:    "free-text region ok when country has no seeded regions",
			country: "CA", region: ptr("Ontario"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newSvc(tc.cfg).ValidateAddress(context.Background(), config.DefaultScope, tc.country, tc.region)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want errors.Is(%v)", err, tc.wantErr)
			}
		})
	}
}

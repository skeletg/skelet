// Package country manages first-class countries - ISO 3166-1 alpha-2 codes registered once and referenced by FK from addresses, locales.default_country_code and admin role scopes - Mirrors the currency and locale package shape
package country

import (
	"context"
	"errors"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Config paths for the per-channel country and region restrictions - multiselect values are comma-separated uppercase ISO alpha-2 codes, and an empty `allowed` means all active
const (
	CfgAllowedCountries   = "general/country/allowed"
	CfgDefaultCountry     = "general/country/default"
	CfgRequiredRegion     = "general/country/required_region"
	CfgAllowOptionalState = "general/country/allow_optional_state"
	CfgTopDestinations    = "general/country/top_destinations"
	CfgEUCountries        = "general/country/eu_countries"

	// The config option source the country pickers read from
	OptionSource = "countries"
)

// The settings that name country codes - a deleted country is taken out of each
var codeSettings = []string{CfgAllowedCountries, CfgDefaultCountry, CfgRequiredRegion, CfgTopDestinations, CfgEUCountries}

const (
	maxMetadataKeys        = 50
	maxMetadataValueLength = 1024
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "countries",
		Name:      "Countries",
		SortOrder: 67,
	})
	permission.Register(permission.Definition{
		Group: "countries",
		Slug:  "countries.view",
		Name:  "View countries",
	})
	permission.Register(permission.Definition{
		Group: "countries",
		Slug:  "countries.manage",
		Name:  "Manage countries",
	})

	// Per-channel country options, grouped under the existing "general" section owned by config - the multiselects pull options from the "countries" source registered in main.go, and they're public so the storefront can read channel-scoped values
	config.RegisterGroup(config.Group{Section: "general", Slug: "country", Label: "Country Options", SortOrder: 4, Scoping: config.ScopingChannel})
	config.Register(config.Definition{Path: CfgAllowedCountries, Type: config.TypeMultiselect, Label: "Allowed countries", Public: true, Source: OptionSource})
	config.Register(config.Definition{Path: CfgDefaultCountry, Type: config.TypeSelect, Label: "Default country", Public: true, Source: OptionSource})
	config.Register(config.Definition{Path: CfgRequiredRegion, Type: config.TypeMultiselect, Label: "Countries requiring a region/state", Public: true, Source: OptionSource})
	config.Register(config.Definition{Path: CfgAllowOptionalState, Type: config.TypeBool, Default: "false", Label: "Show optional state field", Public: true})
	config.Register(config.Definition{Path: CfgTopDestinations, Type: config.TypeMultiselect, Label: "Top destinations", Public: true, Source: OptionSource})
	config.Register(config.Definition{Path: CfgEUCountries, Type: config.TypeMultiselect, Label: "EU countries (VAT grouping)", Public: true, Source: OptionSource})
}

// One ISO 3166-1 alpha-2 code with display and presentation hints - DefaultCurrencyCode is a soft FK to currencies(code), set to NULL on currency delete, unlike the RESTRICT policy everywhere else
type Country struct {
	Code       string
	Name       string
	NativeName string
	EmojiFlag  string
	PhoneCode  string
	// ISO 3166-1 alpha-3 and numeric-3 for integration interop, empty until the core:enrich_countries_iso data patch runs
	Iso3Code            string
	NumericCode         string
	DefaultCurrencyCode string // "" when unset (NULL in DB)
	IsActive            bool
	SortOrder           int32
	Metadata            map[string]string
	BusinessIDFields    []BusinessIDField
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// An ISO 3166-2 subdivision of a country - Code is the subdivision part only, so the full ISO code is Country.Code plus "-" plus Code
type Region struct {
	CountryCode string
	Code        string
	Name        string
	IsActive    bool
	SortOrder   int32
}

type CreateCountryInput struct {
	Code                string
	Name                string
	NativeName          string
	EmojiFlag           string
	PhoneCode           string
	DefaultCurrencyCode string
	IsActive            bool
	SortOrder           int32
	Metadata            map[string]string
	BusinessIDFields    []BusinessIDField
}

type UpdateCountryInput struct {
	Code                string
	Name                *string
	NativeName          *string
	EmojiFlag           *string
	PhoneCode           *string
	DefaultCurrencyCode *string
	IsActive            *bool
	SortOrder           *int32
	Metadata            *map[string]string
	BusinessIDFields    *[]BusinessIDField
}

var (
	ErrCountryNotFound  = errors.New("country not found")
	ErrCountryCodeTaken = errors.New("country code already exists")
	ErrCountryInUse     = errors.New("country has references; cannot delete")
	ErrInvalidCode      = errors.New("invalid ISO 3166-1 alpha-2 country code")
	ErrNameRequired     = errors.New("country name is required")
	ErrMetadataTooLarge = errors.New("country metadata is too large")

	ErrCountryNotAllowed = errors.New("country not available for this store")
	ErrRegionRequired    = errors.New("region/state is required for this country")
	ErrInvalidRegion     = errors.New("region/state is not valid for this country")
)

type Repository interface {
	Create(ctx context.Context, input CreateCountryInput) (Country, error)
	GetByCode(ctx context.Context, code string) (Country, error)
	List(ctx context.Context, activeOnly bool) ([]Country, error)
	Update(ctx context.Context, input UpdateCountryInput) (Country, error)
	Delete(ctx context.Context, code string) error
	CountReferences(ctx context.Context, code string) (int64, error)
	ListRegions(ctx context.Context, countryCode string) ([]Region, error)
}

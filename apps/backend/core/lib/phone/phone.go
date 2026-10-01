// Package phone is the single definition of a valid phone number - the per-country rules (calling codes, lengths, mobile and fixed-line patterns) come from Google's libphonenumber metadata via the nyaruka port, so nothing here is hand-maintained
// Every stored number is E.164 ("+381641234567"): two spellings of one number compare equal, and pretty formatting is a read-side concern for the UI
package phone

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/nyaruka/phonenumbers"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrInvalid           = errors.New("phone number is not valid")
	ErrCountryNotAllowed = errors.New("phone number must be from one of the store's countries")
)

func init() {
	apperr.RegisterSentinel(ErrInvalid, "PHONE_INVALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCountryNotAllowed, "PHONE_COUNTRY_NOT_ALLOWED", apperr.ClassValidation)
}

// Rules are one store's phone policy - the country domain builds them from its settings
type Rules struct {
	Default string   // the country a national number is read in when the caller has none of its own
	Allowed []string // nil takes a number from any country; otherwise it must belong to one of these
}

// Normalize is the package Normalize under a store's rules - region, when given, beats Default, and a store with one
// allowed country reads every national number in it, whatever the address
func (r Rules) Normalize(raw, region string) (string, error) {
	switch {
	case len(r.Allowed) == 1:
		region = r.Allowed[0]
	case strings.TrimSpace(region) == "":
		region = r.Default
	}
	n, err := Normalize(raw, region)
	if err != nil || n == "" || r.Allowed == nil {
		return n, err
	}
	num, err := phonenumbers.Parse(n, "")
	if err != nil {
		return "", ErrInvalid
	}
	// The number's own country, not its calling code - +1 is shared by the US and Canada
	if !slices.Contains(r.Allowed, phonenumbers.GetRegionCodeForNumber(num)) {
		return "", ErrCountryNotAllowed
	}
	return n, nil
}

// NormalizePtr is Rules.Normalize for the nullable columns, with the package NormalizePtr's nil handling
func (r Rules) NormalizePtr(raw *string, region string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	n, err := r.Normalize(*raw, region)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// Normalize returns raw's E.164 form - blank is "" with no error, so an optional field stays optional and the caller decides whether empty is allowed
// region (ISO alpha-2, any case) is only a parsing hint for a nationally typed number like "064 123 4567"; a number typed with its + or 00 prefix is accepted whatever the region, so a shopper in Serbia can still give a German mobile. With no region, only international input parses
func Normalize(raw, region string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	num, err := phonenumbers.Parse(raw, strings.ToUpper(strings.TrimSpace(region)))
	if err != nil || !phonenumbers.IsValidNumber(num) {
		return "", ErrInvalid
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}

// NormalizePtr is Normalize for the nullable columns - nil stays nil (the field isn't being written), anything else comes back normalized, blank included
func NormalizePtr(raw *string, region string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	n, err := Normalize(*raw, region)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// CountryFormat is what a form shows for one country's phones - the prefix, an example to use as the placeholder,
// and the example's shape for an input mask
type CountryFormat struct {
	Country         string // ISO alpha-2
	CallingCode     string // "+381"
	Example         string // "+381 60 1234567", international
	NationalExample string // "060 1234567", as typed at home
	Template        string // "xxx xxxxxxx" - NationalExample with every digit an x
}

// CallingCode is a country's "+381" - "" for a code the metadata doesn't know. The one source for a country's calling
// code: the countries table stores it, but only as this derives it
func CallingCode(country string) string {
	n := phonenumbers.GetCountryCodeForRegion(strings.ToUpper(strings.TrimSpace(country)))
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("+%d", n)
}

// FormatFor builds a country's format from its example mobile number - false for a code the metadata doesn't know
func FormatFor(country string) (CountryFormat, bool) {
	country = strings.ToUpper(strings.TrimSpace(country))
	num := phonenumbers.GetExampleNumberForType(country, phonenumbers.MOBILE)
	if num == nil {
		return CountryFormat{}, false
	}
	national := phonenumbers.Format(num, phonenumbers.NATIONAL)
	template := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return 'x'
		}
		return r
	}, national)
	return CountryFormat{
		Country:         country,
		CallingCode:     CallingCode(country),
		Example:         phonenumbers.Format(num, phonenumbers.INTERNATIONAL),
		NationalExample: national,
		Template:        template,
	}, true
}

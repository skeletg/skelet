package customer

import (
	"fmt"
	"net/mail"
	"unicode/utf8"
)

// ValidateEmail normalizes and checks addr the same way account creation does
func ValidateEmail(addr string) error { return validateEmail(NormalizeEmail(addr)) }

// Expects an already-normalized address - the parsed form has to round-trip, so display names and comments are refused
func validateEmail(addr string) error {
	if addr == "" || len(addr) > MaxEmailBytes {
		return ErrInvalidEmail
	}
	parsed, err := mail.ParseAddress(addr)
	if err != nil || parsed.Address != addr {
		return ErrInvalidEmail
	}
	return nil
}

func checkLen(field, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return fmt.Errorf("%w: %s (at most %d characters)", ErrFieldTooLong, field, max)
	}
	return nil
}

func checkOptLen(field string, value *string, max int) error {
	if value == nil {
		return nil
	}
	return checkLen(field, *value, max)
}

func ValidateProfile(in UpdateCustomerInput) error {
	if err := validateNames(in.FirstName, in.LastName); err != nil {
		return err
	}
	return checkOptLen("phone", in.Phone, MaxPhoneRunes)
}

func validateNames(first, last string) error {
	if err := checkLen("first name", first, MaxNameRunes); err != nil {
		return err
	}
	return checkLen("last name", last, MaxNameRunes)
}

func validateAddressInput(in AddressInput) error {
	for _, c := range []struct {
		field string
		value string
		max   int
	}{
		{"first name", in.FirstName, MaxNameRunes},
		{"last name", in.LastName, MaxNameRunes},
		{"street", in.Street1, MaxStreetRunes},
		{"city", in.City, MaxCityRunes},
		{"postal code", in.PostalCode, MaxPostalCodeRunes},
	} {
		if err := checkLen(c.field, c.value, c.max); err != nil {
			return err
		}
	}
	if err := checkOptLen("company", in.Company, MaxCompanyRunes); err != nil {
		return err
	}
	if err := checkOptLen("street", in.Street2, MaxStreetRunes); err != nil {
		return err
	}
	if err := checkOptLen("state", in.State, MaxStateRunes); err != nil {
		return err
	}
	return checkOptLen("phone", in.Phone, MaxPhoneRunes)
}

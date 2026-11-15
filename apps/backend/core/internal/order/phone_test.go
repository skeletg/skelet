package order

import (
	"errors"
	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
	"testing"
)

func TestResolveAddressNormalizesInlineAndSavedPhones(t *testing.T) {
	raw := "01512 3456789"
	rules := phone.Rules{Default: "DE", Allowed: []string{"DE"}}
	inline := OrderAddress{Street1: "Street", City: "City", PostalCode: "75001", CountryCode: "FR", Phone: &raw}
	id := uuid.New()
	c := customer.Customer{Addresses: []customer.Address{{ID: id, CountryCode: "FR", Phone: &raw}}}
	for _, saved := range []bool{false, true} {
		var a OrderAddress
		var err error
		if saved {
			a, err = resolveAddress(rules, nil, &id, true, c, "shipping", false)
		} else {
			a, err = resolveAddress(rules, &inline, nil, false, c, "billing", false)
		}
		if err != nil || a.Phone == nil || *a.Phone != "+4915123456789" {
			t.Fatalf("saved=%v: phone=%v, error=%v", saved, a.Phone, err)
		}
	}
	if raw != "01512 3456789" {
		t.Fatal("mutated caller's phone")
	}
	foreign := "+33612345678"
	inline.Phone = &foreign
	if _, err := resolveAddress(rules, &inline, nil, false, c, "billing", false); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Fatalf("error = %v, want disallowed country", err)
	}
}

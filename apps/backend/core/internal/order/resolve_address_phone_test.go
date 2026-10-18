package order

import (
	"errors"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

func typedAddress(country, tel string) *OrderAddress {
	return &OrderAddress{
		FirstName: "Ana", LastName: "Ilić", Street1: "Unter den Linden 1", City: "Berlin",
		PostalCode: "10117", CountryCode: country, Phone: &tel,
	}
}

// A typed address - at checkout placement or on an admin draft - stores its phone as E.164, read in its own country
func TestResolveAddressNormalizesTheTypedPhone(t *testing.T) {
	got, err := resolveAddress(phone.Rules{}, typedAddress("DE", "01512 3456789"), nil, false, customer.Customer{}, "shipping", false)
	if err != nil || got.Phone == nil || *got.Phone != "+4915123456789" {
		t.Fatalf("resolveAddress = %v, %v, want phone +4915123456789", got.Phone, err)
	}

	_, err = resolveAddress(phone.Rules{}, typedAddress("US", "01512 3456789"), nil, false, customer.Customer{}, "billing", false)
	if !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("a German national number on a US address = %v, want phone.ErrInvalid", err)
	}

	in := typedAddress("DE", "")
	in.Phone = nil
	got, err = resolveAddress(phone.Rules{}, in, nil, false, customer.Customer{}, "shipping", false)
	if err != nil || got.Phone != nil {
		t.Fatalf("no phone = %v, %v, want none", got.Phone, err)
	}
}

// The caller's address is never rewritten in place - drafts reuse their input
func TestResolveAddressLeavesTheInputAlone(t *testing.T) {
	in := typedAddress("DE", "01512 3456789")
	if _, err := resolveAddress(phone.Rules{}, in, nil, false, customer.Customer{}, "shipping", false); err != nil {
		t.Fatal(err)
	}
	if *in.Phone != "01512 3456789" {
		t.Errorf("input phone = %q, want it untouched", *in.Phone)
	}
}

// A quote only needs the destination, so a partial address - phone and all - passes as typed
func TestResolveAddressQuoteSkipsThePhone(t *testing.T) {
	got, err := resolveAddress(phone.Rules{}, typedAddress("DE", "not yet"), nil, false, customer.Customer{}, "shipping", true)
	if err != nil || got.Phone == nil || *got.Phone != "not yet" {
		t.Fatalf("quote = %v, %v, want the address as typed", got.Phone, err)
	}
}

// A store that keeps phones to its own countries refuses a typed address's phone from anywhere else
func TestResolveAddressAppliesTheStoresRules(t *testing.T) {
	serbiaOnly := phone.Rules{Default: "RS", Allowed: []string{"RS"}}
	_, err := resolveAddress(serbiaOnly, typedAddress("DE", "+49 1512 3456789"), nil, false, customer.Customer{}, "shipping", false)
	if !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Fatalf("a German number on a Serbia-only store = %v, want phone.ErrCountryNotAllowed", err)
	}
}

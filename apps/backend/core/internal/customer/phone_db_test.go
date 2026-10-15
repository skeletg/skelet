package customer

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	_ "github.com/skeletg/skelet/apps/backend/core/internal/country" // registers general/country/default
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
)

func (f *fixture) setDefaultCountry(t *testing.T, channelID uuid.UUID, code string) {
	t.Helper()
	if _, err := f.cfg.Set(f.ctx, pathDefaultCountry, code, config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}); err != nil {
		t.Fatalf("set default country: %v", err)
	}
}

func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// A profile phone has no address, so a national number is read in the store's default country - with none set,
// only an international one can be understood
func TestProfilePhoneIsReadInTheStoresDefaultCountry(t *testing.T) {
	f := newFixture(t)
	serbian := f.newChannel(t)
	f.setDefaultCountry(t, serbian, "RS")
	c := f.register(t, serbian, randomEmail(), "password123")

	national := "064 123 4567"
	got, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &national})
	if err != nil || deref(got.Phone) != "+381641234567" {
		t.Fatalf("national phone on an RS store = %q, %v, want +381641234567", deref(got.Phone), err)
	}

	unset := f.newChannel(t)
	d := f.register(t, unset, randomEmail(), "password123")
	if _, err := f.svc.UpdateProfile(f.ctx, d.ID, UpdateCustomerInput{Phone: &national}); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("national phone with no default country = %v, want phone.ErrInvalid", err)
	}
	international := "+381 64 123 4567"
	got, err = f.svc.UpdateProfile(f.ctx, d.ID, UpdateCustomerInput{Phone: &international})
	if err != nil || deref(got.Phone) != "+381641234567" {
		t.Fatalf("international phone with no default country = %q, %v", deref(got.Phone), err)
	}
}

// An address phone is read in the address's own country, whatever the store's default
func TestAddressPhoneIsReadInTheAddressCountry(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	f.setDefaultCountry(t, ch, "RS")
	c := f.register(t, ch, randomEmail(), "password123")

	in := addressInput() // a US address
	us := "(201) 555-0123"
	in.Phone = &us
	addr, err := f.svc.AddAddress(f.ctx, c.ID, in)
	if err != nil || deref(addr.Phone) != "+12015550123" {
		t.Fatalf("US address phone = %q, %v, want +12015550123", deref(addr.Phone), err)
	}

	serbian := "064 123 4567"
	in.Phone = &serbian
	if _, err := f.svc.AddAddress(f.ctx, c.ID, in); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("Serbian national number on a US address = %v, want phone.ErrInvalid", err)
	}
	if _, err := f.svc.UpdateAddress(f.ctx, c.ID, addr.ID, in); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("update with a Serbian national number on a US address = %v, want phone.ErrInvalid", err)
	}

	// Moving the address abroad moves the country its phone is read in
	in.CountryCode = "DE"
	german := "01512 3456789"
	in.Phone = &german
	updated, err := f.svc.UpdateAddress(f.ctx, c.ID, addr.ID, in)
	if err != nil || deref(updated.Phone) != "+4915123456789" {
		t.Fatalf("DE address phone = %q, %v, want +4915123456789", deref(updated.Phone), err)
	}

	in.Phone = nil
	cleared, err := f.svc.UpdateAddress(f.ctx, c.ID, addr.ID, in)
	if err != nil || cleared.Phone != nil {
		t.Fatalf("address without a phone = %q, %v, want none", deref(cleared.Phone), err)
	}
}

func TestAdminCustomerPhones(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	f.setDefaultCountry(t, ch, "RS")

	bad := "+381 64"
	_, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{
		ChannelID: ch, Email: randomEmail(), Password: "password123", FirstName: "Ana", LastName: "Ilić", Phone: &bad,
	})
	if !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("admin create with an invalid phone = %v, want phone.ErrInvalid", err)
	}

	spaced := "+381 64 123 4567"
	c, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{
		ChannelID: ch, Email: randomEmail(), Password: "password123", FirstName: "Ana", LastName: "Ilić", Phone: &spaced,
	})
	if err != nil || deref(c.Phone) != "+381641234567" {
		t.Fatalf("admin create = %q, %v, want +381641234567", deref(c.Phone), err)
	}

	national := "011 3234567"
	updated, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Phone: &national})
	if err != nil || deref(updated.Phone) != "+381113234567" {
		t.Fatalf("admin update with a national number = %q, %v, want +381113234567", deref(updated.Phone), err)
	}
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Phone: &bad}); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("admin update with an invalid phone = %v, want phone.ErrInvalid", err)
	}
}

// A Serbia-only store, as the country domain would hand it over
type serbiaOnly struct{}

func (serbiaOnly) ValidateAddressForChannel(context.Context, uuid.UUID, string, *string) error {
	return nil
}
func (serbiaOnly) PhoneRulesForChannel(context.Context, uuid.UUID) phone.Rules {
	return phone.Rules{Default: "RS", Allowed: []string{"RS"}}
}

// The store's rules reach every customer phone - a profile phone and an address phone alike
func TestCustomerPhonesFollowTheStoresRules(t *testing.T) {
	f := newFixture(t)
	f.svc.SetCountryValidator(serbiaOnly{})
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")

	german := "+49 1512 3456789"
	if _, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &german}); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Fatalf("a German profile phone on a Serbia-only store = %v, want phone.ErrCountryNotAllowed", err)
	}
	in := addressInput()
	in.CountryCode = "DE" // the address may be anywhere the store ships; the phone still has to be Serbian
	in.Phone = &german
	if _, err := f.svc.AddAddress(f.ctx, c.ID, in); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Fatalf("a German address phone on a Serbia-only store = %v, want phone.ErrCountryNotAllowed", err)
	}

	national := "064 123 4567"
	got, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &national})
	if err != nil || deref(got.Phone) != "+381641234567" {
		t.Fatalf("a Serbian national number = %q, %v, want +381641234567", deref(got.Phone), err)
	}
}

func TestProfilePhoneCanBeCleared(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	number, blank := "+381641234567", "  "
	for _, admin := range []bool{false, true} {
		if _, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &number}); err != nil {
			t.Fatal(err)
		}
		var got Customer
		var err error
		if admin {
			got, err = f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Phone: &blank})
		} else {
			got, err = f.svc.UpdateProfileFrom(f.ctx, c.ID, UpdateCustomerInput{Phone: &blank}, SourceSelf)
		}
		if err != nil || got.Phone != nil {
			t.Fatalf("admin=%v: cleared phone = %v, %v", admin, got.Phone, err)
		}
		persisted, err := f.svc.GetByID(f.ctx, c.ID)
		if err != nil || persisted.Phone != nil {
			t.Fatalf("persisted phone = %v, %v", persisted.Phone, err)
		}
	}
}

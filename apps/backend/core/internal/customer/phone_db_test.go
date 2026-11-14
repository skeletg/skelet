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
	german := f.newChannel(t)
	f.setDefaultCountry(t, german, "DE")
	c := f.register(t, german, randomEmail(), "password123")

	national := "01512 3456789"
	got, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &national})
	if err != nil || deref(got.Phone) != "+4915123456789" {
		t.Fatalf("national phone on a DE store = %q, %v, want +4915123456789", deref(got.Phone), err)
	}

	unset := f.newChannel(t)
	d := f.register(t, unset, randomEmail(), "password123")
	if _, err := f.svc.UpdateProfile(f.ctx, d.ID, UpdateCustomerInput{Phone: &national}); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("national phone with no default country = %v, want phone.ErrInvalid", err)
	}
	international := "+49 1512 3456789"
	got, err = f.svc.UpdateProfile(f.ctx, d.ID, UpdateCustomerInput{Phone: &international})
	if err != nil || deref(got.Phone) != "+4915123456789" {
		t.Fatalf("international phone with no default country = %q, %v", deref(got.Phone), err)
	}
}

// An address phone is read in the address's own country, whatever the store's default
func TestAddressPhoneIsReadInTheAddressCountry(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	f.setDefaultCountry(t, ch, "DE")
	c := f.register(t, ch, randomEmail(), "password123")

	in := addressInput() // a US address
	us := "(201) 555-0123"
	in.Phone = &us
	addr, err := f.svc.AddAddress(f.ctx, c.ID, in)
	if err != nil || deref(addr.Phone) != "+12015550123" {
		t.Fatalf("US address phone = %q, %v, want +12015550123", deref(addr.Phone), err)
	}

	national := "01512 3456789"
	in.Phone = &national
	if _, err := f.svc.AddAddress(f.ctx, c.ID, in); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("German national number on a US address = %v, want phone.ErrInvalid", err)
	}
	if _, err := f.svc.UpdateAddress(f.ctx, c.ID, addr.ID, in); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("update with a German national number on a US address = %v, want phone.ErrInvalid", err)
	}

	// Moving the address abroad moves the country its phone is read in
	in.CountryCode = "FR"
	french := "06 12 34 56 78"
	in.Phone = &french
	updated, err := f.svc.UpdateAddress(f.ctx, c.ID, addr.ID, in)
	if err != nil || deref(updated.Phone) != "+33612345678" {
		t.Fatalf("FR address phone = %q, %v, want +33612345678", deref(updated.Phone), err)
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
	f.setDefaultCountry(t, ch, "DE")

	bad := "+49 151"
	_, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{
		ChannelID: ch, Email: randomEmail(), Password: "password123", FirstName: "Ana", LastName: "Müller", Phone: &bad,
	})
	if !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("admin create with an invalid phone = %v, want phone.ErrInvalid", err)
	}

	spaced := "+49 1512 3456789"
	c, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{
		ChannelID: ch, Email: randomEmail(), Password: "password123", FirstName: "Ana", LastName: "Müller", Phone: &spaced,
	})
	if err != nil || deref(c.Phone) != "+4915123456789" {
		t.Fatalf("admin create = %q, %v, want +4915123456789", deref(c.Phone), err)
	}

	national := "030 12345678"
	updated, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Phone: &national})
	if err != nil || deref(updated.Phone) != "+493012345678" {
		t.Fatalf("admin update with a national number = %q, %v, want +493012345678", deref(updated.Phone), err)
	}
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Phone: &bad}); !errors.Is(err, phone.ErrInvalid) {
		t.Fatalf("admin update with an invalid phone = %v, want phone.ErrInvalid", err)
	}
}

// A Germany-only store, as the country domain would hand it over
type germanyOnly struct{}

func (germanyOnly) ValidateAddressForChannel(context.Context, uuid.UUID, string, *string) error {
	return nil
}
func (germanyOnly) PhoneRulesForChannel(context.Context, uuid.UUID) phone.Rules {
	return phone.Rules{Default: "DE", Allowed: []string{"DE"}}
}

// The store's rules reach every customer phone - a profile phone and an address phone alike
func TestCustomerPhonesFollowTheStoresRules(t *testing.T) {
	f := newFixture(t)
	f.svc.SetCountryValidator(germanyOnly{})
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")

	french := "+33 6 12 34 56 78"
	if _, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &french}); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Fatalf("a French profile phone on a Germany-only store = %v, want phone.ErrCountryNotAllowed", err)
	}
	in := addressInput()
	in.CountryCode = "FR" // the address may be anywhere the store ships; the phone still has to be German
	in.Phone = &french
	if _, err := f.svc.AddAddress(f.ctx, c.ID, in); !errors.Is(err, phone.ErrCountryNotAllowed) {
		t.Fatalf("a French address phone on a Germany-only store = %v, want phone.ErrCountryNotAllowed", err)
	}

	national := "01512 3456789"
	got, err := f.svc.UpdateProfile(f.ctx, c.ID, UpdateCustomerInput{Phone: &national})
	if err != nil || deref(got.Phone) != "+4915123456789" {
		t.Fatalf("a German national number = %q, %v, want +4915123456789", deref(got.Phone), err)
	}
}

func TestProfilePhoneCanBeCleared(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	number, blank := "+4915123456789", "  "
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

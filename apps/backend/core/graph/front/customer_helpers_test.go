package front

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func someAddress(id uuid.UUID, city string) customer.Address {
	return customer.Address{
		ID: id, FirstName: "Ana", LastName: "K",
		Street1: "Main 1", City: city, PostalCode: "11000", CountryCode: "RS",
		CreatedAt: time.Now(),
	}
}

func groupedCustomer(addrs ...customer.Address) customer.Customer {
	return customer.Customer{
		ID: uuid.New(), ChannelID: uuid.New(), Email: "ana@shop.test",
		FirstName: "Ana", LastName: "K", IsActive: true,
		Group:     &customer.CustomerGroup{ID: uuid.New(), Slug: "vip", Name: "VIP"},
		Addresses: addrs,
		CreatedAt: time.Now(),
	}
}

func TestToCustomerModelResolvesTheDefaultAddresses(t *testing.T) { // checkout preselects these, so a mismatched pointer would ship to the wrong saved address
	billing, shipping, other := uuid.New(), uuid.New(), uuid.New()
	c := groupedCustomer(someAddress(other, "Novi Sad"), someAddress(billing, "Beograd"), someAddress(shipping, "Nis"))
	c.DefaultBillingAddressID = &billing
	c.DefaultShippingAddressID = &shipping

	m := toCustomerModel(c)
	if len(m.Addresses) != 3 {
		t.Fatalf("%d addresses, want 3", len(m.Addresses))
	}
	if m.DefaultBillingAddress == nil || m.DefaultBillingAddress.City != "Beograd" {
		t.Errorf("DefaultBillingAddress = %+v, want the Beograd address", m.DefaultBillingAddress)
	}
	if m.DefaultShippingAddress == nil || m.DefaultShippingAddress.City != "Nis" {
		t.Errorf("DefaultShippingAddress = %+v, want the Nis address", m.DefaultShippingAddress)
	}
	// the default must be the SAME model object as the list entry, or a client keying by identity sees four addresses
	if m.DefaultBillingAddress != m.Addresses[1] {
		t.Error("DefaultBillingAddress is a copy rather than the list entry")
	}
}

func TestToCustomerModelWithNoDefaults(t *testing.T) { // a fresh account has addresses but no defaults picked yet
	c := groupedCustomer(someAddress(uuid.New(), "Beograd"))
	m := toCustomerModel(c)
	if m.DefaultBillingAddress != nil || m.DefaultShippingAddress != nil {
		t.Errorf("defaults = %+v/%+v, want both nil", m.DefaultBillingAddress, m.DefaultShippingAddress)
	}
}

func TestToCustomerModelWithADanglingDefault(t *testing.T) { // the default id can point at a just-deleted address - it must resolve to nil, not panic or grab a random entry
	gone := uuid.New()
	c := groupedCustomer(someAddress(uuid.New(), "Beograd"))
	c.DefaultBillingAddressID = &gone

	m := toCustomerModel(c)
	if m.DefaultBillingAddress != nil {
		t.Errorf("DefaultBillingAddress = %+v for a deleted address id, want nil", m.DefaultBillingAddress)
	}
}

func TestToCustomerModelSurvivesAMissingGroup(t *testing.T) { // the schema says non-null, but one bad row must degrade to a field error rather than panic and 500 the whole query
	c := groupedCustomer()
	c.Group = nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("toCustomerModel panicked on a nil group: %v", r)
		}
	}()
	m := toCustomerModel(c)
	if m.Group != nil {
		t.Errorf("Group = %+v, want nil so gqlgen emits the field-level error", m.Group)
	}
	if m.Email != "ana@shop.test" {
		t.Error("the rest of the model was not mapped")
	}
}

func TestToCustomerModelWithNoAddresses(t *testing.T) {
	m := toCustomerModel(groupedCustomer())
	if m.Addresses == nil || len(m.Addresses) != 0 {
		t.Errorf("Addresses = %v, want an empty non-null list - the schema field is non-null", m.Addresses)
	}
}

func TestFrontInputErrorsCarryTheValidationClass(t *testing.T) { // the presenter maps unregistered errors to ClassInternal, so these MUST be classified - an unclassified shopper typo pages the operator as an internal error
	for name, err := range map[string]error{
		"invalid id":        invalidID("cart"),
		"generic id":        invalidID(""),
		"store required":    storeRequired(),
		"selector required": selectorRequired("pass id or slug"),
	} {
		ae, ok := apperr.AsAppError(err)
		if !ok {
			t.Errorf("%s: not an AppError, so the presenter would 500 it", name)
			continue
		}
		if ae.Class != apperr.ClassValidation {
			t.Errorf("%s: class = %v, want ClassValidation", name, ae.Class)
		}
	}

	if got := invalidID("cart").Error(); got != "invalid cart id" {
		t.Errorf("message = %q, want %q", got, "invalid cart id")
	}
	if got := invalidID("").Error(); got != "invalid id" {
		t.Errorf("generic message = %q, want %q", got, "invalid id")
	}
}

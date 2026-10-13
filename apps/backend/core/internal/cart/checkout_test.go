package cart

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestMergeCheckoutGuestWins(t *testing.T) {
	savedID := uuid.New()
	customer := Checkout{
		ShippingAddress:       &Address{City: "Belgrade", CountryCode: "RS"},
		ShippingAddressID:     &savedID,
		BillingSameAsShipping: true,
		ShippingCarrierSlug:   "builtin",
		ShippingMethodCode:    "standard",
		PaymentProviderSlug:   "cod",
		PONumber:              "PO-1",
		UseStoreCredit:        true,
		GiftCardCodes:         []string{"GC-A", "GC-B"},
	}

	t.Run("guest selections replace the customer's", func(t *testing.T) {
		guest := Checkout{
			ShippingAddress:       &Address{City: "Novi Sad", CountryCode: "RS"},
			BillingSameAsShipping: false,
			BillingAddress:        &Address{City: "Nis", CountryCode: "RS"},
			ShippingCarrierSlug:   "pickup",
			ShippingMethodCode:    "loc-center",
			GiftCardCodes:         []string{"GC-B", "GC-C"},
			Destination:           Destination{CountryCode: "RS", PostalCode: "21000"},
			ContactPhone:          "+381 64 000",
			PurchaseType:          PurchaseBusiness,
			Company:               "Acme",
			BusinessIDs:           map[string]string{"pib": "RS1"},
			PickupContact:         &PickupContact{FirstName: "Ana", LastName: "B"},
		}
		got := mergeCheckout(guest, customer)
		if got.ShippingAddress.City != "Novi Sad" || got.ShippingAddressID != nil {
			t.Fatalf("shipping: got %+v id=%v, want the guest's typed-in address", got.ShippingAddress, got.ShippingAddressID)
		}
		if got.BillingSameAsShipping || got.BillingAddress == nil || got.BillingAddress.City != "Nis" {
			t.Fatalf("billing: got same=%v %+v, want the guest's", got.BillingSameAsShipping, got.BillingAddress)
		}
		if got.ShippingCarrierSlug != "pickup" || got.ShippingMethodCode != "loc-center" {
			t.Fatalf("method: got %s/%s", got.ShippingCarrierSlug, got.ShippingMethodCode)
		}
		if got.PaymentProviderSlug != "cod" || got.PONumber != "PO-1" || !got.UseStoreCredit {
			t.Fatalf("fields the guest left empty should keep the customer's: %+v", got)
		}
		if got.Destination.PostalCode != "21000" || got.ContactPhone == "" || got.PurchaseType != PurchaseBusiness || got.BusinessIDs["pib"] != "RS1" || got.PickupContact == nil {
			t.Fatalf("destination and details should travel with the guest: %+v", got)
		}
		if want := []string{"GC-B", "GC-C", "GC-A"}; !slices.Equal(got.GiftCardCodes, want) {
			t.Fatalf("gift cards: got %v, want %v", got.GiftCardCodes, want)
		}
	})

	t.Run("an empty guest checkout keeps the customer's", func(t *testing.T) {
		got := mergeCheckout(Checkout{BillingSameAsShipping: true}, customer)
		if got.ShippingAddressID == nil || *got.ShippingAddressID != savedID || got.ShippingCarrierSlug != "builtin" {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestMergeGuestCartCarriesCheckout(t *testing.T) {
	customerID, channelID, guestID, destID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	guest := Cart{ID: guestID, ChannelID: channelID, Status: "active", Email: "guest@example.test", Checkout: Checkout{
		ShippingAddress: &Address{City: "Novi Sad", CountryCode: "RS"}, BillingSameAsShipping: true,
	}}
	dest := Cart{ID: destID, CustomerID: &customerID, ChannelID: channelID, Status: "active", Checkout: Checkout{PaymentProviderSlug: "cod"}}
	repo := &mergeRepo{
		carts:        map[uuid.UUID]Cart{guestID: guest, destID: dest},
		linesByCart:  map[uuid.UUID][]CartLine{},
		activeByCust: map[uuid.UUID]Cart{customerID: dest},
	}
	svc := NewService(repo, newTestTxManager(t), nil)
	if _, err := svc.MergeGuestCart(context.Background(), customerID, guestID); err != nil {
		t.Fatalf("MergeGuestCart: %v", err)
	}
	got, ok := repo.checkouts[destID]
	if !ok {
		t.Fatal("checkout was not written to the customer cart")
	}
	if got.ShippingAddress == nil || got.ShippingAddress.City != "Novi Sad" || got.PaymentProviderSlug != "cod" {
		t.Fatalf("got %+v", got)
	}
	if repo.emails[destID] != "guest@example.test" {
		t.Errorf("customer cart email = %q, want the receipt address the guest typed", repo.emails[destID])
	}
}

// A pickup is quoted at its point, which the delivery group knows - the cart's own destination is always the shopper's
func TestQuoteDestinationFollowsTheAddress(t *testing.T) {
	state := "CA"
	co := Checkout{
		Destination:     Destination{CountryCode: "US", PostalCode: "10001"},
		ShippingAddress: &Address{CountryCode: "US", PostalCode: "94103", State: &state},
	}
	if d := co.QuoteDestination(); d.PostalCode != "94103" || d.Region != "CA" {
		t.Errorf("delivery quotes for %+v, want the shipping address", d)
	}
	co.ShippingAddress = nil
	if d := co.QuoteDestination(); d.PostalCode != "10001" {
		t.Errorf("quotes for %+v without an address, want the destination", d)
	}
}

func TestUpdateCheckoutRefusesAnotherCustomersCart(t *testing.T) {
	owner, stranger := uuid.New(), uuid.New()
	cartID := uuid.New()
	repo := fakeRepo{getCart: func(uuid.UUID) (Cart, error) {
		return Cart{ID: cartID, CustomerID: &owner, Status: "active"}, nil
	}}
	svc := NewService(repo, newTestTxManager(t), nil)
	called := false
	err := svc.UpdateCheckout(context.Background(), cartID, &stranger, func(*Cart) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrCartForbidden) || called {
		t.Fatalf("got err=%v called=%v, want ErrCartForbidden before the step runs", err, called)
	}
}

package cart

import (
	"context"
	"database/sql"
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
		Deliveries:            map[string]DeliverySelection{"all": {CarrierSlug: "builtin", MethodCode: "standard"}, "k:store_pickup": {CarrierSlug: "pickup"}},
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
			Deliveries:            map[string]DeliverySelection{"all": {CarrierSlug: "courier", MethodCode: "express"}},
			SplitDelivery:         true,
		}
		got := mergeCheckout(guest, customer)
		if got.Deliveries["all"].MethodCode != "express" || got.Deliveries["k:store_pickup"].CarrierSlug != "pickup" || !got.SplitDelivery {
			t.Fatalf("deliveries: got %+v split=%v, want the guest's choice per group over the customer's, and the guest's split", got.Deliveries, got.SplitDelivery)
		}
		if customer.Deliveries["all"].MethodCode != "standard" {
			t.Fatal("merging wrote into the customer's own deliveries")
		}
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
		if got.ShippingAddressID == nil || *got.ShippingAddressID != savedID || got.ShippingCarrierSlug != "builtin" || len(got.Deliveries) != 2 {
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

// versionRepo is one cart with a checkout_version, recording the order calls arrive in - enough to drive
// UpdateCheckout's unlocked prepare and its version-checked commit
type versionRepo struct {
	fakeRepo
	cart   *Cart
	lines  []CartLine
	calls  *[]string
	writes *[]Checkout
}

func (r versionRepo) GetCartByID(context.Context, uuid.UUID) (Cart, error) {
	*r.calls = append(*r.calls, "load")
	return *r.cart, nil
}
func (r versionRepo) ListCartLinesByCart(context.Context, uuid.UUID) ([]CartLine, error) {
	return r.lines, nil
}
func (r versionRepo) LockCart(context.Context, uuid.UUID) (Cart, error) {
	*r.calls = append(*r.calls, "lock")
	return *r.cart, nil
}
func (r versionRepo) SetCheckout(_ context.Context, _ uuid.UUID, c Checkout) error {
	*r.calls = append(*r.calls, "write")
	*r.writes = append(*r.writes, c)
	r.cart.Checkout = c
	r.cart.CheckoutVersion++
	return nil
}
func (r versionRepo) WithTx(*sql.Tx) Repository { return r }

func newVersionRepo() versionRepo {
	return versionRepo{
		cart:   &Cart{ID: uuid.New(), Status: "active", Checkout: Checkout{PaymentProviderSlug: "cod"}},
		lines:  []CartLine{{ProductSku: "SKU-1", Quantity: 2}},
		calls:  &[]string{},
		writes: &[]Checkout{},
	}
}

// The step - quotes, payment checks, remote validators - runs on the cart as loaded, lines included, before any lock is taken
func TestUpdateCheckoutPreparesBeforeTheLock(t *testing.T) {
	repo := newVersionRepo()
	svc := NewService(repo, newTestTxManager(t), nil)
	err := svc.UpdateCheckout(context.Background(), repo.cart.ID, nil, func(c *Cart) error {
		*repo.calls = append(*repo.calls, "step")
		if len(c.Lines) != 1 {
			t.Errorf("step saw %d lines, want the cart's - shipping is quoted from them", len(c.Lines))
		}
		c.Checkout.ShippingCarrierSlug = "builtin"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateCheckout: %v", err)
	}
	if want := []string{"load", "step", "lock", "write"}; !slices.Equal(*repo.calls, want) {
		t.Errorf("calls = %v, want %v", *repo.calls, want)
	}
}

// Another save landing between the load and the commit makes the step run again on the new row, so neither is lost
func TestUpdateCheckoutRecomputesWhenTheCheckoutMoved(t *testing.T) {
	repo := newVersionRepo()
	svc := NewService(repo, newTestTxManager(t), nil)
	runs := 0
	err := svc.UpdateCheckout(context.Background(), repo.cart.ID, nil, func(c *Cart) error {
		runs++
		if runs == 1 {
			// A second tab saves the payment step while this one is still quoting
			repo.cart.Checkout.PaymentProviderSlug = "bank-transfer"
			repo.cart.CheckoutVersion++
		}
		c.Checkout.ShippingCarrierSlug = "builtin"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateCheckout: %v", err)
	}
	if runs != 2 || len(*repo.writes) != 1 {
		t.Fatalf("step ran %d times with %d writes, want 2 runs and one write", runs, len(*repo.writes))
	}
	if got := (*repo.writes)[0]; got.ShippingCarrierSlug != "builtin" || got.PaymentProviderSlug != "bank-transfer" {
		t.Errorf("written checkout = %+v, want this step's method on top of the other tab's payment", got)
	}
}

// A checkout that never holds still is refused rather than overwritten
func TestUpdateCheckoutGivesUpOnAConstantlyMovingCheckout(t *testing.T) {
	repo := newVersionRepo()
	svc := NewService(repo, newTestTxManager(t), nil)
	runs := 0
	err := svc.UpdateCheckout(context.Background(), repo.cart.ID, nil, func(c *Cart) error {
		runs++
		repo.cart.CheckoutVersion++
		return nil
	})
	if !errors.Is(err, ErrCheckoutConflict) {
		t.Fatalf("err = %v, want ErrCheckoutConflict", err)
	}
	if runs != checkoutSaveAttempts || len(*repo.writes) != 0 {
		t.Errorf("step ran %d times with %d writes, want %d runs and nothing written", runs, len(*repo.writes), checkoutSaveAttempts)
	}
}

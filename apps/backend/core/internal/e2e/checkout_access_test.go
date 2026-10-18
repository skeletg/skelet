package e2e

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
)

func TestCheckoutStepsRefuseForeignCartsAndAddresses(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	other := h.NewStore(t, 10, 2000)
	owner, _, _ := h.registeredBuyer(t, f)
	stranger, email, token := h.registeredBuyer(t, f)
	own, err := h.D.Cart.GetOrCreateCart(h.Ctx, owner, f.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := h.D.Cart.CreateGuestCart(h.Ctx, other.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	headers := map[string]string{"Authorization": "Bearer " + token}
	for _, c := range []uuid.UUID{own.ID, foreign.ID} {
		before, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range []string{
			fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, c),
			fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, c),
			fmt.Sprintf(`setCartSplitDelivery(cartId: %q, split: true)`, c),
			fmt.Sprintf(`setCartDelivery(cartId: %q, groupKey: "all", optionId: %q, carrierSlug: "builtin", methodCode: "standard")`, c, uuid.New()),
			fmt.Sprintf(`setCartDeliveryPoint(cartId: %q, groupKey: "all", pointId: "point")`, c),
			details(c.String(), fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)),
			fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, c),
			fmt.Sprintf(`setCartStoreCredit(cartId: %q, use: true)`, c),
		} {
			_, errs := h.gqlPost(t, ts, f.Channel.ID.String(), `mutation { `+step+` { id } }`, headers)
			if len(errs) == 0 || !strings.Contains(strings.Join(errs, "; "), "cart store mismatch") {
				t.Fatalf("%s: %v, want cart access refusal", step, errs)
			}
		}
		after, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, c)
		if err != nil || !reflect.DeepEqual(before.Checkout, after.Checkout) || before.CheckoutVersion != after.CheckoutVersion {
			t.Fatalf("foreign checkout changed: %+v, %v", after, err)
		}
	}
	addr, err := h.D.Customer.AddAddress(h.Ctx, owner, customer.AddressInput{FirstName: "Owner", LastName: "Only", Street1: "1 Test Way", City: "City", CountryCode: "US", PostalCode: "12345"})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := h.D.Cart.GetOrCreateCart(h.Ctx, stranger, f.Channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.D.Cart.AddToCart(h.Ctx, mine.ID, &stranger, f.Product.SKU, 1); err != nil {
		t.Fatal(err)
	}
	if _, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345") { id } }`, mine.ID), headers); len(errs) > 0 {
		t.Fatal(errs)
	}
	for _, field := range []string{"shippingAddressId", "billingAddressId"} {
		extra := ""
		if field == "billingAddressId" {
			extra = ", shippingAddress: " + usAddress
		}
		step := details(mine.ID.String(), fmt.Sprintf(`email: %q, billingSameAsShipping: false, %s: %q%s`, email, field, addr.ID, extra))
		_, errs := h.gqlPost(t, ts, f.Channel.ID.String(), `mutation { `+step+` { id } }`, headers)
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, "; "), "address not found in the customer's address book") {
			t.Fatalf("another customer's %s accepted: %v", field, errs)
		}
	}
	got, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, mine.ID)
	if err != nil || got.Checkout.ShippingAddressID != nil || got.Checkout.BillingAddressID != nil {
		t.Fatalf("foreign address persisted: %+v, %v", got.Checkout, err)
	}
	h.Settle(t)
}

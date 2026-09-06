package e2e

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/apperr"
	"github.com/skeletg/skelet/apps/backend/internal/order"
)

func editAddr(first, last, city, postal string) *order.OrderAddress { // validEditAddress needs names, which e2eAddress deliberately leaves blank
	return &order.OrderAddress{
		FirstName:   first,
		LastName:    last,
		Street1:     "1 Test Way",
		City:        city,
		PostalCode:  postal,
		CountryCode: "US",
	}
}

// Scenario: a BILLING-only correction is not a money event - re-quoting on it would silently change what the customer owes after they already agreed a total
func TestEditBillingAddressLeavesTheTotalsAlone(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)

	edited, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID: o.ID,
		Billing: editAddr("Ada", "Lovelace", "London", "E1 6AN"),
	})
	if err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}

	if edited.SubtotalAmount != o.SubtotalAmount || edited.TaxAmount != o.TaxAmount ||
		edited.ShippingAmount != o.ShippingAmount || edited.TotalAmount != o.TotalAmount {
		t.Errorf("a billing-only edit moved the money to {sub %d tax %d ship %d total %d}, want the original {%d %d %d %d}",
			edited.SubtotalAmount, edited.TaxAmount, edited.ShippingAmount, edited.TotalAmount,
			o.SubtotalAmount, o.TaxAmount, o.ShippingAmount, o.TotalAmount)
	}
	if edited.BillingAddress.FirstName != "Ada" || edited.BillingAddress.City != "London" {
		t.Errorf("billing = %+v, want the new address", edited.BillingAddress)
	}
	if edited.ShippingAddress.City != o.ShippingAddress.City {
		t.Errorf("shipping moved to %q, want the original %q untouched", edited.ShippingAddress.City, o.ShippingAddress.City)
	}
	h.Settle(t)
}

// A shipping move CAN change tax and shipping, so it re-quotes - and whatever it lands on, the total has to stay equal to its own breakdown
func TestEditShippingAddressRecomputesAConsistentTotal(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)

	edited, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID:  o.ID,
		Shipping: editAddr("Grace", "Hopper", "Arlington", "22202"),
	})
	if err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}

	if edited.ShippingAddress.FirstName != "Grace" || edited.ShippingAddress.City != "Arlington" {
		t.Errorf("shipping = %+v, want the new address", edited.ShippingAddress)
	}
	if edited.SubtotalAmount != o.SubtotalAmount {
		t.Errorf("subtotal = %d, want it unchanged at %d - the lines did not move", edited.SubtotalAmount, o.SubtotalAmount)
	}
	if edited.TotalAmount != edited.SubtotalAmount-edited.DiscountAmount+edited.TaxAmount+edited.ShippingAmount {
		t.Errorf("total %d does not equal its own breakdown after the re-quote", edited.TotalAmount)
	}
	h.Settle(t)
}

// Both addresses in one call is the common admin correction, and neither may be dropped
func TestEditBothAddressesAtOnce(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	edited, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID:  o.ID,
		Billing:  editAddr("Ada", "Lovelace", "London", "E1 6AN"),
		Shipping: editAddr("Grace", "Hopper", "Arlington", "22202"),
	})
	if err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}
	if edited.BillingAddress.City != "London" {
		t.Errorf("billing city = %q, want London", edited.BillingAddress.City)
	}
	if edited.ShippingAddress.City != "Arlington" {
		t.Errorf("shipping city = %q, want Arlington", edited.ShippingAddress.City)
	}

	// and the write is durable, not just reflected in the returned struct
	reread, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if reread.BillingAddress.City != "London" || reread.ShippingAddress.City != "Arlington" {
		t.Errorf("re-read addresses = %q / %q, want the edit persisted", reread.BillingAddress.City, reread.ShippingAddress.City)
	}
	h.Settle(t)
}

// An address edit is not a stock event - re-reserving on one would double-hold the order's units
func TestEditAddressLeavesReservationsAlone(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 3)
	h.Settle(t)
	before := h.ReservedFor(t, f)
	if before != 3 {
		t.Fatalf("reserved after placement = %v, want 3", before)
	}

	if _, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID:  o.ID,
		Shipping: editAddr("Grace", "Hopper", "Arlington", "22202"),
	}); err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}
	if got := h.ReservedFor(t, f); got != before {
		t.Errorf("reserved = %v after an address edit, want the original %v", got, before)
	}
	h.Settle(t)
}

// Once units have shipped the address is what the carrier already has, so rewriting it would leave the label and the order disagreeing
func TestEditAddressRefusedAfterFulfillment(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 2)
	h.Settle(t)
	lineID := h.OrderLines(t, o.ID)[0].ID
	for _, st := range []string{order.OrderStatusConfirmed, order.OrderStatusProcessing} {
		if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, st); err != nil {
			t.Fatalf("advance to %s: %v", st, err)
		}
	}
	if _, err := h.D.Order.CreateFulfillment(h.Ctx, order.CreateFulfillmentInput{
		OrderID: o.ID,
		Lines:   []order.FulfillmentLineInput{{OrderLineID: lineID, Quantity: 2}},
	}); err != nil {
		t.Fatalf("fulfill: %v", err)
	}
	h.Settle(t)

	_, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID:  o.ID,
		Shipping: editAddr("Grace", "Hopper", "Arlington", "22202"),
	})
	if err == nil {
		t.Fatal("the address of a fulfilled order was rewritten")
	}
	if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ORDER_NOT_EDITABLE" {
		t.Errorf("error = %v, want code ORDER_NOT_EDITABLE", err)
	}
	h.Settle(t)
}

// Every guard refuses before the write, so a malformed admin form can't half-apply an address
func TestEditAddressGuards(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	original, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}

	for name, tc := range map[string]struct {
		in   order.EditOrderAddressInput
		code string
	}{
		"neither address": {
			order.EditOrderAddressInput{OrderID: o.ID}, "ORDER_EDIT_EMPTY",
		},
		"no first name": {
			order.EditOrderAddressInput{OrderID: o.ID, Shipping: editAddr("", "Hopper", "Arlington", "22202")},
			"ORDER_EDIT_INVALID_ADDRESS",
		},
		"no last name": {
			order.EditOrderAddressInput{OrderID: o.ID, Shipping: editAddr("Grace", "", "Arlington", "22202")},
			"ORDER_EDIT_INVALID_ADDRESS",
		},
		"no city": {
			order.EditOrderAddressInput{OrderID: o.ID, Shipping: editAddr("Grace", "Hopper", "", "22202")},
			"ORDER_EDIT_INVALID_ADDRESS",
		},
		"no postal code": {
			order.EditOrderAddressInput{OrderID: o.ID, Shipping: editAddr("Grace", "Hopper", "Arlington", "")},
			"ORDER_EDIT_INVALID_ADDRESS",
		},
		"bad billing is refused too": {
			order.EditOrderAddressInput{OrderID: o.ID, Billing: editAddr("", "", "", "")},
			"ORDER_EDIT_INVALID_ADDRESS",
		},
	} {
		_, err := h.D.Order.EditOrderAddress(h.Ctx, tc.in)
		if err == nil {
			t.Errorf("%s was accepted, want %s", name, tc.code)
			continue
		}
		if ae, ok := apperr.AsAppError(err); !ok || ae.Code != tc.code {
			t.Errorf("%s: error = %v, want code %s", name, err, tc.code)
		}
	}

	// and the order is untouched by any of them
	after, err := h.D.Order.GetByID(h.Ctx, o.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.ShippingAddress.City != original.ShippingAddress.City || after.BillingAddress.City != original.BillingAddress.City {
		t.Errorf("addresses moved to %q / %q despite every edit being refused", after.ShippingAddress.City, after.BillingAddress.City)
	}
	if after.TotalAmount != original.TotalAmount {
		t.Errorf("total = %d after only refused edits, want the original %d", after.TotalAmount, original.TotalAmount)
	}
	h.Settle(t)
}

// A two-letter country is the carrier contract, so a full name or a three-letter code has to refuse rather than reach the label
func TestEditAddressRequiresATwoLetterCountry(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	for _, code := range []string{"", "U", "USA", "United States"} {
		addr := editAddr("Grace", "Hopper", "Arlington", "22202")
		addr.CountryCode = code
		_, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{OrderID: o.ID, Shipping: addr})
		if err == nil {
			t.Errorf("country %q was accepted, want it refused", code)
			continue
		}
		if ae, ok := apperr.AsAppError(err); !ok || ae.Code != "ORDER_EDIT_INVALID_ADDRESS" {
			t.Errorf("country %q: error = %v, want ORDER_EDIT_INVALID_ADDRESS", code, err)
		}
	}
	h.Settle(t)
}

// A draft is editable too, and correcting its address must not place or reserve anything
func TestEditAddressOnADraftKeepsItADraft(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	d := h.newDraft(t, f, []order.CreateDraftLine{{ProductSku: f.Product.SKU, Quantity: 2}})

	edited, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{
		OrderID:  d.ID,
		Shipping: editAddr("Grace", "Hopper", "Arlington", "22202"),
	})
	if err != nil {
		t.Fatalf("EditOrderAddress on a draft: %v", err)
	}
	if edited.Status != order.OrderStatusDraft {
		t.Errorf("status = %q, want it still draft", edited.Status)
	}
	if edited.ShippingAddress.City != "Arlington" {
		t.Errorf("shipping city = %q, want the edit applied", edited.ShippingAddress.City)
	}
	if got := h.ReservedFor(t, f); got != 0 {
		t.Errorf("reserved = %v after editing a draft's address, want 0", got)
	}
	h.Settle(t)
}

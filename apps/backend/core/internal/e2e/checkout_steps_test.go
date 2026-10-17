package e2e

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

const checkoutFields = `checkout { email phone purchaseType company businessIds { key value } poNumber isPickup
	destination { countryCode postalCode } shippingAddress { city countryCode phone } billingSameAsShipping billingAddress { city }
	pickupContact { firstName lastName someoneElse }
	shippingMethod { carrierSlug code priceCents } paymentProviderSlug
	totals { subtotalCents discountCents shippingCents taxCents totalCents amountDueCents } }`

const usAddress = `{firstName: "E2E", lastName: "Guest", street1: "1 Test Way", city: "Testville", postalCode: "12345", countryCode: "us"}`

func (h *harness) gqlCheckout(t *testing.T, ts *httptest.Server, f *store, mutation string) map[string]any {
	t.Helper()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { step: %s { %s } }`, mutation, checkoutFields))
	if len(errs) > 0 {
		t.Fatalf("%s: %v", mutation, errs)
	}
	return data["step"].(map[string]any)["checkout"].(map[string]any)
}

// gqlCheckoutRefused runs a step that must fail, and returns the error text
func (h *harness) gqlCheckoutRefused(t *testing.T, ts *httptest.Server, f *store, mutation string) string {
	t.Helper()
	_, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { step: %s { id } }`, mutation))
	if len(errs) == 0 {
		t.Fatalf("%s was accepted, want a refusal", mutation)
	}
	return strings.Join(errs, "; ")
}

func (h *harness) gqlCartCheckout(t *testing.T, ts *httptest.Server, f *store, cartID string) map[string]any {
	t.Helper()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ cart(id: %q) { %s } }`, cartID, checkoutFields))
	if len(errs) > 0 {
		t.Fatalf("cart: %v", errs)
	}
	return data["cart"].(map[string]any)["checkout"].(map[string]any)
}

// gqlCartGroups reads the cart's delivery groups with their selection
func (h *harness) gqlCartGroups(t *testing.T, ts *httptest.Server, f *store, cartID string) []map[string]any {
	t.Helper()
	data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ cart(id: %q) { checkout { deliveryGroups {
		key kinds lineIds choices { optionId optionName kind carrierSlug methodCode priceCents }
		selection { optionId kind carrierSlug methodCode priceCents point { id name city } }
		shipments { originName lineIds priceCents } } } } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("cart groups: %v", errs)
	}
	raw := data["cart"].(map[string]any)["checkout"].(map[string]any)["deliveryGroups"].([]any)
	out := make([]map[string]any, len(raw))
	for i, g := range raw {
		out[i] = g.(map[string]any)
	}
	return out
}

func details(cartID, fields string) string {
	return fmt.Sprintf(`setCartDetails(cartId: %q, input: {%s})`, cartID, fields)
}

// Scenario: each checkout step saves to the cart, a fresh read returns all of it, the totals are what placement charges, and a destination change drops the method it no longer supports
func TestCheckoutStepsPersistOnTheCart(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)

	h.gqlCheckoutRefused(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cartID))

	// Step 1 - a postcode alone quotes delivery, and the total is already estimated with shipping and tax
	co := h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "us", postalCode: " 12345 ")`, cartID))
	if d := co["destination"].(map[string]any); d["countryCode"] != "US" || d["postalCode"] != "12345" {
		t.Fatalf("destination = %v, want US 12345", d)
	}
	h.gqlCheckoutRefused(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "overnight")`, cartID))
	co = h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cartID))
	if totals := co["totals"].(map[string]any); totals["shippingCents"].(float64) != 500 || totals["taxCents"].(float64) != 250 {
		t.Fatalf("totals from the destination alone = %v, want 500 shipping and 250 tax", totals)
	}

	// Step 2 - contact and address in one write
	email := "steps-" + uuid.NewString()[:8] + "@e2e.test"
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, fmt.Sprintf(`shippingAddress: %s, billingSameAsShipping: true`, usAddress))); !strings.Contains(msg, "email") {
		t.Errorf("details without an email = %q, want the email refusal", msg)
	}
	co = h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, phone: " (650) 253-0000 ", shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	if a := co["shippingAddress"].(map[string]any); a["countryCode"] != "US" || a["city"] != "Testville" {
		t.Fatalf("shipping address = %v, want the typed one with the country uppercased", a)
	}
	if co["shippingMethod"] == nil {
		t.Fatal("an address in the destination's postcode dropped the chosen method")
	}

	// Step 3
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))

	// A fresh read - the reload a shopper does - returns every step
	co = h.gqlCartCheckout(t, ts, f, cartID)
	if co["email"] != email || co["phone"] != "+16502530000" || co["paymentProviderSlug"] != "cod" || co["billingSameAsShipping"] != true || co["purchaseType"] != "PERSONAL" {
		t.Fatalf("checkout after reload = %v", co)
	}
	if m, _ := co["shippingMethod"].(map[string]any); m == nil || m["code"] != "standard" || m["priceCents"].(float64) != 500 {
		t.Fatalf("shipping method after reload = %v, want standard at 500", co["shippingMethod"])
	}
	totals := co["totals"].(map[string]any)
	if totals["shippingCents"].(float64) != 500 || totals["taxCents"].(float64) != 250 || totals["totalCents"].(float64) != 2750 {
		t.Fatalf("totals = %v, want 2000 + 500 shipping + 250 tax (goods 200 + shipping 50)", totals)
	}
	if totals["amountDueCents"] != totals["totalCents"] {
		t.Errorf("amount due %v, want the total with no tenders", totals["amountDueCents"])
	}

	// placeOrder takes nothing but the cart, and charges what the checkout showed
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id total } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	placed := data["placeOrder"].(map[string]any)
	if placed["total"] != totals["totalCents"] {
		t.Errorf("placed total %v, checkout showed %v", placed["total"], totals["totalCents"])
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(placed["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.Email != email || row.ShippingCarrierSlug.String != "builtin" || row.ShippingMethodCode.String != "standard" || row.PaymentProvider != "cod" || row.ShippingPhone.String != "+16502530000" {
		t.Errorf("order row email=%q carrier=%q method=%q provider=%q phone=%q, want the cart's",
			row.Email, row.ShippingCarrierSlug.String, row.ShippingMethodCode.String, row.PaymentProvider, row.ShippingPhone.String)
	}
	h.Settle(t)

	// A second cart: moving the destination out of the zone drops the method and the address saved for the old one
	cart2 := h.gqlGuestCart(t, ts, f, 1)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cart2))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cart2))
	h.gqlCheckout(t, ts, f, details(cart2, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	co = h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "GB", postalCode: "N1 1AA")`, cart2))
	if co["shippingMethod"] != nil {
		t.Errorf("shipping method = %v after moving to GB, want it dropped - the US zone doesn't cover GB", co["shippingMethod"])
	}
	if co["shippingAddress"] != nil {
		t.Errorf("shipping address = %v after moving to GB, want the US one cleared", co["shippingAddress"])
	}
	// ...and placement names the step still owed rather than picking a method for the shopper
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cart2))
	h.gqlCheckout(t, ts, f, details(cart2, fmt.Sprintf(`shippingAddress: %s, billingSameAsShipping: true`, usAddress)))
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cart2)); len(errs) == 0 || !strings.Contains(errs[0], "delivery or pickup") {
		t.Errorf("placeOrder without a method = %v, want the CHECKOUT_DELIVERY_OPTION_REQUIRED refusal", errs)
	}
}

// Scenario: click & collect needs no home address - the order ships to the store, addressed to whoever collects it, and a gift card covering the total needs no payment method
func TestCheckoutPickupNeedsNoAddress(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	line1, city, postal, country, active := "9 Depot Rd", "Storeton", "54321", "US", true
	if _, err := h.D.Inventory.UpdateLocation(h.Ctx, inventory.UpdateLocationInput{ID: f.Location.ID, IsActive: &active, Line1: &line1, City: &city, PostalCode: &postal, CountryCode: &country}); err != nil {
		t.Fatalf("location address: %v", err)
	}
	if _, err := h.D.Inventory.SetLocationPickup(h.Ctx, f.Location.ID, true, "Ask at the desk"); err != nil {
		t.Fatalf("enable pickup: %v", err)
	}
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatalf("sync carriers: %v", err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, "pickup", true); err != nil {
		t.Fatalf("enable the pickup carrier: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "pickup-" + uuid.NewString()[:8] + "@e2e.test"

	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	co := h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "pickup", code: "collect")`, cartID))
	if co["isPickup"] != true {
		t.Fatalf("isPickup = %v after choosing pickup", co["isPickup"])
	}
	// The store's only pickup location is the point without being asked
	groups := h.gqlCartGroups(t, ts, f, cartID)
	if p, _ := groups[0]["selection"].(map[string]any)["point"].(map[string]any); p == nil || p["id"] != f.Location.Slug {
		t.Fatalf("pickup point = %v, want the store's location %s", groups[0]["selection"], f.Location.Slug)
	}

	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, billingSameAsShipping: false`, email))); !strings.Contains(msg, "collect") {
		t.Errorf("pickup details without a collector = %q, want the pickup contact refusal", msg)
	}
	// Someone else collecting needs the payer's billing address - the order would otherwise name only the collector
	someoneElse := `pickupContact: {firstName: "Ana", lastName: "Friend", someoneElse: true}`
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, %s, billingSameAsShipping: false`, email, someoneElse))); !strings.Contains(msg, "billing") {
		t.Errorf("someone else collecting without billing = %q, want the billing refusal", msg)
	}
	co = h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, %s, billingSameAsShipping: false, billingAddress: %s`, email, someoneElse, usAddress)))
	if p := co["pickupContact"].(map[string]any); p["firstName"] != "Ana" || p["someoneElse"] != true {
		t.Fatalf("pickup contact = %v", p)
	}
	if co["shippingAddress"] != nil {
		t.Errorf("shipping address = %v, want none for a pickup", co["shippingAddress"])
	}

	card, err := h.D.Giftcard.Issue(h.Ctx, giftcard.IssueInput{CurrencyCode: "USD", Amount: 5000, ChannelID: &f.Channel.ID})
	if err != nil {
		t.Fatalf("issue gift card: %v", err)
	}
	co = h.gqlCheckout(t, ts, f, fmt.Sprintf(`applyGiftCard(cartId: %q, code: %q)`, cartID, card.Code))
	if due := co["totals"].(map[string]any)["amountDueCents"].(float64); due != 0 {
		t.Fatalf("amount due = %v, want the gift card to cover it", due)
	}

	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder with no payment method and nothing owed: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.ShippingStreet1 != line1 || row.ShippingCity != city || row.ShippingFirstName != "Ana" || row.BillingFirstName != "E2E" {
		t.Errorf("order ships to %q %q for %q, billed to %q - want the store, the collector, and the payer",
			row.ShippingStreet1, row.ShippingCity, row.ShippingFirstName, row.BillingFirstName)
	}
	h.Settle(t)
}

// Scenario: the business fields and the phone are asked for and required only as the store's settings say, and land on the order
func TestCheckoutDetailsFollowTheSettings(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "biz-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cartID))
	business := fmt.Sprintf(`email: %q, purchaseType: BUSINESS, company: "Acme d.o.o.", poNumber: "PO-7", shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)

	// Business purchases are off by default - the fields are dropped rather than refused
	co := h.gqlCheckout(t, ts, f, details(cartID, business))
	if co["purchaseType"] != "PERSONAL" || co["company"] != "" || co["poNumber"] != "" {
		t.Fatalf("with business purchases off, checkout = %v, want a personal purchase", co)
	}

	h.setChannelConfig(t, f, "sales/checkout/business_purchase", "true")
	h.setChannelConfig(t, f, "sales/checkout/tax_id_field", "required")
	h.setChannelConfig(t, f, "sales/checkout/po_number_field", "true")
	h.setChannelConfig(t, f, "sales/checkout/phone_field", "required")
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, business)); !strings.Contains(msg, "phone") {
		t.Errorf("details without the required phone = %q", msg)
	}
	// Too short to be a US number - read in the destination's country, it can't be one
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, business+`, phone: "650 253"`)); !strings.Contains(msg, "phone number is not valid") {
		t.Errorf("details with a malformed phone = %q", msg)
	}
	// A country with no company ID fields of its own asks for the generic tax ID the setting describes
	business += `, phone: "650 253 0000"`
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, business)); !strings.Contains(msg, "Tax ID") {
		t.Errorf("business details without the required tax ID = %q", msg)
	}
	co = h.gqlCheckout(t, ts, f, details(cartID, business+`, businessIds: [{key: "tax_id", value: " 12-3456789 "}]`))
	if ids := co["businessIds"].([]any); co["purchaseType"] != "BUSINESS" || co["poNumber"] != "PO-7" || len(ids) != 1 ||
		ids[0].(map[string]any)["value"] != "12-3456789" {
		t.Fatalf("business checkout = %v", co)
	}

	// A country that defines its own fields asks for exactly those, each checked against its pattern
	fields := []country.BusinessIDField{
		{Key: "ein", Label: "EIN", Required: true, Pattern: `\d{2}-\d{7}`},
		{Key: "state_id", Label: "State ID"},
	}
	if _, err := h.D.Country.Update(h.Ctx, country.UpdateCountryInput{Code: "US", BusinessIDFields: &fields}); err != nil {
		t.Fatalf("define US company ID fields: %v", err)
	}
	t.Cleanup(func() {
		none := []country.BusinessIDField{}
		_, _ = h.D.Country.Update(h.Ctx, country.UpdateCountryInput{Code: "US", BusinessIDFields: &none})
	})
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, business+`, businessIds: [{key: "tax_id", value: "12-3456789"}]`)); !strings.Contains(msg, "EIN") {
		t.Errorf("details without the country's own required field = %q", msg)
	}
	if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, business+`, businessIds: [{key: "ein", value: "123"}]`)); !strings.Contains(msg, "check the EIN") {
		t.Errorf("details with an EIN that doesn't match the pattern = %q", msg)
	}
	co = h.gqlCheckout(t, ts, f, details(cartID, business+`, businessIds: [{key: "ein", value: "12-3456789"}, {key: "state_id", value: "S-1"}, {key: "tax_id", value: "dropped"}]`))
	if ids := co["businessIds"].([]any); len(ids) != 2 {
		t.Fatalf("business ids = %v, want the two the country asks for", ids)
	}

	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	// The order keeps each identifier with its label, in the country's order, so the invoice reads as the shopper saw it
	var ids []order.BusinessID
	_ = json.Unmarshal(row.BusinessIds, &ids)
	wantIDs := []order.BusinessID{{Key: "ein", Label: "EIN", Value: "12-3456789"}, {Key: "state_id", Label: "State ID", Value: "S-1"}}
	if !slices.Equal(ids, wantIDs) || row.PoNumber.String != "PO-7" || row.BillingCompany.String != "Acme d.o.o." {
		t.Errorf("order business ids=%s po=%q billing company=%q, want the business details", row.BusinessIds, row.PoNumber.String, row.BillingCompany.String)
	}
	h.Settle(t)
}

// Scenario: a typed address's phone is read in the address's own country and stored as E.164, and one that can't be a
// number there is refused
func TestCheckoutAddressPhoneIsReadInItsCountry(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "us", postalCode: "12345")`, cartID))
	email := "addr-phone-" + uuid.NewString()[:8] + "@e2e.test"
	withPhone := func(tel string) string {
		return strings.Replace(usAddress, `countryCode: "us"`, fmt.Sprintf(`countryCode: "us", phone: %q`, tel), 1)
	}

	// A Serbian mobile typed nationally isn't a US number
	msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, withPhone("064 123 4567"))))
	if !strings.Contains(msg, "phone number is not valid") {
		t.Errorf("a Serbian national number on a US address = %q", msg)
	}

	co := h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, withPhone("(212) 555-0123"))))
	if a := co["shippingAddress"].(map[string]any); a["phone"] != "+12125550123" {
		t.Fatalf("shipping address phone = %v, want +12125550123", a["phone"])
	}
	if _, errs := h.gqlPost(t, ts, f.Channel.ID.String(), `mutation { `+details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, withPhone("+1 212 555 0123 ext. 42")))+` { id } }`); len(errs) == 0 {
		t.Fatal("checkout accepted a phone extension")
	}
	persisted, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, uuid.MustParse(cartID))
	if err != nil || persisted.Checkout.ShippingAddress == nil || persisted.Checkout.ShippingAddress.Phone == nil || *persisted.Checkout.ShippingAddress.Phone != "+12125550123" {
		t.Fatalf("persisted phone=%+v, %v", persisted.Checkout.ShippingAddress, err)
	}

}

// Scenario: placement holds the checkout to what the shopper chose and agreed to - an unticked terms box is refused
// under the default checkbox setting, and a method the carriers stopped offering is refused rather than swapped for the
// cheapest one, with the choice left on the cart for when it comes back
func TestCheckoutPlacementRefusesWhatItCantHonour(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatalf("sync carriers: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "honour-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cartID))
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))

	for _, q := range []string{`placeOrder(cartId: %q)`, `placeOrder(cartId: %q, acceptTerms: false)`} {
		if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { `+q+` { id } }`, cartID)); len(errs) == 0 || !strings.Contains(errs[0], "accept the terms") {
			t.Errorf("%s = %v, want TERMS_NOT_ACCEPTED", q, errs)
		}
	}

	// The carrier the shopper chose goes away between the delivery step and "Place order"
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, "builtin", false); err != nil {
		t.Fatalf("disable the builtin carrier: %v", err)
	}
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID)); len(errs) == 0 || !strings.Contains(errs[0], "isn't available") {
		t.Fatalf("placeOrder with the chosen method gone = %v, want SHIPPING_METHOD_UNAVAILABLE", errs)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, "builtin", true); err != nil {
		t.Fatalf("re-enable the builtin carrier: %v", err)
	}
	co := h.gqlCartCheckout(t, ts, f, cartID)
	if m, _ := co["shippingMethod"].(map[string]any); m == nil || m["code"] != "standard" {
		t.Fatalf("shipping method after the refusal = %v, want the shopper's choice kept", co["shippingMethod"])
	}

	// Under notice the order line says placing it accepts the terms, so nothing has to be ticked
	h.setChannelConfig(t, f, "sales/checkout/terms_consent", "notice")
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder under notice: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.ShippingMethodCode.String != "standard" {
		t.Errorf("order shipping method = %q, want the chosen one", row.ShippingMethodCode.String)
	}
	h.Settle(t)
}

// Scenario: purchase order bills against the PO number the details step asks a business buyer for, so it's offered,
// accepted and placed only on such a checkout - and the number lands on the order
func TestCheckoutPurchaseOrderNeedsThePONumberField(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	if _, err := h.D.Payment.SetEnabled(h.Ctx, f.Channel.ID, "purchase-order", true); err != nil {
		t.Fatalf("enable purchase order: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "po-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cartID))
	h.setChannelConfig(t, f, "sales/checkout/business_purchase", "true")

	offered := func() []string {
		t.Helper()
		data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`{ paymentMethods(countryCode: "US", cartId: %q) { providerSlug } }`, cartID))
		if len(errs) > 0 {
			t.Fatalf("paymentMethods: %v", errs)
		}
		var out []string
		for _, m := range data["paymentMethods"].([]any) {
			out = append(out, m.(map[string]any)["providerSlug"].(string))
		}
		return out
	}

	// A personal purchase has no PO number to bill against
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	if got := offered(); slices.Contains(got, "purchase-order") || !slices.Contains(got, "cod") {
		t.Errorf("methods for a personal purchase = %v, want cod without purchase-order", got)
	}
	h.gqlCheckoutRefused(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "purchase-order")`, cartID))

	// Business, but the store doesn't ask for the PO number either
	business := fmt.Sprintf(`email: %q, purchaseType: BUSINESS, company: "Acme", poNumber: "PO-42", shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)
	h.gqlCheckout(t, ts, f, details(cartID, business))
	if got := offered(); slices.Contains(got, "purchase-order") {
		t.Errorf("methods with the PO field off = %v, want no purchase-order", got)
	}

	h.setChannelConfig(t, f, "sales/checkout/po_number_field", "true")
	co := h.gqlCheckout(t, ts, f, details(cartID, business))
	if co["poNumber"] != "PO-42" {
		t.Fatalf("PO number = %v, want PO-42", co["poNumber"])
	}
	if got := offered(); !slices.Contains(got, "purchase-order") {
		t.Errorf("methods for a business purchase with the PO field = %v, want purchase-order", got)
	}
	co = h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "purchase-order")`, cartID))
	if co["paymentProviderSlug"] != "purchase-order" {
		t.Fatalf("payment method = %v, want purchase-order", co["paymentProviderSlug"])
	}

	// Switching back to a personal purchase drops the method it can no longer pay with
	co = h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)))
	if co["paymentProviderSlug"] != nil {
		t.Errorf("payment method after going personal = %v, want purchase-order dropped", co["paymentProviderSlug"])
	}
	h.gqlCheckout(t, ts, f, details(cartID, business))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "purchase-order")`, cartID))
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.PaymentProvider != "purchase-order" || row.PoNumber.String != "PO-42" {
		t.Errorf("order provider=%q po=%q, want purchase-order with PO-42", row.PaymentProvider, row.PoNumber.String)
	}
	h.Settle(t)
}

// Scenario: free text on the details step is capped, and a business purchase saved before the store turned business
// purchases off places as a personal one
func TestCheckoutDetailsAreCappedAndFollowTheSettingAtPlacement(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "caps-" + uuid.NewString()[:8] + "@e2e.test"
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, cartID))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, cartID))
	h.setChannelConfig(t, f, "sales/checkout/business_purchase", "true")
	h.setChannelConfig(t, f, "sales/checkout/po_number_field", "true")
	base := fmt.Sprintf(`email: %q, purchaseType: BUSINESS, shippingAddress: %s, billingSameAsShipping: true`, email, usAddress)
	long := func(n int) string { return strings.Repeat("x", n) }

	for _, tc := range []struct{ name, fields, want string }{
		{"company", fmt.Sprintf(`company: %q`, long(201)), "company name is too long"},
		{"PO number", fmt.Sprintf(`company: "Acme", poNumber: %q`, long(101)), "PO number is too long"},
		{"tax ID", fmt.Sprintf(`company: "Acme", businessIds: [{key: "tax_id", value: %q}]`, long(101)), "Tax ID is too long"},
	} {
		if msg := h.gqlCheckoutRefused(t, ts, f, details(cartID, base+", "+tc.fields)); !strings.Contains(msg, tc.want) {
			t.Errorf("%s over the cap = %q, want %q", tc.name, msg, tc.want)
		}
	}
	co := h.gqlCheckout(t, ts, f, details(cartID, base+fmt.Sprintf(`, company: %q, poNumber: "PO-1", businessIds: [{key: "tax_id", value: "T-1"}]`, long(200))))
	if co["purchaseType"] != "BUSINESS" {
		t.Fatalf("a company name at the cap = %v, want it saved", co)
	}
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))

	h.setChannelConfig(t, f, "sales/checkout/business_purchase", "false")
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.BillingCompany.String != "" || row.PoNumber.String != "" || string(row.BusinessIds) != "[]" {
		t.Errorf("order company=%q po=%q ids=%s, want a personal purchase once business purchases are off",
			row.BillingCompany.String, row.PoNumber.String, row.BusinessIds)
	}
	h.Settle(t)
}

// Scenario: a pickup with no billing address asked for is billed to whoever collects, never to the store's own address,
// and pays by a method checked against the destination's country - the same one the payment step offered it for
func TestCheckoutPickupBillsTheCollectorInTheDestinationCountry(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	line1, city, postal, country, active := "9 Depot Rd", "Storeton", "54321", "US", true
	if _, err := h.D.Inventory.UpdateLocation(h.Ctx, inventory.UpdateLocationInput{ID: f.Location.ID, IsActive: &active, Line1: &line1, City: &city, PostalCode: &postal, CountryCode: &country}); err != nil {
		t.Fatalf("location address: %v", err)
	}
	if _, err := h.D.Inventory.SetLocationPickup(h.Ctx, f.Location.ID, true, ""); err != nil {
		t.Fatalf("enable pickup: %v", err)
	}
	if err := h.D.Shipping.SyncCarriers(h.Ctx, []uuid.UUID{f.Channel.ID}); err != nil {
		t.Fatalf("sync carriers: %v", err)
	}
	if _, err := h.D.Shipping.SetCarrierEnabled(h.Ctx, f.Channel.ID, "pickup", true); err != nil {
		t.Fatalf("enable the pickup carrier: %v", err)
	}
	// Cash on delivery only for shoppers in GB - the store itself is in the US
	if err := h.D.Payment.SetPaymentMethodCountries(h.Ctx, f.Channel.ID, "cod", "specific", []string{"GB"}); err != nil {
		t.Fatalf("limit cod to GB: %v", err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	cartID := h.gqlGuestCart(t, ts, f, 1)
	email := "collector-" + uuid.NewString()[:8] + "@e2e.test"

	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "GB", postalCode: "N1 1AA")`, cartID))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "pickup", code: "collect")`, cartID))
	h.gqlCheckout(t, ts, f, details(cartID, fmt.Sprintf(`email: %q, pickupContact: {firstName: "Ana", lastName: "Self", someoneElse: false}, billingSameAsShipping: false`, email)))
	h.gqlCheckout(t, ts, f, fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, cartID))

	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, cartID))
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v - the payment step offered cod for GB, so placement must check GB too", errs)
	}
	row, err := h.D.Queries.GetOrderByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order row: %v", err)
	}
	if row.ShippingStreet1 != line1 || row.ShippingFirstName != "Ana" {
		t.Errorf("order ships to %q for %q, want the store for the collector", row.ShippingStreet1, row.ShippingFirstName)
	}
	if row.BillingStreet1 == line1 || row.BillingCity == city || row.BillingFirstName != "Ana" || row.BillingLastName != "Self" {
		t.Errorf("order billed to %q %q at %q %q, want the collector and not the store's address",
			row.BillingFirstName, row.BillingLastName, row.BillingStreet1, row.BillingCity)
	}
	h.Settle(t)
}

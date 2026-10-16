package e2e

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// Scenario: an admin builds an offer for a guest email - an admin price on a catalog line, a free-form custom line,
// the lines locked - and the recipient completes it through the storefront API with nothing but the link. The id
// alone opens nothing, the locks hold, the order charges the admin's prices and taxes the custom line by its class,
// and the offer reads completed with the order linked
func TestCartOfferMoneyPath(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 0)
	h.newTaxRule(t, f.Channel.ID, 1000)
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	recipient := "offer-" + uuid.NewString()[:8] + "@e2e.test"

	// The admin's side, through the service the console drives
	offer, err := h.D.Cart.AdminCreateOffer(h.Ctx, cart.CreateOfferInput{ChannelID: f.Channel.ID, Email: recipient, ExpiresAt: time.Now().Add(48 * time.Hour)})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	oldToken := offer.Offer.LinkToken
	if decoded, err := base64.RawURLEncoding.DecodeString(oldToken); err != nil || len(oldToken) != 22 || len(decoded) != 16 {
		t.Fatalf("offer token is not a 22-character random credential")
	}
	offer, err = h.D.Cart.AdminRegenerateOfferLink(h.Ctx, offer.ID)
	if err != nil || offer.Offer.LinkToken == oldToken {
		t.Fatalf("replace offer link: %v", err)
	}
	if _, err := h.D.Cart.OpenOffer(h.Ctx, oldToken, f.Channel.ID, nil); !errors.Is(err, cart.ErrOfferLinkInvalid) {
		t.Fatalf("old opaque link still opens: %v", err)
	}
	c, err := h.D.Cart.AdminAddToCart(h.Ctx, offer.ID, f.Product.SKU, 2)
	if err != nil {
		t.Fatalf("add line: %v", err)
	}
	unit := int64(1500)
	if _, err := h.D.Cart.AdminSetLinePrice(h.Ctx, offer.ID, c.Lines[0].ID, &unit); err != nil {
		t.Fatalf("set price: %v", err)
	}
	if _, err := h.D.Cart.AdminAddCustomLine(h.Ctx, offer.ID, cart.CustomLineInput{Name: "Setup fee", UnitPrice: 5000, Quantity: 1}); err != nil {
		t.Fatalf("custom line: %v", err)
	}
	settings := cart.OfferSettings{ExpiresAt: time.Now().Add(48 * time.Hour), Locks: []string{cart.LockLines, cart.LockCoupons}, PaymentMethods: []string{"bank_transfer"}}
	if _, err := h.D.Cart.AdminSetOffer(h.Ctx, offer.ID, settings, true); err != nil {
		t.Fatalf("set offer: %v", err)
	}
	offerID := offer.ID.String()
	link := map[string]string{cart.OfferLinkHeader: offer.Offer.LinkToken}

	// The id alone is no way in, and a shopper cart was never touched by it
	if data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`{ cart(id: %q) { id } }`, offerID)); len(errs) > 0 || data["cart"] != nil {
		t.Fatalf("offer read without its link = %v %v, want null", data, errs)
	}

	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { openCartOffer(token: %q) { cartId state requiresLogin } }`, link[cart.OfferLinkHeader]))
	if len(errs) > 0 {
		t.Fatalf("openCartOffer: %v", errs)
	}
	if o := data["openCartOffer"].(map[string]any); o["cartId"] != offerID || o["state"] != "OPENED" || o["requiresLogin"] != false {
		t.Fatalf("opening = %v", o)
	}

	data, errs = h.gqlPost(t, ts, chID, fmt.Sprintf(`{ cart(id: %q) { subtotal lines { sku unitPrice isCustom } offer { state locks isGuest } } }`, offerID), link)
	if len(errs) > 0 || data["cart"] == nil {
		t.Fatalf("offer read with its link = %v %v", data, errs)
	}
	read := data["cart"].(map[string]any)
	if read["subtotal"].(float64) != 8000 {
		t.Fatalf("subtotal = %v, want 2 x 1500 + 5000", read["subtotal"])
	}
	if o := read["offer"].(map[string]any); o["isGuest"] != true || fmt.Sprint(o["locks"]) != "[LINES COUPONS]" {
		t.Fatalf("offer = %v", o)
	}

	// The locks hold against the recipient
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { addToCart(cartId: %q, sku: %q, quantity: 1) { id } }`, offerID, f.Product.SKU), link); len(errs) == 0 {
		t.Fatal("added to a locked offer")
	}
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { applyCoupon(cartId: %q, code: "ANY") { id } }`, offerID), link); len(errs) == 0 || !strings.Contains(strings.Join(errs, ";"), "can't be changed") {
		t.Fatalf("a coupon on a coupon-locked offer = %v, want the lock refusal", errs)
	}
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345") { id } }`, offerID), link); len(errs) > 0 {
		t.Fatalf("offer destination: %v", errs)
	}
	if _, errs := h.gqlPost(t, ts, chID, `mutation { `+details(offerID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, "someone@else.test", usAddress))+` { id } }`, link); len(errs) == 0 || !strings.Contains(strings.Join(errs, "; "), "can't be changed") {
		t.Fatalf("recipient email edit = %v, want offer lock", errs)
	}

	// The offer narrows payment - a method outside its list is refused, then the admin allows cash on delivery
	if _, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { setCartPaymentMethod(cartId: %q, providerSlug: "cod") { id } }`, offerID), link); len(errs) == 0 || !strings.Contains(strings.Join(errs, ";"), "isn't available for this offer") {
		t.Fatalf("a payment method outside the offer's = %v, want the refusal", errs)
	}
	settings.PaymentMethods = []string{"cod"}
	if _, err := h.D.Cart.AdminSetOffer(h.Ctx, offer.ID, settings, true); err != nil {
		t.Fatalf("allow cod: %v", err)
	}

	// Checkout as the storefront runs it, every call carrying the link
	for _, step := range []string{
		fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "us", postalCode: "12345") { id }`, offerID),
		fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard") { id }`, offerID),
		details(offerID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, recipient, usAddress)) + ` { id }`,
		fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod") { id }`, offerID),
	} {
		if _, errs := h.gqlPost(t, ts, chID, `mutation { `+step+` }`, link); len(errs) > 0 {
			t.Fatalf("%s: %v", step, errs)
		}
	}
	data, errs = h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id total } }`, offerID), link)
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	orderID := uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string))
	o, err := h.D.Order.GetByID(h.Ctx, orderID)
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	// 8000 goods + 500 shipping, 10% tax on both - the custom line has no class of its own and matches the every-product rule
	if o.SubtotalAmount != 8000 || o.TaxAmount != 850 || o.TotalAmount != 9350 || o.Email != recipient || o.CustomerID != nil {
		t.Fatalf("order subtotal=%d tax=%d total=%d email=%q customer=%v, want 8000/850/9350 to the guest recipient",
			o.SubtotalAmount, o.TaxAmount, o.TotalAmount, o.Email, o.CustomerID)
	}
	var custom, priced int
	for _, l := range o.Lines {
		if l.IsCustom && l.ProductName == "Setup fee" && l.UnitPrice == 5000 {
			custom++
		}
		if l.ProductSku == f.Product.SKU && l.UnitPrice == 1500 && !l.IsCustom {
			priced++
		}
	}
	if custom != 1 || priced != 1 {
		t.Fatalf("order lines = %+v, want the admin-priced line and the custom line", o.Lines)
	}

	done, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, offer.ID)
	if err != nil {
		t.Fatalf("reread offer: %v", err)
	}
	if done.OfferState(time.Now()) != cart.OfferStateCompleted || done.Offer.OrderID == nil || *done.Offer.OrderID != orderID {
		t.Fatalf("offer after placement: state=%s order=%v", done.OfferState(time.Now()), done.Offer.OrderID)
	}
	// A placed offer's link opens nothing more
	if _, err := h.D.Cart.GetCart(h.Ctx, offer.ID, nil); !errors.Is(err, cart.ErrCartNotActive) {
		t.Fatalf("completed offer still opens: %v", err)
	}
	h.Settle(t)
}

// Scenario: an order edit re-prices a custom offer line the way placement did - taxed by the class the admin gave it,
// and left out of shipping, which it never takes. A 500.00 installation fee mustn't lift a 20.00 order over free shipping
func TestCartOfferCustomLineSurvivesAnOrderEdit(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.addFlatShipping(t, f, 500, 10000)
	h.newTaxRule(t, f.Channel.ID, 1000, tax.Condition{Type: "core.product_tax_class", Params: json.RawMessage(`{"classes":["service"]}`), ParamsVersion: 1})
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	chID := f.Channel.ID.String()
	recipient := "offer-" + uuid.NewString()[:8] + "@e2e.test"
	offer, err := h.D.Cart.AdminCreateOffer(h.Ctx, cart.CreateOfferInput{ChannelID: f.Channel.ID, Email: recipient, ExpiresAt: time.Now().Add(48 * time.Hour)})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	if _, err := h.D.Cart.AdminAddToCart(h.Ctx, offer.ID, f.Product.SKU, 1); err != nil {
		t.Fatalf("add line: %v", err)
	}
	if _, err := h.D.Cart.AdminAddCustomLine(h.Ctx, offer.ID, cart.CustomLineInput{Name: "Installation", UnitPrice: 50000, Quantity: 1, TaxClass: "service"}); err != nil {
		t.Fatalf("custom line: %v", err)
	}
	offerID := offer.ID.String()
	link := map[string]string{cart.OfferLinkHeader: offer.Offer.LinkToken}
	for _, step := range []string{
		fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345") { id }`, offerID),
		fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard") { id }`, offerID),
		details(offerID, fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, recipient, usAddress)) + ` { id }`,
		fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod") { id }`, offerID),
	} {
		if _, errs := h.gqlPost(t, ts, chID, `mutation { `+step+` }`, link); len(errs) > 0 {
			t.Fatalf("%s: %v", step, errs)
		}
	}
	data, errs := h.gqlPost(t, ts, chID, fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id } }`, offerID), link)
	if len(errs) > 0 {
		t.Fatalf("placeOrder: %v", errs)
	}
	placed, err := h.D.Order.GetByID(h.Ctx, uuid.MustParse(data["placeOrder"].(map[string]any)["id"].(string)))
	if err != nil {
		t.Fatalf("order: %v", err)
	}
	// Only the service line is taxed - 10% of 50000 - and the 2000 product alone is under the 10000 free-shipping threshold
	if placed.ShippingAmount != 500 || placed.TaxAmount-placed.ShippingTaxAmount != 5000 {
		t.Fatalf("placed shipping=%d goods tax=%d, want 500 and 5000", placed.ShippingAmount, placed.TaxAmount-placed.ShippingTaxAmount)
	}
	moved, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{OrderID: placed.ID, Shipping: editAddr("Ada", "Lovelace", "Boston", "02101")})
	if err != nil {
		t.Fatalf("EditOrderAddress: %v", err)
	}
	if moved.ShippingAmount != placed.ShippingAmount || moved.TaxAmount != placed.TaxAmount || moved.TotalAmount != placed.TotalAmount {
		t.Fatalf("after the address edit shipping=%d tax=%d total=%d, want placement's %d/%d/%d",
			moved.ShippingAmount, moved.TaxAmount, moved.TotalAmount, placed.ShippingAmount, placed.TaxAmount, placed.TotalAmount)
	}
	h.Settle(t)
}

// Scenario: a customer can hold their own cart and several offers at once - the offers never become the active cart,
// and logging in never merges one into it
func TestCartOffersBesideTheShopperCart(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	cu, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, "offers-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	own, err := h.D.Cart.GetOrCreateCart(h.Ctx, cu.ID, f.Channel.ID)
	if err != nil {
		t.Fatalf("own cart: %v", err)
	}
	for range 2 {
		o, err := h.D.Cart.AdminCreateOffer(h.Ctx, cart.CreateOfferInput{ChannelID: f.Channel.ID, CustomerID: &cu.ID, ExpiresAt: time.Now().Add(time.Hour)})
		if err != nil {
			t.Fatalf("offer beside the shopper cart: %v", err)
		}
		if o.ID == own.ID {
			t.Fatal("the offer reused the shopper's cart")
		}
		if _, err := h.D.Cart.MergeGuestCart(h.Ctx, cu.ID, o.ID); !errors.Is(err, cart.ErrCartForbidden) {
			t.Fatalf("an offer merged: %v", err)
		}
	}
	again, err := h.D.Cart.GetOrCreateCart(h.Ctx, cu.ID, f.Channel.ID)
	if err != nil || again.ID != own.ID {
		t.Fatalf("active cart = %v (%v), want the shopper's own %v", again.ID, err, own.ID)
	}
}

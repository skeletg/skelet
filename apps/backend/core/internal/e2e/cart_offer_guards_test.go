package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

func (h *harness) guestOffer(t *testing.T, f *store) cart.Cart {
	t.Helper()
	c, err := h.D.Cart.AdminCreateOffer(h.Ctx, cart.CreateOfferInput{ChannelID: f.Channel.ID, Email: "offer-" + uuid.NewString() + "@e2e.test", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	c, err = h.D.Cart.AdminAddToCart(h.Ctx, c.ID, f.Product.SKU, 1)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestUnlockedOfferLinesAreProtectedOverHTTP(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	c := h.guestOffer(t, f)
	unit := int64(1500)
	if _, err := h.D.Cart.AdminSetLinePrice(h.Ctx, c.ID, c.Lines[0].ID, &unit); err != nil {
		t.Fatal(err)
	}
	c, err := h.D.Cart.AdminAddCustomLine(h.Ctx, c.ID, cart.CustomLineInput{Name: "Fee", UnitPrice: 500, Quantity: 1})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h.D.Root.Handler)
	defer ts.Close()
	link := map[string]string{cart.OfferLinkHeader: c.Offer.LinkToken}
	for _, l := range c.Lines {
		for _, mutation := range []string{
			fmt.Sprintf(`updateCartLine(cartId: %q, lineId: %q, quantity: 2)`, c.ID, l.ID),
			fmt.Sprintf(`removeFromCart(cartId: %q, lineId: %q)`, c.ID, l.ID),
		} {
			_, errs := h.gqlPost(t, ts, f.Channel.ID.String(), `mutation { `+mutation+` { id } }`, link)
			if len(errs) == 0 || !strings.Contains(strings.Join(errs, "; "), "can't be changed") {
				t.Fatalf("%s: %v, want offer lock", mutation, errs)
			}
		}
	}
	got, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, c.ID)
	if err != nil || len(got.Lines) != 2 {
		t.Fatalf("offer changed: %+v, %v", got, err)
	}
	for _, l := range got.Lines {
		if l.Quantity != 1 {
			t.Fatalf("quantity changed: %+v", l)
		}
	}
	h.Settle(t)
}

func TestOfferCouponLockControlsAutomaticPromotions(t *testing.T) {
	for _, locked := range []bool{true, false} {
		t.Run(fmt.Sprintf("locked=%v", locked), func(t *testing.T) {
			h := newHarness(t)
			f := h.NewStore(t, 10, 2000)
			h.addFlatShipping(t, f, 500, 0)
			h.newRule(t, promotion.CreateRuleInput{Rule: promotion.Rule{Slug: "offer-" + uuid.NewString(), Name: "Automatic discount", ChannelID: &f.Channel.ID, IsActive: true}, Actions: []promotion.Action{{Type: "core.fixed_off_subtotal", Params: json.RawMessage(`{"cents":500}`)}, {Type: "core.free_shipping", Params: json.RawMessage(`{}`)}}})
			c := h.guestOffer(t, f)
			if locked {
				if _, err := h.D.Cart.AdminSetOffer(h.Ctx, c.ID, cart.OfferSettings{ExpiresAt: c.Offer.ExpiresAt, Locks: []string{cart.LockCoupons}}, true); err != nil {
					t.Fatal(err)
				}
			}
			ts := httptest.NewServer(h.D.Root.Handler)
			defer ts.Close()
			link := map[string]string{cart.OfferLinkHeader: c.Offer.LinkToken}
			for _, step := range []string{
				fmt.Sprintf(`setCartDestination(cartId: %q, countryCode: "US", postalCode: "12345")`, c.ID),
				fmt.Sprintf(`setCartShippingMethod(cartId: %q, carrierSlug: "builtin", code: "standard")`, c.ID),
				details(c.ID.String(), fmt.Sprintf(`email: %q, shippingAddress: %s, billingSameAsShipping: true`, c.Email, usAddress)),
				fmt.Sprintf(`setCartPaymentMethod(cartId: %q, providerSlug: "cod")`, c.ID),
			} {
				if _, errs := h.gqlPost(t, ts, f.Channel.ID.String(), `mutation { `+step+` { id } }`, link); len(errs) > 0 {
					t.Fatalf("%s: %v", step, errs)
				}
			}
			data, errs := h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`{ cart(id: %q) { totalDiscount checkout { totals { discountCents shippingCents totalCents } } } }`, c.ID), link)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			got := data["cart"].(map[string]any)
			totals := got["checkout"].(map[string]any)["totals"].(map[string]any)
			want := float64(500)
			if locked {
				want = 0
			}
			shipping := float64(0)
			if locked {
				shipping = 500
			}
			if totals["shippingCents"] != shipping {
				t.Fatalf("shipping=%v, want %v", totals["shippingCents"], shipping)
			}
			if got["totalDiscount"] != want || totals["discountCents"] != want {
				t.Fatalf("cart/checkout = %v, want discount %v", got, want)
			}
			data, errs = h.gqlPost(t, ts, f.Channel.ID.String(), fmt.Sprintf(`mutation { placeOrder(cartId: %q, acceptTerms: true) { id total } }`, c.ID), link)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			placed := data["placeOrder"].(map[string]any)
			o, err := h.D.Order.GetByID(h.Ctx, uuid.MustParse(placed["id"].(string)))
			if err != nil || o.DiscountAmount != int64(want) || placed["total"] != totals["totalCents"] {
				t.Fatalf("placed=%+v err=%v quote=%v", o, err, totals)
			}
			h.Settle(t)
		})
	}
}

func TestOfferPlacementRechecksItsLifetimeAndLink(t *testing.T) {
	for _, when := range []string{"before checkout", "during placement"} {
		for _, change := range []string{"expire", "revoke", "replace link"} {
			t.Run(when+"/"+change, func(t *testing.T) {
				h := newHarness(t)
				f := h.NewStore(t, 10, 2000)
				h.addFlatShipping(t, f, 0, 0)
				c := h.guestOffer(t, f)
				ctx := channel.WithChannel(cart.WithOfferGrant(h.Ctx, cart.OfferGrant{CartID: c.ID, LinkVersion: c.Offer.LinkVersion}), &f.Channel)
				modify := func() error {
					switch change {
					case "expire":
						_, err := h.D.DB.ExecContext(h.Ctx, `UPDATE carts SET offer_expires_at = clock_timestamp() - interval '1 second' WHERE id=$1`, c.ID)
						return err
					case "revoke":
						_, err := h.D.Cart.AdminRevokeOffer(h.Ctx, c.ID)
						return err
					default:
						_, err := h.D.Cart.AdminRegenerateOfferLink(h.Ctx, c.ID)
						return err
					}
				}
				if when == "before checkout" {
					if err := modify(); err != nil {
						t.Fatal(err)
					}
				} else {
					h.D.Bus.SubscribeSync(order.EventPlacing, -100, func(_ context.Context, e events.Event) error {
						if e.(*order.OrderPlacing).Cart.ID != c.ID {
							return nil
						}
						return modify()
					})
				}
				cod := "cod"
				_, err := h.D.Order.PlaceOrder(ctx, order.PlaceOrderInput{CartID: &c.ID, Email: c.Email, BillingAddress: e2eAddress(), ShippingAddress: e2eAddress(), PaymentProviderSlug: &cod})
				if err == nil {
					t.Fatal("invalidated offer placed")
				}
				if change == "expire" && !errors.Is(err, cart.ErrOfferExpired) {
					t.Errorf("expiry error = %v", err)
				}
				var orders int
				if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT count(*) FROM orders WHERE email=$1`, c.Email).Scan(&orders); err != nil {
					t.Fatal(err)
				}
				if orders != 0 || h.ReservedFor(t, f) != 0 {
					t.Fatalf("failed placement left orders=%d reserved=%v", orders, h.ReservedFor(t, f))
				}
				current, err := h.D.Cart.GetCartByIDReadOnly(h.Ctx, c.ID)
				if err != nil || current.Status != "active" || current.Offer.OrderID != nil {
					t.Fatalf("converted failed offer: %+v, %v", current, err)
				}
				h.Settle(t)
			})
		}
	}
}

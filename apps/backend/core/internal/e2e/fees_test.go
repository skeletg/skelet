package e2e

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

var (
	feesOnce    sync.Once
	feesPlugin  = sdk.NewPluginForTest("e2e-fees")
	feeChannels sync.Map // the channels whose checkouts the fees charge - every other scenario's go untouched
)

func registerE2EFees() {
	feesOnce.Do(func() {
		charged := func(req sdk.FeeRequest) bool {
			_, ok := feeChannels.Load(req.ChannelID)
			return ok
		}
		feesPlugin.RegisterFee(sdk.Fee{Key: "cod_handling", Label: "Cash on delivery", Charge: func(_ context.Context, req sdk.FeeRequest) (int64, error) {
			if !charged(req) || req.PaymentMethod != "cod" {
				return 0, nil
			}
			if req.SubtotalCents > 10_000 {
				return 0, sdk.Invalid("cash on delivery takes orders up to 100.00")
			}
			return 500, nil
		}})
		feesPlugin.RegisterFee(sdk.Fee{Key: "eco", Label: "Eco fee", Untaxed: true, Charge: func(_ context.Context, req sdk.FeeRequest) (int64, error) {
			if !charged(req) {
				return 0, nil
			}
			var units float64
			for _, l := range req.Lines {
				units += l.Quantity
			}
			return int64(units) * 100, nil
		}})
	})
}

// Scenario: a plugin's fees are quoted the way placement charges them, taxed under their class unless untaxed, kept on
// the order with their labels, and left as they were by an edit that re-taxes the order
func TestPluginFeesAreQuotedChargedTaxedAndKept(t *testing.T) {
	h := newHarness(t)
	registerE2EFees()
	f := h.NewStore(t, 20, 2000)
	feeChannels.Store(f.Channel.ID, true)
	h.newTaxRule(t, f.Channel.ID, 1000)

	cartID := h.NewGuestCart(t, f, 1)
	cod := "cod"
	ctx := channel.WithChannel(h.Ctx, &f.Channel)
	quote, err := h.D.Order.QuoteCheckout(ctx, order.PlaceOrderInput{
		CartID: &cartID, BillingAddress: e2eAddress(), ShippingAddress: e2eAddress(), PaymentProviderSlug: &cod,
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	placed, err := h.TryPlace(t, f.Channel, cartID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}

	// 2000 of goods taxed 200, a 500 handling fee taxed 50, a 100 eco fee untaxed
	want := []order.Fee{
		{Key: "e2e-fees:cod_handling", Label: "Cash on delivery", AmountCents: 500, TaxCents: 50, Taxable: true},
		{Key: "e2e-fees:eco", Label: "Eco fee", AmountCents: 100},
	}
	if !slices.Equal(placed.Fees, want) {
		t.Errorf("fees %+v, want %+v", placed.Fees, want)
	}
	if placed.TaxAmount != 250 || placed.TotalAmount != 2850 {
		t.Errorf("tax %d total %d, want 250 and 2850", placed.TaxAmount, placed.TotalAmount)
	}
	if quote.TotalCents != placed.TotalAmount || !slices.Equal(quote.Fees, placed.Fees) {
		t.Errorf("quoted %d with %+v, charged %d with %+v", quote.TotalCents, quote.Fees, placed.TotalAmount, placed.Fees)
	}

	stored, err := h.D.Order.GetByID(h.Ctx, placed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stored.Fees, want) {
		t.Errorf("stored fees %+v, want %+v", stored.Fees, want)
	}

	edited, err := h.D.Order.EditOrderAddress(h.Ctx, order.EditOrderAddressInput{OrderID: placed.ID, Shipping: editAddr("Grace", "Hopper", "Arlington", "22202")})
	if err != nil {
		t.Fatalf("edit address: %v", err)
	}
	if !slices.Equal(edited.Fees, want) || edited.TotalAmount != 2850 {
		t.Errorf("after an edit: fees %+v total %d, want them as placed", edited.Fees, edited.TotalAmount)
	}
	if got := edited.FeesRefundable(); got != 650 {
		t.Errorf("fees refundable %d, want 650 - the fees and their tax", got)
	}

	// Another payment method leaves the handling off, and the store's other checkouts pay no fee at all
	other := h.NewStore(t, 20, 2000)
	if o := h.PlaceGuestCOD(t, other, 1); len(o.Fees) != 0 || o.TotalAmount != 2000 {
		t.Errorf("an order on a channel without the fees: %+v, total %d", o.Fees, o.TotalAmount)
	}
	h.Settle(t)
}

// Scenario: a fee that refuses the checkout stops the order with the shopper's reason, and nothing is placed
func TestAPluginFeeCanRefuseTheOrder(t *testing.T) {
	h := newHarness(t)
	registerE2EFees()
	f := h.NewStore(t, 20, 2000)
	feeChannels.Store(f.Channel.ID, true)

	_, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 6))
	if apperr.ClassOf(err) != apperr.ClassValidation || !strings.Contains(err.Error(), "up to 100.00") {
		t.Fatalf("a refused fee: %v - want the shopper's reason", err)
	}
	if n := h.countOrdersOn(t, f.Channel.ID); n != 0 {
		t.Errorf("%d orders placed despite the refusal", n)
	}
}

func (h *harness) countOrdersOn(t *testing.T, channelID uuid.UUID) int {
	t.Helper()
	var n int
	if err := h.D.DB.QueryRowContext(h.Ctx, `SELECT count(*) FROM orders WHERE channel_id = $1`, channelID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

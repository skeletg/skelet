package e2e

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
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

// Scenario: on a tax-inclusive channel a fee is a gross amount like every price there - the total adds it once, its
// tax is carved out of it, and the quote and the order agree
func TestPluginFeesOnATaxInclusiveChannel(t *testing.T) {
	h := newHarness(t)
	registerE2EFees()
	f := h.NewStore(t, 20, 2000)
	feeChannels.Store(f.Channel.ID, true)
	h.newTaxRule(t, f.Channel.ID, 1000)
	on := true
	ch, err := h.D.Channel.Update(h.Ctx, channel.UpdateChannelInput{ID: f.Channel.ID, TaxInclusivePricing: &on})
	if err != nil {
		t.Fatal(err)
	}

	cartID := h.NewGuestCart(t, f, 1)
	cod := "cod"
	quote, err := h.D.Order.QuoteCheckout(channel.WithChannel(h.Ctx, &ch), order.PlaceOrderInput{
		CartID: &cartID, BillingAddress: e2eAddress(), ShippingAddress: e2eAddress(), PaymentProviderSlug: &cod,
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	placed, err := h.TryPlace(t, ch, cartID)
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	// 2000 of goods, a 500 handling fee and a 100 eco fee, all gross
	if placed.TotalAmount != 2600 || quote.TotalCents != 2600 || quote.TaxCents != placed.TaxAmount || !slices.Equal(quote.Fees, placed.Fees) {
		t.Errorf("quoted %d (tax %d), charged %d (tax %d) - want 2600 both times, the same tax and fees", quote.TotalCents, quote.TaxCents, placed.TotalAmount, placed.TaxAmount)
	}
	if len(placed.Fees) != 2 || placed.Fees[0].TaxCents != 45 || placed.Fees[1].TaxCents != 0 {
		t.Errorf("fees %+v - want 45 carved out of the 500 handling fee and none out of the untaxed eco fee", placed.Fees)
	}
	if got := placed.FeesRefundable(); got != 600 {
		t.Errorf("fees refundable %d, want 600 - nothing rides on top of a gross fee", got)
	}
	h.Settle(t)
}

// Scenario: a credit memo refunds a fee as a fee - up to what was charged, in the memo's total, and never counted as
// goods handed back
func TestACreditMemoRefundsAFeeApartFromTheGoods(t *testing.T) {
	h := newHarness(t)
	registerE2EFees()
	f := h.NewStore(t, 20, 2000)
	feeChannels.Store(f.Channel.ID, true)
	h.newTaxRule(t, f.Channel.ID, 1000)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	if _, err := h.D.Order.RecordOfflinePayment(h.Ctx, order.RecordOfflinePaymentInput{OrderID: o.ID, Method: "cod", Reference: "e2e-cash"}); err != nil {
		t.Fatalf("collect in full: %v", err)
	}
	if _, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{OrderID: o.ID, FeesRefundCents: 651}); apperr.CodeOf(err) != "CREDIT_MEMO_FEES_EXCEEDED" {
		t.Errorf("a fees refund over 600 + 50 tax: %v, want CREDIT_MEMO_FEES_EXCEEDED", err)
	}
	intent, err := h.D.Order.RecordCreditMemoIntent(h.Ctx, order.IssueCreditMemoInput{OrderID: o.ID, FeesRefundCents: 550})
	if err != nil {
		t.Fatalf("memo intent: %v", err)
	}
	if intent.AmountCents != 550 || intent.Memo.FeesRefundCents != 550 || intent.Memo.LinesTotalCents != 0 {
		t.Errorf("memo %+v refunding %d - want the 550 handling fee and its tax, and no goods", intent.Memo, intent.AmountCents)
	}
	memos, err := h.D.Order.ListCreditMemos(h.Ctx, o.ID)
	if err != nil || len(memos) != 1 || memos[0].FeesRefundCents != 550 {
		t.Errorf("stored memos %+v, %v", memos, err)
	}
	h.Settle(t)
}

// Scenario: a plugin that works out the order's tax does it before the fees are priced, so core taxes the fees on top
// of its figure - untaxed, they'd be charged less than the checkout quoted
func TestFeesAreTaxedOnTopOfAPluginsTax(t *testing.T) {
	h := newHarness(t)
	registerE2EFees()
	f := h.NewStore(t, 20, 2000)
	feeChannels.Store(f.Channel.ID, true)
	h.newTaxRule(t, f.Channel.ID, 1000)
	pluginTaxOnce.Do(func() {
		h.D.Bus.SubscribeSync(order.EventPlacing, 0, func(_ context.Context, e events.Event) error {
			p, ok := e.(*order.OrderPlacing)
			if _, mine := pluginTaxChannels.Load(p.ChannelID); !ok || !mine {
				return nil
			}
			return p.SetPlacingTaxCents(300)
		})
	})
	pluginTaxChannels.Store(f.Channel.ID, true)

	placed, err := h.TryPlace(t, f.Channel, h.NewGuestCart(t, f, 1))
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	// The plugin's 300 on the goods, core's 50 on the 500 handling fee, nothing on the untaxed eco fee
	if placed.TaxAmount != 350 || placed.TotalAmount != 2000+600+350 || placed.Fees[0].TaxCents != 50 {
		t.Errorf("tax %d total %d fees %+v - want 350, 2950 and 50 on the handling fee", placed.TaxAmount, placed.TotalAmount, placed.Fees)
	}
	h.Settle(t)
}

var (
	pluginTaxOnce     sync.Once
	pluginTaxChannels sync.Map
)

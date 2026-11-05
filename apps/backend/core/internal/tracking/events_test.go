package tracking

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func sampleOrder() order.Order {
	cust, line1, line2 := uuid.New(), uuid.New(), uuid.New()
	phone := "+381601234567"
	return order.Order{
		ID: uuid.New(), Number: "ORD-000042", ChannelID: uuid.New(), CustomerID: &cust, Email: "a@example.com",
		CurrencyCode: "EUR", SubtotalAmount: 5000, DiscountAmount: 500, TaxAmount: 900, ShippingAmount: 490, TotalAmount: 5890,
		BillingAddress: order.OrderAddress{Phone: &phone},
		CreatedAt:      time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		Lines: []order.OrderLine{
			{ID: line1, ProductSku: "SKU-1", ProductName: "Mug", UnitPrice: 1500, Quantity: 2},
			{ID: line2, ProductSku: "SKU-2", ProductName: "Tee", UnitPrice: 2000, Quantity: 1},
		},
		Tracking: &auth.Shopper{
			Consent: auth.Consent{Analytics: true, Ads: true}, AnonID: "visitor-1",
			ClickIDs: map[string]string{"gclid": "g1"}, IP: "203.0.113.7", UserAgent: "Mozilla/5.0",
		},
	}
}

func TestAPurchaseCarriesTheOrderAsGA4AndTheConsentOfThePlacement(t *testing.T) {
	o := sampleOrder()
	ev := PurchaseEvent(o)
	if ev.ID != o.ID.String() || ev.Name != Purchase || ev.Ecommerce.TransactionID != "ORD-000042" || !ev.OccurredAt.Equal(o.CreatedAt) {
		t.Errorf("identity: %+v", ev)
	}
	if e := ev.Ecommerce; e.Currency != "EUR" || e.Value != 45 || e.Tax != 9 || e.Shipping != 4.9 || len(e.Items) != 2 {
		t.Errorf("ecommerce: %+v", e)
	}
	if it := ev.Ecommerce.Items[0]; it.ItemID != "SKU-1" || it.ItemName != "Mug" || it.Price != 15 || it.Quantity != 2 {
		t.Errorf("item: %+v", it)
	}
	if ev.Consent == nil || !ev.Consent.Ads || ev.ClickIDs["gclid"] != "g1" || ev.Context.IP != "203.0.113.7" || ev.User.AnonID != "visitor-1" {
		t.Errorf("shopper: consent %v clicks %v context %+v user %+v", ev.Consent, ev.ClickIDs, ev.Context, ev.User)
	}
	if ev.User.Email != "a@example.com" || ev.User.Phone != "+381601234567" || *ev.User.CustomerID != *o.CustomerID {
		t.Errorf("user: %+v", ev.User)
	}

	o.Tracking = nil // an admin draft - no banner applied
	if PurchaseEvent(o).Consent != nil {
		t.Error("an order no shop placed must carry no consent, so only necessary destinations get it")
	}
}

func TestARefundReportsWhatTheMemoGaveBack(t *testing.T) {
	o := sampleOrder()
	// one of two mugs: 1500 less its 10% discount share - the 4.90 shipping and the tax in TotalCents aren't the purchase's
	memo := order.CreditMemo{ID: uuid.New(), TotalCents: 2110, ShippingRefundCents: 490, Lines: []order.CreditMemoLine{{OrderLineID: o.Lines[0].ID, Quantity: 1}}}
	ev := RefundEvent(o, memo, 0, time.Now())
	if ev.Name != Refund || ev.ID != "refund:"+memo.ID.String() || ev.Ecommerce.Value != 13.5 || ev.Ecommerce.Shipping != 4.9 {
		t.Errorf("refund: %+v %+v", ev, ev.Ecommerce)
	}
	if len(ev.Ecommerce.Items) != 1 || ev.Ecommerce.Items[0].ItemID != "SKU-1" || ev.Ecommerce.Items[0].Quantity != 1 {
		t.Errorf("refund items: %+v", ev.Ecommerce.Items)
	}

	// a full refund after it takes back only what is left, so refunds never pass the purchase's 45
	full := order.CreditMemo{ID: uuid.New(), TotalCents: 5890, ShippingRefundCents: 490}
	if v := RefundEvent(o, full, 1350, time.Now()).Ecommerce.Value; v != 31.5 {
		t.Errorf("full refund after a partial = %v, want 31.5", v)
	}
	if v := RefundEvent(o, full, 0, time.Now()).Ecommerce.Value; v != 45 {
		t.Errorf("a line-less full refund = %v, want the purchase's 45", v)
	}

	if c := CancelEvent(o, 0.5, 0, time.Now()); c.ID != "cancel:"+o.ID.String() || c.Name != Refund || c.Ecommerce.Value != 22.5 {
		t.Errorf("cancel: %+v %+v", c, c.Ecommerce)
	}
	if v := CancelEvent(o, 1, 1350, time.Now()).Ecommerce.Value; v != 31.5 {
		t.Errorf("cancel after a refund = %v, want 31.5 - the refund already took 13.5 back", v)
	}
}

type oneChannel struct{ currency string }

func (c oneChannel) GetByID(_ context.Context, id uuid.UUID) (channel.Channel, error) {
	return channel.Channel{ID: id, CurrencyCode: c.currency}, nil
}

type noOrders struct{}

func (noOrders) GetByID(context.Context, uuid.UUID) (order.Order, error) { return order.Order{}, nil }
func (noOrders) ListCreditMemos(context.Context, uuid.UUID) ([]order.CreditMemo, error) {
	return nil, nil
}

func TestCartChangesAndSignUpsBecomeEvents(t *testing.T) {
	ch := uuid.New()
	registerTestKind(t, Kind{Kind: "t-all"})
	dest := uuid.New()
	fs := newFakeStore(Destination{ID: dest, ChannelID: ch, Kind: "t-all", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	svc.Subscribe(bus, noOrders{}, oneChannel{"EUR"})
	ctx := events.WithEmittedAt(context.Background(), time.Unix(1700000000, 0))
	cust, line := uuid.New(), uuid.New()

	_ = bus.DispatchAsyncNow(ctx, &cart.LineAdded{ChannelID: ch, CustomerID: &cust, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, Quantity: 2})
	_ = bus.DispatchAsyncNow(ctx, &cart.LineAdded{ChannelID: ch, CustomerID: &cust, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, Quantity: 2}) // a replay
	_ = bus.DispatchAsyncNow(ctx, &cart.LineRemoved{ChannelID: ch, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, Quantity: 2})
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerRegistered{CustomerID: cust, ChannelID: ch, Email: "a@example.com", Source: customer.SourceSelf})
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerRegistered{CustomerID: uuid.New(), ChannelID: ch, Source: "import"})

	names := map[string]int{}
	for _, d := range fs.byDestination(dest) {
		names[d.EventName]++
	}
	if names[AddToCart] != 1 || names[RemoveFromCart] != 1 || names[SignUp] != 1 || len(names) != 3 {
		t.Errorf("queued %v, want one add (the replay collapsed), one removal and only the shop's own sign-up", names)
	}
}

package tracking

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
	"github.com/skeletg/skelet/apps/backend/sdk"
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
		Shopper: &auth.Shopper{
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

	o.Shopper = nil // an admin draft - no banner applied
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

type optedIn map[string]bool

func (o optedIn) EmailMarketingAllowed(_ context.Context, _ uuid.UUID, _ *uuid.UUID, email string) (bool, error) {
	return o[email], nil
}

func TestCartChangesAccountsAndLeadsBecomeEvents(t *testing.T) {
	ch := uuid.New()
	registerTestKind(t, sdk.Destination{Kind: "t-all"})
	dest := uuid.New()
	fs := newFakeStore(Destination{ID: dest, ChannelID: ch, Kind: "t-all", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	svc.Subscribe(bus, Readers{Orders: noOrders{}, Channels: oneChannel{"EUR"}, Marketing: optedIn{"a@example.com": true}})
	ctx := events.WithEmittedAt(context.Background(), time.Unix(1700000000, 0))
	cust, line := uuid.New(), uuid.New()

	_ = bus.DispatchAsyncNow(ctx, &cart.LineAdded{ChannelID: ch, CustomerID: &cust, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, Quantity: 2})
	_ = bus.DispatchAsyncNow(ctx, &cart.LineAdded{ChannelID: ch, CustomerID: &cust, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, Quantity: 2}) // a replay
	_ = bus.DispatchAsyncNow(ctx, &cart.LineRemoved{ChannelID: ch, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, Quantity: 2})
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerRegistered{CustomerID: cust, ChannelID: ch, Email: "a@example.com", Source: customer.SourceSelf})
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerRegistered{CustomerID: uuid.New(), ChannelID: ch, Source: "import"})
	_ = bus.DispatchAsyncNow(ctx, &auth.LoginSucceeded{Subject: auth.SubjectCustomer, SubjectID: cust, ChannelID: ch, Email: "a@example.com"})
	_ = bus.DispatchAsyncNow(ctx, &auth.LoginSucceeded{Subject: auth.SubjectAdmin, SubjectID: uuid.New(), ChannelID: ch})
	_ = bus.DispatchAsyncNow(ctx, &signupform.Submitted{ChannelID: ch, FormID: uuid.New(), LeadKey: "k1"})
	_ = bus.DispatchAsyncNow(ctx, &signupform.Submitted{ChannelID: ch, FormID: uuid.New(), LeadKey: "k1"}) // a replay

	names := map[string]int{}
	for _, d := range fs.byDestination(dest) {
		names[d.EventName]++
	}
	if names[AddToCart] != 1 || names[RemoveFromCart] != 1 || names[SignUp] != 1 || names[Login] != 1 || names[GenerateLead] != 1 || len(names) != 5 {
		t.Errorf("queued %v, want one add and one lead (replays collapsed), one removal, only the shop's own sign-up and the customer's sign-in", names)
	}
}

func TestAnEventSaysWhetherItsEmailMayBeMarketedTo(t *testing.T) {
	registerTestKind(t, sdk.Destination{Kind: "t-capture"})
	ch := uuid.New()
	fs := newFakeStore(Destination{ID: uuid.New(), ChannelID: ch, Kind: "t-capture", Enabled: true})
	h := handlers{svc: &Service{store: fs}, Marketing: optedIn{"yes@example.com": true}}
	for _, email := range []string{"yes@example.com", "no@example.com", ""} {
		if err := h.track(context.Background(), Event{ID: "e-" + email, Name: Purchase, ChannelID: ch, User: User{Email: email}}); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	for _, ev := range fs.events {
		got[ev.User.Email] = ev.User.MarketingOptIn
	}
	if !got["yes@example.com"] || got["no@example.com"] || got[""] {
		t.Errorf("marketing opt-in per email: %v", got)
	}
}

// An order with its credit memos, for the remaining-value tests
type memoOrders struct {
	o     order.Order
	memos []order.CreditMemo
}

func (m memoOrders) GetByID(context.Context, uuid.UUID) (order.Order, error) { return m.o, nil }
func (m memoOrders) ListCreditMemos(context.Context, uuid.UUID) ([]order.CreditMemo, error) {
	return m.memos, nil
}

// A platform that restates a conversion needs what the order is still worth, not what one refund gave back
func TestRefundsAndCancellationsSayWhatTheOrderIsStillWorth(t *testing.T) {
	registerTestKind(t, sdk.Destination{Kind: "t-worth"})
	o := sampleOrder() // 50.00 - 5.00 discount + 9.00 tax + 4.90 shipping = 58.90
	first := order.CreditMemo{ID: uuid.New(), OrderID: o.ID, TotalCents: 1000}
	second := order.CreditMemo{ID: uuid.New(), OrderID: o.ID, TotalCents: 2000}
	worth := func(orders memoOrders, e events.Event) float64 {
		t.Helper()
		fs := newFakeStore(Destination{ID: uuid.New(), ChannelID: o.ChannelID, Kind: "t-worth", Enabled: true})
		h := handlers{svc: &Service{store: fs}, Orders: orders}
		var err error
		switch ev := e.(type) {
		case *order.OrderRefunded:
			err = h.onRefunded(context.Background(), ev)
		case *order.OrderCancelled:
			err = h.onCancelled(context.Background(), ev)
		}
		if err != nil || len(fs.events) != 1 || fs.events[0].Ecommerce.ValueAfter == nil {
			t.Fatalf("err %v, events %+v", err, fs.events)
		}
		return *fs.events[0].Ecommerce.ValueAfter
	}
	if got := worth(memoOrders{o, []order.CreditMemo{first}}, &order.OrderRefunded{OrderID: o.ID, CreditMemoID: first.ID}); got != 48.90 {
		t.Errorf("after one 10.00 refund: %v, want 48.90", got)
	}
	if got := worth(memoOrders{o, []order.CreditMemo{first, second}}, &order.OrderRefunded{OrderID: o.ID, CreditMemoID: second.ID}); got != 28.90 {
		t.Errorf("after a second, 20.00 refund: %v, want 28.90 - every memo counts, not just this one", got)
	}
	big := order.CreditMemo{ID: uuid.New(), OrderID: o.ID, TotalCents: 99999}
	if got := worth(memoOrders{o, []order.CreditMemo{big}}, &order.OrderRefunded{OrderID: o.ID, CreditMemoID: big.ID}); got != 0 {
		t.Errorf("a refund past the order's worth: %v, want 0", got)
	}
	if got := worth(memoOrders{o, []order.CreditMemo{first}}, &order.OrderCancelled{OrderID: o.ID, ChannelID: o.ChannelID, RefundableFraction: 0.5}); got != 19.45 {
		t.Errorf("half cancelled after a 10.00 refund: %v, want 58.90 - 10.00 - 29.45 = 19.45", got)
	}
}

type oneCustomer struct{ c customer.Customer }

func (o oneCustomer) GetByID(context.Context, uuid.UUID) (customer.Customer, error) { return o.c, nil }

// Taking someone off a list needs no consent - but only kinds built for removals hear it, never a catch-all webhook
func TestOptOutsReachOnlyTheKindsBuiltToRemove(t *testing.T) {
	registerTestKind(t, sdk.Destination{Kind: "t-remover", Purposes: []Purpose{PurposeAds}, DefaultEvents: []string{Purchase, MarketingOptOut}})
	registerTestKind(t, sdk.Destination{Kind: "t-catchall", Purposes: []Purpose{PurposeAnalytics}})
	ch := uuid.New()
	remover, catchall := uuid.New(), uuid.New()
	fs := newFakeStore(Destination{ID: remover, ChannelID: ch, Kind: "t-remover", Enabled: true}, Destination{ID: catchall, ChannelID: ch, Kind: "t-catchall", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	phone := "+381601234567"
	cust := customer.Customer{ID: uuid.New(), ChannelID: ch, Email: "out@example.com", Phone: &phone}
	svc.Subscribe(bus, Readers{Orders: noOrders{}, Channels: oneChannel{"EUR"}, Customers: oneCustomer{cust}})
	ctx := events.WithEmittedAt(context.Background(), time.Unix(1700000000, 0))

	_ = bus.DispatchAsyncNow(ctx, &consent.ConsentChanged{CustomerID: cust.ID, Kind: consentkind.MarketingEmail, OptedIn: false})
	_ = bus.DispatchAsyncNow(ctx, &consent.ConsentChanged{CustomerID: cust.ID, Kind: consentkind.MarketingEmail, OptedIn: true})
	_ = bus.DispatchAsyncNow(ctx, &consent.ConsentChanged{CustomerID: cust.ID, Kind: "marketing_sms", OptedIn: false})
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerDeleted{CustomerID: uuid.New(), ChannelID: ch, Email: "gone@example.com"})
	_ = bus.DispatchAsyncNow(ctx, &customer.CustomerErased{CustomerID: uuid.New(), ChannelID: ch, Email: "erased@example.com", Phone: "+38160"})

	var emails []string
	for _, ev := range fs.events {
		if ev.Name != MarketingOptOut {
			t.Errorf("unexpected %s", ev.Name)
		}
		emails = append(emails, ev.User.Email)
	}
	if len(fs.byDestination(catchall)) != 0 {
		t.Error("a catch-all destination got an opt-out without the visitor's consent")
	}
	if len(fs.byDestination(remover)) != 3 || emails[0] != "out@example.com" || fs.events[0].User.Phone != phone || emails[1] != "gone@example.com" || emails[2] != "erased@example.com" {
		t.Errorf("opt-outs %v, want the withdrawal, the deletion and the erasure only", emails)
	}
}

// GA4 has no quantity-change event - a raise is an add of the difference, a cut a removal of it
func TestAQuantityChangeIsAnAddOrRemovalOfTheDifference(t *testing.T) {
	ch := uuid.New()
	registerTestKind(t, sdk.Destination{Kind: "t-all"})
	fs := newFakeStore(Destination{ID: uuid.New(), ChannelID: ch, Kind: "t-all", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	svc.Subscribe(bus, Readers{Orders: noOrders{}, Channels: oneChannel{"EUR"}})
	line := uuid.New()
	at := time.Unix(1700000000, 0)
	for i, q := range [][2]float64{{1, 3}, {3, 2}, {2, 2}} {
		ctx := events.WithEmittedAt(context.Background(), at.Add(time.Duration(i)*time.Second))
		_ = bus.DispatchAsyncNow(ctx, &cart.LineUpdated{ChannelID: ch, LineID: line, ProductSku: "SKU-1", UnitPrice: 1500, PrevQuantity: q[0], Quantity: q[1]})
	}
	if len(fs.events) != 2 {
		t.Fatalf("queued %d events, want two (an unchanged quantity is nothing)", len(fs.events))
	}
	got := map[string]Event{}
	for _, ev := range fs.events {
		got[ev.Name] = ev
	}
	if add := got[AddToCart]; add.Ecommerce == nil || add.Ecommerce.Items[0].Quantity != 2 || add.Ecommerce.Value != 30 {
		t.Errorf("1 -> 3 should add 2 worth 30: %+v", add.Ecommerce)
	}
	if rm := got[RemoveFromCart]; rm.Ecommerce == nil || rm.Ecommerce.Items[0].Quantity != 1 || rm.Ecommerce.Value != 15 {
		t.Errorf("3 -> 2 should remove 1 worth 15: %+v", rm.Ecommerce)
	}
}

func TestCheckoutChoicesAreShippingAndPaymentInfo(t *testing.T) {
	ch := uuid.New()
	registerTestKind(t, sdk.Destination{Kind: "t-all"})
	fs := newFakeStore(Destination{ID: uuid.New(), ChannelID: ch, Kind: "t-all", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	svc.Subscribe(bus, Readers{Orders: noOrders{}, Channels: oneChannel{"EUR"}})
	lines := []cart.CartLine{{ProductSku: "SKU-1", ProductName: "Mug", UnitPrice: 1500, Quantity: 2}, {ProductSku: "SKU-2", UnitPrice: 990, Quantity: 1}}
	ctx := events.WithEmittedAt(context.Background(), time.Unix(1700000000, 0))
	_ = bus.DispatchAsyncNow(ctx, &cart.CheckoutChosen{ChannelID: ch, CartID: uuid.New(), Shipping: "dhl:express", Payment: "cod", Lines: lines})
	got := map[string]*Ecommerce{}
	for _, ev := range fs.events {
		got[ev.Name] = ev.Ecommerce
	}
	if ship := got[AddShippingInfo]; ship == nil || ship.ShippingTier != "dhl:express" || ship.Value != 39.9 || len(ship.Items) != 2 || ship.Items[0].ItemName != "Mug" {
		t.Errorf("add_shipping_info: %+v", ship)
	}
	if pay := got[AddPaymentInfo]; pay == nil || pay.PaymentType != "cod" || pay.ShippingTier != "" || pay.Currency != "EUR" {
		t.Errorf("add_payment_info: %+v", pay)
	}
}

type oneProduct struct{ p catalog.Product }

func (o oneProduct) GetProduct(_ context.Context, id uuid.UUID, _ string) (catalog.Product, error) {
	if id != o.p.ID {
		return catalog.Product{}, apperr.New("NOT_FOUND", apperr.ClassNotFound, "no product")
	}
	return o.p, nil
}

func TestAWishlistSaveIsAddToWishlist(t *testing.T) {
	ch := uuid.New()
	registerTestKind(t, sdk.Destination{Kind: "t-all"})
	fs := newFakeStore(Destination{ID: uuid.New(), ChannelID: ch, Kind: "t-all", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	mug := catalog.Product{ID: uuid.New(), SKU: "SKU-1", Content: &catalog.ProductContent{Name: "Mug"}}
	svc.Subscribe(bus, Readers{Orders: noOrders{}, Channels: oneChannel{"EUR"}, Products: oneProduct{mug}})
	cust := uuid.New()
	ctx := context.Background()
	shopper := &auth.Shopper{Consent: auth.Consent{Analytics: true}, AnonID: "visitor-1"}
	_ = bus.DispatchAsyncNow(ctx, &wishlist.ItemAdded{CustomerID: cust, ProductID: mug.ID, ChannelID: ch, Shopper: shopper})
	_ = bus.DispatchAsyncNow(ctx, &wishlist.ItemAdded{CustomerID: cust, ProductID: uuid.New(), ChannelID: ch}) // since deleted
	_ = bus.DispatchAsyncNow(ctx, &wishlist.ItemAdded{CustomerID: cust, ProductID: mug.ID})                    // no shop

	if len(fs.events) != 1 {
		t.Fatalf("queued %d events, want only the save on a shop of a product that exists", len(fs.events))
	}
	ev := fs.events[0]
	if ev.Name != AddToWishlist || *ev.User.CustomerID != cust || ev.User.AnonID != "visitor-1" || ev.Ecommerce.Items[0].ItemID != "SKU-1" || ev.Ecommerce.Items[0].ItemName != "Mug" {
		t.Errorf("add_to_wishlist: %+v %+v", ev, ev.Ecommerce)
	}
}

type redemptions []promotion.OrderRedemption

func (r redemptions) RedemptionsForOrder(context.Context, uuid.UUID) ([]promotion.OrderRedemption, error) {
	return r, nil
}

func TestAPurchaseNamesTheCouponsItWasDiscountedBy(t *testing.T) {
	o := sampleOrder()
	registerTestKind(t, sdk.Destination{Kind: "t-all"})
	fs := newFakeStore(Destination{ID: uuid.New(), ChannelID: o.ChannelID, Kind: "t-all", Enabled: true})
	svc := &Service{store: fs}
	bus := events.NewBus()
	// an automatic rule has no code, and one code redeemed twice is named once
	svc.Subscribe(bus, Readers{Orders: memoOrders{o: o}, Channels: oneChannel{"EUR"}, Coupons: redemptions{{Code: "SUMMER"}, {RuleName: "auto"}, {Code: "VIP"}, {Code: "SUMMER"}}})
	if err := bus.DispatchAsyncNow(context.Background(), &order.OrderPlaced{OrderID: o.ID}); err != nil {
		t.Fatal(err)
	}
	if len(fs.events) != 1 || fs.events[0].Ecommerce.Coupon != "SUMMER,VIP" {
		t.Fatalf("purchase coupon: %+v", fs.events)
	}
}

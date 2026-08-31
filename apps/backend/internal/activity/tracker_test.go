package activity

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
)

type fakeRecorder struct{ calls []analytics.ActivitySpec }

func (f *fakeRecorder) RecordActivity(_ context.Context, a analytics.ActivitySpec) (bool, error) {
	f.calls = append(f.calls, a)
	return true, nil
}

type fakeOrders struct {
	o   order.Order
	err error
}

func (f *fakeOrders) GetByID(_ context.Context, _ uuid.UUID) (order.Order, error) {
	return f.o, f.err
}

func newTracker(o order.Order) (*Tracker, *fakeRecorder) {
	fr := &fakeRecorder{}
	return &Tracker{rec: fr, orders: &fakeOrders{o: o}}, fr
}

func TestTracker_MapsEvents(t *testing.T) {
	cid := uuid.New()
	ch := uuid.New()
	oid := uuid.New()

	cases := []struct {
		name       string
		order      order.Order
		invoke     func(tr *Tracker)
		wantName   string
		wantSource string
		wantDedupe string
	}{
		{
			name:  "order placed",
			order: order.Order{CustomerID: &cid, Number: "1001", ChannelID: ch},
			invoke: func(tr *Tracker) {
				_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{
					OrderID: oid, OrderNumber: "1001", CustomerID: &cid, ChannelID: ch, TotalCents: 5000, Currency: "USD",
				})
			},
			wantName: "order_placed", wantSource: analytics.SourceCommerce, wantDedupe: "act:order_placed:" + oid.String(),
		},
		{
			name:  "order status changed → order_fulfilled (resolved customer)",
			order: order.Order{CustomerID: &cid, Number: "1001", ChannelID: ch},
			invoke: func(tr *Tracker) {
				_ = tr.onOrderStatusChanged(context.Background(), &order.OrderStatusChanged{
					OrderID: oid, ChannelID: ch, FromStatus: "confirmed", ToStatus: "fulfilled",
				})
			},
			wantName: "order_fulfilled", wantSource: analytics.SourceCommerce, wantDedupe: "act:order_status:fulfilled:" + oid.String(),
		},
		{
			name:  "cart add (logged in)",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onCartAdd(context.Background(), &cart.LineAdded{
					CartID: uuid.New(), ChannelID: ch, CustomerID: &cid, LineID: oid, ProductSku: "SKU1", UnitPrice: 1200, Quantity: 2,
				})
			},
			wantName: "cart_add", wantSource: analytics.SourceCommerce, wantDedupe: "act:cart_add:" + oid.String(),
		},
		{
			name:  "review submitted",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onReviewSubmitted(context.Background(), &review.Submitted{
					ReviewID: oid, ProductID: uuid.New(), ChannelID: ch, CustomerID: &cid, Status: review.Status("pending"),
				})
			},
			wantName: "review_submitted", wantSource: analytics.SourceCommerce, wantDedupe: "act:review:" + oid.String(),
		},
		{
			name:  "subscription cancelled",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onSubLifecycle(context.Background(), &subscription.Lifecycle{
					Kind: subscription.EventCanceled, SubscriptionID: oid, CustomerID: &cid, ChannelID: ch,
				})
			},
			wantName: "subscription_cancelled", wantSource: analytics.SourceCommerce, wantDedupe: "act:sub_cancelled:" + oid.String(),
		},
		{
			name:  "customer registered",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onRegistered(context.Background(), &customer.CustomerRegistered{
					CustomerID: cid, ChannelID: ch, Source: "self",
				})
			},
			wantName: "registered", wantSource: analytics.SourceAccount, wantDedupe: "act:registered:" + cid.String(),
		},
		{
			name:  "customer login",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onLogin(context.Background(), &auth.LoginSucceeded{
					Subject: auth.SubjectCustomer, SubjectID: cid, ChannelID: ch, IP: "1.2.3.4",
				})
			},
			wantName: "login", wantSource: analytics.SourceAccount, wantDedupe: "act:login:" + cid.String(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, fr := newTracker(tc.order)
			tc.invoke(tr)
			if len(fr.calls) != 1 {
				t.Fatalf("want 1 recorded activity, got %d", len(fr.calls))
			}
			got := fr.calls[0]
			if got.EventName != tc.wantName {
				t.Errorf("EventName = %q, want %q", got.EventName, tc.wantName)
			}
			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}
			// bucketed keys carry a trailing ":<bucket>", so check prefix
			if want := tc.wantDedupe; got.DedupeKey != want && !hasPrefix(got.DedupeKey, want) {
				t.Errorf("DedupeKey = %q, want %q (or prefix)", got.DedupeKey, want)
			}
			if got.CustomerID == nil {
				t.Errorf("CustomerID is nil; want attributed")
			}
		})
	}
}

func TestTracker_GiftCardOnPlacement(t *testing.T) {
	cid := uuid.New()
	ch := uuid.New()
	oid := uuid.New()
	tr, fr := newTracker(order.Order{CustomerID: &cid, Number: "1001", ChannelID: ch, GiftCardAmount: 500, CurrencyCode: "USD"})
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{
		OrderID: oid, OrderNumber: "1001", CustomerID: &cid, ChannelID: ch, TotalCents: 5000, Currency: "USD",
	})
	if len(fr.calls) != 2 {
		t.Fatalf("want order_placed + giftcard_redeemed = 2 calls, got %d", len(fr.calls))
	}
	if fr.calls[1].EventName != "giftcard_redeemed" {
		t.Errorf("second event = %q, want giftcard_redeemed", fr.calls[1].EventName)
	}
}

func TestTracker_GuestCommerceByAnon(t *testing.T) {
	ch := uuid.New()
	anon := "lens-anon-xyz"

	t.Run("guest cart add records under anon", func(t *testing.T) {
		tr, fr := newTracker(order.Order{})
		_ = tr.onCartAdd(context.Background(), &cart.LineAdded{CartID: uuid.New(), ChannelID: ch, CustomerID: nil, AnonID: anon, LineID: uuid.New(), ProductSku: "SKU1", Quantity: 1})
		if len(fr.calls) != 1 {
			t.Fatalf("guest cart add with anon should record once, got %d", len(fr.calls))
		}
		got := fr.calls[0]
		if got.CustomerID != nil {
			t.Errorf("guest cart add: CustomerID should be nil")
		}
		if got.AnonID != anon {
			t.Errorf("AnonID = %q, want %q", got.AnonID, anon)
		}
	})

	t.Run("guest order placed records under anon", func(t *testing.T) {
		oid := uuid.New()
		tr, fr := newTracker(order.Order{Number: "1099"})
		_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: oid, OrderNumber: "1099", CustomerID: nil, AnonID: anon, ChannelID: ch, TotalCents: 1000, Currency: "USD"})
		if len(fr.calls) < 1 {
			t.Fatalf("guest order placed with anon should record, got %d", len(fr.calls))
		}
		if fr.calls[0].EventName != "order_placed" || fr.calls[0].AnonID != anon || fr.calls[0].CustomerID != nil {
			t.Errorf("guest order_placed mis-recorded: %+v", fr.calls[0])
		}
	})
}

func TestTracker_SkipsUnattributable(t *testing.T) {
	ch := uuid.New()

	t.Run("guest cart add with no anon (no customer, no anon)", func(t *testing.T) {
		tr, fr := newTracker(order.Order{})
		_ = tr.onCartAdd(context.Background(), &cart.LineAdded{CartID: uuid.New(), ChannelID: ch, CustomerID: nil, AnonID: "", LineID: uuid.New()})
		if len(fr.calls) != 0 {
			t.Fatalf("cart add with neither customer nor anon must not record, got %d", len(fr.calls))
		}
	})

	t.Run("guest order status (resolveOrder has no customer)", func(t *testing.T) {
		tr, fr := newTracker(order.Order{CustomerID: nil})
		_ = tr.onOrderStatusChanged(context.Background(), &order.OrderStatusChanged{OrderID: uuid.New(), ToStatus: "cancelled"})
		if len(fr.calls) != 0 {
			t.Fatalf("guest order status must not record, got %d", len(fr.calls))
		}
	})

	t.Run("admin login is skipped", func(t *testing.T) {
		tr, fr := newTracker(order.Order{})
		_ = tr.onLogin(context.Background(), &auth.LoginSucceeded{Subject: auth.SubjectAdmin, SubjectID: uuid.New()})
		if len(fr.calls) != 0 {
			t.Fatalf("admin login must not record, got %d", len(fr.calls))
		}
	})
}

// An outbox redelivery hands the tracker the row's original enqueue time via ctx - occurred_at AND the bucketed dedupe key must match the first delivery exactly, even a midnight later
func TestTracker_OutboxReplayKeepsTimeAndKeyStable(t *testing.T) {
	cid, ch, line := uuid.New(), uuid.New(), uuid.New()
	tr, fr := newTracker(order.Order{})
	enqueued := time.Date(2026, 8, 4, 23, 59, 58, 0, time.UTC)
	ev := &cart.LineUpdated{CartID: uuid.New(), ChannelID: ch, CustomerID: &cid, LineID: line, Quantity: 3}

	_ = tr.onCartUpdate(events.WithEmittedAt(context.Background(), enqueued), ev)
	// the replay crosses midnight relative to enqueued - wall clock must not matter
	_ = tr.onCartUpdate(events.WithEmittedAt(context.Background(), enqueued), ev)

	if len(fr.calls) != 2 {
		t.Fatalf("expected both invocations recorded, got %d", len(fr.calls))
	}
	if fr.calls[0].DedupeKey != fr.calls[1].DedupeKey {
		t.Errorf("dedupe key not stable across outbox replay: %q vs %q", fr.calls[0].DedupeKey, fr.calls[1].DedupeKey)
	}
	for i, c := range fr.calls {
		if !c.At.Equal(enqueued) {
			t.Errorf("call %d At = %v, want the outbox enqueue time %v", i, c.At, enqueued)
		}
	}
	want := "act:cart_upd:" + line.String() + ":" + itoa(enqueued.Unix()/60)
	if fr.calls[0].DedupeKey != want {
		t.Errorf("bucketed key = %q, want %q (minute bucket from the enqueue time)", fr.calls[0].DedupeKey, want)
	}
}

func TestTracker_DedupeKeyStable(t *testing.T) {
	cid := uuid.New()
	oid := uuid.New()
	tr, fr := newTracker(order.Order{CustomerID: &cid, Number: "1001"})
	ev := &order.OrderPlaced{OrderID: oid, OrderNumber: "1001", CustomerID: &cid, ChannelID: uuid.New(), TotalCents: 1, Currency: "USD"}
	_ = tr.onOrderPlaced(context.Background(), ev)
	_ = tr.onOrderPlaced(context.Background(), ev)
	if len(fr.calls) != 2 {
		t.Fatalf("expected both invocations recorded into fake, got %d", len(fr.calls))
	}
	// same logical event should get the same key, so the DB collapses replays
	if fr.calls[0].DedupeKey != fr.calls[1].DedupeKey {
		t.Errorf("dedupe key not stable across replay: %q vs %q", fr.calls[0].DedupeKey, fr.calls[1].DedupeKey)
	}
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

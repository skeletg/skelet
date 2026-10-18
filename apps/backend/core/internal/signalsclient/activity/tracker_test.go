package activity

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type fakeRecorder struct {
	calls []signalsclient.ActivitySpec
	err   error
}

func (f *fakeRecorder) RecordActivity(_ context.Context, a signalsclient.ActivitySpec) error {
	f.calls = append(f.calls, a)
	return f.err
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
			wantName: "order_placed", wantSource: sourceCommerce, wantDedupe: "act:order_placed:" + oid.String(),
		},
		{
			name:  "order status changed → order_fulfilled (resolved customer)",
			order: order.Order{CustomerID: &cid, Number: "1001", ChannelID: ch},
			invoke: func(tr *Tracker) {
				_ = tr.onOrderStatusChanged(context.Background(), &order.OrderStatusChanged{
					OrderID: oid, ChannelID: ch, FromStatus: "confirmed", ToStatus: "fulfilled",
				})
			},
			wantName: "order_fulfilled", wantSource: sourceCommerce, wantDedupe: "act:order_status:fulfilled:" + oid.String(),
		},
		{
			name:  "cart add (logged in)",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onCartAdd(context.Background(), &cart.LineAdded{
					CartID: uuid.New(), ChannelID: ch, CustomerID: &cid, LineID: oid, ProductSku: "SKU1", UnitPrice: 1200, Quantity: 2,
				})
			},
			wantName: "cart_add", wantSource: sourceCommerce, wantDedupe: "act:cart_add:" + oid.String(),
		},
		{
			name:  "review submitted",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onReviewSubmitted(context.Background(), &review.Submitted{
					ReviewID: oid, ProductID: uuid.New(), ChannelID: ch, CustomerID: &cid, Status: review.Status("pending"),
				})
			},
			wantName: "review_submitted", wantSource: sourceCommerce, wantDedupe: "act:review:" + oid.String(),
		},
		{
			name:  "subscription cancelled",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onSubLifecycle(context.Background(), &subscription.Lifecycle{
					Kind: subscription.EventCanceled, SubscriptionID: oid, CustomerID: &cid, ChannelID: ch,
				})
			},
			wantName: "subscription_cancelled", wantSource: sourceCommerce, wantDedupe: "act:sub_cancelled:" + oid.String(),
		},
		{
			name:  "customer registered",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onRegistered(context.Background(), &customer.CustomerRegistered{
					CustomerID: cid, ChannelID: ch, Source: "self",
				})
			},
			wantName: "registered", wantSource: sourceAccount, wantDedupe: "act:registered:" + cid.String(),
		},
		{
			name:  "customer login",
			order: order.Order{},
			invoke: func(tr *Tracker) {
				_ = tr.onLogin(context.Background(), &auth.LoginSucceeded{
					Subject: auth.SubjectCustomer, SubjectID: cid, ChannelID: ch, IP: "1.2.3.4",
				})
			},
			wantName: "login", wantSource: sourceAccount, wantDedupe: "act:login:" + cid.String(),
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

func TestAnErasureLeavesNoAccountEventBehind(t *testing.T) {
	tr, fr := newTracker(order.Order{})
	_ = tr.onAccountUpdated(context.Background(), &customer.CustomerUpdated{
		CustomerID: uuid.New(), ChannelID: uuid.New(), ChangedFields: []string{"email"}, Source: customer.SourceGDPR,
	})
	if len(fr.calls) != 0 {
		t.Fatalf("recorded %d spine events for the erasure's own redaction, want none - it names the erased customer", len(fr.calls))
	}
}

func TestAnImportedProfileChangeIsNotTheShoppersOwnEdit(t *testing.T) {
	for source, want := range map[string]string{customer.SourceImport: "account_updated", customer.SourceSelf: "profile_updated"} {
		tr, fr := newTracker(order.Order{})
		_ = tr.onProfileUpdated(context.Background(), &customer.CustomerProfileUpdated{
			CustomerID: uuid.New(), ChannelID: uuid.New(), ChangedFields: []string{"phone"}, Source: source,
		})
		if len(fr.calls) != 1 || fr.calls[0].EventName != want {
			t.Fatalf("source %q recorded %+v, want one %s", source, fr.calls, want)
		}
	}
}

func TestALogoutLandsOnTheChannelItHappenedOn(t *testing.T) {
	tr, fr := newTracker(order.Order{})
	ch := uuid.New()
	ctx := channel.WithChannel(context.Background(), &channel.Channel{ID: ch})
	_ = tr.onLogout(ctx, &auth.Logout{Subject: auth.SubjectCustomer, SubjectID: uuid.New()})
	if len(fr.calls) != 1 || fr.calls[0].ChannelID != ch {
		t.Fatalf("logout recorded %+v, want it on the storefront's channel - a channel-less event drops out of every per-channel report", fr.calls)
	}
}

func TestAnUnreachableSuiteHandsActivityBackToTheOutbox(t *testing.T) {
	cid := uuid.New()
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"recorded":    {nil, false},
		"unreachable": {status.Error(codes.Unavailable, "down"), true},
		"timed out":   {status.Error(codes.DeadlineExceeded, "slow"), true},
		"refused":     {status.Error(codes.InvalidArgument, "bad"), false},
		"no suite":    {signalsclient.ErrUnavailable, true},
	} {
		t.Run(name, func(t *testing.T) {
			bus := events.NewBus()
			fr := &fakeRecorder{err: tc.err}
			(&Tracker{rec: fr, orders: &fakeOrders{o: order.Order{CustomerID: &cid, GiftCardAmount: 500}}}).Subscribe(bus)
			err := bus.DispatchAsyncNow(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid, ChannelID: uuid.New()})
			if (err != nil) != tc.wantErr {
				t.Fatalf("dispatch err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, tc.err) && status.Code(err) != status.Code(tc.err) {
				t.Errorf("err = %v does not carry the cause", err)
			}
			if tc.wantErr && len(fr.calls) != 1 {
				t.Errorf("recorded %d activities after the suite failed, want the gift-card write left for the replay", len(fr.calls))
			}
		})
	}
}

func TestARepeatedTransitionIsItsOwnEvent(t *testing.T) {
	cid, sub, oid := uuid.New(), uuid.New(), uuid.New()
	first := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	later := first.Add(72 * time.Hour)
	for name, fire := range map[string]func(tr *Tracker, ctx context.Context){
		"subscription paused": func(tr *Tracker, ctx context.Context) {
			_ = tr.onSubLifecycle(ctx, &subscription.Lifecycle{Kind: subscription.EventPaused, SubscriptionID: sub, CustomerID: &cid})
		},
		"order on hold": func(tr *Tracker, ctx context.Context) {
			_ = tr.onOrderStatusChanged(ctx, &order.OrderStatusChanged{OrderID: oid, FromStatus: "processing", ToStatus: "on_hold"})
		},
		"order edited to the same balance": func(tr *Tracker, ctx context.Context) {
			_ = tr.onOrderEdited(ctx, &order.OrderEdited{OrderID: oid, OldAmountDue: 100, NewAmountDue: 100})
		},
	} {
		t.Run(name, func(t *testing.T) {
			tr, fr := newTracker(order.Order{CustomerID: &cid, Number: "1001"})
			fire(tr, events.WithEmittedAt(context.Background(), first))
			fire(tr, events.WithEmittedAt(context.Background(), first))
			fire(tr, events.WithEmittedAt(context.Background(), later))
			if len(fr.calls) != 3 {
				t.Fatalf("recorded %d, want 3", len(fr.calls))
			}
			if fr.calls[0].DedupeKey != fr.calls[1].DedupeKey {
				t.Errorf("a replay changed the key: %q vs %q", fr.calls[0].DedupeKey, fr.calls[1].DedupeKey)
			}
			if fr.calls[0].DedupeKey == fr.calls[2].DedupeKey {
				t.Errorf("a second transition days later reused %q - Postgres keeps one row per key forever, so it would never land", fr.calls[0].DedupeKey)
			}
		})
	}
}

func TestAGuestOrderKeepsItsVisitorPastPlacement(t *testing.T) {
	ch, oid := uuid.New(), uuid.New()
	tr, fr := newTracker(order.Order{Number: "1099", ChannelID: ch, AnonID: "lens-anon-xyz"})
	_ = tr.onOrderShipped(context.Background(), &order.OrderFulfillmentCreated{OrderID: oid, FulfillmentID: uuid.New()})
	_ = tr.onReturnRequested(context.Background(), &order.OrderReturnRequested{OrderID: oid, ReturnID: uuid.New()})
	if len(fr.calls) != 2 {
		t.Fatalf("recorded %d, want the shipment and the return on the guest's profile", len(fr.calls))
	}
	for _, c := range fr.calls {
		if c.AnonID != "lens-anon-xyz" || c.CustomerID != nil || c.ChannelID != ch {
			t.Errorf("%s recorded as %+v, want the visitor the order was placed under", c.EventName, c)
		}
	}
}

func TestAnOrderThatWontLoadGoesBackToTheOutbox(t *testing.T) {
	cid := uuid.New()
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"database down": {errors.New("connection refused"), true},
		"order gone":    {fmt.Errorf("get order: %w", sql.ErrNoRows), false},
	} {
		t.Run(name, func(t *testing.T) {
			fr := &fakeRecorder{}
			tr := &Tracker{rec: fr, orders: &fakeOrders{o: order.Order{CustomerID: &cid}, err: tc.err}}
			err := tr.onOrderShipped(context.Background(), &order.OrderFulfillmentCreated{OrderID: uuid.New(), FulfillmentID: uuid.New()})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
			if len(fr.calls) != 0 {
				t.Errorf("recorded %d activities for an order that didn't load", len(fr.calls))
			}
		})
	}
}

func TestADownloadLandsOnAChannel(t *testing.T) {
	cid, orderCh, reqCh := uuid.New(), uuid.New(), uuid.New()
	onReq := channel.WithChannel(context.Background(), &channel.Channel{ID: reqCh})
	for name, tc := range map[string]struct {
		ctx     context.Context
		orderID *uuid.UUID
		want    uuid.UUID
	}{
		"from its order":             {onReq, ref(uuid.New()), orderCh},
		"from the request, no order": {onReq, nil, reqCh},
		"nowhere, so not recorded":   {context.Background(), nil, uuid.Nil},
	} {
		t.Run(name, func(t *testing.T) {
			tr, fr := newTracker(order.Order{CustomerID: &cid, ChannelID: orderCh, Number: "1001"})
			if err := tr.onDownloadAccessed(tc.ctx, &download.Accessed{GrantID: uuid.New(), CustomerID: &cid, OrderID: tc.orderID, Title: "Manual"}); err != nil {
				t.Fatalf("download: %v", err)
			}
			if tc.want == uuid.Nil {
				if len(fr.calls) != 0 {
					t.Errorf("recorded %+v with no channel to put it on", fr.calls)
				}
				return
			}
			if len(fr.calls) != 1 || fr.calls[0].ChannelID != tc.want {
				t.Fatalf("recorded %+v, want one event on channel %s", fr.calls, tc.want)
			}
		})
	}
}

func TestALogoutCarriesNoAllDevicesFlag(t *testing.T) {
	tr, fr := newTracker(order.Order{})
	_ = tr.onLogout(context.Background(), &auth.Logout{Subject: auth.SubjectCustomer, SubjectID: uuid.New()})
	if len(fr.calls) != 1 {
		t.Fatalf("recorded %d, want 1", len(fr.calls))
	}
	if _, ok := fr.calls[0].Props["all_devices"]; ok {
		t.Error("logout still records all_devices - every Logout is one session, so the flag says nothing")
	}
}

func TestAGiftCardLookupThatFailsGoesBackToTheOutbox(t *testing.T) {
	cid := uuid.New()
	for name, tc := range map[string]struct {
		err     error
		wantErr bool
	}{
		"database down": {errors.New("connection refused"), true},
		"order gone":    {fmt.Errorf("get order: %w", sql.ErrNoRows), false},
	} {
		t.Run(name, func(t *testing.T) {
			tr := &Tracker{rec: &fakeRecorder{}, orders: &fakeOrders{err: tc.err}}
			err := tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid, ChannelID: uuid.New()})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestARefusedWriteIsLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	cid := uuid.New()
	refused := status.Error(codes.PermissionDenied, "bad token")

	tr := &Tracker{rec: &fakeRecorder{err: refused}, orders: &fakeOrders{}}
	if err := tr.onRegistered(context.Background(), &customer.CustomerRegistered{CustomerID: cid, ChannelID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	bus := events.NewBus()
	RegisterConversionAttribution(bus, &fakeConversions{err: refused})
	if err := bus.DispatchAsyncNow(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"refused an activity write", "refused a conversion write"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("log %q does not mention %q", buf.String(), want)
		}
	}
}

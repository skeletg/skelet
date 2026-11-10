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
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/wishlist"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

type fakeRecorder struct {
	calls []signalsclient.ActivitySpec
	err   error
	turns []*signalsv1.AssistantTurnRecord
	ai    []signalsclient.AIActivitySpec
}

func (f *fakeRecorder) RecordAIActivity(_ context.Context, a signalsclient.AIActivitySpec) error {
	f.ai = append(f.ai, a)
	return f.err
}

type fakeProducts map[string][]db.ProductRefsBySKURow

func (f fakeProducts) ProductRefsBySKU(_ context.Context, skus []string) ([]db.ProductRefsBySKURow, error) {
	var out []db.ProductRefsBySKURow
	for _, s := range skus {
		out = append(out, f[s]...)
	}
	return out, nil
}

func (f *fakeRecorder) RecordAssistantTurn(_ context.Context, t *signalsv1.AssistantTurnRecord) error {
	f.turns = append(f.turns, t)
	return f.err
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

// countsNobody is a shop's aggregate_counts setting, the same everywhere
type countsNobody bool

func (c countsNobody) GetBool(context.Context, string, config.Scope) (bool, error) {
	return bool(c), nil
}

func newTracker(o order.Order) (*Tracker, *fakeRecorder) {
	fr := &fakeRecorder{}
	return NewTracker(fr, &fakeOrders{o: o}, countsNobody(true), nil), fr
}

func TestASignInCarriesTheVisitorOnlyWithAnalyticsConsent(t *testing.T) {
	cid, ch := uuid.New(), uuid.New()
	consented := &auth.Shopper{Consent: auth.Consent{Analytics: true}, AnonID: "anon-1"}
	refused := &auth.Shopper{Consent: auth.Consent{}} // ShopperFromContext leaves the visitor id out without consent
	for name, tc := range map[string]struct {
		invoke func(*Tracker)
		want   string
	}{
		"login, consented": {func(tr *Tracker) {
			_ = tr.onLogin(context.Background(), &auth.LoginSucceeded{Subject: auth.SubjectCustomer, SubjectID: cid, ChannelID: ch, Shopper: consented})
		}, "anon-1"},
		"login, refused": {func(tr *Tracker) {
			_ = tr.onLogin(context.Background(), &auth.LoginSucceeded{Subject: auth.SubjectCustomer, SubjectID: cid, ChannelID: ch, Shopper: refused})
		}, ""},
		"login, no banner": {func(tr *Tracker) {
			_ = tr.onLogin(context.Background(), &auth.LoginSucceeded{Subject: auth.SubjectCustomer, SubjectID: cid, ChannelID: ch})
		}, ""},
		"register, consented": {func(tr *Tracker) {
			_ = tr.onRegistered(context.Background(), &customer.CustomerRegistered{CustomerID: cid, ChannelID: ch, Shopper: consented})
		}, "anon-1"},
	} {
		t.Run(name, func(t *testing.T) {
			tr, fr := newTracker(order.Order{})
			tc.invoke(tr)
			if len(fr.calls) != 1 || fr.calls[0].AnonID != tc.want || fr.calls[0].CustomerID == nil {
				t.Fatalf("calls = %+v, want one with visitor %q", fr.calls, tc.want)
			}
		})
	}
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
		tr, fr := newTracker(order.Order{Number: "1099", AnonID: anon}) // the order stores the visitor its event carries
		_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: oid, OrderNumber: "1099", CustomerID: nil, AnonID: anon, ChannelID: ch, TotalCents: 1000, Currency: "USD"})
		if len(fr.calls) < 1 {
			t.Fatalf("guest order placed with anon should record, got %d", len(fr.calls))
		}
		if fr.calls[0].EventName != "order_placed" || fr.calls[0].AnonID != anon || fr.calls[0].CustomerID != nil {
			t.Errorf("guest order_placed mis-recorded: %+v", fr.calls[0])
		}
	})
}

// Admin drafts, headless shops and API clients send no visitor - the order must still land, keyed on the receipt email
func TestAGuestOrderWithoutAVisitorLandsUnderItsEmail(t *testing.T) {
	ch := uuid.New()
	tr, fr := newTracker(order.Order{Number: "1100", ChannelID: ch, Email: "Shopper@Example.com "})
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), OrderNumber: "1100", Email: "shopper@example.com", ChannelID: ch, TotalCents: 1500, Currency: "USD"})
	_ = tr.onOrderStatusChanged(context.Background(), &order.OrderStatusChanged{OrderID: uuid.New(), ToStatus: "cancelled"})
	if len(fr.calls) != 2 {
		t.Fatalf("recorded %d, want the placement and the cancellation", len(fr.calls))
	}
	placed, cancelled := fr.calls[0], fr.calls[1]
	if !strings.HasPrefix(placed.AnonID, "guest_") || len(placed.AnonID) != len("guest_")+32 || placed.CustomerID != nil {
		t.Fatalf("order_placed actor = %q (customer %v), want a guest_ email hash", placed.AnonID, placed.CustomerID)
	}
	if cancelled.AnonID != placed.AnonID {
		t.Errorf("cancellation actor %q != placement actor %q - case and spaces in the email must not split one guest", cancelled.AnonID, placed.AnonID)
	}
	if strings.Contains(placed.AnonID, "example") {
		t.Errorf("actor %q leaks the email", placed.AnonID)
	}
}

// An erasure keeps a guest's orders but scrubs their address - a hash of the scrubbed one would rebuild the erased person as one profile
func TestAnErasedGuestsOrderStaysOffTheSpine(t *testing.T) {
	tr, fr := newTracker(order.Order{Number: "1101", ChannelID: uuid.New(), Email: "redacted-" + uuid.NewString() + customer.RedactedEmailDomain})
	_ = tr.onOrderStatusChanged(context.Background(), &order.OrderStatusChanged{OrderID: uuid.New(), ToStatus: "refunded"})
	if len(fr.calls) != 0 {
		t.Fatalf("recorded %+v for an erased guest's order", fr.calls)
	}
}

// A replayed order must collapse onto the live write - same dedupe key - and sit at the time it was placed, not the time of the replay
func TestABackfilledOrderLandsAtItsPlacementTime(t *testing.T) {
	placed := time.Date(2026, 9, 26, 10, 7, 0, 0, time.UTC)
	// a draft opened the day before it was placed - created_at is the wrong day's partition
	o := order.Order{ID: uuid.New(), Number: "ORD-001005", Email: "guest@example.com", ChannelID: uuid.New(), TotalAmount: 1500, CurrencyCode: "USD", CreatedAt: placed.Add(-24 * time.Hour)}
	tr, fr := newTracker(order.Order{})
	if err := tr.BackfillOrder(context.Background(), o, placed); err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("recorded %d, want the one placement", len(fr.calls))
	}
	got := fr.calls[0]
	if !got.At.Equal(placed) || got.DedupeKey != "act:order_placed:"+o.ID.String() || got.Props["total_cents"] != int64(1500) {
		t.Errorf("backfill wrote %+v, want order_placed at %v keyed on the order id", got, placed)
	}
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
			NewTracker(fr, &fakeOrders{o: order.Order{CustomerID: &cid, GiftCardAmount: 500}}, countsNobody(true), nil).Subscribe(bus)
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
			tr := NewTracker(fr, &fakeOrders{o: order.Order{CustomerID: &cid}, err: tc.err}, countsNobody(true), nil)
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
			tr := NewTracker(&fakeRecorder{}, &fakeOrders{err: tc.err}, countsNobody(true), nil)
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

	tr := NewTracker(&fakeRecorder{err: refused}, &fakeOrders{}, countsNobody(true), nil)
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

func TestAnAssistantTurnLandsOnTheVisitorWithoutItsWords(t *testing.T) {
	tr, fr := newTracker(order.Order{})
	ch, turn := uuid.New(), uuid.New()
	ev := &shoptools.AssistantTurn{ID: turn, ChannelID: ch, AnonID: "visitor-1", Products: 3, CartChanged: true}
	if err := tr.onAssistantTurn(context.Background(), ev); err != nil || len(fr.calls) != 1 {
		t.Fatalf("err %v, %d writes", err, len(fr.calls))
	}
	got := fr.calls[0]
	if got.EventName != "assistant_turn" || got.AnonID != "visitor-1" || got.ChannelID != ch || got.DedupeKey != "act:assistant:"+turn.String() {
		t.Errorf("wrote %+v", got)
	}
	if got.Props["products"] != 3 || got.Props["cart_changed"] != true || got.Props["checkout"] != false {
		t.Errorf("props = %v", got.Props)
	}
	fr.calls = nil
	_ = tr.onAssistantTurn(context.Background(), &shoptools.AssistantTurn{ID: uuid.New(), ChannelID: ch})
	if len(fr.calls) != 0 {
		t.Errorf("a turn nobody can be credited with reached the spine: %+v", fr.calls)
	}
	_ = tr.onAssistantTurn(context.Background(), &shoptools.AssistantTurn{ID: uuid.New(), ChannelID: ch, AnonID: "visitor-1", Shopper: &auth.Shopper{}})
	if len(fr.calls) != 1 || fr.calls[0].AnonID != "" || fr.calls[0].Source != sourceAggregate {
		t.Errorf("a turn from a shopper who refused analytics = %+v - want it counted as nobody", fr.calls)
	}
}

func TestAnAssistedOrderSaysWhoHelped(t *testing.T) {
	cid := uuid.New()
	tr, fr := newTracker(order.Order{CustomerID: &cid})
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid, ChannelID: uuid.New(), AssistedBy: cart.AssistedAgent})
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid, ChannelID: uuid.New()})
	if len(fr.calls) != 3 || fr.calls[0].Props["assisted_by"] != cart.AssistedAgent {
		t.Fatalf("calls = %+v", fr.calls)
	}
	if fr.calls[1].EventName != "assisted_order" || fr.calls[1].Props["assisted_by"] != cart.AssistedAgent {
		t.Errorf("an assisted order's own event = %+v - want one segments can pick", fr.calls[1])
	}
	if _, ok := fr.calls[2].Props["assisted_by"]; ok || fr.calls[2].EventName != "order_placed" {
		t.Errorf("an unassisted order carries assisted_by or its own event: %+v", fr.calls[2])
	}
}

func TestCartsAndOrdersNameTheProductsBehindTheirSKUs(t *testing.T) {
	cid, ch := uuid.New(), uuid.New()
	tee, red := uuid.New(), uuid.New()
	refs := fakeProducts{"TEE-RED": {{Sku: "TEE-RED", ID: red, ParentID: uuid.NullUUID{UUID: tee, Valid: true}}}, "MUG": {{Sku: "MUG", ID: uuid.New()}}}
	fr := &fakeRecorder{}
	tr := &Tracker{rec: fr, orders: &fakeOrders{o: order.Order{CustomerID: &cid, ChannelID: ch, Lines: []order.OrderLine{{ProductSku: "TEE-RED"}, {ProductSku: "MUG"}}}}, products: refs}
	_ = tr.onCartAdd(context.Background(), &cart.LineAdded{ChannelID: ch, CustomerID: &cid, LineID: uuid.New(), ProductSku: "TEE-RED"})
	if ids, _ := fr.calls[0].Props["product_ids"].([]string); len(ids) != 2 || ids[0] != red.String() || ids[1] != tee.String() {
		t.Errorf("cart_add product_ids = %v - want the variant and the product it's sold through", fr.calls[0].Props["product_ids"])
	}
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: uuid.New(), CustomerID: &cid, ChannelID: ch})
	if ids, _ := fr.calls[1].Props["product_ids"].([]string); len(ids) != 3 {
		t.Errorf("order_placed product_ids = %v - want both lines' products and the tee's parent", fr.calls[1].Props["product_ids"])
	}
	_ = tr.onReturnRequested(context.Background(), &order.OrderReturnRequested{OrderID: uuid.New(), ReturnID: uuid.New()})
	if last := fr.calls[len(fr.calls)-1]; last.EventName != "return_requested" || last.Props["order_id"] == "" || last.Props["order_id"] == nil {
		t.Errorf("a return = %+v - want it naming its order", last)
	}
}

func TestTheAssistantsAndAgentsActivityReachSignalsWithoutAPerson(t *testing.T) {
	tr, fr := newTracker(order.Order{})
	ch, shown := uuid.New(), uuid.New()
	turn := &shoptools.AssistantTurn{ID: uuid.New(), ChannelID: ch, AnonID: "visitor-1", ShownProducts: []uuid.UUID{shown},
		Tools: []string{"search_products", "show_products"}, Searches: []shoptools.Search{{Term: "wide boots", Results: 0}}}
	if err := tr.onAssistantTurn(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	if ids, _ := fr.calls[0].Props["product_ids"].([]string); len(ids) != 1 || ids[0] != shown.String() {
		t.Errorf("assistant_turn product_ids = %v", fr.calls[0].Props["product_ids"])
	}
	if len(fr.ai) != 1 || fr.ai[0].Source != signalsapi.AISourceAssistant || len(fr.ai[0].Tools) != 2 || fr.ai[0].Searches[0].Term != "wide boots" || fr.ai[0].DedupeKey != "ai:assistant:"+turn.ID.String() {
		t.Fatalf("AI activity = %+v", fr.ai)
	}
	_ = tr.onAssistantTurn(context.Background(), &shoptools.AssistantTurn{ID: uuid.New(), ChannelID: ch, Tools: []string{"get_cart"}})
	if len(fr.ai) != 2 {
		t.Error("a turn with no visitor to credit lost its tools from the AI counts too")
	}
	_ = tr.onAgentCall(context.Background(), &shoptools.AgentCall{ID: uuid.New(), ChannelID: ch, Agent: "ShopBot", Tool: "search_products", Searches: []shoptools.Search{{Term: "mugs", Results: 4}}})
	if a := fr.ai[2]; a.Source != signalsapi.AISourceAgent || a.Agent != "ShopBot" || a.Tools[0] != "search_products" || a.Searches[0].Results != 4 {
		t.Errorf("agent call = %+v", a)
	}
	if len(fr.calls) != 1 {
		t.Errorf("an agent's call reached the person-level spine: %+v", fr.calls)
	}
}

func TestAKeptTranscriptTravelsWithItsTurn(t *testing.T) {
	tr, fr := newTracker(order.Order{})
	cust, turn, conv := uuid.New(), uuid.New(), uuid.New()
	ev := &shoptools.AssistantTurn{ID: turn, ChannelID: uuid.New(), CustomerID: &cust, AnonID: "visitor-1", Products: 1, CartChanged: true,
		Transcript: &shoptools.Transcript{Conversation: conv, Question: "a mug?", Reply: "Here's one.", Model: "m", Rounds: []shoptools.Round{
			{Thinking: "Search first.", Calls: []shoptools.Call{{Tool: "search_products", Input: []byte(`{"query":"mug"}`), Output: "Big mug"}}},
			{Text: "Here's one."},
		}}}
	if err := tr.onAssistantTurn(context.Background(), ev); err != nil || len(fr.turns) != 1 {
		t.Fatalf("err %v, %d transcripts", err, len(fr.turns))
	}
	got := fr.turns[0]
	if got.GetId() != turn.String() || got.GetConversationId() != conv.String() || got.GetCustomerId() != cust.String() || got.GetAnonId() != "visitor-1" ||
		got.GetQuestion() != "a mug?" || !got.GetCartChanged() || len(got.GetRounds()) != 2 {
		t.Fatalf("record = %+v", got)
	}
	if c := got.GetRounds()[0].GetCalls()[0]; got.GetRounds()[0].GetThinking() != "Search first." || c.GetTool() != "search_products" || c.GetInputJson() != `{"query":"mug"}` || c.GetOutput() != "Big mug" {
		t.Errorf("round = %+v", got.GetRounds()[0])
	}
	fr.turns = nil
	_ = tr.onAssistantTurn(context.Background(), &shoptools.AssistantTurn{ID: uuid.New(), ChannelID: uuid.New(), AnonID: "v", Transcript: nil})
	if len(fr.turns) != 0 {
		t.Error("a turn the store doesn't keep was sent as a transcript")
	}
}

// Refusing analytics on the banner still counts the purchase, but as nobody - not the customer, the visitor or the email
func TestAnOrderPlacedWithoutAnalyticsConsentCountsAsNobody(t *testing.T) {
	cid, ch, oid := uuid.New(), uuid.New(), uuid.New()
	o := order.Order{ID: oid, CustomerID: &cid, Number: "1200", ChannelID: ch, Email: "a@example.com", AnonID: "lens-1", Shopper: &auth.Shopper{}}
	tr, fr := newTracker(o)
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: oid, OrderNumber: "1200", CustomerID: &cid, Email: "a@example.com", AnonID: "lens-1", ChannelID: ch, TotalCents: 900, Currency: "USD"})
	_ = tr.onOrderStatusChanged(context.Background(), &order.OrderStatusChanged{OrderID: oid, ToStatus: "cancelled"})
	if len(fr.calls) != 2 {
		t.Fatalf("recorded %d, want the placement and the cancellation", len(fr.calls))
	}
	for _, c := range fr.calls {
		if c.CustomerID != nil || c.AnonID != "" || c.Source != sourceAggregate {
			t.Errorf("%s recorded as %+v, want nobody", c.EventName, c)
		}
	}
	if fr.calls[0].Props["total_cents"] != int64(900) {
		t.Errorf("order_placed props %v, want the revenue kept", fr.calls[0].Props)
	}

	o.Shopper = &auth.Shopper{Consent: auth.Consent{Analytics: true}}
	tr, fr = newTracker(o)
	_ = tr.onOrderPlaced(context.Background(), &order.OrderPlaced{OrderID: oid, CustomerID: &cid, ChannelID: ch})
	if len(fr.calls) != 1 || fr.calls[0].CustomerID == nil || *fr.calls[0].CustomerID != cid {
		t.Errorf("with analytics consent the order must land on the customer: %+v", fr.calls)
	}
}

// A shopper who refused analytics is still named on their cart events - each counts as nobody instead
func TestCartActivityWithAnalyticsRefusedCountsAsNobody(t *testing.T) {
	cid, ch, line := uuid.New(), uuid.New(), uuid.New()
	tr, fr := newTracker(order.Order{})
	ctx := context.Background()
	refused := &auth.Shopper{}
	_ = tr.onCartAdd(ctx, &cart.LineAdded{CustomerID: &cid, AnonID: "lens-1", ChannelID: ch, LineID: line, ProductSku: "SKU-1", Quantity: 1, Shopper: refused})
	_ = tr.onCartBundleAdd(ctx, &cart.BundleAdded{CustomerID: &cid, ChannelID: ch, LineID: line, Shopper: refused})
	_ = tr.onCartUpdate(ctx, &cart.LineUpdated{CustomerID: &cid, ChannelID: ch, LineID: line, Shopper: refused})
	_ = tr.onCartRemove(ctx, &cart.LineRemoved{CustomerID: &cid, ChannelID: ch, LineID: line, Shopper: refused})
	_ = tr.onCartCleared(ctx, &cart.Cleared{CustomerID: &cid, ChannelID: ch, Shopper: refused})
	_ = tr.onCartMerged(ctx, &cart.Merged{CustomerID: cid, ChannelID: ch, Shopper: refused})
	if len(fr.calls) != 5 {
		t.Fatalf("recorded %d, want the five cart changes and no merge", len(fr.calls))
	}
	for _, c := range fr.calls {
		if c.CustomerID != nil || c.AnonID != "" || c.Source != sourceAggregate {
			t.Errorf("%s recorded as %+v, want nobody", c.EventName, c)
		}
	}
	if fr.calls[0].Props["sku"] != "SKU-1" {
		t.Errorf("the add lost its item: %v", fr.calls[0].Props)
	}

	tr.settings = countsNobody(false)
	_ = tr.onCartAdd(ctx, &cart.LineAdded{CustomerID: &cid, ChannelID: ch, LineID: uuid.New(), Shopper: refused})
	if len(fr.calls) != 5 {
		t.Error("with the shop's aggregate counts off, a refusing shopper's activity must not be recorded at all")
	}
	_ = tr.onCartAdd(ctx, &cart.LineAdded{CustomerID: &cid, ChannelID: ch, LineID: uuid.New()})
	if len(fr.calls) != 6 || fr.calls[5].CustomerID == nil {
		t.Error("cart activity without a refusal must still land on the customer")
	}
}

// A backfill skips an order placed without analytics consent: it already counted - under an order-only id before such
// orders counted as nobody, as nobody since - and the key a replay would get now matches neither
func TestABackfillLeavesOrdersPlacedWithoutConsentAlone(t *testing.T) {
	o := order.Order{ID: uuid.New(), Number: "1300", ChannelID: uuid.New(), TotalAmount: 900, CurrencyCode: "EUR", Shopper: &auth.Shopper{}}
	fr := &fakeRecorder{}
	if err := NewTracker(fr, nil, countsNobody(true), nil).BackfillOrder(context.Background(), o, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("recorded %+v, want nothing - a second copy under a new key", fr.calls)
	}
}

// The ecommerce moments Signals didn't see before, under its own names - and a refused visitor counted as nobody
func TestRefundsWishlistsCheckoutChoicesAndLeadsLand(t *testing.T) {
	cid, ch, oid := uuid.New(), uuid.New(), uuid.New()
	memo, cartID, form := uuid.New(), uuid.New(), uuid.New()
	consented := &auth.Shopper{Consent: auth.Consent{Analytics: true}, AnonID: "visitor-1"}
	refused := &auth.Shopper{}
	tr, fr := newTracker(order.Order{CustomerID: &cid, Number: "1001", ChannelID: ch})
	ctx := context.Background()
	_ = tr.onOrderRefunded(ctx, &order.OrderRefunded{OrderID: oid, ChannelID: ch, CreditMemoID: memo, FullyRefunded: true})
	_ = tr.onWishlistAdd(ctx, &wishlist.ItemAdded{CustomerID: cid, ProductID: uuid.New(), ChannelID: ch, Shopper: consented})
	_ = tr.onWishlistRemove(ctx, &wishlist.ItemRemoved{CustomerID: cid, ProductID: uuid.New(), ChannelID: ch, Shopper: refused})
	_ = tr.onWishlistAdd(ctx, &wishlist.ItemAdded{CustomerID: cid, ProductID: uuid.New()}) // not through a shop
	_ = tr.onCheckoutChosen(ctx, &cart.CheckoutChosen{CartID: cartID, ChannelID: ch, AnonID: "visitor-1", Payment: "cod", Shopper: consented})
	_ = tr.onLead(ctx, &signupform.Submitted{ChannelID: ch, FormID: form, LeadKey: "k1", Shopper: consented})

	if len(fr.calls) != 5 {
		t.Fatalf("recorded %d events, want 5: %+v", len(fr.calls), fr.calls)
	}
	want := []struct {
		name, source, anon string
		cust               bool
	}{
		{"order_refunded", sourceCommerce, "", true},
		{"wishlist_add", sourceCommerce, "visitor-1", true},
		{"wishlist_remove", sourceAggregate, "", false},
		{"checkout_payment", sourceCommerce, "visitor-1", false},
		{"lead", sourceCommerce, "visitor-1", false},
	}
	for i, w := range want {
		c := fr.calls[i]
		if c.EventName != w.name || c.Source != w.source || c.AnonID != w.anon || (c.CustomerID != nil) != w.cust || c.ChannelID != ch {
			t.Errorf("%s: got %+v", w.name, c)
		}
	}
	if fr.calls[3].Props["choice"] != "cod" || fr.calls[0].Props["fully_refunded"] != true {
		t.Errorf("props: %v %v", fr.calls[3].Props, fr.calls[0].Props)
	}
}

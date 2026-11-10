package spinech

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

type aiSeed struct {
	t   *testing.T
	st  *Store
	ch  uuid.UUID
	now time.Time
}

func (s aiSeed) event(name string, daysAgo int, anon string, cust *uuid.UUID, props map[string]any) {
	s.t.Helper()
	if _, err := s.st.AppendEvent(context.Background(), analytics.SpineEvent{
		EventName: name, OccurredAt: s.now.Add(-time.Duration(daysAgo) * 24 * time.Hour), ChannelID: &s.ch, CustomerID: cust,
		AnonID: anon, Source: "commerce", Props: props, DedupeKey: "ai:" + uuid.NewString(),
	}); err != nil {
		s.t.Fatal(err)
	}
}

func order(id string, cents int, currency string, extra map[string]any) map[string]any {
	p := map[string]any{"order_id": id, "total_cents": cents, "currency": currency}
	maps.Copy(p, extra)
	return p
}

func TestChattersAreComparedWithEveryoneElseFromTheirFirstChatOn(t *testing.T) {
	st := testStore(t)
	defer st.Close()
	ctx := context.Background()
	s := aiSeed{t: t, st: st, ch: uuid.New(), now: time.Now().UTC()}
	sfx := uuid.NewString()[:8]
	a, c, d, e := "a-"+sfx, "c-"+sfx, "d-"+sfx, "e-"+sfx
	b := uuid.New()

	s.event("page_view", 5, a, nil, nil)
	s.event("order_placed", 6, a, nil, order("before-chat", 9000, "EUR", nil))
	s.event("assistant_turn", 4, a, nil, nil)
	s.event("order_placed", 3, a, nil, order("o-a", 5000, "EUR", map[string]any{"assisted_by": "assistant"}))
	s.event("return_requested", 1, a, nil, map[string]any{"order_id": "o-a"})
	s.event("assistant_turn", 2, "", &b, nil)
	s.event("page_view", 3, c, nil, nil)
	s.event("order_placed", 2, c, nil, order("o-c1", 3000, "EUR", nil))
	s.event("order_placed", 2, c, nil, order("o-c2", 1000, "USD", nil))
	s.event("page_view", 1, d, nil, nil)
	s.event("order_placed", 1, e, nil, order("o-e", 2000, "EUR", map[string]any{"assisted_by": "agent"}))
	s.event("order_placed", 1, e, nil, order("o-e2", 700, "USD", map[string]any{"assisted_by": "agent"}))
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := st.AIImpact(ctx, s.ch, s.now.AddDate(0, 0, -30), "EUR")
	if err != nil {
		t.Fatal(err)
	}
	if want := (analytics.AICohort{People: 2, Buyers: 1, Orders: 1, RevenueCents: 5000, RevenueOrders: 1, ReturnedOrders: 1}); got.Chatted != want {
		t.Errorf("chatted = %+v, want %+v - the order before the chat isn't the assistant's", got.Chatted, want)
	}
	if want := (analytics.AICohort{People: 2, Buyers: 1, Orders: 2, RevenueCents: 3000, RevenueOrders: 1}); got.DidNotChat != want {
		t.Errorf("didn't chat = %+v, want %+v - revenue in euros only", got.DidNotChat, want)
	}
	if got.AgentOrders != 1 || got.AgentRevenueCents != 2000 {
		t.Errorf("agent orders = %d for %d - want the euro one", got.AgentOrders, got.AgentRevenueCents)
	}
}

func TestWhatTheAssistantShowedIsFollowedToTheCartAndTheOrder(t *testing.T) {
	st := testStore(t)
	defer st.Close()
	ctx := context.Background()
	s := aiSeed{t: t, st: st, ch: uuid.New(), now: time.Now().UTC()}
	sfx := uuid.NewString()[:8]
	a := "a-" + sfx
	b := uuid.New()
	p1, p2, variant := uuid.NewString(), uuid.NewString(), uuid.NewString()

	s.event("assistant_turn", 4, a, nil, map[string]any{"product_ids": []string{p1, p2}})
	s.event("cart_add", 3, a, nil, map[string]any{"product_ids": []string{variant, p1}})
	s.event("order_placed", 3, a, nil, order("o-a", 5000, "EUR", map[string]any{"product_ids": []string{variant, p1}}))
	s.event("cart_add", 3, "", &b, map[string]any{"product_ids": []string{p1}})
	s.event("assistant_turn", 2, "", &b, map[string]any{"product_ids": []string{p1}})
	if err := st.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := st.AIProducts(ctx, s.ch, s.now.AddDate(0, 0, -30), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("products = %+v - want the two shown", got)
	}
	if want := (analytics.AIProductStat{ProductID: p1, Shown: 2, People: 2, Carted: 1, Ordered: 1}); got[0] != want {
		t.Errorf("most shown = %+v, want %+v - a cart from before the showing doesn't count", got[0], want)
	}
	if want := (analytics.AIProductStat{ProductID: p2, Shown: 1, People: 1}); got[1] != want {
		t.Errorf("the other = %+v, want %+v", got[1], want)
	}
}

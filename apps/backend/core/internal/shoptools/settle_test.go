package shoptools

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/aiusage"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

type fakeUsage struct {
	mu        sync.Mutex
	turns     []aiusage.Turn
	spent     int64
	overviews []ai.Usage
	written   int
}

func (f *fakeUsage) RecordOverview(_ context.Context, _ uuid.UUID, used ai.Usage, written bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.overviews = append(f.overviews, used)
	if written {
		f.written++
	}
	f.spent += used.Input + used.Output
	return nil
}

func (f *fakeUsage) TokensToday(context.Context, uuid.UUID) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.spent, nil
}

func (f *fakeUsage) RecordTurn(_ context.Context, ch uuid.UUID, t aiusage.Turn) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ch != shop.ID {
		return errors.New("wrong channel")
	}
	f.turns = append(f.turns, t)
	return nil
}

type fakeBus struct{ published []events.Event }

func (f *fakeBus) Publish(_ context.Context, e events.Event) { f.published = append(f.published, e) }

func settledRig(model *fakeModel, carts *fakeCarts) (*Service, *fakeUsage, *fakeBus) {
	s := assistantRig(model, carts)
	usage, bus := &fakeUsage{}, &fakeBus{}
	s.deps.Usage, s.deps.Bus = usage, bus
	return s, usage, bus
}

func shopperCtx() context.Context {
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set(auth.AnonHeader, "visitor-1")
	return auth.WithRequest(context.Background(), req)
}

func TestATurnThatFillsTheCartIsCountedAndCredited(t *testing.T) {
	carts := &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}
	own := webCart(carts, shop.ID, cart.CartLine{ID: uuid.New(), ProductSku: "TEE", Quantity: 1})
	used := ai.Usage{Input: 120, Output: 40, CacheRead: 900, CacheWrite: 300}
	s, usage, bus := settledRig(&fakeModel{calls: []ai.ToolCall{addMugs, toolCall("start_checkout", map[string]any{})}, used: used}, carts)
	turn, err := s.Assist(shopperCtx(), shop, guest(&own), ask, "", Notify{})
	if err != nil || turn.CartID == nil {
		t.Fatalf("turn = %+v, %v", turn, err)
	}
	if want := []aiusage.Turn{{CartChanged: true, Checkout: true, Usage: used}}; !slices.Equal(usage.turns, want) {
		t.Errorf("recorded %+v, want %+v", usage.turns, want)
	}
	if got := carts.assisted[*turn.CartID]; got != cart.AssistedAssistant {
		t.Errorf("the filled cart is marked %q, want %q", got, cart.AssistedAssistant)
	}
	if len(bus.published) != 1 {
		t.Fatalf("published %d events, want one", len(bus.published))
	}
	ev, ok := bus.published[0].(*AssistantTurn)
	if !ok || ev.AnonID != "visitor-1" || ev.ChannelID != shop.ID || !ev.CartChanged || !ev.Checkout || ev.Incomplete {
		t.Errorf("event = %+v, want the visitor's turn that filled and checked out the cart", bus.published[0])
	}
}

func TestATurnThatOnlyTalksLeavesTheCartUncredited(t *testing.T) {
	carts := &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}
	own := webCart(carts, shop.ID)
	s, usage, _ := settledRig(&fakeModel{calls: []ai.ToolCall{toolCall("get_cart", map[string]any{})}}, carts)
	if _, err := s.Assist(context.Background(), shop, guest(&own), ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	if len(carts.assisted) != 0 {
		t.Errorf("a cart the turn only read got marked: %v", carts.assisted)
	}
	if len(usage.turns) != 1 || usage.turns[0].CartChanged {
		t.Errorf("recorded %+v, want one turn that left the cart alone", usage.turns)
	}
}

func TestAFailedTurnStillCountsWhatItSpent(t *testing.T) {
	used := ai.Usage{Input: 50, Output: 10}
	s, usage, bus := settledRig(&fakeModel{err: errors.New("529 overloaded"), used: used}, nil)
	if _, err := s.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); !errors.Is(err, ErrAssistantUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if want := []aiusage.Turn{{Failed: true, Unanswered: true, Usage: used}}; !slices.Equal(usage.turns, want) {
		t.Errorf("recorded %+v, want %+v", usage.turns, want)
	}
	if ev, ok := bus.published[0].(*AssistantTurn); !ok || !ev.Incomplete {
		t.Errorf("event = %+v, want an incomplete turn", bus.published[0])
	}
	s, usage, _ = settledRig(&fakeModel{}, nil)
	s.deps.Pace = denyAll{}
	if _, err := s.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); !errors.Is(err, ErrAssistantLimited) || len(usage.turns) != 0 {
		t.Errorf("a turn refused before the model ran was counted: %v, %+v", err, usage.turns)
	}
}

func TestATurnTheStoreCouldntAnswerIsCountedSo(t *testing.T) {
	for name, model := range map[string]*fakeModel{
		"the model said it didn't know": {reply: "The store's pages don't say.\n[[unanswered]]"},
		"nothing said or shown":         {silent: true},
	} {
		s, usage, _ := settledRig(model, nil)
		if _, err := s.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); err != nil && !errors.Is(err, ErrAssistantUnavailable) {
			t.Fatalf("%s: %v", name, err)
		}
		if len(usage.turns) != 1 || !usage.turns[0].Unanswered || usage.turns[0].Failed {
			t.Errorf("%s: recorded %+v", name, usage.turns)
		}
	}
	s, usage, _ := settledRig(&fakeModel{reply: "Here are two mugs."}, nil)
	if _, err := s.Assist(context.Background(), shop, guest(nil), ask, "", Notify{}); err != nil || len(usage.turns) != 1 || usage.turns[0].Unanswered {
		t.Errorf("an answered turn: %v, %+v", err, usage.turns)
	}
}

func TestStepsNameWhatTheAssistantIsLookingAt(t *testing.T) {
	calls := []ai.ToolCall{
		toolCall("search_products", map[string]any{"query": "  waterproof\njacket "}),
		toolCall("get_product", map[string]any{"product": "MUG"}),
		toolCall("estimate_shipping", map[string]any{"country": "de"}),
		toolCall("show_product_details", map[string]any{"id": lookedAt["MUG"].String(), "summary": "ok"}),
	}
	var steps []Step
	s, _, _ := settledRig(&fakeModel{calls: calls}, nil)
	if _, err := s.Assist(context.Background(), shop, guest(nil), ask, "", Notify{Step: func(st Step) { steps = append(steps, st) }}); err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{Tool: "search_products", Detail: "waterproof jacket"},
		{Tool: "get_product", Detail: "Rain Shell"},
		{Tool: "estimate_shipping", Detail: "DE"},
		{Tool: "show_product_details", Detail: "Rain Shell"},
	}
	if !slices.Equal(steps, want) {
		t.Errorf("steps = %+v, want %+v", steps, want)
	}
}

func TestATurnThatOnlyEmptiesTheCartTakesNoCredit(t *testing.T) {
	carts := &fakeCarts{carts: map[uuid.UUID]cart.Cart{}}
	own := webCart(carts, shop.ID, cart.CartLine{ID: uuid.New(), ProductSku: "TEE", Quantity: 1})
	takeOut := toolCall("update_cart", map[string]any{"lines": []map[string]any{{"sku": "TEE", "quantity": 0}}})
	s, usage, _ := settledRig(&fakeModel{calls: []ai.ToolCall{takeOut}}, carts)
	if _, err := s.Assist(shopperCtx(), shop, guest(&own), ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	if len(carts.assisted) != 0 {
		t.Errorf("a turn that only took the tee out got the cart's later order credited: %v", carts.assisted)
	}
	if len(usage.turns) != 1 || !usage.turns[0].CartChanged {
		t.Errorf("recorded %+v - want the turn counted as changing the cart", usage.turns)
	}
}

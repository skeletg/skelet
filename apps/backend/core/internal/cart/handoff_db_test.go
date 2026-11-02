package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func handoffRig(t *testing.T) dbRig {
	return newDBRig(t, catalogStub{prices: map[string]SimplePricing{
		"tee": {ProductName: "Tee", UnitPrice: 2500},
		"mug": {ProductName: "Mug", UnitPrice: 1000},
	}})
}

func (r dbRig) agentCart(t *testing.T, channelID uuid.UUID, lines map[string]float64) Cart {
	t.Helper()
	ctx := context.Background()
	c, err := r.svc.CreateAgentCart(ctx, channelID)
	if err != nil {
		t.Fatal(err)
	}
	r.dropCart(t, c.ID)
	for sku, qty := range lines {
		if _, err := r.svc.AddToCart(ctx, c.ID, nil, sku, qty); err != nil {
			t.Fatalf("add %s: %v", sku, err)
		}
	}
	return c
}

func (r dbRig) dropCart(t *testing.T, id uuid.UUID) {
	t.Cleanup(func() { _, _ = r.conn.Exec(`DELETE FROM carts WHERE id = $1`, id) })
}

func (r dbRig) handOff(t *testing.T, agent Cart) string {
	t.Helper()
	c, err := r.svc.StartHandoff(context.Background(), agent.ID, agent.ChannelID)
	if err != nil {
		t.Fatalf("start handoff: %v", err)
	}
	if c.Handoff == nil || c.Status != StatusHandedOff {
		t.Fatalf("started handoff = %+v, want a handed-off cart with a link", c)
	}
	return c.Handoff.Token
}

func (r dbRig) redeem(t *testing.T, token string, customerID, into *uuid.UUID) Cart {
	t.Helper()
	c, err := r.svc.RedeemHandoff(context.Background(), token, r.channel, customerID, into)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	r.dropCart(t, c.ID)
	return c
}

func TestAHandoffMovesTheAgentCartIntoAFreshCartAndClosesIt(t *testing.T) {
	r := handoffRig(t)
	ctx := context.Background()
	agent := r.agentCart(t, r.channel, map[string]float64{"tee": 2})
	token := r.handOff(t, agent)

	if _, err := r.svc.AgentCart(ctx, agent.ID, r.channel); !errors.Is(err, ErrCartNotActive) {
		t.Fatalf("agent read after handoff = %v, want ErrCartNotActive", err)
	}
	if _, err := r.svc.AddToCart(ctx, agent.ID, nil, "mug", 1); !errors.Is(err, ErrCartNotActive) {
		t.Fatalf("add to a handed-off cart = %v, want ErrCartNotActive", err)
	}

	got := r.redeem(t, token, nil, nil)
	if got.ID == agent.ID || got.Source != SourceWeb || got.Status != "active" {
		t.Fatalf("redeemed into %+v, want a new active web cart", got)
	}
	if l := lineBySku(t, got, "tee"); l.Quantity != 2 || len(got.Lines) != 1 {
		t.Fatalf("redeemed lines = %+v, want 2 tees", got.Lines)
	}

	again := r.redeem(t, token, nil, &got.ID)
	if again.ID != got.ID || lineBySku(t, again, "tee").Quantity != 2 {
		t.Fatalf("second redeem = %+v, want the same cart still holding 2 tees", again)
	}
}

func TestAHandoffLandsInTheShoppersOwnCart(t *testing.T) {
	r := handoffRig(t)
	ctx := context.Background()
	mine := r.guestCart(t)
	if _, err := r.svc.AddToCart(ctx, mine.ID, nil, "mug", 1); err != nil {
		t.Fatal(err)
	}
	got := r.redeem(t, r.handOff(t, r.agentCart(t, r.channel, map[string]float64{"tee": 2, "mug": 1})), nil, &mine.ID)
	if got.ID != mine.ID || lineBySku(t, got, "tee").Quantity != 2 || lineBySku(t, got, "mug").Quantity != 2 {
		t.Fatalf("redeemed into %+v, want the shopper's cart with 2 tees and 2 mugs", got)
	}

	customerID := r.customer(t)
	own, err := r.svc.GetOrCreateCart(ctx, customerID, r.channel)
	if err != nil {
		t.Fatal(err)
	}
	r.dropCart(t, own.ID)
	got = r.redeem(t, r.handOff(t, r.agentCart(t, r.channel, map[string]float64{"tee": 1})), &customerID, &mine.ID)
	if got.ID != own.ID || lineBySku(t, got, "tee").Quantity != 1 {
		t.Fatalf("signed-in redeem landed in %+v, want the customer's cart %s", got, own.ID)
	}
}

func TestAHandoffNeverLandsInACartItShouldnt(t *testing.T) {
	r := handoffRig(t)
	ctx := context.Background()
	other := seedAbandonChannel(t, r.conn)
	elsewhere, err := r.svc.CreateGuestCart(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	r.dropCart(t, elsewhere.ID)
	agentTarget := r.agentCart(t, r.channel, map[string]float64{"mug": 1})
	closed := r.guestCart(t)
	if err := r.svc.repo.SetCartMerged(ctx, closed.ID); err != nil {
		t.Fatal(err)
	}

	for name, into := range map[string]uuid.UUID{"another store's cart": elsewhere.ID, "another agent cart": agentTarget.ID, "a closed cart": closed.ID, "a missing cart": uuid.New()} {
		t.Run(name, func(t *testing.T) {
			got := r.redeem(t, r.handOff(t, r.agentCart(t, r.channel, map[string]float64{"tee": 1})), nil, &into)
			if got.ID == into || got.Source != SourceWeb {
				t.Fatalf("redeemed into %+v, want a fresh web cart", got)
			}
		})
	}
}

func TestStartingAHandoffTwiceGivesTheSameLink(t *testing.T) {
	r := handoffRig(t)
	ctx := context.Background()
	agent := r.agentCart(t, r.channel, map[string]float64{"tee": 1})
	first := r.handOff(t, agent)
	if second := r.handOff(t, agent); second != first {
		t.Fatalf("second start minted %q, want the first link %q", second, first)
	}

	if _, err := r.svc.StartHandoff(ctx, r.agentCart(t, r.channel, nil).ID, r.channel); !errors.Is(err, ErrHandoffEmpty) {
		t.Errorf("empty cart = %v, want ErrHandoffEmpty", err)
	}
	if _, err := r.svc.StartHandoff(ctx, r.guestCart(t).ID, r.channel); !errors.Is(err, ErrCartForbidden) {
		t.Errorf("a shopper's cart = %v, want ErrCartForbidden", err)
	}
	if _, err := r.svc.StartHandoff(ctx, r.agentCart(t, r.channel, map[string]float64{"tee": 1}).ID, seedAbandonChannel(t, r.conn)); !errors.Is(err, ErrCartForbidden) {
		t.Errorf("another store = %v, want ErrCartForbidden", err)
	}
	if _, err := r.svc.AgentCart(ctx, r.guestCart(t).ID, r.channel); !errors.Is(err, ErrCartForbidden) {
		t.Errorf("agent read of a shopper's cart = %v, want ErrCartForbidden", err)
	}
}

func TestABadHandoffLinkOpensNothing(t *testing.T) {
	r := handoffRig(t)
	ctx := context.Background()
	token := r.handOff(t, r.agentCart(t, r.channel, map[string]float64{"tee": 1}))

	cases := map[string]struct {
		token   string
		channel uuid.UUID
		want    error
	}{
		"malformed":     {"not-a-token", r.channel, ErrHandoffInvalid},
		"unknown":       {"AAAAAAAAAAAAAAAAAAAAAA", r.channel, ErrHandoffInvalid},
		"another store": {token, seedAbandonChannel(t, r.conn), ErrHandoffInvalid},
	}
	for name, tc := range cases {
		if _, err := r.svc.RedeemHandoff(ctx, tc.token, tc.channel, nil, nil); !errors.Is(err, tc.want) {
			t.Errorf("%s = %v, want %v", name, err, tc.want)
		}
	}

	if _, err := r.conn.Exec(`UPDATE carts SET handoff_expires_at = now() - interval '1 minute' WHERE handoff_token = $1`, token); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.RedeemHandoff(ctx, token, r.channel, nil, nil); !errors.Is(err, ErrHandoffExpired) {
		t.Errorf("expired = %v, want ErrHandoffExpired", err)
	}
}

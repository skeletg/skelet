package shoptools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// traceTurn opens the turn's trace. What the shopper wrote, who they are and the conversation it belongs to
// go only when the store keeps the conversation - the same consent as its transcript - and the rest as numbers alone
func (s *Service) traceTurn(ctx context.Context, c *conversation, who Shopper, question string) context.Context {
	kept := c.rec != nil
	ctx = aitrace.WithContent(ctx, kept)
	ctx, c.trace = aitrace.Start(ctx, "Shopping assistant", aitrace.Agent)
	var session, person string
	if kept {
		session = c.rec.Conversation.String()
		if anon := auth.AnonID(ctx); anon != "" {
			person = "visitor:" + anon
		}
		if who.Customer != nil {
			person = "customer:" + who.Customer.String()
		}
	}
	c.trace.Trace("Shopping assistant", session, person, "storefront")
	c.trace.Meta("store", c.ch.Name)
	c.trace.Meta("channel_id", c.ch.ID.String())
	c.trace.Meta("kept", kept)
	if kept && c.page != "" {
		c.trace.Meta("page", c.page)
	}
	c.trace.Input(question)
	return ctx
}

// Outcome names, as scores on the turn's trace
const (
	scoreUnanswered = "unanswered"
	scoreFilledCart = "filled_cart"
	scoreOrdered    = "led_to_order"
)

// promptVersion fingerprints what the model is given before the shopper says anything - the prompt with the store's
// voice and the tools - so the traces can set the answers of one version beside another's
func promptVersion(system string, tools []byte) string {
	h := sha256.New()
	h.Write([]byte(system))
	h.Write(tools)
	return "assistant-" + hex.EncodeToString(h.Sum(nil)[:4])
}

func toolsDigest(tools []ai.Tool) []byte {
	h := sha256.New()
	for _, t := range tools {
		schema, _ := json.Marshal(t.Schema)
		h.Write([]byte(t.Name + "\x00" + t.Description + "\x00"))
		h.Write(schema)
	}
	return h.Sum(nil)
}

// the tools never change while the process runs, so they're hashed once
var assistantToolsDigest = sync.OnceValue(func() []byte { return toolsDigest(assistantTools) })

// SubscribeToOrders scores the turn that filled a cart once an order is placed from it with the assistant's credit
func SubscribeToOrders(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventPlaced, onOrderPlaced)
}

func onOrderPlaced(ctx context.Context, e events.Event) error {
	if placed, ok := e.(*order.OrderPlaced); ok && placed.AssistTrace != "" {
		aitrace.Score(ctx, placed.AssistTrace, aitrace.Outcome{Name: scoreOrdered, Value: true, Comment: "Order " + placed.OrderNumber})
	}
	return nil
}

func (c *conversation) endTrace(ctx context.Context, turn Turn, failed, unanswered bool) {
	t := c.trace
	if t == nil {
		return
	}
	aitrace.Score(ctx, t.TraceID(), aitrace.Outcome{Name: scoreUnanswered, Value: unanswered}, aitrace.Outcome{Name: scoreFilledCart, Value: c.added})
	t.Output(turn.Reply)
	t.Meta("products_shown", len(c.shown))
	t.Meta("cart_changed", c.changed)
	t.Meta("checkout", c.checkout)
	t.Meta("incomplete", failed)
	t.Meta("unanswered", unanswered)
	t.End()
}

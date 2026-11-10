package shoptools

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/aiusage"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

const EventAssistantTurn = "assistant.turn"

type AssistantTurn struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	CustomerID  *uuid.UUID
	AnonID      string
	Shopper     *auth.Shopper // the shopper's consent as they asked, so a turn from one who refused analytics counts as nobody
	Products    int
	CartChanged bool
	Checkout    bool
	Incomplete  bool
	// Couldn't answer from the store's information: every search came back empty, or the model said it didn't know
	FoundNothing bool
	Unsure       bool
	Transcript   *Transcript // only when the store keeps them
	// What the turn showed the shopper and did, for the AI analytics - ids, tool names and search words, never the chat
	ShownProducts []uuid.UUID
	Tools         []string
	Searches      []Search
}

func (*AssistantTurn) Type() string                { return EventAssistantTurn }
func (e *AssistantTurn) EventChannelID() uuid.UUID { return e.ChannelID }

type Step struct {
	Tool   string
	Detail string
}

const maxStepDetail = 80

const maxShownLogged = 24

func (s *Service) settle(ctx context.Context, ch channel.Channel, c *conversation, used ai.Usage, turn Turn) {
	failed := turn.Incomplete
	ctx = context.WithoutCancel(ctx)
	// The same test the conversations' unanswered filter applies, so the counts agree
	unanswered := failed || c.foundNothing() || c.unsure || (turn.Reply == "" && turn.Recap == "")
	c.endTrace(ctx, turn, failed, unanswered)
	if c.added && c.cartID != nil {
		if err := s.deps.Carts.MarkAssisted(ctx, *c.cartID, cart.AssistedAssistant, c.trace.TraceID()); err != nil {
			slog.ErrorContext(ctx, "shoptools: marking the assisted cart failed", "cart", *c.cartID, "err", err)
		}
	}
	if s.deps.Usage != nil {
		t := aiusage.Turn{Failed: failed, Unanswered: unanswered, CartChanged: c.changed, Checkout: c.checkout, Usage: used}
		if err := s.deps.Usage.RecordTurn(ctx, ch.ID, t); err != nil {
			slog.ErrorContext(ctx, "shoptools: recording assistant usage failed", "channel", ch.ID, "err", err)
		}
	}
	ev := &AssistantTurn{ID: uuid.New(), ChannelID: ch.ID, CustomerID: c.customer, AnonID: auth.AnonID(ctx), Shopper: auth.ShopperFromContext(ctx),
		Products: len(c.shown), CartChanged: c.changed, Checkout: c.checkout, Incomplete: failed, FoundNothing: c.foundNothing(), Unsure: c.unsure,
		ShownProducts: c.shown[:min(len(c.shown), maxShownLogged)], Tools: c.tools, Searches: c.searchLog.list()}
	if c.rec != nil {
		ev.Transcript = c.rec.settle(turn, used, c.spoke)
	}
	if err := s.emit(ctx, ev); err != nil {
		slog.ErrorContext(ctx, "shoptools: queueing the assistant turn failed", "channel", ch.ID, "err", err)
	}
}

// get_product names its product once it's found instead
func (c *conversation) announce(tc ai.ToolCall) {
	if c.notify.Step == nil || tc.Name == "get_product" {
		return
	}
	var a struct {
		Query   string   `json:"query"`
		ID      string   `json:"id"`
		IDs     []string `json:"ids"`
		Country string   `json:"country"`
		Order   string   `json:"order"`
	}
	_ = json.Unmarshal(tc.Input, &a)
	if a.ID == "" && len(a.IDs) == 1 {
		a.ID = a.IDs[0]
	}
	detail := cmp.Or(strings.TrimSpace(a.Query), strings.TrimPrefix(strings.TrimSpace(a.Order), "#"))
	if id, err := uuid.Parse(strings.TrimSpace(a.ID)); err == nil {
		detail = c.names[id]
	} else if tc.Name == "find_similar_products" && strings.ContainsRune(a.ID, ' ') {
		detail = clip(a.ID, 80)
	}
	if tc.Name == "estimate_shipping" {
		detail = strings.ToUpper(strings.TrimSpace(a.Country))
	}
	c.tell(tc.Name, detail)
}

func (c *conversation) tell(tool, detail string) {
	if c.notify.Step != nil {
		c.notify.Step(Step{Tool: tool, Detail: clip(detail, maxStepDetail)})
	}
}

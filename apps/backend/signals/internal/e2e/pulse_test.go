package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
)

// Pulse from the suite's side: the mail a flow delivers leaves this process, so this is where its delivery can be seen
// Core's scenarios cover the hand-off, that its own mail stays quiet when a flow takes the moment

// A shop on the channel roster the way core pushes it, sender identity and all
func (h *harness) shop(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, _, err := h.d.Channels.SyncChannels(h.ctx, []analytics.ChannelEntry{{
		ID: id, IsActive: true, Name: "E2E Shop", PublicURL: "https://shop.e2e.test", FromAddress: "hello@e2e.test", FromName: "E2E Shop",
	}}); err != nil {
		t.Fatalf("push channel: %v", err)
	}
	return id
}

// A customer on the roster, opted in to marketing mail
func (h *harness) shopper(t *testing.T, channelID uuid.UUID, optedIn bool) (uuid.UUID, string) {
	t.Helper()
	id := uuid.New()
	addr := "shopper-" + id.String()[:8] + "@e2e.test"
	if err := h.d.Queries.UpsertCustomerRosterEntry(h.ctx, signalsdb.UpsertCustomerRosterEntryParams{
		CustomerID: id, ChannelID: channelID, Email: addr, FirstName: "Ada", IsActive: true, MarketingOptedIn: optedIn,
		Consents: json.RawMessage("{}")}); err != nil {
		t.Fatalf("push customer: %v", err)
	}
	return id, addr
}

func (h *harness) sendEmailFlow(t *testing.T, channelID uuid.UUID, trigger, subject string, transactional bool) flow.Flow {
	t.Helper()
	e, err := h.d.Builder.CreateLibraryEmail(h.ctx, emailtemplate.LibraryEmail{
		ChannelID: channelID, Name: "E2E " + trigger, Subject: subject,
		Blocks: []emailtemplate.Block{{Type: "heading", Text: "Hello {{customer.firstName}}"}, {Type: "text", Content: "From {{store.name}}"}},
	})
	if err != nil {
		t.Fatalf("save the email: %v", err)
	}
	tc, _ := json.Marshal(flow.TriggerConfig{Transactional: transactional})
	f, err := h.d.Flow.Create(h.ctx, flow.Input{
		ChannelID: channelID, Name: "E2E " + trigger, TriggerType: trigger, TriggerConfig: tc,
		Graph: flow.Graph{
			Nodes: []flow.Node{
				{ID: "trigger", Type: flow.NodeTrigger},
				{ID: "email", Type: flow.NodeSendEmail, Data: flow.NodeData{EmailID: e.ID.String()}},
			},
			Edges: []flow.Edge{{ID: "trigger-email", Source: "trigger", Target: "email"}},
		},
	})
	if err != nil {
		t.Fatalf("create the flow: %v", err)
	}
	if f, err = h.d.Flow.SetStatus(h.ctx, f.ID, flow.StatusActive); err != nil {
		t.Fatalf("activate the flow: %v", err)
	}
	return f
}

// The order receipt a guest gets from a flow: enrolled by the address alone, rendered from the
// snapshot the enrollment carried, sent as the shop the roster describes
func TestAFlowDeliversAGuestsReceiptFromTheSnapshot(t *testing.T) {
	h := newHarness(t)
	ch := h.shop(t)
	f := h.sendEmailFlow(t, ch, signalsapi.TriggerOrderPlaced, "Order {{order.number}} confirmed", true)
	addr := "guest-" + uuid.NewString()[:8] + "@e2e.test"
	payload, _ := json.Marshal(map[string]any{
		signalsapi.PayloadOrderID: uuid.NewString(), signalsapi.PayloadOrderNumber: "ORD-E2E-1",
		signalsapi.PayloadOrder: email.OrderSnapshot{Number: "ORD-E2E-1", FirstName: "Grace", Currency: "USD", Total: 2500},
	})

	if n := h.d.Flow.Enroll(h.ctx, signalsapi.TriggerOrderPlaced, ch, uuid.Nil, addr, payload, ""); n != 1 {
		t.Fatalf("enrolled into %d flows, want the one active order_placed flow", n)
	}
	if err := h.d.Flow.ProcessRuns(h.ctx); err != nil {
		t.Fatalf("advance: %v", err)
	}

	got := h.mail.to(addr)
	if len(got) != 1 {
		t.Fatalf("mail delivered = %d, want the one receipt", len(got))
	}
	if got[0].Subject != "Order ORD-E2E-1 confirmed" {
		t.Errorf("subject = %q, want the order number rendered in", got[0].Subject)
	}
	if !strings.Contains(got[0].From, "hello@e2e.test") || !strings.Contains(got[0].From, "E2E Shop") {
		t.Errorf("From = %q, want the roster's sender identity", got[0].From)
	}
	if !strings.Contains(got[0].HTMLBody, "Hello Grace") {
		t.Error("the snapshot's first name did not reach the body")
	}
	runs, err := h.d.Flow.ListRuns(h.ctx, f.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].Status != flow.RunCompleted || runs[0].Email != addr {
		t.Errorf("runs = %+v (%v), want one completed run keyed by the guest's address", runs, err)
	}
}

// A marketing flow reads consent off the roster at send time, and an opted-out customer is not mailed
func TestAMarketingFlowHonoursTheRosterConsent(t *testing.T) {
	h := newHarness(t)
	ch := h.shop(t)
	h.sendEmailFlow(t, ch, signalsapi.TriggerCustomerRegistered, "Welcome to {{store.name}}", false)
	in, inAddr := h.shopper(t, ch, true)
	out, outAddr := h.shopper(t, ch, false)

	for _, id := range []uuid.UUID{in, out} {
		if n := h.d.Flow.Enroll(h.ctx, signalsapi.TriggerCustomerRegistered, ch, id, "", nil, ""); n != 1 {
			t.Fatalf("enrolled %s into %d flows, want 1", id, n)
		}
	}
	if err := h.d.Flow.ProcessRuns(h.ctx); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if got := h.mail.to(inAddr); len(got) != 1 || got[0].Subject != "Welcome to E2E Shop" {
		t.Errorf("opted-in shopper got %d mail(s), want the welcome with the shop's name", len(got))
	}
	if got := h.mail.to(outAddr); len(got) != 0 {
		t.Errorf("opted-out shopper got %d mail(s), want none", len(got))
	}
}

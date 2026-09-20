package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/e2ekit"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// orderFlow gives a channel one active order_placed flow that sends a library email - built over the
// contract, since flows and the emails they send are the suite's
func (h *harness) orderFlow(t *testing.T, channelID uuid.UUID) uuid.UUID {
	t.Helper()
	emailID := h.CreateLibraryEmail(t, channelID, "Receipt", "Order {{order.number}} confirmed",
		[]map[string]any{{"type": "heading", "text": "Thanks for your order"}})
	return h.CreateFlow(t, channelID, "Order placed", signalsapi.TriggerOrderPlaced, "active",
		map[string]any{"transactional": true}, e2ekit.SendEmailGraph(emailID))
}

// Scenario: the receipt goes out even though the store has no order_placed flow
// A channel whose flow is paused, deleted or never seeded would otherwise send no receipts at all
func TestOrderConfirmationSendsWithoutAFlow(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	h.Mail.Reset()
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	got := h.Mail.To(o.Email)
	if len(got) != 1 {
		t.Fatalf("mail delivered to the buyer = %d, want exactly one receipt (subjects: %v)", len(got), h.Mail.SubjectsTo(o.Email))
	}
	if got[0].From == "" {
		t.Error("the receipt carries no From header, so a relay would refuse it")
	}
}

// Scenario: an active flow owns the receipt, and core must not send a second one beside it
// The flow's own mail leaves the Signals process, so what core can see is the enrollment taken and its own sender quiet
func TestOrderConfirmationDoesNotDoubleSendWithAFlow(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	flowID := h.orderFlow(t, f.Channel.ID)

	h.Mail.Reset()
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.AdvanceFlows(t, flowID) // enrollment only creates the run - the send happens when the engine advances it

	if got := len(h.Mail.To(o.Email)); got != 0 {
		t.Errorf("core delivered %d mail(s) to the buyer beside the flow, want none (subjects: %v)", got, h.Mail.SubjectsTo(o.Email))
	}
	runs := h.FlowRuns(t, flowID)
	if len(runs) != 1 || runs[0].GetStatus() != "completed" {
		t.Fatalf("flow runs = %d, want the one order enrolled and its run completed", len(runs))
	}
	if runs[0].GetEmail() != o.Email {
		t.Errorf("the run is keyed by %q, want the guest's order email %q", runs[0].GetEmail(), o.Email)
	}
}

// Scenario: a paused verification flow must not swallow the account mail it was going to deliver
func TestDirectAccountMailStillReachesTheCustomer(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	emailID := h.CreateLibraryEmail(t, f.Channel.ID, "Verify", "Verify your address",
		[]map[string]any{{"type": "button", "label": "Verify", "url": "{{verify.link}}"}})
	h.CreateFlow(t, f.Channel.ID, "Email verification", signalsapi.TriggerEmailVerification, "paused",
		map[string]any{"transactional": true}, e2ekit.SendEmailGraph(emailID))

	addr := "direct-" + uuid.NewString()[:8] + "@e2e.test"
	h.Mail.Reset()
	if _, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, addr, "password123", "", "", nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.Settle(t)

	got := h.Mail.To(addr)
	if len(got) != 1 {
		t.Fatalf("verification mail delivered = %d, want 1 - a paused flow must not swallow it (subjects: %v)", len(got), h.Mail.SubjectsTo(addr))
	}
	if got[0].From == "" {
		t.Error("the verification mail carries no From header, so it will be refused")
	}
}

// Scenario: an active flow owns delivery and the operator resends the receipt anyway, which has to send directly rather than re-enroll into the run that already took the placement
func TestResendOrderConfirmationBypassesTheFlow(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	flowID := h.orderFlow(t, f.Channel.ID)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	if h.RunnableFlowRuns(t, flowID) == 0 {
		t.Fatal("no flow run was enrolled for this order, so there is no flow to bypass")
	}

	h.Mail.Reset()
	if err := h.D.OrderMail.Resend(h.Ctx, o.ID, email.KeyOrderConfirmation, nil); err != nil {
		t.Fatalf("resend the receipt: %v", err)
	}
	h.Settle(t)

	if got := len(h.Mail.To(o.Email)); got != 1 {
		t.Errorf("mail delivered by the resend = %d, want exactly one (subjects: %v)", got, h.Mail.SubjectsTo(o.Email))
	}
	// Still enrolled and untouched - a resend that re-enrolled would show up as a second run
	if got := h.RunnableFlowRuns(t, flowID); got != 1 {
		t.Errorf("runnable flow runs = %d after the resend, want the single original one left alone", got)
	}
}

// A shipment notice belongs to one fulfillment, so an order with none has nothing to resend
func TestResendShipmentNeedsAFulfillment(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	err := h.D.OrderMail.Resend(h.Ctx, o.ID, email.KeyOrderShipment, nil)
	if err == nil {
		t.Fatal("resent a shipment notice for an unfulfilled order, want an error")
	}
}

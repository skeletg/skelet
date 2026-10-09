package e2e

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/form"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// Scenario: a shop with nothing built in Pulse still emails its customers - core renders from templates compiled into it
func TestCoreSendsItsTransactionalMailWithoutTheBuilder(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.Settle(t)

	h.Mail.Reset()
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	got := h.Mail.To(o.Email)
	if len(got) != 1 {
		t.Fatalf("mail delivered = %d, want the receipt (subjects: %v)", len(got), h.Mail.SubjectsTo(o.Email))
	}
	msg := got[0]
	if !strings.Contains(msg.Subject, o.Number) {
		t.Errorf("subject = %q, want the order number in it", msg.Subject)
	}
	if msg.From == "" {
		t.Error("no From header, so the mail will be refused")
	}
	if !strings.Contains(msg.HTMLBody, o.Number) || msg.TextBody == "" {
		t.Error("the receipt rendered without the order number or without a text part")
	}

	// The same holds for the rest of the lifecycle
	h.Mail.Reset()
	if _, err := h.D.Order.UpdateStatus(h.Ctx, o.ID, "cancelled"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	h.Settle(t)
	if n := len(h.Mail.To(o.Email)); n != 1 {
		t.Errorf("cancellation mail = %d, want 1 (subjects: %v)", n, h.Mail.SubjectsTo(o.Email))
	}
}

// An operator resending a receipt must not need the builder either - the keys core resends by are
// its own now, not the builder's slot names
func TestResendUsesCoresOwnMessageKeys(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	h.Mail.Reset()
	if err := h.D.OrderMail.Resend(h.Ctx, o.ID, email.KeyOrderConfirmation, nil); err != nil {
		t.Fatalf("resend: %v", err)
	}
	h.Settle(t)
	if n := len(h.Mail.To(o.Email)); n != 1 {
		t.Errorf("resent mail = %d, want exactly one", n)
	}

	if err := h.D.OrderMail.Resend(h.Ctx, o.ID, "not_a_message", nil); err == nil {
		t.Error("an unknown message key was accepted for resend")
	}
}

// Account mail has no flow and no library behind it either
func TestAccountMailRendersFromCore(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	addr := "core-mail-" + uuid.NewString()[:8] + "@e2e.test"
	h.Mail.Reset()
	if _, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, addr, "password123", "Ada", "L", nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.Settle(t)

	got := h.Mail.To(addr)
	if len(got) != 1 {
		t.Fatalf("verification mail = %d, want 1 (subjects: %v)", len(got), h.Mail.SubjectsTo(addr))
	}
	if got[0].From == "" || got[0].TextBody == "" {
		t.Error("the verification mail rendered without a From header or a text part")
	}
}

// A channel created with no Pulse content gets no seeded emails and no seeded flows, and still mails
// its customers - core seeds nothing because it renders from templates compiled into it
func TestANewChannelNeedsNoSeedingToSendMail(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2500)
	h.Settle(t)

	if emails, flows := h.CountLibraryEmails(t, f.Channel.ID), h.CountFlows(t, f.Channel.ID); emails != 0 || flows != 0 {
		t.Errorf("a new channel was seeded with %d emails and %d flows, want none - core does not own that content", emails, flows)
	}

	h.Mail.Reset()
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	if n := len(h.Mail.To(o.Email)); n != 1 {
		t.Errorf("mail delivered = %d, want the receipt (subjects: %v)", n, h.Mail.SubjectsTo(o.Email))
	}
}

// Scenario: starter content is asked for, asking twice adds nothing, and the seeded flow takes the receipt
func TestStarterContentSeedsOnceAndTakesTheReceipt(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	blocks, emails, flows := h.SeedStarterContent(t, f.Channel.ID)
	if blocks == 0 || emails != 8 || flows != 8 {
		t.Fatalf("seeded %d blocks, %d emails, %d flows - want some blocks and one email and one flow per transactional slot", blocks, emails, flows)
	}
	if b, e, fl := h.SeedStarterContent(t, f.Channel.ID); b+e+fl != 0 {
		t.Errorf("a second seeding added %d blocks, %d emails, %d flows - want nothing, the channel had it all", b, e, fl)
	}
	if got := h.CountLibraryEmails(t, f.Channel.ID); got != emails {
		t.Errorf("library holds %d emails after two seedings, want %d", got, emails)
	}

	flowID := h.FlowIDByTrigger(t, f.Channel.ID, signalsapi.TriggerOrderPlaced)
	h.Mail.Reset()
	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	h.AdvanceFlows(t, flowID)

	if got := len(h.Mail.To(o.Email)); got != 0 {
		t.Errorf("core delivered %d mail(s) beside the seeded flow, want none (subjects: %v)", got, h.Mail.SubjectsTo(o.Email))
	}
	runs := h.FlowRuns(t, flowID)
	if len(runs) != 1 || runs[0].GetStatus() != "completed" {
		t.Fatalf("seeded order_placed flow has %d runs, want the one order enrolled and completed", len(runs))
	}
}

func TestTheSeededFlowTakesTheSignupConfirmation(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)
	h.SeedStarterContent(t, f.Channel.ID)
	flowID := h.FlowIDByTrigger(t, f.Channel.ID, signalsapi.TriggerSignupConfirmation)

	created, err := h.D.Form.Create(h.Ctx, form.SaveInput{ChannelID: f.Channel.ID, Slug: "e2e-seeded-" + uuid.NewString()[:8], Name: "News", Status: "active"})
	if err != nil {
		t.Fatalf("create form: %v", err)
	}
	addr := "seeded-" + uuid.NewString()[:8] + "@e2e.test"
	h.Mail.Reset()
	if _, err := h.D.Form.Submit(h.Ctx, f.Channel.ID, created.Slug, addr, "Ana"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got := len(h.Mail.To(addr)); got != 0 {
		t.Errorf("core delivered %d mail(s) beside the seeded flow, want none (subjects: %v)", got, h.Mail.SubjectsTo(addr))
	}
	runs := h.FlowRuns(t, flowID)
	if len(runs) != 1 || runs[0].GetStatus() != "completed" {
		t.Fatalf("seeded signup_confirmation flow has %d runs, want the one guest run completed in the call", len(runs))
	}
}

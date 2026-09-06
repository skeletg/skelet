package e2e

import (
	"testing"

	"github.com/google/uuid"
)

// sendsBySource returns how many rows the send log holds for an order, per sender
func (h *harness) sendsBySource(t *testing.T, orderID uuid.UUID) map[string]int {
	t.Helper()
	rows, err := h.D.DB.Query(`SELECT source, count(*) FROM email_sends WHERE order_id = $1 GROUP BY source`, orderID)
	if err != nil {
		t.Fatalf("count sends: %v", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var src string
		var n int
		if err := rows.Scan(&src, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[src] = n
	}
	return out
}

// Scenario: the receipt goes out even though the store has no order_placed flow.
// Delivery used to belong entirely to the flow engine, so a channel whose flow was paused, deleted
// or never seeded silently sent no receipts, with no admin resend to recover them
func TestOrderConfirmationSendsWithoutAFlow(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	// Whatever the channel was seeded with, the operator has turned it off
	if _, err := h.D.DB.Exec(
		`UPDATE flows SET status = 'paused' WHERE channel_id = $1 AND trigger_type = 'order_placed'`,
		f.Channel.ID); err != nil {
		t.Fatalf("pause the order_placed flow: %v", err)
	}

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)

	sends := h.sendsBySource(t, o.ID)
	if sends["transactional"] != 1 {
		t.Errorf("direct sends = %d, want 1 - the receipt has to go out with no flow to deliver it (all sends: %v)", sends["transactional"], sends)
	}
	if sends["flow"] != 0 {
		t.Errorf("flow sends = %d for a paused flow, want 0", sends["flow"])
	}
}

// Scenario: an active flow owns delivery, and nothing sends the receipt twice.
// A duplicate receipt reads to a customer as a duplicate order, which is worse than a late one
func TestOrderConfirmationDoesNotDoubleSendWithAFlow(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	var status string
	if err := h.D.DB.QueryRow(
		`SELECT status FROM flows WHERE channel_id = $1 AND trigger_type = 'order_placed' AND deleted_at IS NULL`,
		f.Channel.ID).Scan(&status); err != nil {
		t.Fatalf("the seeded order_placed flow is missing, so this proves nothing: %v", err)
	}
	if status != "active" {
		t.Fatalf("seeded order_placed flow is %q, want active", status)
	}

	o := h.PlaceGuestCOD(t, f, 1)
	h.Settle(t)
	// Enrollment only creates the run - the send happens when the scheduled job advances it
	if err := h.D.Flow.ProcessRuns(h.Ctx); err != nil {
		t.Fatalf("advance flow runs: %v", err)
	}
	h.Settle(t)

	sends := h.sendsBySource(t, o.ID)
	if sends["transactional"] != 0 {
		t.Errorf("direct sends = %d with an active flow, want 0 - the customer would get two receipts (all sends: %v)", sends["transactional"], sends)
	}
	if sends["flow"] == 0 {
		t.Error("the active flow sent nothing, so the fallback should have covered it")
	}
}

// Scenario: account mail sent without a flow is still visible in reporting
func TestDirectAccountMailIsRecorded(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 5, 2500)
	h.Settle(t)

	if _, err := h.D.DB.Exec(
		`UPDATE flows SET status = 'paused' WHERE channel_id = $1 AND trigger_type = 'email_verification'`,
		f.Channel.ID); err != nil {
		t.Fatalf("pause the email_verification flow: %v", err)
	}

	addr := "direct-" + uuid.NewString()[:8] + "@e2e.test"
	if _, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, addr, "password123", "", "", nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.Settle(t)

	var n int
	if err := h.D.DB.QueryRow(
		`SELECT count(*) FROM email_sends WHERE email = $1 AND source = 'transactional' AND message_key = $2`,
		addr, "customer_verification").Scan(&n); err != nil {
		t.Fatalf("count sends: %v", err)
	}
	if n != 1 {
		t.Errorf("recorded direct verification sends = %d, want 1 - the mail went out but reporting can't see it", n)
	}
}

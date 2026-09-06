package e2e

import (
	"context"
	"testing"
	"time"
)

// Scenario: a new store sends its own receipts and account mail.
// Seeding used to be a manual `skelet emails seed-transactional`, so a channel created through the
// admin API had no delivery flows and silently sent nothing - the storefront still said "check your email"
func TestNewChannelSeedsTransactionalEmailsAndFlows(t *testing.T) {
	h := newHarness(t)
	s := h.NewStore(t, 1, 1000)

	ctx, cancel := context.WithTimeout(h.Ctx, 30*time.Second)
	defer cancel()
	if err := h.D.Bus.WaitIdle(ctx); err != nil { // seeding rides the async channel.created handler
		t.Fatalf("waiting for the channel.created handler: %v", err)
	}

	var emails int
	if err := h.D.DB.QueryRow(`SELECT count(*) FROM email_library WHERE channel_id = $1 AND slot <> ''`, s.Channel.ID).Scan(&emails); err != nil {
		t.Fatalf("count slot emails: %v", err)
	}
	if emails == 0 {
		t.Fatal("the new channel has no slot emails, so every seeded flow would have been skipped")
	}

	// The two that lock a customer out of their account when they don't send
	for _, trigger := range []string{"email_verification", "password_reset", "order_placed"} {
		var status string
		err := h.D.DB.QueryRow(
			`SELECT status FROM flows WHERE channel_id = $1 AND trigger_type = $2 AND deleted_at IS NULL`,
			s.Channel.ID, trigger).Scan(&status)
		if err != nil {
			t.Errorf("no %s flow seeded for the new channel: %v", trigger, err)
			continue
		}
		if status != "active" {
			t.Errorf("%s flow seeded as %q, want active - a draft delivers nothing", trigger, status)
		}
	}
}

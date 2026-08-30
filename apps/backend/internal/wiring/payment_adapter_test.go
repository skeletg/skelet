package wiring

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/order"
)

// This seam once dropped every card token on the floor (gateways authorized with nothing) - these
// assert the typed slots survive the order->payment mapping byte for byte
func TestAuthorizeRequestCarriesTheTokens(t *testing.T) {
	oid, chid, cid := uuid.New(), uuid.New(), uuid.New()
	for name, tc := range map[string]struct {
		in          order.PaymentAuthorizeInput
		wantToken   string
		wantVaulted string
	}{
		"one-shot client token": {
			in:        order.PaymentAuthorizeInput{Token: "tok_abc123"},
			wantToken: "tok_abc123",
		},
		"vaulted saved method": {
			in:          order.PaymentAuthorizeInput{VaultedToken: "pm_saved456"},
			wantVaulted: "pm_saved456",
		},
		"intent-based provider sends neither": {
			in: order.PaymentAuthorizeInput{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			tc.in.OrderID, tc.in.ChannelID, tc.in.CustomerID = oid, chid, &cid
			tc.in.AmountCents, tc.in.CurrencyCode, tc.in.Email = 12345, "EUR", "a@b.c"

			got := authorizeRequestFrom(tc.in)
			if got.PaymentToken != tc.wantToken {
				t.Errorf("PaymentToken = %q, want %q", got.PaymentToken, tc.wantToken)
			}
			if got.VaultedToken != tc.wantVaulted {
				t.Errorf("VaultedToken = %q, want %q", got.VaultedToken, tc.wantVaulted)
			}
			if got.Metadata != nil {
				t.Errorf("no metadata should be fabricated, got %v", got.Metadata)
			}
			if got.OrderID != oid || got.ChannelID != chid || got.AmountCents != 12345 || got.CurrencyCode != "EUR" || got.Email != "a@b.c" || got.CustomerID != &cid {
				t.Error("a non-token field was dropped in the mapping")
			}
		})
	}
}

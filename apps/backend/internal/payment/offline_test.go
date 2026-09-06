package payment

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// Boot-critical: these are the engine's only built-in way to take an order, so a drifted slug or a lost IsOffline means checkout offers nothing or tries to authorize a method with no gateway behind it
func TestOfflineMethodsRegistered(t *testing.T) {
	want := map[string]string{
		"cod":            "Cash on Delivery",
		"bank-transfer":  "Bank Transfer / Invoice",
		"pay-on-pickup":  "Pay on Pickup",
		"purchase-order": "Purchase Order",
	}
	for slug, displayName := range want {
		reg, err := GetProvider(slug)
		if err != nil {
			t.Errorf("offline method %q is not registered: %v", slug, err)
			continue
		}
		if !reg.IsOffline {
			t.Errorf("%q is not marked IsOffline - the placement bridge would authorize it through a gateway", slug)
		}
		if reg.DisplayName != displayName {
			t.Errorf("%q display name: got %q, want %q", slug, reg.DisplayName, displayName)
		}
		if reg.Provider == nil {
			t.Errorf("%q registered with a nil provider", slug)
		}
	}
}

// A reachable gateway call means something upstream lost track of IsOffline, and a zero value would mark an unpaid order settled
func TestOfflineProviderRefusesGatewayCalls(t *testing.T) {
	p := &offlineProvider{slug: "cod"}
	ctx := context.Background()

	if _, err := p.Authorize(ctx, AuthorizeRequest{}); err == nil {
		t.Error("Authorize returned nil error")
	}
	if _, err := p.Capture(ctx, CaptureRequest{}); err == nil {
		t.Error("Capture returned nil error")
	}
	if _, err := p.Refund(ctx, RefundRequest{}); err == nil {
		t.Error("Refund returned nil error")
	}
	if _, err := p.Void(ctx, VoidRequest{}); err == nil {
		t.Error("Void returned nil error")
	}
	if _, err := p.HandleWebhook(ctx, (*http.Request)(nil)); err == nil {
		t.Error("HandleWebhook returned nil error")
	}

	_, err := p.Capture(ctx, CaptureRequest{})
	if !strings.Contains(err.Error(), "cod") { // operator-facing - "no gateway capture" alone doesn't say which method
		t.Errorf("error should name the method, got: %v", err)
	}
}

// The storefront renders this under the selected method, so a missing default is a blank panel on a live store
func TestOfflineInstructionsRegistered(t *testing.T) {
	for _, m := range offlineMethods {
		path := "payment/" + m.slug + "/instructions"
		def, ok := config.Find(path)
		if !ok {
			t.Errorf("config field %q not registered", path)
			continue
		}
		if def.Default == "" {
			t.Errorf("config field %q has no default", path)
		}
	}
}

// config.Validate runs at boot and aborts on a definition under an undeclared section or group, so registering the offline groups from init() has to satisfy it
func TestOfflineConfigRegistrationValidates(t *testing.T) {
	if err := config.Validate(); err != nil {
		t.Fatalf("config registry is structurally invalid, boot would abort: %v", err)
	}
}

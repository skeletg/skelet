package tracking

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A kind whose secret is required, like GA4's API secret
func secretKind(t *testing.T, name string, send func(context.Context, *http.Client, sdk.DestinationConfig, Event) error) {
	t.Helper()
	registerTestKind(t, sdk.Destination{Kind: name, Label: name, Send: send, Fields: []sdk.DestinationField{
		{Key: "account", Label: "Account", Required: true},
		{Key: "api_secret", Label: "API secret", Required: true, Secret: true},
	}})
}

// A destination saved without the secret its platform needs stays off until the secret is entered
func TestADestinationWaitsForItsSecret(t *testing.T) {
	secretKind(t, "t-secret", nil)
	fs := newFakeStore()
	svc := &Service{store: fs}
	ctx := context.Background()
	d := Destination{ID: uuid.New(), ChannelID: uuid.New(), Kind: "t-secret", Name: "GA4", Config: map[string]string{"account": "G-1"}}
	fs.dests[d.ID] = d
	if !NeedsSecret(d) {
		t.Error("a destination of a kind with a required secret should need it")
	}
	d.Enabled = true
	if _, err := svc.Update(ctx, d); !errors.Is(err, ErrNeedsSecret) {
		t.Errorf("switched on without its secret: %v", err)
	}
	d.Secret = "now-entered"
	if on, err := svc.Update(ctx, d); err != nil || !on.Enabled {
		t.Errorf("with the secret entered it should switch on: %+v %v", on, err)
	}
}

// The platform's answer reaches an admin screen, so a URL quoting the secret must not
func TestATestSendNeverShowsTheSecret(t *testing.T) {
	secretKind(t, "t-leaky", func(context.Context, *http.Client, sdk.DestinationConfig, Event) error {
		return errors.New(`Post "https://collect.example/mp?api_secret=s3cr3t-value&id=G-1": dial tcp: refused`)
	})
	d := Destination{ID: uuid.New(), Kind: "t-leaky", Name: "x", Secret: "s3cr3t-value", Config: map[string]string{"account": "G-1"}}
	svc := &Service{store: newFakeStore(d)}
	_, err := svc.SendTest(context.Background(), http.DefaultClient, d.ID)
	if err == nil || strings.Contains(err.Error(), "s3cr3t-value") || !strings.Contains(err.Error(), "[redacted]") {
		t.Errorf("test send error %v", err)
	}
	if got := redact("u?api_secret=a%2Fb%2Bc", &Destination{Secret: "a/b+c"}); strings.Contains(got, "a%2Fb") {
		t.Errorf("a URL-encoded secret survived: %s", got)
	}
}

func TestOnlyADeadDeliveryCanBeRetried(t *testing.T) {
	fs := newFakeStore()
	live, dead := uuid.New(), uuid.New()
	fs.deliveries[live] = &Delivery{ID: live, Status: "sent"}
	fs.deliveries[dead] = &Delivery{ID: dead, Status: "dead"}
	svc := &Service{store: fs}
	if err := svc.Redeliver(context.Background(), live); !errors.Is(err, ErrNotDead) {
		t.Errorf("retrying a delivered event: %v", err)
	}
	if err := svc.Redeliver(context.Background(), dead); err != nil || fs.deliveries[dead].Status != "pending" {
		t.Errorf("retrying a dead letter: %v, %s", err, fs.deliveries[dead].Status)
	}
}

// A test-mode setting holding anything but "false" is on, and a kind without one is never in test mode
func TestTestModeIsTheKindsOwnSetting(t *testing.T) {
	registerTestKind(t, sdk.Destination{Kind: "t-testmode", Label: "t", Fields: []sdk.DestinationField{{Key: "validate", TestMode: true}}})
	for v, want := range map[string]bool{"": false, "false": false, "true": true, "TEST123": true} {
		if got := InTestMode(Destination{Kind: "t-testmode", Config: map[string]string{"validate": v}}); got != want {
			t.Errorf("validate=%q: in test mode %v, want %v", v, got, want)
		}
	}
	secretKind(t, "t-nomode", nil)
	if InTestMode(Destination{Kind: "t-nomode", Config: map[string]string{"validate": "true"}}) {
		t.Error("a kind without a test-mode field reported test mode")
	}
}

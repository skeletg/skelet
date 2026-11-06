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

// An imported destination arrives off and without its secret, once however often it is imported, and can't be
// switched on until the secret is entered
func TestImportedDestinationsWaitForTheirSecret(t *testing.T) {
	secretKind(t, "t-import", nil)
	fs := newFakeStore()
	svc := &Service{store: fs}
	ctx := context.Background()
	legacy := uuid.New()
	ga4 := Destination{ChannelID: uuid.New(), Kind: "t-import", Name: "GA4", Enabled: true, Secret: "smuggled", Config: map[string]string{"account": "G-1"}}
	got, err := svc.Import(ctx, []Imported{{ga4, legacy}})
	if err != nil || len(got) != 1 {
		t.Fatalf("imported %+v, %v", got, err)
	}
	d := got[0]
	if d.Enabled || d.Secret != "" || d.ImportedFrom == nil || *d.ImportedFrom != legacy {
		t.Fatalf("imported %+v", d)
	}
	if !NeedsSecret(d) {
		t.Error("an imported destination of a kind with a required secret should need it")
	}
	if again, err := svc.Import(ctx, []Imported{{ga4, legacy}}); err != nil || len(again) != 0 {
		t.Errorf("a second import of the same Signals destination made another: %v %v", again, err)
	}

	d.Enabled = true
	if _, err := svc.Update(ctx, d); !errors.Is(err, ErrNeedsSecret) {
		t.Errorf("switched on without its secret: %v", err)
	}
	d.Secret = "now-entered"
	on, err := svc.Update(ctx, d)
	if err != nil || !on.Enabled || on.ImportedFrom == nil {
		t.Errorf("with the secret entered it should switch on and keep its origin: %+v %v", on, err)
	}

	// One bad row refuses the whole import, before the good one is saved
	good := Imported{Destination{ChannelID: d.ChannelID, Kind: "t-import", Name: "ok", Config: map[string]string{"account": "G-2"}}, uuid.New()}
	bad := Imported{Destination{ChannelID: d.ChannelID, Kind: "t-import", Name: "x"}, uuid.New()}
	if _, err := svc.Import(ctx, []Imported{good, bad}); !errors.Is(err, ErrFieldMissing) {
		t.Errorf("an import missing a required setting should be refused: %v", err)
	}
	if n := len(fs.dests); n != 1 {
		t.Errorf("%d destinations after a refused import, want only the first import's", n)
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

package customer

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// What a customer.registering subscriber decides has to reach the account Register creates - both seams were read by nothing
func (f *fixture) onRegistering(t *testing.T, fn func(*CustomerRegistering)) {
	t.Helper()
	f.svc.bus = events.NewBus()
	f.svc.bus.SubscribeSync(EventRegistering, 100, func(_ context.Context, e events.Event) error {
		fn(e.(*CustomerRegistering))
		return nil
	})
}

func (f *fixture) defaultGroup(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := f.conn.QueryRow(`SELECT id FROM customer_groups WHERE is_default`).Scan(&id); err != nil {
		t.Skipf("no default customer group: %v", err)
	}
	return id
}

func (f *fixture) marketingConsent(t *testing.T, customerID uuid.UUID) (optedIn bool, source string, found bool) {
	t.Helper()
	err := f.conn.QueryRow(`SELECT opted_in, source FROM customer_consents WHERE customer_id = $1 AND kind = 'marketing_email'`, customerID).Scan(&optedIn, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", false
	}
	if err != nil {
		t.Fatalf("read consent: %v", err)
	}
	return optedIn, source, true
}

func TestARegisteringPluginRoutesTheAccountIntoAGroup(t *testing.T) {
	f := newFixture(t)
	corporate := f.newGroup(t).ID // before the channel, so cleanup drops its customers first
	ch := f.newChannel(t)
	f.onRegistering(t, func(e *CustomerRegistering) { e.GroupID = &corporate })

	c, err := f.svc.Register(f.ctx, ch, randomEmail(), "password123", "Co", "Worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.CustomerGroupID != corporate {
		t.Errorf("customer landed in group %s, want the plugin's %s", c.CustomerGroupID, corporate)
	}
}

// A group deleted since the plugin was configured must not refuse every signup it routes
func TestAMissingPluginGroupFallsBackToTheDefault(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	def := f.defaultGroup(t)
	gone := uuid.New()
	f.onRegistering(t, func(e *CustomerRegistering) { e.GroupID = &gone })

	c, err := f.svc.Register(f.ctx, ch, randomEmail(), "password123", "Co", "Worker", nil)
	if err != nil {
		t.Fatalf("a stale group refused the signup: %v", err)
	}
	if c.CustomerGroupID != def {
		t.Errorf("customer landed in group %s, want the default %s", c.CustomerGroupID, def)
	}
}

func TestMarketingConsentTheShopperGaveWinsOverAPlugins(t *testing.T) {
	yes, no := true, false
	for name, tc := range map[string]struct {
		shopper, plugin *bool
		wantSet         bool
		wantOptedIn     bool
		wantSource      string
	}{
		"plugin decides when no checkbox was shown": {nil, &yes, true, true, consent.SourcePlugin},
		"the shopper's no beats a plugin's yes":     {&no, &yes, true, false, consent.SourceRegistration},
		"the shopper's yes stands alone":            {&yes, nil, true, true, consent.SourceRegistration},
		"neither leaves the registry default":       {nil, nil, false, false, ""},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ch := f.newChannel(t)
			f.onRegistering(t, func(e *CustomerRegistering) { e.MarketingConsent = tc.plugin })

			c, err := f.svc.Register(f.ctx, ch, randomEmail(), "password123", "Co", "Worker", tc.shopper)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = f.conn.Exec(`DELETE FROM customer_consents WHERE customer_id = $1`, c.ID) })
			optedIn, source, found := f.marketingConsent(t, c.ID)
			if found != tc.wantSet || optedIn != tc.wantOptedIn || source != tc.wantSource {
				t.Errorf("consent recorded=%v opted_in=%v source=%q, want recorded=%v opted_in=%v source=%q",
					found, optedIn, source, tc.wantSet, tc.wantOptedIn, tc.wantSource)
			}
		})
	}
}

package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// A DSAR bundle is handed to the data subject verbatim, so no half of it may carry the bcrypt hash
func TestExportDataNeverCarriesThePasswordHash(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	f.addAddress(t, c.ID)

	export, err := f.svc.ExportData(f.ctx, c.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if export.Profile.PasswordHash != "" {
		t.Error("the exported profile carried the password hash")
	}
	if len(export.Addresses) != 1 {
		t.Errorf("exported %d addresses, want 1", len(export.Addresses))
	}
	if export.Profile.Email != c.Email {
		t.Errorf("exported email = %q, want %q", export.Profile.Email, c.Email)
	}
	blob, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	if bytes.Contains(blob, []byte("$2")) {
		t.Error("a bcrypt hash reached the serialized bundle")
	}
}

// Erasure keeps the row as an FK anchor for retained orders, so everything identifying has to go and
// the account has to become unreachable - a surviving session or a working old password defeats it
func TestAnonymizeInPlaceErasesPIIAndLocksTheAccount(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")
	f.markVerified(t, c.ID)
	f.addAddress(t, c.ID)
	live := f.newSession(t, c.ID)

	if err := f.svc.AnonymizeInPlace(f.ctx, c.ID); err != nil {
		t.Fatalf("anonymize: %v", err)
	}
	after, err := f.svc.GetByIDWithAddresses(f.ctx, c.ID)
	if err != nil {
		t.Fatalf("re-read customer: %v", err)
	}
	if after.Email == c.Email || !strings.HasSuffix(after.Email, "@anonymized.invalid") {
		t.Errorf("email after erasure = %q, want a redacted @anonymized.invalid address", after.Email)
	}
	if after.FirstName != "Redacted" || after.LastName != "" {
		t.Errorf("name after erasure = %q %q, want \"Redacted\" and empty", after.FirstName, after.LastName)
	}
	if len(after.Addresses) != 0 {
		t.Errorf("%d addresses survived erasure", len(after.Addresses))
	}
	if after.IsActive || after.IsEmailVerified {
		t.Errorf("erased account is active=%v verified=%v, want both false", after.IsActive, after.IsEmailVerified)
	}
	if f.sessionLive(t, live) {
		t.Error("a refresh token survived erasure")
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("the erased address still authenticates: %v", err)
	}
}

// Order history is retained for tax law, so a hard delete has to be refused rather than cascade into it
func TestDeleteRefusesACustomerWithOrders(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	f.seedOrder(t, ch, c.ID)

	if err := f.svc.Delete(f.ctx, c.ID); !errors.Is(err, ErrCustomerHasOrders) {
		t.Fatalf("delete = %v, want ErrCustomerHasOrders", err)
	}
	if _, err := f.svc.GetByID(f.ctx, c.ID); err != nil {
		t.Errorf("the customer was removed anyway: %v", err)
	}
}

// The row is gone but refresh tokens live in their own store with a 30-day TTL of their own
func TestDeleteRemovesAFreshAccountAndItsSessions(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	f.addAddress(t, c.ID)
	live := f.newSession(t, c.ID)

	if err := f.svc.Delete(f.ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := f.svc.GetByID(f.ctx, c.ID); !errors.Is(err, ErrCustomerNotFound) {
		t.Errorf("read after delete = %v, want ErrCustomerNotFound", err)
	}
	if f.sessionLive(t, live) {
		t.Error("a deleted account's refresh token still validates")
	}
}

type failingEraser struct{}

func (failingEraser) DeleteCustomerIdentities(context.Context, uuid.UUID, string, time.Time) error {
	return errors.New("signals unreachable")
}

type stampingEraser struct{ at time.Time }

func (e *stampingEraser) DeleteCustomerIdentities(_ context.Context, _ uuid.UUID, _ string, at time.Time) error {
	e.at = at
	return nil
}

func TestAnErasureCarriesCommercesClockToTheSuite(t *testing.T) {
	f := newFixture(t)
	eraser := &stampingEraser{}
	f.svc.repo.signals = eraser
	c := f.register(t, f.newChannel(t), randomEmail(), "password123")
	if err := f.svc.AnonymizeInPlace(f.ctx, c.ID); err != nil {
		t.Fatalf("anonymize: %v", err)
	}
	if eraser.at.IsZero() || time.Since(eraser.at).Abs() > time.Minute {
		t.Fatalf("erased at %v, want the commerce clock's now - roster read_at is stamped on it", eraser.at)
	}
}

func TestAnonymizeWhenSignalsFailsKeepsTheAddressAndStillRevokes(t *testing.T) {
	f := newFixture(t)
	f.svc.repo.signals = failingEraser{}
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	live := f.newSession(t, c.ID)

	if err := f.svc.AnonymizeInPlace(f.ctx, c.ID); err == nil {
		t.Fatal("anonymize reported success with the Signals half undone")
	}
	after, err := f.svc.GetByID(f.ctx, c.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if after.Email != c.Email {
		t.Errorf("email = %q, want the real one kept for the retry", after.Email)
	}
	if f.sessionLive(t, live) {
		t.Error("a refresh token survived an erasure whose Signals half failed")
	}
}

func TestAnonymizeClearsConsentAndAnnouncesTheChange(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	if _, err := f.conn.Exec(`INSERT INTO customer_consents (customer_id, kind, opted_in, source) VALUES ($1, 'marketing_email', true, 'profile')
		ON CONFLICT (customer_id, kind) DO UPDATE SET opted_in = true`, c.ID); err != nil {
		t.Fatalf("consent: %v", err)
	}
	got := make(chan *CustomerUpdated, 1)
	f.svc.bus.Subscribe(EventUpdated, func(_ context.Context, e events.Event) error {
		if u := e.(*CustomerUpdated); u.CustomerID == c.ID {
			got <- u
		}
		return nil
	})

	if err := f.svc.AnonymizeInPlace(f.ctx, c.ID); err != nil {
		t.Fatalf("anonymize: %v", err)
	}
	var n int
	if err := f.conn.QueryRow(`SELECT count(*) FROM customer_consents WHERE customer_id = $1`, c.ID).Scan(&n); err != nil {
		t.Fatalf("count consents: %v", err)
	}
	if n != 0 {
		t.Errorf("%d consent rows survived erasure, so the roster would still read them opted in", n)
	}
	select {
	case u := <-got:
		if strings.Contains(u.Email, c.Email) {
			t.Errorf("the event carries the real address %q", u.Email)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no customer.updated after erasure, so the roster keeps the real address until the sweep")
	}
}

func TestActiveAndGroupChangesAnnounceThemselves(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	got := make(chan string, 4)
	f.svc.bus.Subscribe(EventUpdated, func(_ context.Context, e events.Event) error {
		if u := e.(*CustomerUpdated); u.CustomerID == c.ID {
			got <- strings.Join(u.ChangedFields, ",")
		}
		return nil
	})
	if _, err := f.svc.SetCustomerActive(f.ctx, c.ID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	def, err := f.svc.GetDefaultGroup(f.ctx)
	if err != nil {
		t.Skipf("no default group: %v", err)
	}
	if _, err := f.svc.SetCustomerGroup(f.ctx, c.ID, def.ID); err != nil {
		t.Fatalf("set group: %v", err)
	}
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case fields := <-got:
			seen[fields] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("saw %v, want both isActive and customerGroup announced", seen)
		}
	}
	if !seen["isActive"] || !seen["customerGroup"] {
		t.Errorf("announced %v, want isActive and customerGroup", seen)
	}
}

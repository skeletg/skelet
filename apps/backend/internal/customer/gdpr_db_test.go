package customer

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
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

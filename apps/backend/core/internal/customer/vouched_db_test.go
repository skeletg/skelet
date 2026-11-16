package customer

import "testing"

// Someone who signed up with another person's address, never proving it, loses the account once its owner signs in
// through the store's provider: their password stops working and their sessions end
func TestAProviderSignInClaimsAnAccountNobodyVerified(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	squatted := f.register(t, ch, "owner@example.com", "Sup3r-secret-pw!")
	session := f.newSession(t, squatted.ID)

	claimed, err := f.svc.ClaimVouched(f.ctx, squatted)
	if err != nil {
		t.Fatal(err)
	}
	if !claimed.IsEmailVerified || f.sessionLive(t, session) {
		t.Errorf("claimed: verified %v, old session live %v", claimed.IsEmailVerified, f.sessionLive(t, session))
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, "owner@example.com", "Sup3r-secret-pw!"); err == nil {
		t.Error("the squatter's password still signs in")
	}

	verified := f.register(t, ch, "kept@example.com", "Sup3r-secret-pw!")
	f.markVerified(t, verified.ID)
	verified, _ = f.svc.GetByID(f.ctx, verified.ID)
	if _, err := f.svc.ClaimVouched(f.ctx, verified); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, "kept@example.com", "Sup3r-secret-pw!"); err != nil {
		t.Errorf("a verified owner's own password stopped working: %v", err)
	}
}

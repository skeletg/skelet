package customer

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
)

// Stray case or whitespace still has to resolve, and a case-variant can't become a second account
func TestEmailIdentityIsNormalizedAtWriteAndLookup(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := "Mixed-" + uuid.NewString()[:8] + "@Example.INVALID"

	c := f.register(t, ch, "  "+addr+" ", "password123")
	if want := NormalizeEmail(addr); c.Email != want {
		t.Errorf("stored email = %q, want %q", c.Email, want)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, strings.ToUpper(addr), "password123"); err != nil {
		t.Errorf("login with an upper-cased address: %v", err)
	}
	if _, err := f.svc.Register(f.ctx, ch, strings.ToUpper(addr), "password123", "", "", nil); err == nil {
		t.Error("a case-variant of a registered address opened a second account")
	}
}

// Customers are per-channel rows, so one store's credentials must be inert on another
func TestAccountsDoNotCrossChannels(t *testing.T) {
	f := newFixture(t)
	tires, wheels := f.newChannel(t), f.newChannel(t)
	addr := randomEmail()

	onTires := f.register(t, tires, addr, "password123")
	onWheels := f.register(t, wheels, addr, "different456")
	if onTires.ID == onWheels.ID {
		t.Fatal("the same address on two channels resolved to one account")
	}
	if _, err := f.svc.Authenticate(f.ctx, wheels, addr, "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("the tires password on the wheels store = %v, want ErrInvalidCredentials", err)
	}
	got, err := f.svc.Authenticate(f.ctx, tires, addr, "password123")
	if err != nil || got.ID != onTires.ID {
		t.Errorf("own-channel login returned %v / %v", got.ID, err)
	}
}

// An unregistered address has to fail the same way a wrong password does, or the response enumerates accounts
func TestAuthenticateRefusals(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")

	if _, err := f.svc.Authenticate(f.ctx, ch, randomEmail(), "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown email = %v, want ErrInvalidCredentials", err)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password = %v, want ErrInvalidCredentials", err)
	}
	if _, err := f.svc.SetCustomerActive(f.ctx, c.ID, false); err != nil {
		t.Fatalf("disable account: %v", err)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); !errors.Is(err, ErrAccountDisabled) {
		t.Errorf("disabled account = %v, want ErrAccountDisabled", err)
	}
}

// The gate is what makes the emailed link mean something - with it on, a correct password is not enough
func TestEmailVerificationGateHoldsUntilTheLinkIsRedeemed(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")

	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); err != nil {
		t.Fatalf("login while the gate is off: %v", err)
	}
	f.setVerificationGate(t, ch, true)
	if f.svc.RequiresEmailVerification(f.ctx, ch) != true {
		t.Error("RequiresEmailVerification disagrees with the channel config the resolver withholds a session on")
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); !errors.Is(err, ErrEmailNotVerified) {
		t.Fatalf("unverified login with the gate on = %v, want ErrEmailNotVerified", err)
	}

	token, err := f.verify.Create(f.ctx, c.ID, c.Email)
	if err != nil {
		t.Fatalf("mint verification token: %v", err)
	}
	if err := f.svc.VerifyEmail(f.ctx, token); err != nil {
		t.Fatalf("verify email: %v", err)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); err != nil {
		t.Errorf("login after verifying: %v", err)
	}
	if err := f.svc.VerifyEmail(f.ctx, token); err == nil {
		t.Error("the verification link was redeemable a second time")
	}
	if err := f.svc.ResendVerificationEmail(f.ctx, c.ID); err == nil {
		t.Error("resend was allowed for an already-verified account")
	}
}

// The response is identical either way, so the only observable is whether a token gets minted
func TestRequestPasswordResetRevealsNothing(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")

	if err := f.svc.RequestPasswordReset(f.ctx, ch, randomEmail()); err != nil {
		t.Errorf("unknown address = %v, want nil", err)
	}
	before := f.authTokens(t, c.ID) // registering already minted a verification token, so only the delta is meaningful
	if err := f.svc.RequestPasswordReset(f.ctx, ch, strings.ToUpper(addr)); err != nil {
		t.Fatalf("known address: %v", err)
	}
	f.svc.FlushMail()
	if after := f.authTokens(t, c.ID); after != before+1 {
		t.Errorf("tokens for the customer went %d -> %d, want one reset token minted", before, after)
	}
}

// The store match lives in the service, so a caller that forgets it can't write a password across stores
func TestResetPasswordRefusesATokenFromAnotherChannel(t *testing.T) {
	f := newFixture(t)
	tires, wheels := f.newChannel(t), f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, tires, addr, "password123")

	token, err := f.svc.resetStore.Create(f.ctx, c.ID)
	if err != nil {
		t.Fatalf("mint reset token: %v", err)
	}
	if err := f.svc.ResetPassword(f.ctx, wheels, token, "brand-new-password"); err == nil {
		t.Fatal("a tires-store reset token set a password through the wheels store")
	}
	if _, err := f.svc.Authenticate(f.ctx, tires, addr, "password123"); err != nil {
		t.Errorf("the original password stopped working after a refused reset: %v", err)
	}
}

// Resetting is how someone evicts an attacker, so a refresh token minted before it has to die with it
func TestResetPasswordRevokesEverySessionAndBurnsTheToken(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")
	stolen := f.newSession(t, c.ID)

	token, err := f.svc.resetStore.Create(f.ctx, c.ID)
	if err != nil {
		t.Fatalf("mint reset token: %v", err)
	}
	if err := f.svc.ResetPassword(f.ctx, ch, token, "fresh-password-9"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if f.sessionLive(t, stolen) {
		t.Error("a refresh token minted before the reset still validates")
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "fresh-password-9"); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("the old password still works: %v", err)
	}
	if err := f.svc.ResetPassword(f.ctx, ch, token, "another-password-1"); err == nil {
		t.Error("the emailed reset link was redeemable twice")
	}
}

// Checking the token and spending it are one statement, so a link redeemed from two tabs at once sets one password, not two
func TestResetTokenRedeemedConcurrentlySucceedsOnce(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	token, err := f.svc.resetStore.Create(f.ctx, c.ID)
	if err != nil {
		t.Fatalf("mint reset token: %v", err)
	}

	const racers = 8
	var wg sync.WaitGroup
	var won atomic.Int32
	for i := range racers {
		wg.Go(func() {
			if f.svc.ResetPassword(f.ctx, ch, token, fmt.Sprintf("racing-password-%d", i)) == nil {
				won.Add(1)
			}
		})
	}
	wg.Wait()
	if n := won.Load(); n != 1 {
		t.Errorf("%d of %d concurrent redemptions succeeded, want exactly 1", n, racers)
	}
}

// Same eviction contract as a reset, plus the current-password proof a logged-in change rests on
func TestChangePasswordRequiresTheCurrentOneAndRevokesSessions(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")
	live := f.newSession(t, c.ID)

	if err := f.svc.ChangePassword(f.ctx, c.ID, "not-the-password", "fresh-password-9"); err == nil {
		t.Error("the password changed without the current one")
	}
	if err := f.svc.ChangePassword(f.ctx, c.ID, "password123", "short"); err == nil {
		t.Error("a password below the minimum length was accepted")
	}
	if !f.sessionLive(t, live) {
		t.Fatal("a refused password change already revoked the session")
	}
	if err := f.svc.ChangePassword(f.ctx, c.ID, "password123", "fresh-password-9"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if f.sessionLive(t, live) {
		t.Error("the session survived a password change")
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("the old password still works: %v", err)
	}
}

// Disabling stops Authenticate, but an un-revoked refresh chain would keep minting fresh access tokens
func TestDisablingAnAccountKillsItsSessions(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	live := f.newSession(t, c.ID)

	if _, err := f.svc.SetCustomerActive(f.ctx, c.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if f.sessionLive(t, live) {
		t.Error("a disabled account's refresh token still validates")
	}
	if active, err := f.svc.IsActive(f.ctx, c.ID); err != nil || active {
		t.Errorf("IsActive = %v / %v, want false - the session guard reads this per request", active, err)
	}
}

// The row exists to be targetable by flows and segments, never to be logged into - only a reset can activate it
func TestEnsureContactIsIdempotentAndCannotSignIn(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()

	c, created, err := f.svc.EnsureContact(f.ctx, ch, addr, "Newsletter", false)
	if err != nil || !created {
		t.Fatalf("first EnsureContact = created %v, err %v", created, err)
	}
	again, created, err := f.svc.EnsureContact(f.ctx, ch, strings.ToUpper(addr), "Different", false)
	if err != nil {
		t.Fatalf("second EnsureContact: %v", err)
	}
	if created || again.ID != c.ID {
		t.Errorf("second EnsureContact created %v and returned %s, want false and %s", created, again.ID, c.ID)
	}
	for _, guess := range []string{"", "password123"} {
		if _, err := f.svc.Authenticate(f.ctx, ch, addr, guess); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("a contact row authenticated with %q: %v", guess, err)
		}
	}
}

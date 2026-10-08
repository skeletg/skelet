package customer

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// A link proves the inbox it was mailed to, so it can't vouch for whatever address the account holds by the time it's clicked
func TestVerificationLinkIsBoundToTheAddressItWasMailedTo(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")

	foreign, err := f.verify.Create(f.ctx, c.ID, randomEmail())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.VerifyEmail(f.ctx, foreign); !errors.Is(err, ErrVerificationEmailMismatch) {
		t.Fatalf("link minted for another address = %v, want ErrVerificationEmailMismatch", err)
	}
	if got, _ := f.svc.GetByID(f.ctx, c.ID); got.IsEmailVerified {
		t.Fatal("a link minted for another address verified the account")
	}
	if err := f.svc.VerifyEmail(f.ctx, f.verifyLink(t, c)); err != nil {
		t.Fatalf("link minted for the account's own address: %v", err)
	}
}

// The verified flag and the claim event commit together, and a refused link enqueues nothing
func TestVerifyingEnqueuesTheClaimInTheSameTransaction(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	box := f.withOutbox()

	foreign, err := f.verify.Create(f.ctx, c.ID, randomEmail())
	if err != nil {
		t.Fatal(err)
	}
	_ = f.svc.VerifyEmail(f.ctx, foreign)
	if got := box.types(); len(got) != 0 {
		t.Fatalf("a refused link enqueued %v", got)
	}
	if err := f.svc.VerifyEmail(f.ctx, f.verifyLink(t, c)); err != nil {
		t.Fatal(err)
	}
	if got := box.types(); !slices.Equal(got, []string{EventEmailVerified}) {
		t.Errorf("enqueued %v, want one %s", got, EventEmailVerified)
	}
}

// Deactivating or re-addressing through the admin edit form has to end sessions the way the dedicated toggle does
func TestAdminUpdateEndsSessionsOnDeactivationAndEmailChange(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")

	old := f.newSession(t, c.ID)
	off, on := false, true
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{IsActive: &on}); err != nil {
		t.Fatal(err)
	}
	if f.sessionLive(t, old) {
		t.Error("a refresh token from before the deactivation still works after re-enabling")
	}

	kept := f.newSession(t, c.ID)
	note := "vip"
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Note: &note}); err != nil {
		t.Fatal(err)
	}
	if !f.sessionLive(t, kept) {
		t.Error("an unrelated admin edit ended the customer's session")
	}
	addr := randomEmail()
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Email: &addr}); err != nil {
		t.Fatal(err)
	}
	if f.sessionLive(t, kept) {
		t.Error("a session signed in under the old address survived the email change")
	}
}

// Links mailed to the old address die with it, and an unverified new address gets its own link
func TestEmailChangeRevokesLinksAndMailsTheNewAddress(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	fb := f.withFallback()
	verify, reset := f.verifyLink(t, c), f.resetLink(t, c.ID)

	addr, unverified := randomEmail(), false
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Email: &addr, IsEmailVerified: &unverified}); err != nil {
		t.Fatal(err)
	}
	f.svc.FlushMail()
	if err := f.svc.VerifyEmail(f.ctx, verify); !errors.Is(err, ErrVerificationLinkInvalid) {
		t.Errorf("verification link for the old address = %v, want revoked", err)
	}
	if err := f.svc.ResetPassword(f.ctx, ch, reset, "brand-new-password"); !errors.Is(err, ErrResetLinkInvalid) {
		t.Errorf("reset link mailed to the old address = %v, want revoked", err)
	}
	if fb.verifications != 1 || fb.lastTo != addr {
		t.Errorf("verification mails = %d to %q, want one to the new address %q", fb.verifications, fb.lastTo, addr)
	}
}

// Registering a taken address answers like any registration and tells the real owner instead
func TestRegisteringATakenAddressMailsItsOwner(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	owner := f.register(t, ch, addr, "password123")
	fb := f.withFallback()

	if _, err := f.svc.Register(f.ctx, ch, strings.ToUpper(addr), "other-password", "Mal", "Lory", nil); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("second registration = %v, want ErrEmailTaken for the resolver to mask", err)
	}
	f.svc.FlushMail()
	if fb.exists != 1 || fb.lastTo != owner.Email || fb.verifications != 0 {
		t.Errorf("account-exists mails = %d to %q (verifications %d), want one to the owner %q", fb.exists, fb.lastTo, fb.verifications, owner.Email)
	}
	if !strings.HasSuffix(fb.lastLink, SignInPath) {
		t.Errorf("sign-in link = %q, want the store's sign-in page", fb.lastLink)
	}
}

// The gate locks an unverified customer out of the signed-in resend, so the signed-out one has to work - and say nothing
func TestResendVerificationByEmailRevealsNothing(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	f.setVerificationGate(t, ch, true)
	unverified := f.register(t, ch, randomEmail(), "password123")
	verified := f.register(t, ch, randomEmail(), "password123")
	f.markVerified(t, verified.ID)
	fb := f.withFallback()

	for _, addr := range []string{randomEmail(), verified.Email, strings.ToUpper(unverified.Email)} {
		if err := f.svc.ResendVerificationByEmail(f.ctx, ch, addr); err != nil {
			t.Errorf("resend for %s = %v, want nil", addr, err)
		}
	}
	f.svc.FlushMail()
	if fb.verifications != 1 || fb.lastTo != unverified.Email {
		t.Errorf("verification mails = %d to %q, want one to the unverified %q", fb.verifications, fb.lastTo, unverified.Email)
	}
	if err := f.svc.ResendVerificationByEmail(f.ctx, f.newChannel(t), unverified.Email); err != nil {
		t.Errorf("resend on another channel = %v", err)
	}
	f.svc.FlushMail()
	if fb.verifications != 1 {
		t.Error("a resend on another channel mailed this channel's customer")
	}
}

// The reset link reached the inbox, which proves the address - and spends every other outstanding link
func TestResetPasswordVerifiesTheAddressAndRevokesOtherLinks(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	f.setVerificationGate(t, ch, true)
	addr := randomEmail()
	c := f.register(t, ch, addr, "password123")
	box := f.withOutbox()
	first, second := f.resetLink(t, c.ID), f.resetLink(t, c.ID)

	if err := f.svc.ResetPassword(f.ctx, ch, first, "fresh-password-9"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(f.ctx, ch, addr, "fresh-password-9"); err != nil {
		t.Errorf("login after the reset with the gate on = %v, want the reset to have verified the address", err)
	}
	if !slices.Contains(box.types(), EventEmailVerified) {
		t.Errorf("enqueued %v, want the verification that claims guest orders", box.types())
	}
	if err := f.svc.ResetPassword(f.ctx, ch, second, "another-password-1"); !errors.Is(err, ErrResetLinkInvalid) {
		t.Errorf("the other outstanding link = %v, want revoked by the reset", err)
	}
}

// A link refused on the wrong store is not spent, so the shopper can still use it where it belongs
func TestResetOnTheWrongChannelLeavesTheLinkUsable(t *testing.T) {
	f := newFixture(t)
	tires, wheels := f.newChannel(t), f.newChannel(t)
	addr := randomEmail()
	c := f.register(t, tires, addr, "password123")
	link := f.resetLink(t, c.ID)

	if err := f.svc.ResetPassword(f.ctx, wheels, link, "brand-new-password"); !errors.Is(err, ErrResetLinkInvalid) {
		t.Fatalf("cross-channel reset = %v, want ErrResetLinkInvalid", err)
	}
	if err := f.svc.ResetPassword(f.ctx, tires, link, "brand-new-password"); err != nil {
		t.Errorf("the link on its own store after a refused cross-store attempt = %v", err)
	}
}

func TestChangePasswordNamesAWrongPasswordAndRevokesResetLinks(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	link := f.resetLink(t, c.ID)

	if err := f.svc.ChangePassword(f.ctx, c.ID, "not-it-at-all", "fresh-password-9"); !errors.Is(err, ErrIncorrectCurrentPassword) {
		t.Fatalf("wrong current password = %v, want ErrIncorrectCurrentPassword", err)
	}
	if err := f.svc.ChangePassword(f.ctx, c.ID, "password123", "fresh-password-9"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ResetPassword(f.ctx, ch, link, "another-password-1"); !errors.Is(err, ErrResetLinkInvalid) {
		t.Errorf("reset link minted before the change = %v, want revoked", err)
	}
	if err := f.svc.ChangePassword(f.ctx, uuid.New(), "x", "fresh-password-9"); !errors.Is(err, ErrCustomerNotFound) {
		t.Errorf("unknown customer = %v, want ErrCustomerNotFound", err)
	}
}

// A storage failure is not a wrong password - folding it into invalid-credentials would count it toward a lockout and hide the outage
func TestLookupFailuresAreNotReportedAsUnknownAddresses(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	addr := randomEmail()
	f.register(t, ch, addr, "password123")
	dead, cancel := context.WithCancel(f.ctx)
	cancel()

	if _, err := f.svc.Authenticate(dead, ch, addr, "password123"); err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("Authenticate on a failed lookup = %v, want the storage error", err)
	}
	if err := f.svc.RequestPasswordReset(dead, ch, addr); err == nil {
		t.Error("RequestPasswordReset swallowed a failed lookup")
	}
	if _, err := f.svc.repo.GetByStoreAndEmail(f.ctx, ch, randomEmail()); !errors.Is(err, ErrCustomerNotFound) {
		t.Errorf("unknown address = %v, want ErrCustomerNotFound", err)
	}
}

func TestAccountInputsAreValidated(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	long := strings.Repeat("x", MaxNameRunes+1)

	for _, tc := range []struct {
		name             string
		email, pw, first string
		want             error
	}{
		{"malformed email", "not-an-email", "password123", "Ana", ErrInvalidEmail},
		{"display-name email", "Ana <" + randomEmail() + ">", "password123", "Ana", ErrInvalidEmail},
		{"long name", randomEmail(), "password123", long, ErrFieldTooLong},
		{"password past bcrypt's 72 bytes", randomEmail(), strings.Repeat("p", auth.MaxPasswordBytes+1), "Ana", auth.ErrPasswordTooLong},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.svc.Register(f.ctx, ch, tc.email, tc.pw, tc.first, "", nil); !errors.Is(err, tc.want) {
				t.Errorf("Register = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{ChannelID: ch, Email: randomEmail(), Password: "short"}); !errors.Is(err, auth.ErrWeakPassword) {
		t.Errorf("admin create with a weak password = %v, want ErrWeakPassword", err)
	}
	if _, err := f.svc.AdminCreateCustomer(f.ctx, AdminCreateCustomerInput{ChannelID: ch, Email: "nope", Password: "password123"}); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("admin create with a bad email = %v, want ErrInvalidEmail", err)
	}
	c := f.register(t, ch, randomEmail(), "password123")
	bad := "bad@"
	if _, err := f.svc.AdminUpdateCustomer(f.ctx, c.ID, AdminUpdateCustomerInput{Email: &bad}); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("admin update with a bad email = %v, want ErrInvalidEmail", err)
	}
	if _, _, err := f.svc.EnsureContact(f.ctx, ch, "bad@", "", false); !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("EnsureContact with a bad email = %v, want ErrInvalidEmail", err)
	}
	in := addressInput()
	in.City = strings.Repeat("c", MaxCityRunes+1)
	if _, err := f.svc.AddAddress(f.ctx, c.ID, in); !errors.Is(err, ErrFieldTooLong) {
		t.Errorf("address with a long city = %v, want ErrFieldTooLong", err)
	}
}

func TestAddressBookIsCapped(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, randomEmail(), "password123")
	if _, err := f.conn.Exec(`INSERT INTO addresses (customer_id, first_name, last_name, street_1, city, postal_code, country_code)
		SELECT $1, 'a', 'b', 's', 'c', 'p', 'US' FROM generate_series(1, $2)`, c.ID, MaxAddressesPerCustomer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddAddress(f.ctx, c.ID, addressInput()); !errors.Is(err, ErrTooManyAddresses) {
		t.Errorf("address past the cap = %v, want ErrTooManyAddresses", err)
	}
}

// Only a confirmed double opt-in has proven the inbox
func TestEnsureContactStartsVerifiedOnlyWhenTold(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	box := f.withOutbox()
	plain, _, err := f.svc.EnsureContact(f.ctx, ch, randomEmail(), "", false)
	if err != nil || plain.IsEmailVerified {
		t.Fatalf("unconfirmed contact verified=%v err=%v, want unverified", plain.IsEmailVerified, err)
	}
	confirmed, _, err := f.svc.EnsureContact(f.ctx, ch, randomEmail(), "", true)
	if err != nil || !confirmed.IsEmailVerified {
		t.Fatalf("confirmed contact verified=%v err=%v, want verified", confirmed.IsEmailVerified, err)
	}
	if n := strings.Count(strings.Join(box.types(), ","), EventEmailVerified); n != 1 {
		t.Errorf("enqueued %v, want one %s for the confirmed contact's guest orders", box.types(), EventEmailVerified)
	}
}

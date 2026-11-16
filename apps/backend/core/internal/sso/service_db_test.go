package sso

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

type fakeAdmins struct {
	byEmail  map[string]uuid.UUID
	totp     map[uuid.UUID]string
	signedIn []uuid.UUID
}

func (f *fakeAdmins) GetByID(_ context.Context, id uuid.UUID) (admin.AdminUser, error) {
	for _, known := range f.byEmail {
		if known == id {
			return admin.AdminUser{ID: id}, nil
		}
	}
	return admin.AdminUser{}, admin.ErrAdminNotFound
}

func (f *fakeAdmins) SignInVouched(_ context.Context, id uuid.UUID, code string, providerMFA bool) (admin.AdminUser, error) {
	if want, on := f.totp[id]; on && !providerMFA && code != want {
		if code == "" {
			return admin.AdminUser{}, admin.ErrTotpRequired
		}
		return admin.AdminUser{}, errors.New("invalid credentials")
	}
	f.signedIn = append(f.signedIn, id)
	return admin.AdminUser{ID: id}, nil
}

type fakeCustomers struct {
	all        map[uuid.UUID]customer.Customer
	registered int
}

func (f *fakeCustomers) GetByID(_ context.Context, id uuid.UUID) (customer.Customer, error) {
	if c, ok := f.all[id]; ok {
		return c, nil
	}
	return customer.Customer{}, customer.ErrCustomerNotFound
}
func (f *fakeCustomers) FindForSignIn(_ context.Context, ch uuid.UUID, email string) (customer.Customer, error) {
	for _, c := range f.all {
		if c.ChannelID == ch && c.Email == email {
			return c, nil
		}
	}
	return customer.Customer{}, customer.ErrCustomerNotFound
}
func (f *fakeCustomers) RegisterVouched(_ context.Context, ch uuid.UUID, email, first, last string) (customer.Customer, error) {
	f.registered++
	c := customer.Customer{ID: uuid.New(), ChannelID: ch, Email: email, FirstName: first, LastName: last, IsActive: true, IsEmailVerified: true}
	f.all[c.ID] = c
	return c, nil
}
func (f *fakeCustomers) SignInVouched(context.Context, customer.Customer) error { return nil }
func (f *fakeCustomers) AttemptVouched(_ context.Context, _ uuid.UUID, email string) error {
	if email == "blocked@example.com" {
		return errors.New("login rejected: blocked")
	}
	return nil
}
func (f *fakeCustomers) ClaimVouched(_ context.Context, c customer.Customer) (customer.Customer, error) {
	c.IsEmailVerified, c.PasswordHash = true, ""
	f.all[c.ID] = c
	return c, nil
}

func newTestService(t *testing.T, idp *fakeIdP, s Settings, admins *fakeAdmins, customers *fakeCustomers) (*Service, uuid.UUID) {
	t.Helper()
	conn := testdb.Require(t)
	var ch uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&ch); err != nil {
		t.Skipf("no channel: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM identity_links WHERE issuer = $1`, idp.URL) })
	svc := NewService(s, &memTokens{m: map[string]auth.Token{}}, db.New(conn), admins, customers, []string{"http://localhost:5173"})
	return svc, ch
}

func signIn(t *testing.T, svc *Service, idp *fakeIdP, ch uuid.UUID, claims jwt.MapClaims) (customer.Customer, error) {
	t.Helper()
	ctx := context.Background()
	st, err := svc.StartCustomer(ctx, channel.Channel{ID: ch, PublicURL: "http://localhost:3000"}, "http://localhost:3000/account/sso")
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, claims)
	return svc.CompleteCustomer(ctx, ch, state, code, st.Binding)
}

func TestACustomerIsFoundByTheirIdentityAfterTheFirstVisit(t *testing.T) {
	idp := newFakeIdP(t)
	customers := &fakeCustomers{all: map[uuid.UUID]customer.Customer{}}
	svc, ch := newTestService(t, idp, idp.settings(Customers), &fakeAdmins{}, customers)

	if _, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-1", "email": "new@example.com"}); !errors.Is(err, ErrUnverified) || customers.registered != 0 {
		t.Fatalf("an unverified address: %v, %d registered", err, customers.registered)
	}
	first, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-1", "email": "new@example.com", "email_verified": true, "given_name": "Ada", "family_name": "L"})
	if err != nil || customers.registered != 1 || first.FirstName != "Ada" {
		t.Fatalf("first visit: %+v, %v, %d registered", first, err, customers.registered)
	}
	again, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-1", "email": "renamed@example.com", "email_verified": true})
	if err != nil || again.ID != first.ID || customers.registered != 1 {
		t.Errorf("after an email change at the provider: %+v, %v - want the same account", again, err)
	}

	existing := customer.Customer{ID: uuid.New(), ChannelID: ch, Email: "grace@example.com", IsActive: true, PasswordHash: "set-by-whoever-signed-up"}
	customers.all[existing.ID] = existing
	if got, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-2", "email": "grace@example.com", "email_verified": true}); err != nil || got.ID != existing.ID || customers.registered != 1 {
		t.Errorf("an existing account: %+v, %v", got, err)
	}
	if c := customers.all[existing.ID]; !c.IsEmailVerified || c.PasswordHash != "" {
		t.Errorf("an account opened with nobody proving its address kept its password: %+v", c)
	}
}

// adminFlow runs one provider round trip, for a sign-in or, given an admin, for connecting their account
func adminFlow(t *testing.T, svc *Service, idp *fakeIdP, linkFor uuid.UUID, claims jwt.MapClaims) (AdminSignIn, AdminLink, error) {
	t.Helper()
	ctx := context.Background()
	start := func() (Start, error) { return svc.StartAdmin(ctx, "http://localhost:5173/login/sso") }
	if linkFor != uuid.Nil {
		start = func() (Start, error) { return svc.StartAdminLink(ctx, linkFor, "http://localhost:5173/login/sso") }
	}
	st, err := start()
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, claims)
	if linkFor != uuid.Nil {
		link, err := svc.CompleteAdminLink(ctx, linkFor, state, code, st.Binding)
		return AdminSignIn{}, link, err
	}
	in, err := svc.CompleteAdmin(ctx, state, code, st.Binding)
	return in, AdminLink{}, err
}

func TestAnAdminSignsInOnlyThroughAProviderAccountTheyConnected(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	ops, other := uuid.New(), uuid.New()
	admins := &fakeAdmins{byEmail: map[string]uuid.UUID{"ops@acme.example": ops, "other@acme.example": other}}
	s := idp.settings(Admins)
	svc, _ := newTestService(t, idp, s, admins, &fakeCustomers{})
	opsClaims := jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true}

	if _, _, err := adminFlow(t, svc, idp, uuid.Nil, opsClaims); !errors.Is(err, ErrNotLinked) {
		t.Fatalf("an address matching an admin, never connected: %v", err)
	}
	if _, _, err := adminFlow(t, svc, idp, ops, opsClaims); err != nil {
		t.Fatalf("connecting: %v", err)
	}
	if in, _, err := adminFlow(t, svc, idp, uuid.Nil, opsClaims); err != nil || in.User.ID != ops {
		t.Fatalf("signing in after connecting: %+v, %v", in, err)
	}
	if _, _, err := adminFlow(t, svc, idp, other, opsClaims); !errors.Is(err, ErrLinkTaken) {
		t.Errorf("another admin taking a connected account: %v", err)
	}

	s[groupFor(Admins)+"oidc_email_domains"] = "corp.example"
	if _, _, err := adminFlow(t, svc, idp, uuid.Nil, opsClaims); !errors.Is(err, ErrNoAdmin) {
		t.Errorf("a connected account the domain list no longer allows: %v", err)
	}
	if _, _, err := adminFlow(t, svc, idp, other, jwt.MapClaims{"sub": "a-2", "email": "other@corp.example"}); !errors.Is(err, ErrNoAdmin) {
		t.Errorf("connecting an account whose address was never verified: %v", err)
	}
	delete(s, groupFor(Admins)+"oidc_email_domains")

	links, err := svc.AdminLinks(ctx, ops)
	if err != nil || len(links) != 1 || links[0].Email != "ops@acme.example" {
		t.Fatalf("links %+v, %v", links, err)
	}
	if err := svc.UnlinkAdmin(ctx, other, links[0].ID); !errors.Is(err, ErrLinkAbsent) {
		t.Errorf("another admin disconnecting it: %v", err)
	}
	if err := svc.UnlinkAdmin(ctx, ops, links[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := adminFlow(t, svc, idp, uuid.Nil, opsClaims); !errors.Is(err, ErrNotLinked) {
		t.Errorf("signing in after disconnecting: %v", err)
	}
}

func TestASignInAndAConnectionCantStandInForEachOther(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	ops := uuid.New()
	svc, _ := newTestService(t, idp, idp.settings(Admins), &fakeAdmins{byEmail: map[string]uuid.UUID{"ops@acme.example": ops}}, &fakeCustomers{})
	st, err := svc.StartAdmin(ctx, "http://localhost:5173/login/sso")
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true})
	if _, err := svc.CompleteAdminLink(ctx, ops, state, code, st.Binding); !errors.Is(err, ErrRefused) {
		t.Errorf("a sign-in's code connecting an account: %v", err)
	}
	st, _ = svc.StartAdminLink(ctx, ops, "http://localhost:5173/login/sso")
	state, code = idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "a-1"})
	if _, err := svc.CompleteAdminLink(ctx, uuid.New(), state, code, st.Binding); !errors.Is(err, ErrRefused) {
		t.Errorf("one admin's connection finished by another: %v", err)
	}
}

func TestReturnAddressesArePinned(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	svc, _ := newTestService(t, idp, idp.settings(Admins), &fakeAdmins{}, &fakeCustomers{})
	for _, bad := range []string{"https://evil.example/login/sso", "http://localhost:5173/oauth/catch", "http://localhost:5173/login/sso?next=/x"} {
		if _, err := svc.StartAdmin(ctx, bad); !errors.Is(err, ErrRedirect) {
			t.Errorf("admin return %s: %v", bad, err)
		}
	}
	shop := channel.Channel{PublicURL: "https://shop.example"}
	for _, bad := range []string{"https://evil.example/account/sso", "https://shop.example/blog/post"} {
		if _, err := svc.StartCustomer(ctx, shop, bad); !errors.Is(err, ErrRedirect) {
			t.Errorf("store return %s: %v", bad, err)
		}
	}
}

func TestTheLoginVetoRunsBeforeAnAccountIsOpenedOrClaimed(t *testing.T) {
	idp := newFakeIdP(t)
	customers := &fakeCustomers{all: map[uuid.UUID]customer.Customer{}}
	svc, ch := newTestService(t, idp, idp.settings(Customers), &fakeAdmins{}, customers)
	if _, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-9", "email": "blocked@example.com", "email_verified": true}); err == nil || customers.registered != 0 {
		t.Errorf("a vetoed sign-in: %v, %d accounts opened", err, customers.registered)
	}
	off := customer.Customer{ID: uuid.New(), ChannelID: ch, Email: "off@example.com", PasswordHash: "theirs"}
	customers.all[off.ID] = off
	if _, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-8", "email": "off@example.com", "email_verified": true}); !errors.Is(err, customer.ErrAccountDisabled) {
		t.Errorf("a disabled account: %v", err)
	}
	if customers.all[off.ID].PasswordHash != "theirs" {
		t.Error("a disabled account was claimed anyway")
	}
}

func TestAnAdminStillOwesTheirOwnSecondFactor(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	known := uuid.New()
	admins := &fakeAdmins{byEmail: map[string]uuid.UUID{"ops@acme.example": known}, totp: map[uuid.UUID]string{known: "123456"}}
	s := idp.settings(Admins)
	svc, _ := newTestService(t, idp, s, admins, &fakeCustomers{})
	complete := func(amr ...string) AdminSignIn {
		t.Helper()
		st, err := svc.StartAdmin(ctx, "http://localhost:5173/login/sso")
		if err != nil {
			t.Fatal(err)
		}
		state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true, "amr": amr})
		in, err := svc.CompleteAdmin(ctx, state, code, st.Binding)
		if err != nil {
			t.Fatal(err)
		}
		return in
	}
	if _, _, err := adminFlow(t, svc, idp, known, jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true}); err != nil {
		t.Fatal(err)
	}

	in := complete("pwd", "mfa")
	if in.TotpTicket == "" || len(admins.signedIn) != 0 {
		t.Fatalf("a provider not trusted to ask for a second factor: %+v", in)
	}
	if _, err := svc.CompleteAdminTotp(ctx, in.TotpTicket, "000000"); err == nil {
		t.Error("a wrong code signed in")
	}
	if u, err := svc.CompleteAdminTotp(ctx, in.TotpTicket, "123456"); err != nil || u.ID != known {
		t.Errorf("the right code after a wrong one: %+v, %v", u, err)
	}
	if _, err := svc.CompleteAdminTotp(ctx, in.TotpTicket, "123456"); !errors.Is(err, ErrExpired) {
		t.Errorf("a ticket used twice: %v", err)
	}

	s[groupFor(Admins)+"oidc_enforces_mfa"] = "true"
	if in := complete("pwd"); in.TotpTicket == "" {
		t.Error("a password alone at a provider trusted with second factors skipped the code")
	}
	if in := complete("otp"); in.TotpTicket == "" {
		t.Error("a single method counted as a second factor")
	}
	if in := complete("pwd", "mfa"); in.TotpTicket != "" || in.User.ID != known {
		t.Errorf("a proven second factor at a trusted provider: %+v", in)
	}
}

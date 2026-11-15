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

func (f *fakeAdmins) IDByEmail(_ context.Context, email string) (uuid.UUID, error) {
	if id, ok := f.byEmail[email]; ok {
		return id, nil
	}
	return uuid.Nil, admin.ErrAdminNotFound
}
func (f *fakeAdmins) SignInVouched(_ context.Context, id uuid.UUID) (admin.AdminUser, error) {
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

	existing := customer.Customer{ID: uuid.New(), ChannelID: ch, Email: "grace@example.com", IsActive: true}
	customers.all[existing.ID] = existing
	if got, err := signIn(t, svc, idp, ch, jwt.MapClaims{"sub": "u-2", "email": "grace@example.com", "email_verified": true}); err != nil || got.ID != existing.ID || customers.registered != 1 {
		t.Errorf("an existing account: %+v, %v", got, err)
	}
}

func TestOnlyAnExistingAdminSignsIn(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	known := uuid.New()
	admins := &fakeAdmins{byEmail: map[string]uuid.UUID{"ops@acme.example": known}}
	s := idp.settings(Admins)
	svc, _ := newTestService(t, idp, s, admins, &fakeCustomers{})
	complete := func(claims jwt.MapClaims) (admin.AdminUser, error) {
		st, err := svc.StartAdmin(ctx, "http://localhost:5173/login/sso")
		if err != nil {
			t.Fatal(err)
		}
		state, code := idp.authorize(t, st.AuthorizeURL, claims)
		return svc.CompleteAdmin(ctx, state, code, st.Binding)
	}
	if _, err := complete(jwt.MapClaims{"sub": "a-9", "email": "stranger@acme.example", "email_verified": true}); !errors.Is(err, ErrNoAdmin) {
		t.Errorf("a stranger: %v", err)
	}
	if u, err := complete(jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true}); err != nil || u.ID != known {
		t.Errorf("the admin: %+v, %v", u, err)
	}
	s[groupFor(Admins)+"oidc_email_domains"] = "acme.example"
	if _, err := complete(jwt.MapClaims{"sub": "a-3", "email": "ops@gmail.example", "email_verified": true}); !errors.Is(err, ErrNoAdmin) {
		t.Errorf("an address off the company's domain: %v", err)
	}
	if _, err := svc.StartAdmin(ctx, "https://evil.example/login/sso"); !errors.Is(err, ErrRedirect) {
		t.Errorf("a return address off the dashboard: %v", err)
	}
	if _, err := svc.StartCustomer(ctx, channel.Channel{PublicURL: "https://shop.example"}, "https://evil.example/account/sso"); !errors.Is(err, ErrRedirect) {
		t.Errorf("a return address off the store: %v", err)
	}
}

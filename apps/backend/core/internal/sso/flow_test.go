package sso

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

const shopRedirect = "http://localhost:3000/account/sso"

func newFlow(s Settings) *flow {
	return &flow{settings: s, tokens: &memTokens{m: map[string]auth.Token{}}, client: &http.Client{Timeout: 5 * time.Second}, providers: map[string]cachedProvider{}}
}

func TestASignInRoundTrips(t *testing.T) {
	idp := newFakeIdP(t)
	f, ctx, ch := newFlow(idp.settings(Customers)), context.Background(), uuid.New()
	st, err := f.start(ctx, Customers, ch, shopRedirect)
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "u-1", "email": "Ada@Example.com", "email_verified": true, "given_name": "Ada", "family_name": "Lovelace"})
	id, _, err := f.complete(ctx, Customers, ch, state, code, st.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "u-1" || id.Email != "ada@example.com" || !id.EmailVerified || id.FirstName != "Ada" || id.Issuer != idp.URL {
		t.Errorf("identity %+v", id)
	}
	if _, _, err := f.complete(ctx, Customers, ch, state, code, st.Binding); !errors.Is(err, ErrExpired) {
		t.Errorf("a replayed state: %v, want it spent", err)
	}
}

func TestASignInIsRefusedWhenAnythingDoesntMatch(t *testing.T) {
	idp := newFakeIdP(t)
	ctx, ch := context.Background(), uuid.New()
	verified := jwt.MapClaims{"sub": "u-1", "email": "ada@example.com", "email_verified": true}
	for name, tc := range map[string]struct {
		edit    func(jwt.MapClaims)
		binding func(string) string
		other   uuid.UUID
		want    error
	}{
		"another browser's binding": {binding: func(string) string { return "stolen" }, want: ErrRefused},
		"a replayed nonce":          {edit: func(c jwt.MapClaims) { c["nonce"] = "from-another-sign-in" }, want: ErrRefused},
		"a token for another app":   {edit: func(c jwt.MapClaims) { c["aud"] = "someone-else" }, want: ErrRefused},
		"an expired token":          {edit: func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }, want: ErrRefused},
		"another issuer":            {edit: func(c jwt.MapClaims) { c["iss"] = "https://evil.example" }, want: ErrRefused},
		"another store's state":     {other: uuid.New(), want: ErrRefused},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFlow(idp.settings(Customers))
			st, err := f.start(ctx, Customers, ch, shopRedirect)
			if err != nil {
				t.Fatal(err)
			}
			edits := []func(jwt.MapClaims){}
			if tc.edit != nil {
				edits = append(edits, tc.edit)
			}
			state, code := idp.authorize(t, st.AuthorizeURL, verified, edits...)
			binding, store := st.Binding, ch
			if tc.binding != nil {
				binding = tc.binding(binding)
			}
			if tc.other != uuid.Nil {
				store = tc.other
			}
			if _, _, err := f.complete(ctx, Customers, store, state, code, binding); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAStolenCodeIsUselessWithoutItsVerifier(t *testing.T) {
	idp := newFakeIdP(t)
	f, ctx, ch := newFlow(idp.settings(Customers)), context.Background(), uuid.New()
	mine, _ := f.start(ctx, Customers, ch, shopRedirect)
	theirs, _ := f.start(ctx, Customers, ch, shopRedirect)
	_, code := idp.authorize(t, theirs.AuthorizeURL, jwt.MapClaims{"sub": "u-2", "email": "eve@example.com", "email_verified": true})
	state, _ := idp.authorize(t, mine.AuthorizeURL, jwt.MapClaims{"sub": "u-1"})
	if _, _, err := f.complete(ctx, Customers, ch, state, code, mine.Binding); !errors.Is(err, ErrRefused) {
		t.Errorf("a code issued to another sign-in: %v", err)
	}
}

func TestSettingsDecideWhetherAndWhere(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	off := idp.settings(Admins)
	off[groupFor(Admins)+"oidc_enabled"] = "false"
	if _, err := newFlow(off).start(ctx, Admins, uuid.Nil, "http://localhost:5173/login/sso"); !errors.Is(err, ErrOff) {
		t.Errorf("switched off: %v", err)
	}
	s := idp.settings(Admins)
	f := newFlow(s)
	st, err := f.start(ctx, Admins, uuid.Nil, "http://localhost:5173/login/sso")
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true})
	s[groupFor(Admins)+"oidc_client_id"] = "rotated"
	if _, _, err := f.complete(ctx, Admins, uuid.Nil, state, code, st.Binding); !errors.Is(err, ErrExpired) {
		t.Errorf("a provider changed mid sign-in: %v", err)
	}
	for _, bad := range []string{"ftp://idp.example", "http://idp.example", "not a url"} {
		if checkIssuer(bad) == nil {
			t.Errorf("issuer %q was taken", bad)
		}
	}
}

func TestWhoAProviderIsTrustedFor(t *testing.T) {
	open := provider{}
	acme := provider{Domains: []string{"acme.example"}}
	for name, tc := range map[string]struct {
		p    provider
		id   Identity
		want bool
	}{
		"a verified address":                {open, Identity{Email: "a@x.example", EmailVerified: true}, true},
		"an unverified address":             {open, Identity{Email: "a@x.example"}, false},
		"the company's own domain":          {acme, Identity{Email: "ops@acme.example"}, true},
		"a verified address on another one": {acme, Identity{Email: "ops@gmail.example", EmailVerified: true}, false},
		"no address":                        {open, Identity{EmailVerified: true}, false},
	} {
		if got := tc.p.trusted(tc.id); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

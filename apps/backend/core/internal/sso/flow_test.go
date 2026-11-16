package sso

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

const shopRedirect = "http://localhost:3000/account/sso"

func newFlow(s Settings) *flow {
	return &flow{settings: s, tokens: &memTokens{m: map[string]auth.Token{}}, client: &http.Client{Timeout: 5 * time.Second}, providers: map[string]cachedProvider{}}
}

func TestASignInRoundTrips(t *testing.T) {
	idp := newFakeIdP(t)
	f, ctx, ch := newFlow(idp.settings(Customers)), context.Background(), uuid.New()
	st, err := f.start(ctx, Customers, ch, shopRedirect, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "u-1", "email": "Ada@Example.com", "email_verified": true, "given_name": "Ada", "family_name": "Lovelace"})
	id, _, err := f.complete(ctx, Customers, ch, state, code, st.Binding, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "u-1" || id.Email != "ada@example.com" || !id.EmailVerified || id.FirstName != "Ada" || id.Issuer != idp.URL {
		t.Errorf("identity %+v", id)
	}
	if _, _, err := f.complete(ctx, Customers, ch, state, code, st.Binding, uuid.Nil); !errors.Is(err, ErrExpired) {
		t.Errorf("a replayed state: %v, want it spent", err)
	}
}

func TestWhatTheProviderSaysAboutTheSignIn(t *testing.T) {
	idp := newFakeIdP(t)
	f, ctx, ch := newFlow(idp.settings(Customers)), context.Background(), uuid.New()
	st, _ := f.start(ctx, Customers, ch, shopRedirect, uuid.Nil)
	for key := range f.tokens.(*memTokens).m {
		if strings.Contains(key, st.State) {
			t.Errorf("the state is kept under its own value: %s", key)
		}
	}
	state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "u-1", "email": "ops@acme.example", "xms_edov": true, "amr": []string{"pwd", "mfa"}})
	id, _, err := f.complete(ctx, Customers, ch, state, code, st.Binding, uuid.Nil)
	if err != nil || !id.EmailVerified || !id.MFA {
		t.Errorf("an Entra domain owner with a second factor: %+v, %v", id, err)
	}
	st, _ = f.start(ctx, Customers, ch, shopRedirect, uuid.Nil)
	state, code = idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "u-1", "email": "ops@acme.example", "email_verified": "false", "amr": []string{"pwd"}})
	if id, _, err := f.complete(ctx, Customers, ch, state, code, st.Binding, uuid.Nil); err != nil || id.EmailVerified || id.MFA {
		t.Errorf("a password alone and an unchecked address: %+v, %v", id, err)
	}
}

func TestAStateStoreOutageIsNotAnExpiredSignIn(t *testing.T) {
	f := newFlow(fakeSettings{})
	f.tokens = brokenTokens{&memTokens{m: map[string]auth.Token{}}}
	if _, _, err := f.complete(context.Background(), Customers, uuid.New(), "s", "c", "b", uuid.Nil); err == nil || errors.Is(err, ErrExpired) {
		t.Errorf("got %v, want the outage itself", err)
	}
}

type brokenTokens struct{ *memTokens }

func (brokenTokens) Consume(context.Context, string) (auth.Token, error) {
	return auth.Token{}, errors.New("valkey down")
}

func TestTheProviderIsReachedOnlyOnPublicAddressesAndInSmallAnswers(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_WEBHOOK_TARGETS", "false")
	idp := newFakeIdP(t)
	f := newFlow(idp.settings(Customers))
	f.client = providerHTTP()
	if _, err := f.start(context.Background(), Customers, uuid.New(), shopRedirect, uuid.Nil); err == nil {
		t.Error("discovery reached a provider on a private address")
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 2*maxProviderBody))
	}))
	defer big.Close()
	res, err := (&http.Client{Transport: capped{http.DefaultTransport}}).Get(big.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if n, _ := io.Copy(io.Discard, res.Body); n != maxProviderBody {
		t.Errorf("read %d bytes of an oversized answer", n)
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
			st, err := f.start(ctx, Customers, ch, shopRedirect, uuid.Nil)
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
			if _, _, err := f.complete(ctx, Customers, store, state, code, binding, uuid.Nil); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAStolenCodeIsUselessWithoutItsVerifier(t *testing.T) {
	idp := newFakeIdP(t)
	f, ctx, ch := newFlow(idp.settings(Customers)), context.Background(), uuid.New()
	mine, _ := f.start(ctx, Customers, ch, shopRedirect, uuid.Nil)
	theirs, _ := f.start(ctx, Customers, ch, shopRedirect, uuid.Nil)
	_, code := idp.authorize(t, theirs.AuthorizeURL, jwt.MapClaims{"sub": "u-2", "email": "eve@example.com", "email_verified": true})
	state, _ := idp.authorize(t, mine.AuthorizeURL, jwt.MapClaims{"sub": "u-1"})
	if _, _, err := f.complete(ctx, Customers, ch, state, code, mine.Binding, uuid.Nil); !errors.Is(err, ErrRefused) {
		t.Errorf("a code issued to another sign-in: %v", err)
	}
}

func TestSettingsDecideWhetherAndWhere(t *testing.T) {
	idp := newFakeIdP(t)
	ctx := context.Background()
	off := idp.settings(Admins)
	off[groupFor(Admins)+"oidc_enabled"] = "false"
	if _, err := newFlow(off).start(ctx, Admins, uuid.Nil, "http://localhost:5173/login/sso", uuid.Nil); !errors.Is(err, ErrOff) {
		t.Errorf("switched off: %v", err)
	}
	s := idp.settings(Admins)
	f := newFlow(s)
	st, err := f.start(ctx, Admins, uuid.Nil, "http://localhost:5173/login/sso", uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	state, code := idp.authorize(t, st.AuthorizeURL, jwt.MapClaims{"sub": "a-1", "email": "ops@acme.example", "email_verified": true})
	s[groupFor(Admins)+"oidc_client_id"] = "rotated"
	if _, _, err := f.complete(ctx, Admins, uuid.Nil, state, code, st.Binding, uuid.Nil); !errors.Is(err, ErrExpired) {
		t.Errorf("a provider changed mid sign-in: %v", err)
	}
	for _, bad := range []string{"ftp://idp.example", "http://idp.example", "not a url", "https://login.microsoftonline.com/common/v2.0", "https://login.microsoftonline.com/Organizations/v2.0"} {
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
		"a verified address":                        {open, Identity{Email: "a@x.example", EmailVerified: true}, true},
		"an unverified address":                     {open, Identity{Email: "a@x.example"}, false},
		"the company's own domain, verified":        {acme, Identity{Email: "ops@acme.example", EmailVerified: true}, true},
		"the company's own domain, never confirmed": {acme, Identity{Email: "ops@acme.example"}, false},
		"a verified address on another one":         {acme, Identity{Email: "ops@gmail.example", EmailVerified: true}, false},
		"no address":                                {open, Identity{EmailVerified: true}, false},
	} {
		if got := tc.p.trusted(tc.id); got != tc.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestASecretNeverFollowsAnIssuerSetElsewhere(t *testing.T) {
	idp := newFakeIdP(t)
	s := idp.settings(Customers)
	g := groupFor(Customers)
	s[g+"oidc_issuer@channel"] = "https://their-idp.example"
	if _, ok := providerFor(context.Background(), s, Customers, uuid.New()); ok {
		t.Error("a channel's own issuer took the store-wide secret")
	}
	s[g+"oidc_client_secret@channel"] = "theirs"
	if p, ok := providerFor(context.Background(), s, Customers, uuid.New()); !ok || p.ClientSecret != "theirs" {
		t.Errorf("an issuer and secret set together on the channel: %+v, %v", p, ok)
	}
}

func TestADiscoveredSignInAddressIsHeldToHTTPS(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)//", "http://idp.example/authorize", ""} {
		if checkEndpoints(oauth2.Endpoint{AuthURL: bad, TokenURL: "https://idp.example/token"}) == nil {
			t.Errorf("authorization endpoint %q was taken", bad)
		}
	}
	if err := checkEndpoints(oauth2.Endpoint{AuthURL: "https://idp.example/authorize", TokenURL: "https://idp.example/token"}); err != nil {
		t.Error(err)
	}
}

func TestOneMethodIsntASecondFactor(t *testing.T) {
	for amr, want := range map[string]bool{"mfa": true, "pwd otp": true, "otp": false, "hwk": false, "pwd pwd": false, "": false} {
		if got := multiFactor(strings.Fields(amr)); got != want {
			t.Errorf("amr %q: %v", amr, got)
		}
	}
}

package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// An OpenID provider in-process: discovery, a key set, and a token endpoint that holds codes to the PKCE challenge
// and redirect they were issued for, the way a real one does
type fakeIdP struct {
	*httptest.Server
	key      *rsa.PrivateKey
	clientID string
	mu       sync.Mutex
	codes    map[string]issued
}

type issued struct {
	challenge, redirect string
	claims              jwt.MapClaims
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, clientID: "shop-client", codes: map[string]issued{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.URL, "authorization_endpoint": f.URL + "/authorize", "token_endpoint": f.URL + "/token", "jwks_uri": f.URL + "/jwks",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		b64 := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": "k1", "use": "sig", "alg": "RS256", "n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		got, ok := f.codes[r.Form.Get("code")]
		delete(f.codes, r.Form.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != got.challenge || r.Form.Get("redirect_uri") != got.redirect {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, got.claims)
		tok.Header["kid"] = "k1"
		signed, _ := tok.SignedString(key)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": signed})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeIdP) authorize(t *testing.T, authorizeURL string, claims jwt.MapClaims, edit ...func(jwt.MapClaims)) (state, code string) {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	c := jwt.MapClaims{"iss": f.URL, "aud": f.clientID, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": q.Get("nonce")}
	maps.Copy(c, claims)
	for _, e := range edit {
		e(c)
	}
	code = uuid.NewString()
	f.mu.Lock()
	f.codes[code] = issued{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), claims: c}
	f.mu.Unlock()
	return q.Get("state"), code
}

type memTokens struct {
	mu sync.Mutex
	m  map[string]auth.Token
}

func (m *memTokens) Put(_ context.Context, key string, t auth.Token, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = t
	return nil
}
func (m *memTokens) Get(_ context.Context, key string) (auth.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.m[key]
	if !ok {
		return auth.Token{}, auth.ErrTokenNotFound
	}
	return t, nil
}
func (m *memTokens) Consume(ctx context.Context, key string) (auth.Token, error) {
	t, err := m.Get(ctx, key)
	if err == nil {
		_ = m.Delete(ctx, key)
	}
	return t, err
}
func (m *memTokens) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
	return nil
}
func (m *memTokens) DeleteSubject(context.Context, string, uuid.UUID) error { return nil }

type fakeSettings map[string]string

func (f fakeSettings) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return f[path], nil
}
func (f fakeSettings) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return f[path] == "true", nil
}

func (f *fakeIdP) settings(a Audience) fakeSettings {
	g := groupFor(a)
	return fakeSettings{g + "oidc_enabled": "true", g + "oidc_issuer": f.URL, g + "oidc_client_id": f.clientID, g + "oidc_client_secret": "s3cret", g + "oidc_button_label": "Sign in with Acme"}
}

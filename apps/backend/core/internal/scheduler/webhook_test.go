package scheduler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

func testCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	c, err := secrets.NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func payloadOf(t *testing.T, c WebhookConfig) []byte {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A followed redirect would hand the job's API key and signature to whatever host the target names
func TestWebhookDoesNotFollowARedirect(t *testing.T) {
	var reached atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer elsewhere.Close()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	defer target.Close()

	r := newWebhookRunner(true)
	err := r.run(context.Background(), payloadOf(t, WebhookConfig{URL: target.URL, Headers: map[string]string{"X-Api-Key": "k"}}))
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("run = %v, want the redirect refused", err)
	}
	if reached.Load() != 0 {
		t.Error("the redirect target was called")
	}
}

func TestWebhookSealsItsSecretsAndSendsThemOpened(t *testing.T) {
	var gotKey, gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotSig = r.Header.Get("X-Api-Key"), r.Header.Get("X-Job-Signature-256")
	}))
	defer srv.Close()

	r := newWebhookRunner(true)
	r.cipher = testCipher(t)
	sealed, err := r.seal(WebhookConfig{URL: srv.URL, Method: "POST", Body: `{"a":1}`, Secret: "sign-me", Headers: map[string]string{"X-Api-Key": "api-key"}})
	if err != nil {
		t.Fatal(err)
	}
	stored := string(payloadOf(t, sealed))
	if strings.Contains(stored, "sign-me") || strings.Contains(stored, "api-key") {
		t.Fatalf("stored payload holds a plaintext secret: %s", stored)
	}
	if err := r.run(context.Background(), []byte(stored)); err != nil {
		t.Fatalf("run: %v", err)
	}
	mac := hmac.New(sha256.New, []byte("sign-me"))
	mac.Write([]byte(`{"a":1}`))
	if gotKey != "api-key" || gotSig != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
		t.Errorf("receiver got key %q sig %q, want the opened header and a signature over the body", gotKey, gotSig)
	}
}

func TestASealedWebhookNeedsTheKeyToRun(t *testing.T) {
	sealer := newWebhookRunner(true)
	sealer.cipher = testCipher(t)
	sealed, err := sealer.seal(WebhookConfig{URL: "http://127.0.0.1:1/x", Secret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if err := newWebhookRunner(true).run(context.Background(), payloadOf(t, sealed)); err == nil || !strings.Contains(err.Error(), "no encryption key") {
		t.Errorf("run without the key = %v, want it refused by name", err)
	}
}

func TestValidateMethod(t *testing.T) {
	for _, m := range []string{"", "get", " PUT ", "DELETE"} {
		if err := validateMethod(m); err != nil {
			t.Errorf("validateMethod(%q) = %v", m, err)
		}
	}
	for _, m := range []string{"TRACE", "CONNECT", "G E T"} {
		if validateMethod(m) == nil {
			t.Errorf("validateMethod(%q) accepted", m)
		}
	}
}

// Half a character is invalid UTF-8, which Postgres refuses - and a refused result leaves the job leased
func TestTruncateKeepsCharactersWhole(t *testing.T) {
	s := strings.Repeat("a", 1999) + "é" // é spans bytes 1999-2000
	got := truncate(s, 2000)
	if !utf8.ValidString(got) || len(got) > 2000 || got != strings.Repeat("a", 1999) {
		t.Errorf("truncate split a character: %q (%d bytes)", got[len(got)-3:], len(got))
	}
	if truncate("short", 2000) != "short" {
		t.Error("truncate changed a string under the limit")
	}
}

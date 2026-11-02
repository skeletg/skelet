package trackinggoogleads

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

var update = flag.Bool("update", false, "rewrite the golden request files")

func purchase() sdk.ShopEvent {
	return sdk.ShopEvent{
		ID: "0193a1b2-0000-7000-8000-000000000042", Name: "purchase", OccurredAt: time.Now().Add(-time.Hour).Truncate(time.Second),
		Ecommerce: &sdk.ShopEcommerce{Currency: "EUR", Value: 45, Tax: 9, Shipping: 5, TransactionID: "ORD-000042"},
		User:      sdk.ShopUser{Email: " Cloudy.SanFrancisco+shopping@gmail.com ", Phone: "+1 (800) 555-0100"},
		ClickIDs:  map[string]string{"gclid": "Cj0KCQ-test", "wbraid": "wb-test"},
		Consent:   &sdk.ShopConsent{Ads: true, Personalization: false},
	}
}

// A Google stand-in: the token endpoint checks the service account's signed assertion, the ingest endpoint records
func google(t *testing.T, ingestStatus int) (*http.Client, *[]byte, *http.Request, *int32, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	var body []byte
	var ingest http.Request
	var tokenCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		_ = r.ParseForm()
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || len(parts) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
		if rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig) != nil || !strings.Contains(string(claims), `"scope":"`+scope+`"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"ya29.test","expires_in":3600}`)
	})
	mux.HandleFunc("/v1/events:ingest", func(w http.ResponseWriter, r *http.Request) {
		ingest = *r
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(ingestStatus)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	prev := apiHost
	apiHost = srv.URL
	t.Cleanup(func() { apiHost = prev })
	creds, _ := json.Marshal(map[string]string{
		"type": "service_account", "client_email": "ads@example.iam.gserviceaccount.com", "token_uri": srv.URL + "/token",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	return srv.Client(), &body, &ingest, &tokenCalls, string(creds)
}

func config(creds string) sdk.DestinationConfig {
	return sdk.DestinationConfig{ID: uuid.New(), Config: map[string]string{"customer_id": "123-456-7890", "conversion_action_id": "987654321"}, Secret: creds}
}

// The request a purchase becomes, held to a golden file in the shape of the Data Manager API's offline conversion example
func TestAPurchaseIsAnImportedClickConversion(t *testing.T) {
	client, body, req, _, creds := google(t, http.StatusOK)
	ev := purchase()
	ev.OccurredAt = time.Now().Add(-time.Hour)
	if err := Send(context.Background(), client, config(creds), ev); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v1/events:ingest" || req.Header.Get("Authorization") != "Bearer ya29.test" {
		t.Errorf("request %s, auth %q", req.URL, req.Header.Get("Authorization"))
	}
	var sent map[string]any
	_ = json.Unmarshal(*body, &sent)
	sent["events"].([]any)[0].(map[string]any)["eventTimestamp"] = "2026-11-01T10:00:00Z" // the golden file can't hold "an hour ago"
	got, _ := json.MarshalIndent(sent, "", "  ")
	const golden = "testdata/purchase.request.json"
	if *update {
		_ = os.WriteFile(golden, append(got, '\n'), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil || strings.TrimSpace(string(want)) != string(got) {
		t.Errorf("request body drifted from %s (run with -update after checking it):\n%s", golden, got)
	}
	if strings.Contains(string(*body), "gmail") || strings.Contains(string(*body), "555") {
		t.Error("an email or phone went to Google unhashed")
	}
}

// Google's own examples from its formatting guide
func TestNormalizationFollowsGoogle(t *testing.T) {
	for in, want := range map[string]string{
		"cloudy.sanfrancisco+shopping@gmail.com": "cloudysanfrancisco@gmail.com",
		"user.name+NYC@Example.com":              "user.name+nyc@example.com",
		" A.B@GoogleMail.com ":                   "ab@googlemail.com",
		"not-an-email":                           "",
	} {
		if got := normalizeEmail(in); got != want {
			t.Errorf("normalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
	if got := "+" + phoneDigits("(800) 555-0100 "); got != "+8005550100" {
		t.Errorf("phone %q", got)
	}
	if got := "+" + phoneDigits("+1 800-555-0100"); got != "+18005550100" {
		t.Errorf("phone %q", got)
	}
}

func TestConsentAndWhatCanBeMatched(t *testing.T) {
	e, ok := toEvent(purchase())
	if !ok || e.Consent != (consent{AdUserData: "CONSENT_GRANTED", AdPersonalization: "CONSENT_DENIED"}) {
		t.Errorf("consent %+v", e.Consent)
	}
	if e.AdIdentifiers.Gclid != "Cj0KCQ-test" || e.AdIdentifiers.Wbraid != "wb-test" || e.ConversionValue != 59 {
		t.Errorf("event %+v", e)
	}
	ev := purchase()
	ev.ClickIDs, ev.User = nil, sdk.ShopUser{}
	if _, ok := toEvent(ev); ok {
		t.Error("a purchase with no click id, email or phone was sent - Google can match it to nobody")
	}
	ev.User.Email = "a@example.com"
	if e, ok := toEvent(ev); !ok || e.AdIdentifiers != nil || len(e.UserData.UserIdentifiers) != 1 {
		t.Errorf("an email alone should be an enhanced conversion: %+v", e)
	}
}

func TestTheTokenIsReusedUntilItExpires(t *testing.T) {
	client, _, _, calls, creds := google(t, http.StatusOK)
	d := config(creds)
	for range 3 {
		if err := Send(context.Background(), client, d, purchase()); err != nil {
			t.Fatal(err)
		}
	}
	if *calls != 1 {
		t.Errorf("%d token exchanges for three sends", *calls)
	}
}

func TestAnAuthorizedUserRefreshTokenWorks(t *testing.T) {
	var form string
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.Form.Encode()
		_, _ = io.WriteString(w, `{"access_token":"ya29.user","expires_in":3600}`)
	}))
	defer tok.Close()
	prev := googleToken
	googleToken = tok.URL
	defer func() { googleToken = prev }()
	got, err := accessToken(context.Background(), tok.Client(), uuid.NewString(), `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r"}`)
	if err != nil || got != "ya29.user" || !strings.Contains(form, "grant_type=refresh_token") || !strings.Contains(form, "refresh_token=r") {
		t.Errorf("token %q, err %v, form %s", got, err, form)
	}
}

func TestRefusalsAreClassified(t *testing.T) {
	client, _, _, _, creds := google(t, http.StatusBadRequest)
	if err := Send(context.Background(), client, config(creds), purchase()); !sdk.IsPermanent(err) {
		t.Errorf("a 400 should not be retried: %v", err)
	}
	client, _, _, _, creds = google(t, http.StatusTooManyRequests)
	if err := Send(context.Background(), client, config(creds), purchase()); err == nil || sdk.IsPermanent(err) {
		t.Errorf("a 429 should be retried: %v", err)
	}
	for name, secret := range map[string]string{"not json": "nope", "unknown type": `{"type":"external_account"}`, "no key": `{"type":"service_account","client_email":"x"}`} {
		if _, err := accessToken(context.Background(), client, uuid.NewString(), secret); !sdk.IsPermanent(err) {
			t.Errorf("%s: bad credentials should fail for good: %v", name, err)
		}
	}
	d := config(creds)
	d.Config["conversion_action_id"] = ""
	if err := Send(context.Background(), client, d, purchase()); !sdk.IsPermanent(err) {
		t.Errorf("a missing conversion action should fail for good: %v", err)
	}
	old := purchase()
	old.OccurredAt = time.Now().Add(-100 * 24 * time.Hour)
	if err := Send(context.Background(), client, config(creds), old); !sdk.IsPermanent(err) {
		t.Errorf("a purchase past the click id's 90 days should fail for good: %v", err)
	}
	view := purchase()
	view.Name = "view_item"
	if err := Send(context.Background(), client, config(creds), view); err != nil {
		t.Errorf("only purchases are conversions: %v", err)
	}
}

package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestTheAdminViewMasksHeaderValues(t *testing.T) {
	got := MaskHeaders(map[string]string{"X-Api-Key": "tok_live"})
	if got["X-Api-Key"] != MaskedHeaderValue {
		t.Fatalf("MaskHeaders = %v", got)
	}
	merged := keepMasked(map[string]string{"X-Api-Key": MaskedHeaderValue, "X-New": "v", "X-Gone": MaskedHeaderValue}, map[string]string{"X-Api-Key": "tok_live"})
	if merged["X-Api-Key"] != "tok_live" || merged["X-New"] != "v" || len(merged) != 2 {
		t.Fatalf("keepMasked = %v, want the stored token kept, the new header added and the unknown mask dropped", merged)
	}
}

// A receiver that redirects must not get the merchant's token sent on to another host, and the log must not keep it
func TestADeliveryNeitherFollowsRedirectsNorLogsHeaderValues(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	var elsewhere, gotToken string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere = r.Header.Get("X-Api-Key") }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Api-Key")
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer srv.Close()
	wh := syncWebhook(srv.URL, FailOpen, 0)
	wh.CustomHeaders = map[string]string{"X-Api-Key": "tok_live"}
	d, repo := dispatcherFor(wh)
	_ = d.dispatchSync(context.Background(), &plainEvent{"test.eventing"})

	if gotToken != "tok_live" {
		t.Fatalf("the receiver got %q, want the real token", gotToken)
	}
	if elsewhere != "" {
		t.Fatal("the redirect was followed with the token")
	}
	if len(repo.deliveries) != 1 || repo.deliveries[0].Success {
		t.Fatalf("deliveries = %+v, want one failed 3xx", repo.deliveries)
	}
	if v := repo.deliveries[0].RequestHeaders["X-Api-Key"]; v != "[redacted]" {
		t.Errorf("logged header = %q, want [redacted]", v)
	}
}

func TestSecretsAndHeadersAreSealedAtRest(t *testing.T) {
	conn := testdb.Require(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	cipher, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db.New(conn), txrunner.New(conn), cipher)
	ctx := context.Background()
	w, err := repo.Create(ctx, CreateInput{Name: "t", TargetURL: "https://hooks.test", Kind: KindAsync, EventTypes: []string{"order.placed"},
		FailurePolicy: FailClosed, PayloadVersion: PayloadVersionV1, CustomHeaders: map[string]string{"X-Api-Key": "tok_live"}}, "whsec_plain")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM webhooks WHERE id = $1`, w.ID) })

	var storedSecret, storedHeaders string
	if err := conn.QueryRow(`SELECT signing_secret, custom_headers::text FROM webhooks WHERE id = $1`, w.ID).Scan(&storedSecret, &storedHeaders); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedSecret, "whsec_plain") || strings.Contains(storedHeaders, "tok_live") {
		t.Fatalf("stored in the clear: secret %q headers %q", storedSecret, storedHeaders)
	}
	if got, _ := repo.GetSigningSecret(ctx, w.ID); got != "whsec_plain" {
		t.Errorf("GetSigningSecret = %q", got)
	}
	if got, _ := repo.GetByID(ctx, w.ID); got.CustomHeaders["X-Api-Key"] != "tok_live" {
		t.Errorf("GetByID headers = %v", got.CustomHeaders)
	}
	updated, err := repo.Update(ctx, UpdateInput{ID: w.ID, CustomHeaders: map[string]string{"X-Api-Key": MaskedHeaderValue, "X-Env": "prod"}})
	if err != nil || updated.CustomHeaders["X-Api-Key"] != "tok_live" || updated.CustomHeaders["X-Env"] != "prod" {
		t.Fatalf("Update with the mask = %v, %v - want the token kept", updated.CustomHeaders, err)
	}
}

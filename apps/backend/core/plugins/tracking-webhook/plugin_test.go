package trackingwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A receiver verifying the way it verifies the store's webhooks must accept the event as sent
func TestTheEventArrivesSignedLikeTheStoresWebhooks(t *testing.T) {
	var got sdk.ShopEvent
	verified := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mac := hmac.New(sha256.New, []byte("whsec"))
		mac.Write([]byte(r.Header.Get("X-Webhook-Timestamp") + "." + string(body)))
		verified = hmac.Equal([]byte(r.Header.Get("X-Webhook-Signature-256")), []byte("sha256="+hex.EncodeToString(mac.Sum(nil))))
		_ = json.Unmarshal(body, &got)
		if r.Header.Get("X-Webhook-Event") != "tracking.purchase" || r.Header.Get("X-Webhook-Id") != "o-1" {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	ev := sdk.ShopEvent{ID: "o-1", Name: "purchase", OccurredAt: time.Now().UTC(), Ecommerce: &sdk.ShopEcommerce{Currency: "EUR", Value: 45}}
	d := sdk.DestinationConfig{Config: map[string]string{"url": srv.URL}, Secret: "whsec"}
	if err := Send(context.Background(), srv.Client(), d, ev); err != nil {
		t.Fatal(err)
	}
	if !verified || got.ID != "o-1" || got.Ecommerce.Value != 45 {
		t.Errorf("verified %v, received %+v", verified, got)
	}
}

func TestAReceiverRefusalIsFinalAndItsOutageIsRetried(t *testing.T) {
	for status, permanent := range map[int]bool{http.StatusUnauthorized: true, http.StatusServiceUnavailable: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		err := Send(context.Background(), srv.Client(), sdk.DestinationConfig{Config: map[string]string{"url": srv.URL}}, sdk.ShopEvent{ID: "e", Name: "view_item"})
		srv.Close()
		if err == nil || sdk.IsPermanent(err) != permanent {
			t.Errorf("%d: %v, want permanent=%v", status, err, permanent)
		}
	}
}

package feedwebhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/lib/feedtransport"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestCatalogueWebhookSignsTheCompleteFileAndClassifiesRefusals(t *testing.T) {
	const payload = "{\"sku\":\"MUG\"}\n{\"sku\":\"TEE\"}\n"
	status := http.StatusNoContent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(body) != payload || r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("wrong catalogue: %s, %v", body, r.Header)
		}
		mac := hmac.New(sha256.New, []byte("test-secret"))
		_, _ = io.WriteString(mac, r.Header.Get("X-Skelet-Timestamp")+".")
		_, _ = mac.Write(body)
		if r.Header.Get("X-Skelet-Signature") != "sha256="+hex.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid signature")
		}
		w.WriteHeader(status)
	}))
	defer server.Close()
	d := sdk.DestinationConfig{ID: uuid.New(), ChannelID: uuid.New(), Config: map[string]string{"endpoint": server.URL}, Secret: "test-secret"}
	file := sdk.ProductFeedFile{Name: "catalog.jsonl", ContentType: "application/x-ndjson", Size: int64(len(payload)), Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(payload)), nil }}
	client := feedtransport.New(true)
	for _, code := range []int{204, 400, 503} {
		status = code
		err := Send(context.Background(), client, d, file)
		if code == 204 && err != nil {
			t.Fatal(err)
		}
		if code == 400 && !sdk.IsPermanent(err) {
			t.Fatalf("400 must not retry: %v", err)
		}
		if code == 503 && (err == nil || sdk.IsPermanent(err)) {
			t.Fatalf("503 must retry: %v", err)
		}
	}
}

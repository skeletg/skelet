package media

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// Gated live check against dev Garage - the size cap only exists if the store actually enforces the signed Content-Length, so prove it
// end-to-end rather than trusting the SDK. Run: PRESIGN_LIVECHECK=1 go test ./internal/media/ -run LiveCheck
func TestPresignContentLengthLiveCheck(t *testing.T) {
	if os.Getenv("PRESIGN_LIVECHECK") != "1" {
		t.Skip("set PRESIGN_LIVECHECK=1 (needs dev Garage on localhost:9900)")
	}
	ctx := context.Background()
	store, err := New(ctx, Config{
		Bucket:           "media",
		InternalEndpoint: "http://localhost:9900",
		UsePathStyle:     true,
		AccessKeyID:      "skeletdev",
		SecretAccessKey:  "skeletdev-secret-not-for-prod",
	})
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	key := "livecheck/presign-" + time.Now().Format("150405") + ".bin"
	t.Cleanup(func() { _ = store.Delete(ctx, key) })

	url, err := store.PresignUpload(ctx, key, "application/octet-stream", 5, time.Minute)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}
	put := func(body []byte) int {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	if code := put([]byte("1234567890")); code < 400 {
		t.Fatalf("oversized PUT accepted with %d - signed Content-Length not enforced", code)
	}
	if code := put([]byte("12345")); code != 200 {
		t.Fatalf("exact-size PUT rejected with %d", code)
	}
}

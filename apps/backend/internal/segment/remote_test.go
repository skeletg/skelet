package segment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

func segSource(t *testing.T, url string) RemoteSegmentSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindCustomerSegments, Slug: "cdp", Name: "CDP", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteSegmentSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteSegments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"segments": ["vip", "lapsed"]}`))
	}))
	defer srv.Close()

	slugs, ok := segSource(t, srv.URL).Slugs(context.Background(), uuid.New())
	if !ok || len(slugs) != 2 || slugs[0] != "vip" {
		t.Fatalf("segments: ok=%v slugs=%v", ok, slugs)
	}
}

func TestRemoteSegmentsFailOpen(t *testing.T) {
	if _, ok := segSource(t, "http://127.0.0.1:9/down").Slugs(context.Background(), uuid.New()); ok {
		t.Fatal("down provider should fall back to native (ok=false)")
	}
}

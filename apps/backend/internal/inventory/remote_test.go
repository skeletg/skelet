package inventory

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/internal/remoteprovider"
)

func availSource(t *testing.T, url string) RemoteAvailabilitySource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindInventoryAvail, Slug: "wms", Name: "WMS", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteAvailabilitySource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteAvailability(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"salable": {"%s": 12, "%s": -1}}`, a, b)
	}))
	defer srv.Close()

	m, ok := availSource(t, srv.URL).Salable(context.Background(), []uuid.UUID{a, b}, uuid.New())
	if !ok || m[a] != 12 || m[b] != -1 {
		t.Fatalf("availability: ok=%v m=%v", ok, m)
	}
}

func TestRemoteAvailabilityFailOpen(t *testing.T) {
	if _, ok := availSource(t, "http://127.0.0.1:9/down").Salable(context.Background(), []uuid.UUID{uuid.New()}, uuid.New()); ok {
		t.Fatal("down provider should fall back to native (ok=false)")
	}
}

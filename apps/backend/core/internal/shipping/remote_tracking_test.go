package shipping

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func trackingSource(t *testing.T, url string) RemoteTrackingSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindShippingTracking, Slug: "aftership-ish", Name: "Tracking", TargetURL: url,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteTrackingSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteTracking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"in_transit","url":"https://track/1","events":[{"time":"2026-07-26T10:00:00Z","status":"picked_up","location":"LA"}]}`))
	}))
	defer srv.Close()

	res, ok := trackingSource(t, srv.URL).Track(context.Background(), "ups", "1Z999")
	if !ok || res.Status != "in_transit" || res.URL != "https://track/1" {
		t.Fatalf("tracking: ok=%v res=%+v", ok, res)
	}
	if len(res.Events) != 1 || res.Events[0].Status != "picked_up" || res.Events[0].Location != "LA" {
		t.Fatalf("events not mapped: %+v", res.Events)
	}
	if res.Events[0].Time.IsZero() {
		t.Fatal("event time not parsed")
	}
}

func TestRemoteTrackingFailOpen(t *testing.T) {
	if _, ok := trackingSource(t, "http://127.0.0.1:9/down").Track(context.Background(), "ups", "1Z999"); ok {
		t.Fatal("down provider should fall back to the carrier (ok=false)")
	}
}

package order

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func returnsPolicy(t *testing.T, url string, policy remoteprovider.FailurePolicy) RemoteReturnsPolicy {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindReturnsPolicy, Slug: "loop", Name: "Loop", TargetURL: url, FailurePolicy: policy,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteReturnsPolicy(svc, remoteprovider.NewInvoker(svc, nil))
}

func returnInput() RemoteReturnInput {
	return RemoteReturnInput{
		OrderID: uuid.New(), Resolution: "refund", RefundTarget: "original",
		Lines: []RemoteReturnLine{{OrderLineID: uuid.New(), Quantity: 1, Reason: "defective"}},
	}
}

func TestReturnsPolicyAllows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"allowed": true}`))
	}))
	defer srv.Close()
	allowed, _ := returnsPolicy(t, srv.URL, remoteprovider.FailOpen).Allowed(context.Background(), returnInput())
	if !allowed {
		t.Fatal("expected allowed")
	}
}

func TestReturnsPolicyDenies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"allowed": false, "reason": "outside 30-day window"}`))
	}))
	defer srv.Close()
	allowed, reason := returnsPolicy(t, srv.URL, remoteprovider.FailOpen).Allowed(context.Background(), returnInput())
	if allowed || reason != "outside 30-day window" {
		t.Fatalf("expected denial: allowed=%v reason=%q", allowed, reason)
	}
}

func TestReturnsPolicyFailOpenAllows(t *testing.T) {
	allowed, _ := returnsPolicy(t, "http://127.0.0.1:9/down", remoteprovider.FailOpen).Allowed(context.Background(), returnInput())
	if !allowed {
		t.Fatal("fail_open should allow when the policy service is down")
	}
}

func TestReturnsPolicyFailClosedDenies(t *testing.T) {
	allowed, reason := returnsPolicy(t, "http://127.0.0.1:9/down", remoteprovider.FailClosed).Allowed(context.Background(), returnInput())
	if allowed || reason == "" {
		t.Fatalf("fail_closed should deny on outage: allowed=%v reason=%q", allowed, reason)
	}
}

package order

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

func fraudStack(t *testing.T, url string, policy remoteprovider.FailurePolicy) RemoteFraudSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindFraudRisk, Slug: "screen", Name: "Screen", TargetURL: url, FailurePolicy: policy,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteFraudSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func fraudInput() RemoteFraudInput {
	return RemoteFraudInput{
		ChannelID: uuid.New(), OrderID: uuid.New(), Email: "x@example.com", Currency: "USD",
		SubtotalCents: 10000, Shipping: OrderAddress{CountryCode: "US"}, Billing: OrderAddress{CountryCode: "US"},
		Lines: []RemoteTaxLine{{SKU: "SKU-1", Quantity: 1, UnitPriceCents: 10000}},
	}
}

func TestFraudApproveWithScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decision": "approve", "score": 42}`))
	}))
	defer srv.Close()
	score, reject, _, err := fraudStack(t, srv.URL, remoteprovider.FailOpen).Screen(context.Background(), fraudInput())
	if err != nil || reject || score != 42 {
		t.Fatalf("approve: score=%d reject=%v err=%v", score, reject, err)
	}
}

func TestFraudReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"decision": "reject", "score": 95, "reason": "stolen card"}`))
	}))
	defer srv.Close()
	_, reject, reason, err := fraudStack(t, srv.URL, remoteprovider.FailOpen).Screen(context.Background(), fraudInput())
	if err != nil || !reject || reason != "stolen card" {
		t.Fatalf("reject: reject=%v reason=%q err=%v", reject, reason, err)
	}
}

func TestFraudFailOpenApproves(t *testing.T) {
	src := fraudStack(t, "http://127.0.0.1:9/down", remoteprovider.FailOpen)
	score, reject, _, err := src.Screen(context.Background(), fraudInput())
	if err != nil || reject || score != 0 {
		t.Fatalf("fail_open should approve with 0 score: score=%d reject=%v err=%v", score, reject, err)
	}
}

func TestFraudFailClosedRejects(t *testing.T) {
	src := fraudStack(t, "http://127.0.0.1:9/down", remoteprovider.FailClosed)
	_, reject, reason, err := src.Screen(context.Background(), fraudInput())
	if err != nil || !reject || reason == "" {
		t.Fatalf("fail_closed should reject: reject=%v reason=%q err=%v", reject, reason, err)
	}
}

func TestFraudNoProviderNoOp(t *testing.T) {
	// service with no fraud providers registered
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	src := NewRemoteFraudSource(svc, remoteprovider.NewInvoker(svc, nil))
	score, reject, _, err := src.Screen(context.Background(), fraudInput())
	if err != nil || reject || score != 0 {
		t.Fatalf("no provider should be a no-op: score=%d reject=%v err=%v", score, reject, err)
	}
}

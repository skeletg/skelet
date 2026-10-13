package wiring

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

func pricingStack(t *testing.T, url string, policy remoteprovider.FailurePolicy) cart.RemotePricingSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if url != "" {
		if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
			Kind: remoteprovider.KindPricing, Slug: "contract", Name: "Contract", TargetURL: url, FailurePolicy: policy,
		}); err != nil {
			t.Fatalf("create provider: %v", err)
		}
	}
	return NewCartRemotePricing(svc, remoteprovider.NewInvoker(svc, nil))
}

func answering(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestCartRemotePricing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		url     string
		policy  remoteprovider.FailurePolicy
		want    int64
		wantOK  bool
		wantErr bool
	}{
		{"an override replaces the price", answering(t, `{"unitPriceCents": 7500}`), remoteprovider.FailOpen, 7500, true, false},
		{"no unitPriceCents declines", answering(t, `{}`), remoteprovider.FailOpen, 0, false, false},
		{"a negative price declines", answering(t, `{"unitPriceCents": -1}`), remoteprovider.FailOpen, 0, false, false},
		{"fail_open keeps the catalog on an outage", "http://127.0.0.1:9/down", remoteprovider.FailOpen, 0, false, false},
		{"fail_closed aborts on an outage", "http://127.0.0.1:9/down", remoteprovider.FailClosed, 0, false, true},
		{"no provider is the catalog", "", remoteprovider.FailOpen, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cents, ok, err := pricingStack(t, tc.url, tc.policy).Price(context.Background(), uuid.New(), "SKU-1", nil, 10000)
			if (err != nil) != tc.wantErr || ok != tc.wantOK || cents != tc.want {
				t.Fatalf("got (%d, %v, %v), want (%d, %v, err=%v)", cents, ok, err, tc.want, tc.wantOK, tc.wantErr)
			}
		})
	}
}

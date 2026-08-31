package currency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/remoteprovider"
)

func rateStack(t *testing.T, url string, policy remoteprovider.FailurePolicy) RemoteRateSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindCurrencyRate, Slug: "fx", Name: "FX", TargetURL: url, FailurePolicy: policy,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewRemoteRateSource(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestRemoteRatesReturnsRates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"base": "USD", "rates": {"EUR": 0.92, "GBP": 0.79}}`))
	}))
	defer srv.Close()

	rates, ok, err := rateStack(t, srv.URL, remoteprovider.FailOpen).Rates(context.Background(), "USD", []string{"EUR", "GBP"})
	if err != nil || !ok {
		t.Fatalf("rates: ok=%v err=%v", ok, err)
	}
	if rates["EUR"] != 0.92 || rates["GBP"] != 0.79 {
		t.Fatalf("rates wrong: %v", rates)
	}
}

func TestRemoteRatesFailOpen(t *testing.T) {
	_, ok, err := rateStack(t, "http://127.0.0.1:9/down", remoteprovider.FailOpen).Rates(context.Background(), "USD", []string{"EUR"})
	if err != nil || ok {
		t.Fatalf("fail_open should fall back (ok=false, no err), got ok=%v err=%v", ok, err)
	}
}

func TestRemoteRatesFailClosed(t *testing.T) {
	_, _, err := rateStack(t, "http://127.0.0.1:9/down", remoteprovider.FailClosed).Rates(context.Background(), "USD", []string{"EUR"})
	if err == nil {
		t.Fatal("fail_closed should surface the error to abort the refresh")
	}
}

func TestRemoteRatesNoProvider(t *testing.T) {
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	src := NewRemoteRateSource(svc, remoteprovider.NewInvoker(svc, nil))
	_, ok, err := src.Rates(context.Background(), "USD", []string{"EUR"})
	if err != nil || ok {
		t.Fatalf("no provider -> built-in feed (ok=false): ok=%v err=%v", ok, err)
	}
}

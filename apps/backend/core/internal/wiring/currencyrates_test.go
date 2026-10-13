package wiring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func fxStack(t *testing.T, url string, policy remoteprovider.FailurePolicy) currency.RemoteRateSource {
	t.Helper()
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	svc.AllowPrivateTargets = true
	if _, err := svc.Create(context.Background(), remoteprovider.CreateInput{
		Kind: remoteprovider.KindCurrencyRate, Slug: "fx", Name: "FX", TargetURL: url, FailurePolicy: policy,
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	return NewCurrencyRemoteRates(svc, remoteprovider.NewInvoker(svc, nil))
}

func TestCurrencyRemoteRatesReturnsRates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"base": "USD", "rates": {"EUR": 0.92, "GBP": 0.79}}`))
	}))
	defer srv.Close()

	quote, ok, err := fxStack(t, srv.URL, remoteprovider.FailOpen).Rates(context.Background(), "USD", []string{"EUR", "GBP"})
	if err != nil || !ok {
		t.Fatalf("rates: ok=%v err=%v", ok, err)
	}
	if quote.Rates["EUR"] != 0.92 || quote.Rates["GBP"] != 0.79 || quote.Base != "USD" {
		t.Fatalf("rates wrong: %+v", quote)
	}
}

func TestCurrencyRemoteRatesFailOpen(t *testing.T) {
	_, ok, err := fxStack(t, "http://127.0.0.1:9/down", remoteprovider.FailOpen).Rates(context.Background(), "USD", []string{"EUR"})
	if err != nil || ok {
		t.Fatalf("fail_open should fall back (ok=false, no err), got ok=%v err=%v", ok, err)
	}
}

func TestCurrencyRemoteRatesFailClosed(t *testing.T) {
	_, _, err := fxStack(t, "http://127.0.0.1:9/down", remoteprovider.FailClosed).Rates(context.Background(), "USD", []string{"EUR"})
	if err == nil {
		t.Fatal("fail_closed should surface the error to abort the refresh")
	}
}

func TestCurrencyRemoteRatesNoProvider(t *testing.T) {
	svc := remoteprovider.NewService(remoteprovider.NewMemRepo())
	src := NewCurrencyRemoteRates(svc, remoteprovider.NewInvoker(svc, nil))
	_, ok, err := src.Rates(context.Background(), "USD", []string{"EUR"})
	if err != nil || ok {
		t.Fatalf("no provider -> built-in feed (ok=false): ok=%v err=%v", ok, err)
	}
}

type baseReader struct {
	value string
	err   error
}

func (b baseReader) GetString(context.Context, string, config.Scope) (string, error) {
	return b.value, b.err
}

type refreshSpy chan string

func (r refreshSpy) RefreshRates(_ context.Context, base string) error {
	r <- base
	return nil
}

func TestCurrencyRefreshReturnsTheBaseReadError(t *testing.T) {
	spy := make(refreshSpy, 1)
	readErr := errors.New("settings down")
	if err := refreshCurrencyRates(context.Background(), baseReader{err: readErr}, spy); !errors.Is(err, readErr) {
		t.Fatalf("err = %v, want the read error", err)
	}
	if len(spy) != 0 {
		t.Fatalf("refreshed against %q after a failed read", <-spy)
	}
	if err := refreshCurrencyRates(context.Background(), baseReader{}, spy); err != nil {
		t.Fatal(err)
	}
	if got := <-spy; got != config.DefaultBaseCurrency {
		t.Errorf("empty setting refreshed against %q, want the default", got)
	}
}

type changeHooks []func(context.Context, config.Definition)

func (h *changeHooks) OnChange(fn func(context.Context, config.Definition)) { *h = append(*h, fn) }

func TestABaseCurrencyChangeRefreshesRates(t *testing.T) {
	var hooks changeHooks
	spy := make(refreshSpy, 1)
	registerBaseCurrencyRefresh(&hooks, baseReader{value: "EUR"}, spy)
	if len(hooks) != 1 {
		t.Fatalf("registered %d hooks", len(hooks))
	}
	ctx, cancel := context.WithCancel(context.Background())
	hooks[0](ctx, config.Definition{Path: "general/store_information/support_phone"})
	hooks[0](ctx, config.Definition{Path: config.BaseCurrencyPath})
	cancel()
	select {
	case got := <-spy:
		if got != "EUR" {
			t.Errorf("refreshed against %q, want EUR", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a base change never refreshed the rates")
	}
	select {
	case got := <-spy:
		t.Errorf("an unrelated setting refreshed rates against %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

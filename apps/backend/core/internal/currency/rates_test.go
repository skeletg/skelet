package currency

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }

func TestConvert(t *testing.T) {
	usd := Currency{Code: "USD", Decimals: 2, ExchangeRate: f64(1)}
	eur := Currency{Code: "EUR", Decimals: 2, ExchangeRate: f64(0.9)}
	jpy := Currency{Code: "JPY", Decimals: 0, ExchangeRate: f64(150)}
	unset := Currency{Code: "GBP", Decimals: 2}

	tests := []struct {
		name   string
		amount int64
		from   Currency
		to     Currency
		want   int64
		ok     bool
	}{
		{"same code passthrough", 999, usd, usd, 999, true},
		{"usd to eur", 10000, usd, eur, 9000, true},                    // $100.00 -> 90.00 EUR
		{"eur to usd", 9000, eur, usd, 10000, true},                    // round trip
		{"usd to jpy honors 0 decimals", 10000, usd, jpy, 15000, true}, // $100 -> 15000 yen units
		{"jpy to usd", 15000, jpy, usd, 10000, true},
		{"missing rate fails", 1000, usd, unset, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Convert(tt.amount, tt.from, tt.to)
			if ok != tt.ok || got != tt.want {
				t.Errorf("Convert(%d, %s, %s) = (%d, %v), want (%d, %v)", tt.amount, tt.from.Code, tt.to.Code, got, ok, tt.want, tt.ok)
			}
		})
	}
}

type fakeRepo struct {
	Repository
	list  []Currency
	rates map[string]float64
}

func (f *fakeRepo) List(_ context.Context, _ bool) ([]Currency, error) { return f.list, nil }
func (f *fakeRepo) SetRate(_ context.Context, code string, rate float64) error {
	f.rates[code] = rate
	return nil
}

func TestRefreshRates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("base"); got != "USD" {
			t.Errorf("base = %q, want USD", got)
		}
		_, _ = w.Write([]byte(`{"base":"USD","rates":{"EUR":0.91,"JPY":149.5}}`))
	}))
	defer srv.Close()
	origEndpoint := ratesEndpoint
	ratesEndpoint = srv.URL
	defer func() { ratesEndpoint = origEndpoint }()

	repo := &fakeRepo{
		list: []Currency{
			{Code: "USD", Decimals: 2},
			{Code: "EUR", Decimals: 2},
			{Code: "JPY", Decimals: 0},
			{Code: "XTS", Decimals: 2}, // unknown to provider -> skipped
		},
		rates: map[string]float64{},
	}
	svc := NewService(repo)
	svc.SetHTTPClient(&http.Client{Timeout: 5 * time.Second})

	if err := svc.RefreshRates(context.Background(), "usd"); err != nil {
		t.Fatalf("RefreshRates: %v", err)
	}
	if repo.rates["USD"] != 1 {
		t.Errorf("base rate = %v, want 1", repo.rates["USD"])
	}
	if repo.rates["EUR"] != 0.91 || repo.rates["JPY"] != 149.5 {
		t.Errorf("rates = %v", repo.rates)
	}
	if _, ok := repo.rates["XTS"]; ok {
		t.Error("unknown code should be skipped, not stored")
	}
}

type activeOnlyRepo struct {
	fakeRepo
	askedActive bool
}

func (f *activeOnlyRepo) List(ctx context.Context, activeOnly bool) ([]Currency, error) {
	f.askedActive = activeOnly
	return f.fakeRepo.List(ctx, activeOnly)
}

func feed(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	orig := ratesEndpoint
	ratesEndpoint = srv.URL
	t.Cleanup(func() { ratesEndpoint = orig })
}

func ratesRepo() *activeOnlyRepo {
	r := &activeOnlyRepo{}
	r.list = []Currency{{Code: "USD"}, {Code: "EUR"}}
	r.rates = map[string]float64{}
	return r
}

func TestRefreshRatesOnlyTouchesActiveCurrencies(t *testing.T) {
	feed(t, `{"base":"USD","rates":{"EUR":0.9}}`)
	repo := ratesRepo()
	if err := NewService(repo).RefreshRates(context.Background(), "USD"); err != nil {
		t.Fatal(err)
	}
	if !repo.askedActive {
		t.Error("refresh listed inactive currencies too")
	}
}

func TestAFeedQuotingAnotherBaseStoresNothing(t *testing.T) {
	feed(t, `{"base":"EUR","rates":{"USD":1.1}}`)
	repo := ratesRepo()
	if err := NewService(repo).RefreshRates(context.Background(), "USD"); !errors.Is(err, ErrRateBaseMismatch) {
		t.Fatalf("err = %v, want ErrRateBaseMismatch", err)
	}
	if len(repo.rates) != 0 {
		t.Errorf("stored %v from a mismatched feed", repo.rates)
	}
}

func TestAnOversizedFeedIsRefused(t *testing.T) {
	feed(t, `{"base":"USD","rates":{"EUR":0.9},"pad":"`+strings.Repeat("x", maxRatesResponseBytes)+`"}`)
	repo := ratesRepo()
	if err := NewService(repo).RefreshRates(context.Background(), "USD"); err == nil {
		t.Fatal("a body past the cap decoded")
	}
	if len(repo.rates) != 0 {
		t.Errorf("stored %v from a truncated feed", repo.rates)
	}
}

type quoteSource struct{ quote RateQuote }

func (q quoteSource) Rates(context.Context, string, []string) (RateQuote, bool, error) {
	return q.quote, true, nil
}

func TestARemoteQuoteForAnotherBaseStoresNothing(t *testing.T) {
	repo := ratesRepo()
	svc := NewService(repo)
	svc.SetRemoteRateSource(quoteSource{RateQuote{Base: "eur", Rates: map[string]float64{"USD": 1.1}}})
	if err := svc.RefreshRates(context.Background(), "USD"); !errors.Is(err, ErrRateBaseMismatch) {
		t.Fatalf("err = %v, want ErrRateBaseMismatch", err)
	}
	if len(repo.rates) != 0 {
		t.Errorf("stored %v from a mismatched quote", repo.rates)
	}

	svc.SetRemoteRateSource(quoteSource{RateQuote{Rates: map[string]float64{"EUR": 0.9}}})
	if err := svc.RefreshRates(context.Background(), "USD"); err != nil {
		t.Fatalf("a quote that doesn't echo its base: %v", err)
	}
	if repo.rates["EUR"] != 0.9 {
		t.Errorf("rates = %v", repo.rates)
	}
}

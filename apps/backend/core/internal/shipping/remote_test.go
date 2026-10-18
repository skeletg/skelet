package shipping

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeRemoteRates is a hand-rolled RemoteRateSource so QuoteAll/PriceSelected
// integration can be exercised without the remoteprovider HTTP stack
type fakeRemoteRates struct {
	quotes      []RatedQuote
	failed      []string
	priceByCode map[string]int64
}

func (f *fakeRemoteRates) Quotes(_ context.Context, _ RateRequest) ([]RatedQuote, []string) {
	return f.quotes, f.failed
}
func (f *fakeRemoteRates) Price(_ context.Context, _, code string, _ RateRequest) (int64, error) {
	if c, ok := f.priceByCode[code]; ok {
		return c, nil
	}
	return 0, ErrMethodNotFound
}

func TestQuoteAllMergesRemoteRates(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	svc := NewService(fakeCarrierRepo{})
	svc.SetRemoteRateSource(&fakeRemoteRates{
		quotes: []RatedQuote{
			{CarrierSlug: "acme-3pl", CarrierName: "Acme 3PL", Method: RatedMethod{Code: "express", Name: "Express", PriceCents: 1299, EstimatedDays: 2}},
		},
	})

	out, err := svc.QuoteAll(context.Background(), RateRequest{
		ChannelID:    uuid.New(),
		Destination:  Address{CountryCode: "US"},
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, q := range out {
		if q.CarrierSlug == "acme-3pl" && q.Method.Code == "express" && q.Method.PriceCents == 1299 {
			found = true
		}
	}
	if !found {
		t.Fatalf("remote rate not merged into QuoteAll output: %+v", out)
	}
}

func TestPriceSelectedUsesRemoteSource(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	svc := NewService(fakeCarrierRepo{})
	svc.SetRemoteRateSource(&fakeRemoteRates{priceByCode: map[string]int64{"express": 1299}})

	cents, id, err := svc.PriceSelected(context.Background(), "acme-3pl", "express", RateRequest{
		ChannelID:   uuid.New(),
		Destination: Address{CountryCode: "US"},
	})
	if err != nil {
		t.Fatalf("PriceSelected via remote: %v", err)
	}
	if cents != 1299 || id != nil {
		t.Fatalf("unexpected result: cents=%d id=%v", cents, id)
	}

	// unknown code on a real remote provider is fail-closed at placement
	if _, _, err := svc.PriceSelected(context.Background(), "acme-3pl", "nope", RateRequest{
		ChannelID: uuid.New(), Destination: Address{CountryCode: "US"},
	}); !errors.Is(err, ErrMethodNotFound) {
		t.Fatalf("expected ErrMethodNotFound, got %v", err)
	}
}

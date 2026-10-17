package shipping

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestPackSkipsAShipmentPastTheUnitCap(t *testing.T) {
	boxes := []Box{{Code: "S", InnerLengthMm: 100, InnerWidthMm: 100, InnerHeightMm: 100, IsActive: true}}
	if got := Pack([]RateItem{{SKU: "a", Quantity: 1e9}}, boxes); got != nil {
		t.Fatalf("a billion units packed into %d parcels, want nil so carriers rate the raw lines", len(got))
	}
	if got := Pack([]RateItem{{SKU: "a", Quantity: maxPackUnits}}, boxes); len(got) == 0 {
		t.Fatal("a shipment at the cap was not packed")
	}
}

func TestCheckMethodPricingRefusesNegativePrices(t *testing.T) {
	for _, m := range []Method{{FlatCents: -1}, {FreeAboveCents: -1}} {
		if err := CheckMethodPricing(m); !errors.Is(err, errMethodPrice) {
			t.Errorf("CheckMethodPricing(%+v) = %v, want %v", m, err, errMethodPrice)
		}
	}
}

func TestNormCountries(t *testing.T) {
	got, err := normCountries([]string{" de", "DE", "", "us"})
	if err != nil || len(got) != 2 || got[0] != "DE" || got[1] != "US" {
		t.Fatalf("normCountries = %v, %v - want [DE US]", got, err)
	}
	for _, bad := range []string{"USA", "1A", "d"} {
		if _, err := normCountries([]string{bad}); !errors.Is(err, errCountryInvalid) {
			t.Errorf("normCountries(%q) = %v, want %v", bad, err, errCountryInvalid)
		}
	}
}

func TestQuoteNormalizesTheCountry(t *testing.T) {
	svc, _, _, ch := newShipFixture()
	got, err := svc.Quote(context.Background(), QuoteInput{ChannelID: ch, CountryCode: " us ", SubtotalCents: 1000})
	if err != nil || len(got) != 2 {
		t.Fatalf("Quote(' us ') = %v, %v - want both methods", got, err)
	}
}

func TestQuoteCarriersDropsNegativeAndForeignRates(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: "cheat", Carrier: rateStub{methods: []RatedMethod{{Code: "minus", PriceCents: -500}, {Code: "ok", PriceCents: 300}}}})
	svc := NewService(fakeCarrierRepo{configs: []CarrierConfig{{ID: uuid.New(), CarrierSlug: "cheat", IsEnabled: true}}})

	res, err := svc.QuoteCarriers(context.Background(), RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Quotes) != 1 || res.Quotes[0].Method.Code != "ok" {
		t.Errorf("quotes = %+v, want only the non-negative one", res.Quotes)
	}
	if _, _, err := svc.PriceSelected(context.Background(), "cheat", "minus", RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}}); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("PriceSelected(negative) = %v, want %v", err, ErrMethodNotFound)
	}

	for _, tc := range []struct {
		rate remoteRate
		want bool
	}{
		{remoteRate{AmountCents: 500, Currency: "eur"}, true},
		{remoteRate{AmountCents: 500}, true},
		{remoteRate{AmountCents: 500, Currency: "USD"}, false},
		{remoteRate{AmountCents: -1, Currency: "EUR"}, false},
	} {
		if got := usableRate(tc.rate, "EUR"); got != tc.want {
			t.Errorf("usableRate(%+v) = %v, want %v", tc.rate, got, tc.want)
		}
	}
	svc.SetRemoteRateSource(&fakeRemoteRates{priceByCode: map[string]int64{"neg": -100}})
	if _, _, err := svc.PriceSelected(context.Background(), "acme", "neg", RateRequest{ChannelID: uuid.New()}); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("remote PriceSelected(negative) = %v, want %v", err, ErrMethodNotFound)
	}
}

// QuoteCarriers leaves out a carrier limited to other countries, so placement must refuse it too
func TestPriceSelectedHonoursTheCarriersCountries(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: BuiltinCarrierSlug, Carrier: rateStub{}})
	RegisterCarrier(CarrierRegistration{Slug: "dhl", Carrier: rateStub{methods: []RatedMethod{{Code: "x", PriceCents: 900}}}})
	builtin, dhl := uuid.New(), uuid.New()
	zone := Zone{ID: uuid.New(), IsActive: true}
	repo := fakeCarrierRepo{
		configs: []CarrierConfig{
			{ID: builtin, CarrierSlug: BuiltinCarrierSlug, IsEnabled: true, CountryMode: "specific"},
			{ID: dhl, CarrierSlug: "dhl", IsEnabled: true, CountryMode: "specific"},
		},
		countries: map[uuid.UUID][]string{builtin: {"DE"}, dhl: {"DE"}},
		zone:      zone,
		method:    Method{ID: uuid.New(), ZoneID: zone.ID, Code: "std", CalcType: CalcFlat, FlatCents: 500, IsActive: true},
	}
	svc := NewService(repo)
	for _, tc := range []struct{ slug, code string }{{BuiltinCarrierSlug, "std"}, {"dhl", "x"}} {
		if _, _, err := svc.PriceSelected(context.Background(), tc.slug, tc.code, RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}}); !errors.Is(err, ErrMethodNotFound) {
			t.Errorf("%s to US = %v, want %v", tc.slug, err, ErrMethodNotFound)
		}
		if _, _, err := svc.PriceSelected(context.Background(), tc.slug, tc.code, RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "DE"}}); err != nil {
			t.Errorf("%s to DE = %v, want a price", tc.slug, err)
		}
	}
}

func TestPriceSelectedSurvivesAPanickingCarrier(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: "boom", Carrier: rateStub{panics: true}})
	svc := NewService(fakeCarrierRepo{configs: []CarrierConfig{{ID: uuid.New(), CarrierSlug: "boom", IsEnabled: true}}})
	if _, _, err := svc.PriceSelected(context.Background(), "boom", "x", RateRequest{ChannelID: uuid.New()}); err == nil {
		t.Fatal("a panicking carrier priced an order")
	}
}

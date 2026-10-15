package shipping

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// rateStub is a Carrier returning canned methods (or an error / panic)
type rateStub struct {
	methods []RatedMethod
	err     error
	panics  bool
}

func (s rateStub) Quote(context.Context, RateRequest) ([]RatedMethod, error) {
	if s.panics {
		panic("carrier blew up")
	}
	return s.methods, s.err
}

// fakeCarrierRepo embeds Repository (nil) and implements only the carrier-config
// reads QuoteAll / PriceSelected touch
type fakeCarrierRepo struct {
	Repository
	configs   []CarrierConfig
	countries map[uuid.UUID][]string
	// Canned zone + method so the builtin branch of PriceSelected can reach a price
	zone   Zone
	method Method
}

func (f fakeCarrierRepo) FindZoneForCountry(context.Context, uuid.UUID, string) (Zone, error) {
	return f.zone, nil
}
func (f fakeCarrierRepo) GetMethodByCode(context.Context, uuid.UUID, string) (Method, error) {
	return f.method, nil
}

func (f fakeCarrierRepo) ListCarrierConfigsByChannel(context.Context, uuid.UUID) ([]CarrierConfig, error) {
	return f.configs, nil
}
func (f fakeCarrierRepo) ListCountriesForCarrier(_ context.Context, id uuid.UUID) ([]string, error) {
	return f.countries[id], nil
}
func (f fakeCarrierRepo) ListActiveBoxesByChannel(context.Context, uuid.UUID) ([]Box, error) {
	return nil, nil
}
func (f fakeCarrierRepo) GetCarrierConfig(_ context.Context, _ uuid.UUID, slug string) (CarrierConfig, error) {
	for _, c := range f.configs {
		if c.CarrierSlug == slug {
			return c, nil
		}
	}
	return CarrierConfig{}, ErrCarrierNotFound
}

func TestQuoteAll_PartialResultsAndOrder(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: BuiltinCarrierSlug, Carrier: rateStub{methods: []RatedMethod{{Code: "std", PriceCents: 500}}}, DisplayName: "Store"})
	RegisterCarrier(CarrierRegistration{Slug: "fast", Carrier: rateStub{methods: []RatedMethod{{Code: "express", PriceCents: 1500}}}, DisplayName: "Fast"})
	RegisterCarrier(CarrierRegistration{Slug: "flaky", Carrier: rateStub{err: errors.New("boom")}, DisplayName: "Flaky"})
	RegisterCarrier(CarrierRegistration{Slug: "boom", Carrier: rateStub{panics: true}, DisplayName: "Boom"})

	repo := fakeCarrierRepo{configs: []CarrierConfig{
		{ID: uuid.New(), CarrierSlug: BuiltinCarrierSlug, IsEnabled: true, SortOrder: 0},
		{ID: uuid.New(), CarrierSlug: "fast", IsEnabled: true, SortOrder: 1},
		{ID: uuid.New(), CarrierSlug: "flaky", IsEnabled: true, SortOrder: 2},
		{ID: uuid.New(), CarrierSlug: "boom", IsEnabled: true, SortOrder: 3},
	}}
	svc := NewService(repo)

	got, err := svc.QuoteAll(context.Background(), RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}, SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("QuoteAll: %v", err)
	}
	// flaky (error) + boom (panic) skipped; std + express survive
	if len(got) != 2 {
		t.Fatalf("got %d quotes, want 2 (errored/panicked carriers skipped): %+v", len(got), got)
	}
	// Deterministic: builtin (sort 0) before fast (sort 1)
	if got[0].CarrierSlug != BuiltinCarrierSlug || got[0].Method.Code != "std" {
		t.Errorf("quote[0] = %+v, want builtin/std", got[0])
	}
	if got[1].CarrierSlug != "fast" || got[1].Method.Code != "express" {
		t.Errorf("quote[1] = %+v, want fast/express", got[1])
	}
}

// A carrier that errored, panicked or failed remotely is named, so checkout can tell "not offered" from "couldn't ask"
func TestQuoteCarriers_NamesFailedCarriers(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: BuiltinCarrierSlug, Carrier: rateStub{methods: []RatedMethod{{Code: "std", PriceCents: 500}}}, DisplayName: "Store"})
	RegisterCarrier(CarrierRegistration{Slug: "flaky", Carrier: rateStub{err: errors.New("boom")}, DisplayName: "Flaky"})
	RegisterCarrier(CarrierRegistration{Slug: "boom", Carrier: rateStub{panics: true}, DisplayName: "Boom"})
	repo := fakeCarrierRepo{configs: []CarrierConfig{
		{ID: uuid.New(), CarrierSlug: "flaky", IsEnabled: true},
		{ID: uuid.New(), CarrierSlug: "boom", IsEnabled: true},
	}}
	svc := NewService(repo)
	svc.SetRemoteRateSource(&fakeRemoteRates{failed: []string{"acme-3pl"}})

	res, err := svc.QuoteCarriers(context.Background(), RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}, SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("QuoteCarriers: %v", err)
	}
	if len(res.Quotes) != 1 || res.Quotes[0].CarrierSlug != BuiltinCarrierSlug {
		t.Errorf("quotes = %+v, want only the builtin's", res.Quotes)
	}
	for _, slug := range []string{"flaky", "boom", "acme-3pl"} {
		if res.Answered(slug) {
			t.Errorf("Answered(%q) = true, want the failure reported (failed = %v)", slug, res.Failed)
		}
	}
	if !res.Answered(BuiltinCarrierSlug) || !res.Answered("not-a-carrier") {
		t.Errorf("a carrier that answered, or one that was never asked, reads as failed: %v", res.Failed)
	}
}

func TestQuoteAll_DisabledExcluded_BuiltinFailOpen(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: BuiltinCarrierSlug, Carrier: rateStub{methods: []RatedMethod{{Code: "std", PriceCents: 500}}}, DisplayName: "Store"})
	RegisterCarrier(CarrierRegistration{Slug: "fast", Carrier: rateStub{methods: []RatedMethod{{Code: "express", PriceCents: 1500}}}, DisplayName: "Fast"})

	// fast explicitly disabled; builtin has NO row → must fail-open (enabled)
	repo := fakeCarrierRepo{configs: []CarrierConfig{
		{ID: uuid.New(), CarrierSlug: "fast", IsEnabled: false, SortOrder: 1},
	}}
	svc := NewService(repo)

	got, err := svc.QuoteAll(context.Background(), RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}, SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("QuoteAll: %v", err)
	}
	if len(got) != 1 || got[0].CarrierSlug != BuiltinCarrierSlug {
		t.Errorf("got %+v, want only builtin (fail-open, fast disabled)", got)
	}
}

// The built-in has to honour is_enabled exactly like QuoteAll does, else a merchant who migrated to an external carrier still serves stale zone rates to anyone posting slug="builtin"
func TestPriceSelected_BuiltinHonoursEnabled(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: BuiltinCarrierSlug, Carrier: rateStub{}, DisplayName: "Store"})

	zone := Zone{ID: uuid.New(), IsActive: true}
	method := Method{ID: uuid.New(), ZoneID: zone.ID, Code: "flat-standard", CalcType: CalcFlat, FlatCents: 499, IsActive: true, CountryMode: "all"}

	tests := []struct {
		name    string
		configs []CarrierConfig
		slug    string
		wantOff bool
	}{
		{name: "no row fails open", slug: BuiltinCarrierSlug},
		{name: "empty slug means builtin, no row fails open", slug: ""},
		{
			name:    "enabled row prices normally",
			configs: []CarrierConfig{{ID: uuid.New(), CarrierSlug: BuiltinCarrierSlug, IsEnabled: true}},
			slug:    BuiltinCarrierSlug,
		},
		{
			name:    "disabled row refuses",
			configs: []CarrierConfig{{ID: uuid.New(), CarrierSlug: BuiltinCarrierSlug, IsEnabled: false}},
			slug:    BuiltinCarrierSlug,
			wantOff: true,
		},
		{
			name:    "disabled row refuses the empty slug too",
			configs: []CarrierConfig{{ID: uuid.New(), CarrierSlug: BuiltinCarrierSlug, IsEnabled: false}},
			slug:    "",
			wantOff: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(fakeCarrierRepo{configs: tc.configs, zone: zone, method: method})
			req := RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}, SubtotalCents: 1000}
			cents, id, err := svc.PriceSelected(context.Background(), tc.slug, "flat-standard", req)
			if tc.wantOff {
				if !errors.Is(err, ErrMethodNotFound) {
					t.Errorf("disabled builtin: want ErrMethodNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("enabled builtin: %v", err)
			}
			if cents != 499 || id == nil || *id != method.ID {
				t.Errorf("got (%d,%v), want (499,%v)", cents, id, method.ID)
			}
		})
	}
}

func TestPriceSelected_External(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	RegisterCarrier(CarrierRegistration{Slug: "fast", Carrier: rateStub{methods: []RatedMethod{{Code: "express", PriceCents: 1500}}}, DisplayName: "Fast"})
	RegisterCarrier(CarrierRegistration{Slug: "off", Carrier: rateStub{methods: []RatedMethod{{Code: "x", PriceCents: 100}}}, DisplayName: "Off"})

	repo := fakeCarrierRepo{configs: []CarrierConfig{
		{ID: uuid.New(), CarrierSlug: "fast", IsEnabled: true},
		{ID: uuid.New(), CarrierSlug: "off", IsEnabled: false},
	}}
	svc := NewService(repo)
	req := RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}, SubtotalCents: 1000}

	cents, id, err := svc.PriceSelected(context.Background(), "fast", "express", req)
	if err != nil || cents != 1500 || id != nil {
		t.Errorf("PriceSelected(fast,express) = (%d,%v,%v), want (1500,nil,nil)", cents, id, err)
	}
	if _, _, err := svc.PriceSelected(context.Background(), "fast", "ghost", req); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("unknown code: want ErrMethodNotFound, got %v", err)
	}
	if _, _, err := svc.PriceSelected(context.Background(), "off", "x", req); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("disabled carrier: want ErrMethodNotFound, got %v", err)
	}
	if _, _, err := svc.PriceSelected(context.Background(), "ghost", "x", req); !errors.Is(err, ErrMethodNotFound) {
		t.Errorf("unknown carrier: want ErrMethodNotFound, got %v", err)
	}
}

type capturingCarrier struct{ got *RateRequest }

func (c capturingCarrier) Quote(_ context.Context, req RateRequest) ([]RatedMethod, error) {
	*c.got = req
	return []RatedMethod{{Code: "ground", PriceCents: 900}}, nil
}

type dimsStub map[string]Dimensions

func (d dimsStub) DimensionsBySKUs(context.Context, []string) (map[string]Dimensions, error) {
	return d, nil
}

// Callers send SKUs and quantities - the carrier still sees each product's weight, and a line that brought its own keeps it
func TestPriceSelected_FillsProductDimensions(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	var got RateRequest
	RegisterCarrier(CarrierRegistration{Slug: "weighty", Carrier: capturingCarrier{got: &got}, DisplayName: "Weighty"})
	svc := NewService(fakeCarrierRepo{configs: []CarrierConfig{{ID: uuid.New(), CarrierSlug: "weighty", IsEnabled: true}}})
	svc.SetDimensionLookup(dimsStub{"A": {WeightGrams: 250, LengthMm: 100}, "B": {WeightGrams: 999}})

	items := []RateItem{{SKU: "A", Quantity: 2}, {SKU: "B", Quantity: 1, WeightGrams: 40}, {SKU: "C", Quantity: 1}}
	if _, _, err := svc.PriceSelected(context.Background(), "weighty", "ground", RateRequest{ChannelID: uuid.New(), Items: items}); err != nil {
		t.Fatalf("PriceSelected: %v", err)
	}
	if len(got.Items) != 3 || got.Items[0].WeightGrams != 250 || got.Items[0].LengthMm != 100 {
		t.Errorf("A reached the carrier as %+v, want the catalog's 250g x 100mm", got.Items)
	}
	if got.Items[1].WeightGrams != 40 {
		t.Errorf("B = %+v, want its own 40g kept", got.Items[1])
	}
	if got.Items[2].WeightGrams != 0 {
		t.Errorf("C = %+v, want weightless with no catalog row", got.Items[2])
	}
	if items[0].WeightGrams != 0 {
		t.Error("the caller's items were mutated")
	}
}

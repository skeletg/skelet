package shipping

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// stubCarrier is a minimal Carrier for registry tests
type stubCarrier struct{}

func (stubCarrier) Quote(context.Context, RateRequest) ([]RatedMethod, error) { return nil, nil }

func TestRegisterCarrier_DuplicatePanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	RegisterCarrier(CarrierRegistration{Slug: "dup", Carrier: stubCarrier{}})
	defer func() {
		if r := recover(); r == nil {
			t.Error("registering a duplicate slug should panic")
		}
	}()
	RegisterCarrier(CarrierRegistration{Slug: "dup", Carrier: stubCarrier{}})
}

func TestRegisterCarrier_NilPanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	defer func() {
		if r := recover(); r == nil {
			t.Error("registering a nil carrier should panic")
		}
	}()
	RegisterCarrier(CarrierRegistration{Slug: "nilcar"})
}

func TestRegisterCarrier_DisplayNameDefaultsToSlug(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	RegisterCarrier(CarrierRegistration{Slug: "noname", Carrier: stubCarrier{}})
	reg, err := GetCarrier("noname")
	if err != nil {
		t.Fatalf("GetCarrier: %v", err)
	}
	if reg.DisplayName != "noname" {
		t.Errorf("DisplayName = %q, want fallback to slug %q", reg.DisplayName, "noname")
	}
}

func TestGetCarrier_Unknown(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	if _, err := GetCarrier("ghost"); !errors.Is(err, ErrCarrierNotFound) {
		t.Errorf("GetCarrier(unknown) = %v, want ErrCarrierNotFound", err)
	}
}

// TestBuiltinCarrierQuoteParity asserts the built-in carrier returns exactly what Service.Quote returns (same
// code/price/name per method) - wrapping the engine must not change its pricing
func TestBuiltinCarrierQuoteParity(t *testing.T) {
	svc, _, _, ch := newShipFixture()
	bc := NewBuiltinCarrier(svc)
	ctx := context.Background()

	want, err := svc.Quote(ctx, QuoteInput{ChannelID: ch, CountryCode: "US", SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("Service.Quote: %v", err)
	}
	got, err := bc.Quote(ctx, RateRequest{
		ChannelID:     ch,
		Destination:   Address{CountryCode: "US"},
		SubtotalCents: 1000,
	})
	if err != nil {
		t.Fatalf("BuiltinCarrier.Quote: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rated methods, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Code != want[i].Code || got[i].PriceCents != want[i].PriceCents || got[i].Name != want[i].Name {
			t.Errorf("method %d: got {%s,%s,%d}, want {%s,%s,%d}",
				i, got[i].Code, got[i].Name, got[i].PriceCents,
				want[i].Code, want[i].Name, want[i].PriceCents)
		}
	}

	// Country filter is preserved through the carrier wrapper
	ca, err := bc.Quote(ctx, RateRequest{ChannelID: ch, Destination: Address{CountryCode: "CA"}, SubtotalCents: 1000})
	if err != nil {
		t.Fatalf("BuiltinCarrier.Quote(CA): %v", err)
	}
	if len(ca) != 1 || ca[0].Code != "std" {
		t.Errorf("CA: got %v, want only the all-countries method", ca)
	}
}

// A built-in method's delivery days and weekend rule ride through the carrier wrapper unchanged
func TestBuiltinCarrierCarriesEstimates(t *testing.T) {
	zone := uuid.New()
	std := Method{ID: uuid.New(), ZoneID: zone, Code: "std", CalcType: CalcFlat, FlatCents: 500, IsActive: true, CountryMode: "all", EstimatedDays: 4}
	express := Method{ID: uuid.New(), ZoneID: zone, Code: "express", CalcType: CalcFlat, FlatCents: 1200, IsActive: true, CountryMode: "all", EstimatedDays: 1, DeliversOnWeekends: true}
	none := Method{ID: uuid.New(), ZoneID: zone, Code: "none", CalcType: CalcFree, IsActive: true, CountryMode: "all"}
	svc := NewService(fakeShipRepo{zoneID: zone, methods: []Method{std, express, none}})

	got, err := NewBuiltinCarrier(svc).Quote(context.Background(), RateRequest{ChannelID: uuid.New(), Destination: Address{CountryCode: "US"}})
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	want := map[string]struct {
		days     int
		weekends bool
	}{"std": {4, false}, "express": {1, true}, "none": {0, false}}
	if len(got) != len(want) {
		t.Fatalf("got %d methods, want %d", len(got), len(want))
	}
	for _, m := range got {
		if w := want[m.Code]; m.EstimatedDays != w.days || m.DeliversOnWeekends != w.weekends {
			t.Errorf("%s: got {%d, %v}, want {%d, %v}", m.Code, m.EstimatedDays, m.DeliversOnWeekends, w.days, w.weekends)
		}
	}
}

func TestCheckMethodPricing_RefusesNegativeDays(t *testing.T) {
	if err := CheckMethodPricing(Method{EstimatedDays: -1}); !errors.Is(err, errMethodDays) {
		t.Errorf("got %v, want errMethodDays", err)
	}
	if err := CheckMethodPricing(Method{EstimatedDays: 0}); err != nil {
		t.Errorf("0 days is no estimate, not an error: %v", err)
	}
}

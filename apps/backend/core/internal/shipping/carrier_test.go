package shipping

import (
	"context"
	"errors"
	"testing"
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

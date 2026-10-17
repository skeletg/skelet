package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type fakeShippedRepo struct {
	Repository
	shipped map[uuid.UUID]float64
	err     error
}

func (f fakeShippedRepo) ShippedByOrderLine(context.Context, uuid.UUID) (map[uuid.UUID]float64, error) {
	return f.shipped, f.err
}

func fractionOf(o Order, shipped map[uuid.UUID]float64, err error) float64 {
	s := &Service{repo: fakeShippedRepo{shipped: shipped, err: err}}
	return unshippedFraction(context.Background(), s.repo, o)
}

// The fraction scales tender reversals on cancellation, so it has to be weighted by LINE VALUE and not by unit count - a cheap line and an expensive one don't refund the same
func TestUnshippedFractionIsWeightedByValueNotQuantity(t *testing.T) {
	cheap, pricey := uuid.New(), uuid.New()
	o := Order{ID: uuid.New(), Lines: []OrderLine{
		{ID: cheap, UnitPrice: 100, Quantity: 10},  // 1000
		{ID: pricey, UnitPrice: 9000, Quantity: 1}, // 9000
	}}

	// the cheap line shipped in full, the expensive one did not - 9 of 11 units are unshipped but 9000 of 10000 in value
	got := fractionOf(o, map[uuid.UUID]float64{cheap: 10}, nil)
	if got != 0.9 {
		t.Errorf("unshippedFraction = %v, want 0.9 by value (a quantity weighting would give ~0.09)", got)
	}
}

func TestUnshippedFractionBounds(t *testing.T) {
	lineA, lineB := uuid.New(), uuid.New()
	o := Order{ID: uuid.New(), Lines: []OrderLine{
		{ID: lineA, UnitPrice: 1000, Quantity: 2}, // 2000
		{ID: lineB, UnitPrice: 500, Quantity: 4},  // 2000
	}}

	for name, tc := range map[string]struct {
		shipped map[uuid.UUID]float64
		want    float64
	}{
		"nothing shipped":              {nil, 1},
		"everything shipped":           {map[uuid.UUID]float64{lineA: 2, lineB: 4}, 0},
		"one line shipped":             {map[uuid.UUID]float64{lineA: 2}, 0.5},
		"half of one line":             {map[uuid.UUID]float64{lineA: 1}, 0.75},
		"over-shipped is not negative": {map[uuid.UUID]float64{lineA: 5, lineB: 9}, 0},
	} {
		got := fractionOf(o, tc.shipped, nil)
		if got != tc.want {
			t.Errorf("%s: unshippedFraction = %v, want %v", name, got, tc.want)
		}
		if got < 0 || got > 1 {
			t.Errorf("%s: unshippedFraction = %v, want it inside 0..1", name, got)
		}
	}
}

// Documented fail-safe: a lookup error over-restores rather than under-refunding, since an operator can claw back but a shorted customer files a chargeback
func TestUnshippedFractionFailsSafeAtOne(t *testing.T) {
	lineA := uuid.New()
	o := Order{ID: uuid.New(), Lines: []OrderLine{{ID: lineA, UnitPrice: 1000, Quantity: 2}}}

	if got := fractionOf(o, map[uuid.UUID]float64{lineA: 2}, errors.New("db down")); got != 1 {
		t.Errorf("unshippedFraction on a lookup error = %v, want the fail-safe 1", got)
	}
	// an empty map is the same short-circuit - nothing has shipped
	if got := fractionOf(o, map[uuid.UUID]float64{}, nil); got != 1 {
		t.Errorf("unshippedFraction with an empty shipped map = %v, want 1", got)
	}
}

// A zero-value order has nothing to prorate, and dividing by its total would be a NaN reaching the tender reversal
func TestUnshippedFractionOnAZeroValueOrder(t *testing.T) {
	lineA := uuid.New()
	for name, o := range map[string]Order{
		"free line": {ID: uuid.New(), Lines: []OrderLine{{ID: lineA, UnitPrice: 0, Quantity: 3}}},
		"zero qty":  {ID: uuid.New(), Lines: []OrderLine{{ID: lineA, UnitPrice: 1000, Quantity: 0}}},
		"no lines":  {ID: uuid.New()},
	} {
		got := fractionOf(o, map[uuid.UUID]float64{lineA: 1}, nil)
		if got != 1 {
			t.Errorf("%s: unshippedFraction = %v, want 1", name, got)
		}
	}
}

// A line the shipment map never mentions counts as fully unshipped, so a partially-recorded fulfillment can't understate the refund
func TestUnshippedFractionCountsUnknownLinesAsUnshipped(t *testing.T) {
	known, unknown := uuid.New(), uuid.New()
	o := Order{ID: uuid.New(), Lines: []OrderLine{
		{ID: known, UnitPrice: 1000, Quantity: 1},
		{ID: unknown, UnitPrice: 1000, Quantity: 1},
	}}
	got := fractionOf(o, map[uuid.UUID]float64{known: 1}, nil)
	if got != 0.5 {
		t.Errorf("unshippedFraction = %v, want 0.5 - the unmentioned line is unshipped", got)
	}
}

// A carrier rate quote is built from these, so a dropped line under-quotes the shipment
func TestRateItemsFromLines(t *testing.T) {
	lines := []OrderLine{
		{ProductSku: "A-1", Quantity: 2, UnitPrice: 1000},
		{ProductSku: "B-2", Quantity: 1.5, UnitPrice: 250},
		{ProductSku: "CUSTOM-1", Quantity: 1, UnitPrice: 50000, IsCustom: true},
	}
	got, subtotal := rateItemsFromLines(lines)
	if len(got) != 2 {
		t.Fatalf("%d rate items, want 2 - a custom line never ships", len(got))
	}
	if subtotal != 2375 {
		t.Errorf("shipping subtotal = %d, want 2375 - the custom line's value is not goods shipped", subtotal)
	}
	if got[0] != (shippingpkg.RateItem{SKU: "A-1", Quantity: 2, UnitPriceCents: 1000}) {
		t.Errorf("item 0 = %+v, want the line's sku, qty and unit price", got[0])
	}
	if got[1].Quantity != 1.5 {
		t.Errorf("fractional quantity = %v, want 1.5 preserved", got[1].Quantity)
	}
	if empty, _ := rateItemsFromLines(nil); empty == nil || len(empty) != 0 {
		t.Errorf("rateItemsFromLines(nil) = %v, want an empty non-nil slice", empty)
	}
}

// The label goes to a carrier API, so an optional field that arrives as nil must become an empty string rather than panicking mid-shipment
func TestOrderAddressToShippingFlattensOptionalFields(t *testing.T) {
	got := orderAddressToShipping(OrderAddress{
		FirstName: "Ada", LastName: "Lovelace",
		Street1: "1 Bridge St", City: "London", PostalCode: "E1 6AN", CountryCode: "GB",
	})
	if got.Company != "" || got.Street2 != "" || got.Region != "" || got.Phone != "" {
		t.Errorf("optional fields = %+v, want them empty rather than nil-dereferenced", got)
	}
	if got.FirstName != "Ada" || got.Street1 != "1 Bridge St" || got.CountryCode != "GB" {
		t.Errorf("required fields = %+v, want them carried through", got)
	}
}

// State maps onto the carrier's Region - the two names differ, so a missed mapping drops the state from every label
func TestOrderAddressToShippingMapsStateOntoRegion(t *testing.T) {
	company, street2, state, phone := "Analytical", "Apt 2", "VA", "+15550000"
	got := orderAddressToShipping(OrderAddress{
		FirstName: "Grace", LastName: "Hopper", Company: &company,
		Street1: "9 Navy Yard", Street2: &street2, City: "Arlington",
		State: &state, PostalCode: "22202", CountryCode: "US", Phone: &phone,
	})
	if got.Region != "VA" {
		t.Errorf("Region = %q, want the address's state VA", got.Region)
	}
	if got.Company != "Analytical" || got.Street2 != "Apt 2" || got.Phone != "+15550000" {
		t.Errorf("optional fields = %+v, want them dereferenced", got)
	}
}

// A rewritten address reaches the carrier and the invoice, so the shape check is what stops a label being bought for somewhere undeliverable
func TestValidEditAddress(t *testing.T) {
	ok := OrderAddress{
		FirstName: "Ada", LastName: "Lovelace",
		Street1: "1 Bridge St", City: "London", PostalCode: "E1 6AN", CountryCode: "GB",
	}
	if err := validEditAddress(ok); err != nil {
		t.Fatalf("a complete address = %v, want nil", err)
	}

	for name, mutate := range map[string]func(*OrderAddress){
		"no first name":        func(a *OrderAddress) { a.FirstName = "" },
		"no last name":         func(a *OrderAddress) { a.LastName = "" },
		"no street":            func(a *OrderAddress) { a.Street1 = "" },
		"no city":              func(a *OrderAddress) { a.City = "" },
		"no postal code":       func(a *OrderAddress) { a.PostalCode = "" },
		"no country":           func(a *OrderAddress) { a.CountryCode = "" },
		"one-letter country":   func(a *OrderAddress) { a.CountryCode = "G" },
		"three-letter country": func(a *OrderAddress) { a.CountryCode = "GBR" },
	} {
		bad := ok
		mutate(&bad)
		err := validEditAddress(bad)
		if err == nil {
			t.Errorf("%s was accepted, want it refused", name)
			continue
		}
		if code := apperr.CodeOf(err); code != "ORDER_EDIT_INVALID_ADDRESS" {
			t.Errorf("%s: error code = %q, want ORDER_EDIT_INVALID_ADDRESS", name, code)
		}
	}
}

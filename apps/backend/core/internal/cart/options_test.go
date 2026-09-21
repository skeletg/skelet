package cart

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func selOpt(required bool, inputType string, values ...ProductOptionValueConfig) ProductOptionConfig {
	return ProductOptionConfig{ID: uuid.New(), InputType: inputType, IsRequired: required, Title: "opt", Values: values}
}

func val(price int32, qty float64) ProductOptionValueConfig {
	return ProductOptionValueConfig{ID: uuid.New(), Title: "v", PriceValue: price, PriceType: "fixed", Quantity: qty}
}

func res(base int64, opts ...ProductOptionConfig) ProductOptionsResolution {
	return ProductOptionsResolution{ProductID: uuid.New(), UnitPrice: base, BasePrice: base, Options: opts}
}

func TestBuildOptions_FixedValueDelta(t *testing.T) {
	o := selOpt(true, "dropdown", val(500, 1))
	r := res(1000, o)
	rows, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{o.Values[0].ID}}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 500 {
		t.Errorf("delta = %d, want 500", delta)
	}
	if len(rows) != 1 || rows[0].PriceDelta != 500 || rows[0].OptionValueID == nil {
		t.Errorf("bad snapshot row: %+v", rows)
	}
}

func TestBuildOptions_ValueQuantityMultiplies(t *testing.T) {
	o := selOpt(false, "dropdown", val(250, 3)) // 250 * 3 = 750
	r := res(1000, o)
	_, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{o.Values[0].ID}}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 750 {
		t.Errorf("delta = %d, want 750", delta)
	}
}

func TestBuildOptions_PercentValueOfBase(t *testing.T) {
	v := ProductOptionValueConfig{ID: uuid.New(), Title: "v", PriceValue: 10, PriceType: "percent", Quantity: 1}
	o := selOpt(false, "dropdown", v)
	r := res(1999, o) // 10% of 1999 = 199.9 -> 200 (round half away)
	_, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{v.ID}}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 200 {
		t.Errorf("delta = %d, want 200", delta)
	}
}

func TestBuildOptions_SourceProductPrice(t *testing.T) {
	v := ProductOptionValueConfig{ID: uuid.New(), Title: "engraving plate", PriceValue: 100, PriceType: "fixed", Quantity: 1, HasSource: true, SourceProductPrice: 1500}
	o := selOpt(false, "dropdown", v)
	r := res(1000, o) // 1500 + 100 = 1600
	_, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{v.ID}}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 1600 {
		t.Errorf("delta = %d, want 1600", delta)
	}
}

func TestBuildOptions_MultiSelectSumsValues(t *testing.T) {
	o := selOpt(false, "multiselect", val(100, 1), val(200, 1), val(300, 1))
	r := res(1000, o)
	in := []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{o.Values[0].ID, o.Values[2].ID}}}
	rows, delta, _, err := buildConfiguredLineOptions(r, in, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 400 { // 100 + 300
		t.Errorf("delta = %d, want 400", delta)
	}
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}
}

func TestBuildOptions_RequiredMissing(t *testing.T) {
	o := selOpt(true, "dropdown", val(500, 1))
	r := res(1000, o)
	_, _, _, err := buildConfiguredLineOptions(r, nil, nil)
	if !errors.Is(err, ErrProductOptionMissing) {
		t.Fatalf("got %v, want ErrProductOptionMissing", err)
	}
}

func TestBuildOptions_SingleSelectCardinality(t *testing.T) {
	o := selOpt(false, "radio", val(100, 1), val(200, 1))
	r := res(1000, o)
	in := []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{o.Values[0].ID, o.Values[1].ID}}}
	_, _, _, err := buildConfiguredLineOptions(r, in, nil)
	if !errors.Is(err, ErrProductOptionCardinality) {
		t.Fatalf("got %v, want ErrProductOptionCardinality", err)
	}
}

func TestBuildOptions_UnknownOptionRejected(t *testing.T) {
	o := selOpt(false, "dropdown", val(100, 1))
	r := res(1000, o)
	_, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: uuid.New(), ValueIDs: []uuid.UUID{uuid.New()}}}, nil)
	if !errors.Is(err, ErrProductOptionUnknown) {
		t.Fatalf("got %v, want ErrProductOptionUnknown", err)
	}
}

func TestBuildOptions_ValueNotOnOptionRejected(t *testing.T) {
	o := selOpt(false, "dropdown", val(100, 1))
	r := res(1000, o)
	_, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, ValueIDs: []uuid.UUID{uuid.New()}}}, nil)
	if !errors.Is(err, ErrProductOptionInvalid) {
		t.Fatalf("got %v, want ErrProductOptionInvalid", err)
	}
}

func TestBuildOptions_TextMaxChars(t *testing.T) {
	max := int32(5)
	o := ProductOptionConfig{ID: uuid.New(), InputType: "field", IsRequired: false, Title: "engrave", MaxChars: &max}
	r := res(1000, o)
	_, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, TextValue: "toolong"}}, nil)
	if !errors.Is(err, ErrProductOptionInvalid) {
		t.Fatalf("got %v, want ErrProductOptionInvalid", err)
	}
}

func TestBuildOptions_DateValidation(t *testing.T) {
	cases := []struct {
		inputType string
		value     string
		ok        bool
	}{
		{"date", "2026-06-14", true},
		{"date", "06/14/2026", false},
		{"date", "not-a-date", false},
		{"time", "15:04", true},
		{"time", "15:04:05", true},
		{"time", "25:99", false},
		{"date_time", "2026-06-14T15:04", true},
		{"date_time", "2026-06-14T15:04:05Z", true},
		{"date_time", "2026-06-14 15:04", false},
	}
	for _, c := range cases {
		o := ProductOptionConfig{ID: uuid.New(), InputType: c.inputType, Title: "when"}
		r := res(1000, o)
		_, _, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, TextValue: c.value}}, nil)
		if c.ok && err != nil {
			t.Errorf("%s %q: unexpected error %v", c.inputType, c.value, err)
		}
		if !c.ok && !errors.Is(err, ErrProductOptionInvalid) {
			t.Errorf("%s %q: got %v, want ErrProductOptionInvalid", c.inputType, c.value, err)
		}
	}
}

func TestBuildOptions_TextOptionLevelFixedPrice(t *testing.T) {
	price := int32(300)
	o := ProductOptionConfig{ID: uuid.New(), InputType: "field", Title: "engrave", PriceValue: &price, PriceType: "fixed"}
	r := res(1000, o)
	rows, delta, _, err := buildConfiguredLineOptions(r, []CartLineOptionInput{{OptionID: o.ID, TextValue: "Hi"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 300 {
		t.Errorf("delta = %d, want 300", delta)
	}
	if len(rows) != 1 || rows[0].TextValue != "Hi" || rows[0].OptionValueID != nil {
		t.Errorf("bad text row: %+v", rows)
	}
}

func TestBuildOptions_OptionalOmittedIsSkipped(t *testing.T) {
	o := selOpt(false, "dropdown", val(500, 1))
	r := res(1000, o)
	rows, delta, _, err := buildConfiguredLineOptions(r, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if delta != 0 || len(rows) != 0 {
		t.Errorf("optional omitted should contribute nothing, got delta=%d rows=%d", delta, len(rows))
	}
}

func TestBuildOptions_DuplicateOptionRejected(t *testing.T) {
	o := selOpt(false, "dropdown", val(100, 1))
	r := res(1000, o)
	in := []CartLineOptionInput{
		{OptionID: o.ID, ValueIDs: []uuid.UUID{o.Values[0].ID}},
		{OptionID: o.ID, ValueIDs: []uuid.UUID{o.Values[0].ID}},
	}
	_, _, _, err := buildConfiguredLineOptions(r, in, nil)
	if !errors.Is(err, ErrProductOptionInvalid) {
		t.Fatalf("got %v, want ErrProductOptionInvalid", err)
	}
}

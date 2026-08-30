package conditions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/promotion"
)

// These are pure-logic tests of the built-in condition evaluators - Each
// condition's Evaluate takes the read-only promotion.Environment view
// (built via the exported promotion.BuildEnvironment) plus a JSON params
// blob - no database, no engine, no repository - The structs themselves
// are unexported, so the tests live in package conditions

func i64(v int64) *int64           { return &v }
func f64(v float64) *float64       { return &v }
func raw(s string) json.RawMessage { return json.RawMessage(s) }

// env is a tiny helper to build an Environment from the fields a given
// test cares about
func env(in promotion.EnvironmentInput) promotion.Environment {
	return promotion.BuildEnvironment(in)
}

func line(sku string, qty float64, cats ...uuid.UUID) promotion.CartLineView {
	return promotion.CartLineView{
		LineID:      uuid.New(),
		ProductID:   uuid.New(),
		ProductSKU:  sku,
		Quantity:    qty,
		CategoryIDs: cats,
	}
}

func mustEval(t *testing.T, c interface {
	Evaluate(context.Context, promotion.Environment, json.RawMessage) (bool, error)
}, e promotion.Environment, params json.RawMessage) bool {
	t.Helper()
	got, err := c.Evaluate(context.Background(), e, params)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	return got
}

func TestCartSubtotalRangeEvaluate(t *testing.T) {
	cases := []struct {
		name   string
		params cartSubtotalRangeParams
		sub    int64
		want   bool
	}{
		{"no bounds is always true", cartSubtotalRangeParams{}, 0, true},
		{"at min boundary inclusive", cartSubtotalRangeParams{MinCents: i64(1000)}, 1000, true},
		{"one below min", cartSubtotalRangeParams{MinCents: i64(1000)}, 999, false},
		{"at max boundary inclusive", cartSubtotalRangeParams{MaxCents: i64(5000)}, 5000, true},
		{"one above max", cartSubtotalRangeParams{MaxCents: i64(5000)}, 5001, false},
		{"inside both bounds", cartSubtotalRangeParams{MinCents: i64(1000), MaxCents: i64(5000)}, 3000, true},
		{"below both bounds", cartSubtotalRangeParams{MinCents: i64(1000), MaxCents: i64(5000)}, 500, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := json.Marshal(tc.params)
			got := mustEval(t, cartSubtotalRange{}, env(promotion.EnvironmentInput{SubtotalCents: tc.sub}), p)
			if got != tc.want {
				t.Errorf("subtotal=%d params=%s => %v, want %v", tc.sub, p, got, tc.want)
			}
		})
	}
}

func TestCartSubtotalRangeValidateParams(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty object ok", `{}`, false},
		{"null ok", `null`, false},
		{"valid range", `{"min_cents":100,"max_cents":500}`, false},
		{"negative min", `{"min_cents":-1}`, true},
		{"negative max", `{"max_cents":-5}`, true},
		{"min > max", `{"min_cents":500,"max_cents":100}`, true},
		{"malformed json", `{not json}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cartSubtotalRange{}.ValidateParams(raw(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateParams(%s) err=%v, wantErr=%v", tc.raw, err, tc.wantErr)
			}
		})
	}
}

func TestCartQtyRangeEvaluate(t *testing.T) {
	lines := []promotion.CartLineView{line("A", 2), line("B", 3)} // total 5
	cases := []struct {
		name   string
		params cartQtyRangeParams
		want   bool
	}{
		{"no bounds always true", cartQtyRangeParams{}, true},
		{"at min inclusive", cartQtyRangeParams{Min: f64(5)}, true},
		{"above min", cartQtyRangeParams{Min: f64(4)}, true},
		{"below min", cartQtyRangeParams{Min: f64(6)}, false},
		{"at max inclusive", cartQtyRangeParams{Max: f64(5)}, true},
		{"above max", cartQtyRangeParams{Max: f64(4)}, false},
		{"within range", cartQtyRangeParams{Min: f64(1), Max: f64(10)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := json.Marshal(tc.params)
			got := mustEval(t, cartQtyRange{}, env(promotion.EnvironmentInput{Lines: lines}), p)
			if got != tc.want {
				t.Errorf("params=%s => %v, want %v", p, got, tc.want)
			}
		})
	}
}

func TestCartContainsSkuEvaluate(t *testing.T) {
	lines := []promotion.CartLineView{line("WIDGET", 2), line("GADGET", 1)}
	cases := []struct {
		name   string
		params string
		want   bool
	}{
		{"sku present", `{"skus":["WIDGET"]}`, true},
		{"sku absent", `{"skus":["NOPE"]}`, false},
		{"one of several present", `{"skus":["NOPE","GADGET"]}`, true},
		{"min_qty met across line", `{"skus":["WIDGET"],"min_qty":2}`, true},
		{"min_qty not met", `{"skus":["WIDGET"],"min_qty":3}`, false},
		{"min_qty met by aggregate", `{"skus":["WIDGET","GADGET"],"min_qty":3}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustEval(t, cartContainsSku{}, env(promotion.EnvironmentInput{Lines: lines}), raw(tc.params))
			if got != tc.want {
				t.Errorf("params=%s => %v, want %v", tc.params, got, tc.want)
			}
		})
	}
}

func TestCustomerGroupInEvaluate(t *testing.T) {
	cases := []struct {
		name  string
		group string
		want  bool
	}{
		{"member of group", "vip", true},
		{"not a member", "regular", false},
		{"guest (empty group) never matches", "", false},
	}
	params := raw(`{"group_slugs":["vip","wholesale"]}`)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustEval(t, customerGroupIn{}, env(promotion.EnvironmentInput{CustomerGroupSlug: tc.group}), params)
			if got != tc.want {
				t.Errorf("group=%q => %v, want %v", tc.group, got, tc.want)
			}
		})
	}
}

func TestChannelInEvaluate(t *testing.T) {
	params := raw(`{"slugs":["us","ca"]}`)
	cases := []struct {
		name string
		slug string
		want bool
	}{
		{"channel in list", "us", true},
		{"channel not in list", "eu", false},
		{"empty channel never matches", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustEval(t, channelIn{}, env(promotion.EnvironmentInput{ChannelSlug: tc.slug}), params)
			if got != tc.want {
				t.Errorf("slug=%q => %v, want %v", tc.slug, got, tc.want)
			}
		})
	}
}

func TestCountryInEvaluate(t *testing.T) {
	params := raw(`{"codes":["US","ca"]}`) // mixed case in config
	cases := []struct {
		name string
		code string
		want bool
	}{
		{"exact upper match", "US", true},
		{"env lower vs config upper", "us", true},
		{"env upper vs config lower", "CA", true},
		{"not listed", "DE", false},
		{"empty country never matches", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustEval(t, countryIn{}, env(promotion.EnvironmentInput{CountryCode: tc.code}), params)
			if got != tc.want {
				t.Errorf("code=%q => %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

func TestProductInCategoryEvaluate(t *testing.T) {
	catA := uuid.New()
	catB := uuid.New()
	other := uuid.New()
	lines := []promotion.CartLineView{line("A", 1, catA), line("B", 1, other)}
	params := func(ids ...uuid.UUID) json.RawMessage {
		strs := make([]string, len(ids))
		for i, id := range ids {
			strs[i] = id.String()
		}
		b, _ := json.Marshal(productInCategoryParams{CategoryIDs: strs})
		return b
	}
	cases := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{"line in target category", params(catA), true},
		{"no line in target category", params(catB), false},
		{"one of several matches", params(catB, catA), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mustEval(t, productInCategory{}, env(promotion.EnvironmentInput{Lines: lines}), tc.raw)
			if got != tc.want {
				t.Errorf("%s => %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestProductInCategoryValidateParams(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid uuid", `{"category_ids":["` + uuid.New().String() + `"]}`, false},
		{"empty list", `{"category_ids":[]}`, true},
		{"missing key", `{}`, true},
		{"not a uuid", `{"category_ids":["not-a-uuid"]}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := productInCategory{}.ValidateParams(raw(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateParams(%s) err=%v, wantErr=%v", tc.raw, err, tc.wantErr)
			}
		})
	}
}

// TestUnmarshalParamsEmptyAndNull confirms the shared helper treats an empty blob and a literal "null" as an empty
// struct (no error), which is what lets a `params = '{}'`-style row evaluate without special casing
func TestUnmarshalParamsEmptyAndNull(t *testing.T) {
	// A condition with no bounds set is always-true; feed it empty/null
	// params and confirm it evaluates without error to true
	for _, p := range []json.RawMessage{nil, raw(""), raw("null"), raw("{}")} {
		got, err := cartSubtotalRange{}.Evaluate(context.Background(), env(promotion.EnvironmentInput{SubtotalCents: 100}), p)
		if err != nil {
			t.Fatalf("params=%q: unexpected error %v", p, err)
		}
		if !got {
			t.Errorf("params=%q: no-bounds condition should be true", p)
		}
	}
}

// TestEvaluateMalformedParamsErrors confirms Evaluate surfaces a decode
// error rather than silently matching when the params blob is malformed
func TestEvaluateMalformedParamsErrors(t *testing.T) {
	_, err := cartSubtotalRange{}.Evaluate(context.Background(), env(promotion.EnvironmentInput{}), raw(`{"min_cents": "not-a-number"}`))
	if err == nil {
		t.Fatal("expected a decode error for a malformed params blob")
	}
}

// A window that wraps past midnight covers both legs, and its day filter is keyed to the window's own start day - so a Friday 22:00-02:00
// happy hour is still on at 01:00 on Saturday. Requiring start < end made late-night promotions unexpressible
func TestScheduleOvernightWindow(t *testing.T) {
	// 2026-08-07 is a Friday, ISO weekday 5
	friday := func(hour int) string { return "2026-08-07T" + twoDigit(hour) + ":30:00Z" }
	saturday := func(hour int) string { return "2026-08-08T" + twoDigit(hour) + ":30:00Z" }

	tests := []struct {
		name   string
		params string
		now    string
		want   bool
	}{
		{"friday evening leg", `{"days":[5],"hours":[[22,2]],"timezone":"UTC"}`, friday(23), true},
		{"saturday morning leg belongs to friday", `{"days":[5],"hours":[[22,2]],"timezone":"UTC"}`, saturday(1), true},
		{"saturday evening is not friday's window", `{"days":[5],"hours":[[22,2]],"timezone":"UTC"}`, saturday(23), false},
		{"friday morning is outside", `{"days":[5],"hours":[[22,2]],"timezone":"UTC"}`, friday(3), false},
		{"no day filter matches either leg", `{"hours":[[22,2]],"timezone":"UTC"}`, saturday(1), true},
		{"plain window still works", `{"hours":[[6,9]],"timezone":"UTC"}`, friday(7), true},
		{"plain window excludes outside", `{"hours":[[6,9]],"timezone":"UTC"}`, friday(9), false},
		{"day-only filter", `{"days":[5],"timezone":"UTC"}`, friday(3), true},
		{"day-only filter excludes", `{"days":[5],"timezone":"UTC"}`, saturday(3), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := promotion.PinNowForTest(mustTime(t, tt.now))
			defer restore()
			if got := mustEval(t, schedule{}, env(promotion.EnvironmentInput{}), raw(tt.params)); got != tt.want {
				t.Errorf("schedule at %s = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestScheduleValidateParams(t *testing.T) {
	tests := []struct {
		name    string
		params  string
		wantErr bool
	}{
		{"overnight window accepted", `{"hours":[[22,2]]}`, false},
		{"plain window accepted", `{"hours":[[6,9]]}`, false},
		{"empty window rejected", `{"hours":[[6,6]]}`, true},
		{"out of range rejected", `{"hours":[[6,25]]}`, true},
		{"neither days nor hours rejected", `{}`, true},
		{"bad weekday rejected", `{"days":[8]}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (schedule{}).ValidateParams(raw(tt.params))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateParams(%s) error = %v, wantErr %v", tt.params, err, tt.wantErr)
			}
		})
	}
}

// A multi-valued attribute arrives comma-joined, so the condition has to match on any single value - comparing the joined string whole
// meant a rule on ["red"] missed a product whose colour is "red,blue" and the operator had to guess the exact combination
func TestProductHasAttributeMultiValue(t *testing.T) {
	withAttrs := func(kvs map[string]string) promotion.Environment {
		return env(promotion.EnvironmentInput{Lines: []promotion.CartLineView{{
			LineID: uuid.New(), ProductSKU: "SHIRT", Quantity: 1, AttributeKVs: kvs,
		}}})
	}
	tests := []struct {
		name   string
		kvs    map[string]string
		params string
		want   bool
	}{
		{"single value", map[string]string{"color": "red"}, `{"attribute":"color","values":["red"]}`, true},
		{"first of many", map[string]string{"color": "red,blue"}, `{"attribute":"color","values":["red"]}`, true},
		{"last of many", map[string]string{"color": "red,blue"}, `{"attribute":"color","values":["blue"]}`, true},
		{"middle of many", map[string]string{"color": "red,green,blue"}, `{"attribute":"color","values":["green"]}`, true},
		{"tolerates spacing", map[string]string{"color": "red, blue"}, `{"attribute":"color","values":["blue"]}`, true},
		{"no overlap", map[string]string{"color": "red,blue"}, `{"attribute":"color","values":["black"]}`, false},
		{"other attribute", map[string]string{"size": "l"}, `{"attribute":"color","values":["red"]}`, false},
		{"substring is not a match", map[string]string{"color": "reddish"}, `{"attribute":"color","values":["red"]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustEval(t, productHasAttribute{}, withAttrs(tt.kvs), raw(tt.params)); got != tt.want {
				t.Errorf("productHasAttribute = %v, want %v", got, tt.want)
			}
		})
	}
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

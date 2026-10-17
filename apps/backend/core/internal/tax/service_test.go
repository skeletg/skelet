package tax_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	taxconditions "github.com/skeletg/skelet/apps/backend/core/internal/tax/conditions"
	taxeffects "github.com/skeletg/skelet/apps/backend/core/internal/tax/effects"
)

// classRule builds a rule that taxes lines of a given product tax class
// at a percentage - An empty class means "match any line"
func classRule(name, class string, bps int32) tax.Rule {
	var conds []tax.Condition
	if class != "" {
		conds = []tax.Condition{{
			Type:   "core.product_tax_class",
			Params: json.RawMessage(fmt.Sprintf(`{"classes":[%q]}`, class)),
		}}
	}
	return tax.Rule{
		ID:         uuid.New(),
		Name:       name,
		IsActive:   true,
		Conditions: conds,
		Effects: []tax.Effect{{
			Type:   "core.percentage",
			Params: json.RawMessage(fmt.Sprintf(`{"bps":%d}`, bps)),
		}},
	}
}

// effectRule builds a no-condition (matches any), non-exclusive
// (stacks) rule with a single effect of the given type + rate
func effectRule(name, effectType string, bps, priority int32) tax.Rule {
	return tax.Rule{
		ID:       uuid.New(),
		Name:     name,
		IsActive: true,
		Priority: priority,
		Effects: []tax.Effect{{
			Type:   effectType,
			Params: json.RawMessage(fmt.Sprintf(`{"bps":%d}`, bps)),
		}},
	}
}

func TestStackingAndCompound(t *testing.T) {
	channel := uuid.New()

	t.Run("empty exclusivity groups stack (GST + PST)", func(t *testing.T) {
		svc := tax.NewService(fakeRepo{rules: []tax.Rule{
			effectRule("gst", "core.percentage", 500, 0),
			effectRule("pst", "core.percentage", 700, 0),
		}})
		res, err := svc.Compute(context.Background(), tax.ComputeInput{ChannelID: channel, TaxableCents: 10000})
		if err != nil {
			t.Fatalf("Compute: %v", err)
		}
		if res.Cents != 1200 { // 500 + 700, both apply
			t.Errorf("got %d, want 1200", res.Cents)
		}
	})

	t.Run("compound taxes on base + prior tax", func(t *testing.T) {
		svc := tax.NewService(fakeRepo{rules: []tax.Rule{
			effectRule("gst", "core.percentage", 500, 0),           // 10000*5% = 500
			effectRule("qst", "core.compound_percentage", 700, 10), // (10000+500)*7% = 735
		}})
		res, err := svc.Compute(context.Background(), tax.ComputeInput{ChannelID: channel, TaxableCents: 10000})
		if err != nil {
			t.Fatalf("Compute: %v", err)
		}
		if res.Cents != 1235 { // 500 + 735
			t.Errorf("got %d, want 1235", res.Cents)
		}
	})
}

func TestCustomerGroupDimension(t *testing.T) {
	channel := uuid.New()
	// A 0% rule gated to the "b2b" group, sharing an exclusivity group with the standard rule so it wins (reverse charge),
	// plus the standard 20% fallback
	b2bZero := tax.Rule{
		ID: uuid.New(), Name: "b2b reverse charge", IsActive: true,
		Priority: 0, ExclusivityGroup: "jurisdiction",
		Conditions: []tax.Condition{{Type: "core.customer_group", Params: json.RawMessage(`{"groups":["b2b"]}`)}},
		Effects:    []tax.Effect{{Type: "core.percentage", Params: json.RawMessage(`{"bps":0}`)}},
	}
	standard := tax.Rule{
		ID: uuid.New(), Name: "standard", IsActive: true,
		Priority: 10, ExclusivityGroup: "jurisdiction",
		Effects: []tax.Effect{{Type: "core.percentage", Params: json.RawMessage(`{"bps":2000}`)}},
	}
	svc := tax.NewService(fakeRepo{rules: []tax.Rule{b2bZero, standard}})

	run := func(group string) int64 {
		res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{
			ChannelID:         channel,
			CustomerGroupSlug: group,
			Lines:             []tax.LineTaxInput{{TaxableCents: 10000}},
		})
		if err != nil {
			t.Fatalf("ComputeOrder(%q): %v", group, err)
		}
		return res.TotalCents
	}

	if got := run("b2b"); got != 0 {
		t.Errorf("b2b customer: got %d, want 0 (reverse charge)", got)
	}
	if got := run(""); got != 2000 {
		t.Errorf("regular customer: got %d, want 2000", got)
	}
}

func TestComputeOrder(t *testing.T) {
	channel := uuid.New()

	t.Run("mixed classes taxed per line", func(t *testing.T) {
		svc := tax.NewService(fakeRepo{rules: []tax.Rule{
			classRule("standard", "standard", 2000),
			classRule("reduced", "reduced", 700),
		}})
		res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{
			ChannelID: channel,
			Lines: []tax.LineTaxInput{
				{ProductTaxClass: "standard", TaxableCents: 10000},
				{ProductTaxClass: "reduced", TaxableCents: 10000},
			},
		})
		if err != nil {
			t.Fatalf("ComputeOrder: %v", err)
		}
		if res.TotalCents != 2700 { // 2000 + 700
			t.Errorf("total: got %d, want 2700", res.TotalCents)
		}
		if res.Lines[0].TotalCents != 2000 || res.Lines[1].TotalCents != 700 {
			t.Errorf("per-line: got %d/%d, want 2000/700", res.Lines[0].TotalCents, res.Lines[1].TotalCents)
		}
	})

	t.Run("round-per-line-then-sum differs from whole-order", func(t *testing.T) {
		svc := tax.NewService(fakeRepo{rules: []tax.Rule{classRule("std", "standard", 875)}})
		res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{
			ChannelID: channel,
			Lines: []tax.LineTaxInput{
				{ProductTaxClass: "standard", TaxableCents: 1000},
				{ProductTaxClass: "standard", TaxableCents: 1000},
			},
		})
		if err != nil {
			t.Fatalf("ComputeOrder: %v", err)
		}
		// halfUp(1000*875/10000)=88 per line → 176, vs whole-order
		// halfUp(2000*875/10000)=175 - The per-line value is canonical
		if res.TotalCents != 176 {
			t.Errorf("total: got %d, want 176 (88+88)", res.TotalCents)
		}
	})

	t.Run("any-class rule matches every line (parity)", func(t *testing.T) {
		svc := tax.NewService(fakeRepo{rules: []tax.Rule{classRule("vat", "", 2000)}})
		res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{
			ChannelID: channel,
			Lines:     []tax.LineTaxInput{{ProductTaxClass: "anything", TaxableCents: 2500}},
		})
		if err != nil {
			t.Fatalf("ComputeOrder: %v", err)
		}
		if res.TotalCents != 500 {
			t.Errorf("total: got %d, want 500", res.TotalCents)
		}
	})

	t.Run("line with no matching class → untaxed", func(t *testing.T) {
		svc := tax.NewService(fakeRepo{rules: []tax.Rule{classRule("reduced", "reduced", 700)}})
		res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{
			ChannelID: channel,
			Lines:     []tax.LineTaxInput{{ProductTaxClass: "standard", TaxableCents: 10000}},
		})
		if err != nil {
			t.Fatalf("ComputeOrder: %v", err)
		}
		if res.TotalCents != 0 {
			t.Errorf("total: got %d, want 0", res.TotalCents)
		}
	})
}

// fakeRepo embeds the Repository interface (nil) so it satisfies the
// type; Compute only ever calls ListActiveRulesByChannel
type fakeRepo struct {
	tax.Repository
	rules []tax.Rule
	loads *int
}

func (f fakeRepo) ListActiveRulesByChannel(context.Context, uuid.UUID) ([]tax.Rule, error) {
	if f.loads != nil {
		*f.loads++
	}
	return f.rules, nil
}

// jpRule builds a jurisdiction + percentage rule shaped like the seeded
// parity rules
func jpRule(name, country, region string, bps, priority int32, group string) tax.Rule {
	return tax.Rule{
		ID:               uuid.New(),
		Name:             name,
		Priority:         priority,
		ExclusivityGroup: group,
		IsActive:         true,
		Conditions: []tax.Condition{{
			Type:   "core.jurisdiction",
			Params: json.RawMessage(fmt.Sprintf(`{"country_code":%q,"region_code":%q}`, country, region)),
		}},
		Effects: []tax.Effect{{
			Type:   "core.percentage",
			Params: json.RawMessage(fmt.Sprintf(`{"bps":%d}`, bps)),
		}},
	}
}

func TestComputeParity(t *testing.T) {
	channel := uuid.New()
	tests := []struct {
		name      string
		rules     []tax.Rule
		taxable   int64
		inclusive bool
		country   string
		region    string
		wantCents int64
		wantRule  bool // expect a matched rule reported
	}{
		{
			name:    "no rules → zero",
			taxable: 19999,
		},
		{
			name:    "zero taxable → zero, no eval",
			rules:   []tax.Rule{jpRule("default", "", "", 875, 20, "jurisdiction")},
			taxable: 0,
		},
		{
			name:      "8.75% on $19.99 (half-up)",
			rules:     []tax.Rule{jpRule("default", "", "", 875, 20, "jurisdiction")},
			taxable:   1999,
			wantCents: 175, // halfUp(1999*875/10000) = halfUp(174.9125)
			wantRule:  true,
		},
		{
			name:      "20% VAT exclusive on €25.00",
			rules:     []tax.Rule{jpRule("vat", "", "", 2000, 20, "jurisdiction")},
			taxable:   2500,
			wantCents: 500,
			wantRule:  true,
		},
		{
			name:      "20% VAT inclusive extracts portion",
			rules:     []tax.Rule{jpRule("vat", "", "", 2000, 20, "jurisdiction")},
			taxable:   12000,
			inclusive: true,
			wantCents: 2000, // 12000*2000/12000
			wantRule:  true,
		},
		{
			name:      "0% active rule → zero cents but rule reported",
			rules:     []tax.Rule{jpRule("zero", "", "", 0, 20, "jurisdiction")},
			taxable:   5000,
			wantCents: 0,
			wantRule:  true,
		},
		{
			name: "most specific jurisdiction wins, no stacking",
			rules: []tax.Rule{
				jpRule("default", "", "", 1000, 20, "jurisdiction"),
				jpRule("us-ca", "US", "CA", 875, 0, "jurisdiction"),
			},
			taxable:   10000,
			country:   "US",
			region:    "CA",
			wantCents: 875, // only the US/CA rule applies
			wantRule:  true,
		},
		{
			name:      "jurisdiction non-match → zero",
			rules:     []tax.Rule{jpRule("us-only", "US", "", 875, 10, "jurisdiction")},
			taxable:   10000,
			country:   "DE",
			wantCents: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := tax.NewService(fakeRepo{rules: tc.rules})
			res, err := svc.Compute(context.Background(), tax.ComputeInput{
				ChannelID:    channel,
				CountryCode:  tc.country,
				RegionCode:   tc.region,
				TaxableCents: tc.taxable,
				TaxInclusive: tc.inclusive,
			})
			if err != nil {
				t.Fatalf("Compute: %v", err)
			}
			if res.Cents != tc.wantCents {
				t.Errorf("Cents: got %d, want %d", res.Cents, tc.wantCents)
			}
			if tc.wantRule && res.RuleID == nil {
				t.Error("expected a matched rule, got none")
			}
			if !tc.wantRule && res.RuleID != nil {
				t.Errorf("expected no matched rule, got %v", *res.RuleID)
			}
		})
	}
}

// core.jurisdiction, core.product_tax_class and core.percentage, installed the way wiring installs them
func init() {
	taxconditions.Register(tax.DefaultRegistry)
	taxeffects.Register(tax.DefaultRegistry)
}

// Placement taxes every line, so the rules load once per order, not once per line
func TestComputeOrderLoadsTheRulesOnce(t *testing.T) {
	loads := 0
	svc := tax.NewService(fakeRepo{rules: []tax.Rule{classRule("standard", "", 2000)}, loads: &loads})
	lines := make([]tax.LineTaxInput, 40)
	for i := range lines {
		lines[i] = tax.LineTaxInput{TaxableCents: 1000}
	}
	res, err := svc.ComputeOrder(context.Background(), tax.ComputeOrderInput{ChannelID: uuid.New(), Lines: lines})
	if err != nil || res.TotalCents != 40*200 {
		t.Fatalf("ComputeOrder = %d, %v - want %d", res.TotalCents, err, 40*200)
	}
	if loads != 1 {
		t.Errorf("rules loaded %d times for one order, want 1", loads)
	}
}

func TestJurisdictionMatchesWhateverTheCase(t *testing.T) {
	svc := tax.NewService(fakeRepo{rules: []tax.Rule{jpRule("bavaria", "de", "by", 1900, 0, "")}})
	res, err := svc.Compute(context.Background(), tax.ComputeInput{ChannelID: uuid.New(), CountryCode: "DE", RegionCode: "BY", TaxableCents: 10000})
	if err != nil || res.Cents != 1900 {
		t.Fatalf("Compute(DE/BY) against a de/by rule = %d, %v - want 1900", res.Cents, err)
	}
}

package e2e

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

func (h *harness) newTaxRule(t *testing.T, channelID uuid.UUID, bps int, conditions ...tax.Condition) {
	t.Helper()
	if _, err := h.D.Tax.CreateRule(h.Ctx, tax.CreateRuleInput{
		Rule:       tax.Rule{ChannelID: channelID, Name: "E2E tax", IsActive: true},
		Conditions: conditions,
		Effects: []tax.Effect{{Type: "core.percentage",
			Params: json.RawMessage(fmt.Sprintf(`{"bps":%d}`, bps))}},
	}); err != nil {
		t.Fatalf("create tax rule: %v", err)
	}
}

// Scenario: tax-exclusive pricing adds the computed tax on top - 8.75% on a US order lands on
// TaxAmount, in the total, and as a snapshot tax line carrying the rate
func TestTaxExclusiveAddsOnTop(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.newTaxRule(t, f.Channel.ID, 875, tax.Condition{
		Type: "core.jurisdiction", Params: json.RawMessage(`{"country_code":"US"}`),
	})

	o := h.PlaceGuestCOD(t, f, 1)
	if o.TaxAmount != 175 { // round(2000 * 875 / 10000)
		t.Errorf("tax = %d, want 175", o.TaxAmount)
	}
	if o.TotalAmount != 2175 {
		t.Errorf("total = %d, want subtotal 2000 + tax 175", o.TotalAmount)
	}
	taxLines, err := h.D.Order.TaxLines(h.Ctx, o.ID)
	if err != nil || len(taxLines) != 1 || taxLines[0].RateBPS != 875 || taxLines[0].AmountCents != 175 {
		t.Errorf("tax lines = %+v (%v), want one snapshot at 875bps/175", taxLines, err)
	}
	h.Settle(t)
}

// Scenario: a tax-inclusive channel keeps the shelf price fixed and carves the tax OUT of it -
// the total equals the subtotal and TaxAmount reports the extracted share
func TestTaxInclusiveCarvesOutOfTotal(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 2000)
	h.newTaxRule(t, f.Channel.ID, 875)

	// No Go setter exists for the flag - flip the column and re-read (GetByID is uncached)
	if _, err := h.D.DB.ExecContext(h.Ctx,
		`UPDATE channels SET tax_inclusive_pricing = TRUE WHERE id = $1`, f.Channel.ID); err != nil {
		t.Fatalf("mark channel inclusive: %v", err)
	}
	ch, err := h.D.Channel.GetByID(h.Ctx, f.Channel.ID)
	if err != nil {
		t.Fatalf("re-read channel: %v", err)
	}

	o, err := h.TryPlace(t, ch, h.NewGuestCart(t, f, 1))
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if o.TotalAmount != 2000 {
		t.Errorf("total = %d, want the fixed shelf 2000 - inclusive tax must not add", o.TotalAmount)
	}
	if o.TaxAmount != 161 { // round(2000 * 875 / 10875) - the extraction formula
		t.Errorf("carved-out tax = %d, want 161", o.TaxAmount)
	}
	h.Settle(t)
}

// Scenario: tax-class scoping - a reduced-class rule ignores standard products and fires once the
// product is reclassified
func TestTaxClassScopesRules(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 10, 1000)
	h.newTaxRule(t, f.Channel.ID, 500, tax.Condition{
		Type: "core.product_tax_class", Params: json.RawMessage(`{"classes":["reduced"]}`),
	})

	standard := h.PlaceGuestCOD(t, f, 1)
	if standard.TaxAmount != 0 {
		t.Errorf("standard-class product taxed %d by a reduced-class rule, want 0", standard.TaxAmount)
	}

	if _, err := h.D.Catalog.SetProductTaxClass(h.Ctx, f.Product.ID, "reduced"); err != nil {
		t.Fatalf("reclassify: %v", err)
	}
	reduced := h.PlaceGuestCOD(t, f, 1)
	if reduced.TaxAmount != 50 {
		t.Errorf("reduced-class product taxed %d, want 50", reduced.TaxAmount)
	}
	h.Settle(t)
}

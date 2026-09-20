package effects

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// Applies a rate to the base PLUS all tax so far, modelling jurisdictions like Quebec's QST historically applying on the GST-inclusive amount - give it a higher priority number so it runs after the taxes it compounds on
// In tax-inclusive mode it falls back to a plain percentage
type compound struct{}

type compoundParams struct {
	BPS int32 `json:"bps"`
}

func (compound) Slug() string  { return "core.compound_percentage" }
func (compound) Label() string { return "Compound percentage (tax on tax)" }
func (compound) Description() string {
	return "Taxes the base plus all tax accumulated so far. Order with priority so it runs after the taxes it compounds on."
}

func (compound) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["bps"],
  "properties": {
    "bps": {"type": "integer", "minimum": 0, "maximum": 100000, "title": "Rate (basis points, 875 = 8.75%)"}
  },
  "additionalProperties": false
}`)
}

func (compound) ValidateParams(raw json.RawMessage) error {
	var p compoundParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.BPS < 0 || p.BPS > 100000 {
		return fmt.Errorf("bps must be in 0..100000")
	}
	return nil
}

func (compound) Apply(_ context.Context, env tax.Environment, raw json.RawMessage, ledger *tax.Ledger) error {
	var p compoundParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.BPS <= 0 {
		return nil
	}
	base := env.TaxableBaseCents()
	if base <= 0 {
		return nil
	}
	if env.TaxInclusive() {
		// Compounding on a gross price is ill-defined - treat it as a plain percentage of the gross base
		ledger.Add(divRoundHalfUp(base*int64(p.BPS), 10000+int64(p.BPS)), p.BPS)
		return nil
	}
	// Tax-on-tax - the rate applies to base plus the tax accumulated so far
	compoundBase := base + ledger.Total()
	ledger.Add(divRoundHalfUp(compoundBase*int64(p.BPS), 10000), p.BPS)
	return nil
}

func init() { add(compound{}) }

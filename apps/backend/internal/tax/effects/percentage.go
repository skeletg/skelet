package effects

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/internal/tax"
)

// percentage applies a basis-point rate to the taxable base - Tax-inclusive extracts from the gross (base*r/(10000+r)),
// else it's an additive surcharge (base*r/10000) - Integer math keeps cents deterministic, rounded half-up
type percentage struct{}

type percentageParams struct {
	BPS int32 `json:"bps"`
}

func (percentage) Slug() string  { return "core.percentage" }
func (percentage) Label() string { return "Percentage rate" }
func (percentage) Description() string {
	return "Taxes the base at a basis-point rate (875 = 8.75%)."
}

func (percentage) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["bps"],
  "properties": {
    "bps": {"type": "integer", "minimum": 0, "maximum": 100000, "title": "Rate (basis points, 875 = 8.75%)"}
  },
  "additionalProperties": false
}`)
}

func (percentage) ValidateParams(raw json.RawMessage) error {
	var p percentageParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.BPS < 0 || p.BPS > 100000 {
		return fmt.Errorf("bps must be in 0..100000")
	}
	return nil
}

func (percentage) Apply(_ context.Context, env tax.Environment, raw json.RawMessage, ledger *tax.Ledger) error {
	var p percentageParams
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
	var cents int64
	if env.TaxInclusive() {
		cents = divRoundHalfUp(base*int64(p.BPS), 10000+int64(p.BPS))
	} else {
		cents = divRoundHalfUp(base*int64(p.BPS), 10000)
	}
	ledger.Add(cents, p.BPS)
	return nil
}

func init() { add(percentage{}) }

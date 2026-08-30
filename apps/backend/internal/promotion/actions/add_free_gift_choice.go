package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/internal/promotion"
)

// The multi-SKU variant of core.add_free_gift - instead of forcing one SKU the rule offers a list, and the engine queues each as a candidate on the Ledger at qty 0, meaning "offered, not picked"
// INCOMPLETE: nothing picks yet. Order placement grants qty > 0 gifts only, so a rule carrying just this action discounts nothing and adds no line
// What's missing is the selection round-trip - exposing the candidates through the cart API and recording the shopper's pick against the rule - until then use core.add_free_gift, which grants outright
type addFreeGiftChoice struct{}

type addFreeGiftChoiceParams struct {
	ProductSKUs []string `json:"product_skus"`
	MaxPicks    int32    `json:"max_picks"`
}

func (addFreeGiftChoice) Slug() string  { return "core.add_free_gift_choice" }
func (addFreeGiftChoice) Label() string { return "Add free gift (customer picks) - not yet selectable" }
func (addFreeGiftChoice) Description() string {
	return "Offers a choice of free gifts from a list of SKUs. The picker isn't built yet, so nothing is granted - use \"Add free gift\" to grant a gift outright."
}

func (addFreeGiftChoice) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["product_skus", "max_picks"],
  "properties": {
    "product_skus": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "title": "Gift options (SKUs)",
      "x-widget": "sku-picker"
    },
    "max_picks": {"type": "integer", "minimum": 1, "title": "Max items the customer can pick"}
  },
  "additionalProperties": false
}`)
}

func (addFreeGiftChoice) ValidateParams(raw json.RawMessage) error {
	var p addFreeGiftChoiceParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.ProductSKUs) == 0 {
		return fmt.Errorf("product_skus is required and must have at least one entry")
	}
	if p.MaxPicks <= 0 {
		return fmt.Errorf("max_picks must be > 0")
	}
	return nil
}

func (addFreeGiftChoice) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p addFreeGiftChoiceParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	// Queue each candidate at qty 0 - offered, not granted, so no line is created and no stock committed
	// Dropping these outright makes even the intent invisible
	for _, sku := range p.ProductSKUs {
		ledger.AddFreeGift(sku, 0)
	}
	return nil
}

func init() {
	add(addFreeGiftChoice{})
}

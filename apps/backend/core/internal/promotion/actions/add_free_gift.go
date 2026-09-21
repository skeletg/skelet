package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// addFreeGift queues a free-gift entry on the Ledger - PlaceOrder turns each entry into an order_line with unit_price=0
// and is_gift=true, and reserves inventory at the same time - The cart UI marks the gift line as non-removable
type addFreeGift struct{}

type addFreeGiftParams struct {
	ProductSKU string  `json:"product_sku"`
	Qty        float64 `json:"qty"`
}

func (addFreeGift) Slug() string  { return "core.add_free_gift" }
func (addFreeGift) Label() string { return "Add free gift" }
func (addFreeGift) Description() string {
	return "Add a zero-priced gift line to the cart at qty N when the rule matches."
}

func (addFreeGift) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["product_sku", "qty"],
  "properties": {
    "product_sku": {"type": "string", "title": "Gift product SKU", "x-widget": "sku-picker"},
    "qty": {"type": "integer", "minimum": 1, "title": "Gift quantity"}
  },
  "additionalProperties": false
}`)
}

func (addFreeGift) ValidateParams(raw json.RawMessage) error {
	var p addFreeGiftParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.ProductSKU == "" {
		return fmt.Errorf("product_sku is required")
	}
	if p.Qty <= 0 {
		return fmt.Errorf("qty must be > 0")
	}
	return nil
}

func (addFreeGift) Apply(ctx context.Context, env promotion.Environment, raw json.RawMessage, ledger promotion.Ledger, _ promotion.AppliedRuleContext) error {
	var p addFreeGiftParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	ledger.AddFreeGift(p.ProductSKU, p.Qty)
	return nil
}

func init() {
	add(addFreeGift{})
}

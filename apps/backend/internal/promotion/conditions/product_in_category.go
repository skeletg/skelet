package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
)

// productInCategory matches when any cart line's product belongs to one of the listed category IDs - Category membership is pre-resolved
// into CartLineView.CategoryIDs by the environment builder so this condition doesn't round-trip the catalog at eval time
type productInCategory struct{}

type productInCategoryParams struct {
	CategoryIDs []string `json:"category_ids"`
}

func (productInCategory) Slug() string  { return "core.product_in_category" }
func (productInCategory) Label() string { return "Cart contains product in category" }
func (productInCategory) Description() string {
	return "Any cart line's product belongs to one of the selected categories."
}

func (productInCategory) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["category_ids"],
  "properties": {
    "category_ids": {
      "type": "array",
      "items": {"type": "string", "format": "uuid"},
      "minItems": 1,
      "title": "Categories",
      "x-widget": "category-picker"
    }
  },
  "additionalProperties": false
}`)
}

func (productInCategory) ValidateParams(raw json.RawMessage) error {
	var p productInCategoryParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.CategoryIDs) == 0 {
		return fmt.Errorf("category_ids is required and must have at least one entry")
	}
	for _, id := range p.CategoryIDs {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("category_ids: %q is not a valid uuid", id)
		}
	}
	return nil
}

func (productInCategory) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p productInCategoryParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	want := make(map[uuid.UUID]struct{}, len(p.CategoryIDs))
	for _, id := range p.CategoryIDs {
		parsed, err := uuid.Parse(id)
		if err != nil {
			continue
		}
		want[parsed] = struct{}{}
	}
	for _, line := range env.Lines() {
		for _, catID := range line.CategoryIDs {
			if _, hit := want[catID]; hit {
				return true, nil
			}
		}
	}
	return false, nil
}

func init() {
	add(productInCategory{})
}

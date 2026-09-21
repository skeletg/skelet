package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// customerGroupIn matches when the customer's static group slug is in the configured list - Guest carts (no customer) never match Rules targeting "guests only" should set this condition with an empty list and combine with `core.not` - or use a dedicated `core.is_guest` once it ships
type customerGroupIn struct{}

type customerGroupInParams struct {
	GroupSlugs []string `json:"group_slugs"`
}

func (customerGroupIn) Slug() string        { return "core.customer_group_in" }
func (customerGroupIn) Label() string       { return "Customer group in list" }
func (customerGroupIn) Description() string { return "Customer belongs to one of the listed groups." }

func (customerGroupIn) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["group_slugs"],
  "properties": {
    "group_slugs": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "title": "Group slugs",
      "x-widget": "customer-group-picker"
    }
  },
  "additionalProperties": false
}`)
}

func (customerGroupIn) ValidateParams(raw json.RawMessage) error {
	var p customerGroupInParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.GroupSlugs) == 0 {
		return fmt.Errorf("group_slugs is required and must have at least one entry")
	}
	return nil
}

func (customerGroupIn) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p customerGroupInParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	have := env.CustomerGroupSlug()
	if have == "" {
		return false, nil
	}
	if slices.Contains(p.GroupSlugs, have) {
		return true, nil
	}
	return false, nil
}

func init() {
	add(customerGroupIn{})
}

package conditions

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// customerGroup matches when the customer's group is one of the configured ones - the customer tax dimension, for B2B
// reverse-charge or tax-exempt orgs - An empty list matches any customer (guests included, whose group is "")
type customerGroup struct{}

type customerGroupParams struct {
	Groups []string `json:"groups"`
}

func (customerGroup) Slug() string  { return "core.customer_group" }
func (customerGroup) Label() string { return "Customer group" }
func (customerGroup) Description() string {
	return "Matches when the customer's group is one of the listed groups (e.g. wholesale, b2b, exempt). Empty = any."
}

func (customerGroup) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "groups": {
      "type": "array",
      "title": "Customer groups (empty = any)",
      "x-widget": "customer-group-picker",
      "items": {"type": "string"}
    }
  },
  "additionalProperties": false
}`)
}

func (customerGroup) ValidateParams(raw json.RawMessage) error {
	var p customerGroupParams
	return unmarshalParams(raw, &p)
}

func (customerGroup) Evaluate(_ context.Context, env tax.Environment, raw json.RawMessage) (bool, error) {
	var p customerGroupParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	if len(p.Groups) == 0 {
		return true, nil
	}
	got := env.CustomerGroupSlug()
	if slices.Contains(p.Groups, got) {
		return true, nil
	}
	return false, nil
}

func init() { add(customerGroup{}) }

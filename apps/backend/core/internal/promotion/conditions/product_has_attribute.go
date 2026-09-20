package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// productHasAttribute matches when any cart line's product has attribute X with a value in the configured list - AttributeKVs is pre-resolved into a `slug → value` map per line, with multi-valued attributes comma-joined The comparison splits on `,` and matches if ANY of the product's values is in the list, so a rule on values ["red"] fires for a product whose colour is "red,blue" - comparing the joined string whole meant the operator had to guess the exact stored combination and its ordering
type productHasAttribute struct{}

type productHasAttributeParams struct {
	Attribute string   `json:"attribute"`
	Values    []string `json:"values"`
}

func (productHasAttribute) Slug() string  { return "core.product_has_attribute" }
func (productHasAttribute) Label() string { return "Cart contains product with attribute" }
func (productHasAttribute) Description() string {
	return "Any cart line's product has attribute X set to a value in the list."
}

func (productHasAttribute) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["attribute", "values"],
  "properties": {
    "attribute": {"type": "string", "title": "Attribute slug", "x-widget": "attribute-picker"},
    "values": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "title": "Allowed values"
    }
  },
  "additionalProperties": false
}`)
}

func (productHasAttribute) ValidateParams(raw json.RawMessage) error {
	var p productHasAttributeParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.Attribute == "" {
		return fmt.Errorf("attribute is required")
	}
	if len(p.Values) == 0 {
		return fmt.Errorf("values is required and must have at least one entry")
	}
	return nil
}

func (productHasAttribute) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p productHasAttributeParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	want := make(map[string]struct{}, len(p.Values))
	for _, v := range p.Values {
		want[v] = struct{}{}
	}
	for _, line := range env.Lines() {
		if line.AttributeKVs == nil {
			continue
		}
		val, ok := line.AttributeKVs[p.Attribute]
		if !ok {
			continue
		}
		for one := range strings.SplitSeq(val, ",") {
			if _, hit := want[strings.TrimSpace(one)]; hit {
				return true, nil
			}
		}
	}
	return false, nil
}

func init() {
	add(productHasAttribute{})
}

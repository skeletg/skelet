package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/internal/promotion"
)

// customerInSegment matches when the customer belongs to any of the
// configured segments (by slug) - Segments are admin-defined buckets
// (see internal/segment) - Membership is materialised - the env
// builder pre-resolves the customer's slug list at request time so
// the condition is a cheap intersection check
//
// Guests (no customer) never match
type customerInSegment struct{}

type customerInSegmentParams struct {
	Slugs []string `json:"slugs"`
}

func (customerInSegment) Slug() string  { return "core.customer_in_segment" }
func (customerInSegment) Label() string { return "Customer in segment" }
func (customerInSegment) Description() string {
	return "Customer is a member of one of the listed segments. Use for VIP / loyalty / lapsed-buyer promotions."
}

func (customerInSegment) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["slugs"],
  "properties": {
    "slugs": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "title": "Segment slugs",
      "x-widget": "segment-picker"
    }
  },
  "additionalProperties": false
}`)
}

func (customerInSegment) ValidateParams(raw json.RawMessage) error {
	var p customerInSegmentParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Slugs) == 0 {
		return fmt.Errorf("slugs is required and must have at least one entry")
	}
	return nil
}

func (customerInSegment) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p customerInSegmentParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	have := env.CustomerSegmentSlugs()
	if len(have) == 0 {
		return false, nil
	}
	want := make(map[string]struct{}, len(p.Slugs))
	for _, s := range p.Slugs {
		want[s] = struct{}{}
	}
	for _, s := range have {
		if _, ok := want[s]; ok {
			return true, nil
		}
	}
	return false, nil
}

func init() {
	add(customerInSegment{})
}

package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// countryIn matches when the shipping country (2-letter ISO code) is in the configured list - Codes are normalised to
// upper-case for comparison so case-mismatched inputs don't silently fail to match
type countryIn struct{}

type countryInParams struct {
	Codes []string `json:"codes"`
}

func (countryIn) Slug() string  { return "core.country_in" }
func (countryIn) Label() string { return "Shipping country in list" }
func (countryIn) Description() string {
	return "Cart's shipping country code is one of the listed ISO codes."
}

func (countryIn) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["codes"],
  "properties": {
    "codes": {
      "type": "array",
      "items": {"type": "string", "minLength": 2, "maxLength": 2},
      "minItems": 1,
      "title": "ISO country codes"
    }
  },
  "additionalProperties": false
}`)
}

func (countryIn) ValidateParams(raw json.RawMessage) error {
	var p countryInParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Codes) == 0 {
		return fmt.Errorf("codes is required and must have at least one entry")
	}
	for _, c := range p.Codes {
		if len(c) != 2 {
			return fmt.Errorf("country code %q must be 2 characters", c)
		}
	}
	return nil
}

func (countryIn) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p countryInParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	have := strings.ToUpper(env.CountryCode())
	if have == "" {
		return false, nil
	}
	for _, code := range p.Codes {
		if strings.ToUpper(code) == have {
			return true, nil
		}
	}
	return false, nil
}

func init() {
	add(countryIn{})
}

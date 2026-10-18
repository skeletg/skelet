package conditions

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
)

// jurisdiction matches the order's shipping country/region - An empty field means "any" - {country:"US"} matches any US
// region, {} is the store-wide fallback Specificity is expressed through the rule's priority, not here
type jurisdiction struct{}

type jurisdictionParams struct {
	CountryCode string `json:"country_code"`
	RegionCode  string `json:"region_code"`
}

func (jurisdiction) Slug() string  { return "core.jurisdiction" }
func (jurisdiction) Label() string { return "Jurisdiction" }
func (jurisdiction) Description() string {
	return "Matches the order's shipping country and region. Empty country or region means 'any'."
}

func (jurisdiction) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "country_code": {"type": "string", "title": "Country (empty = any)", "x-widget": "country-picker"},
    "region_code": {"type": "string", "title": "Region/state code (empty = any)"}
  },
  "additionalProperties": false
}`)
}

func (jurisdiction) ValidateParams(raw json.RawMessage) error {
	var p jurisdictionParams
	return unmarshalParams(raw, &p)
}

func (jurisdiction) Evaluate(_ context.Context, env tax.Environment, raw json.RawMessage) (bool, error) {
	var p jurisdictionParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	if p.CountryCode != "" && !strings.EqualFold(strings.TrimSpace(p.CountryCode), strings.TrimSpace(env.CountryCode())) {
		return false, nil
	}
	if p.RegionCode != "" && !strings.EqualFold(strings.TrimSpace(p.RegionCode), strings.TrimSpace(env.RegionCode())) {
		return false, nil
	}
	return true, nil
}

func init() { add(jurisdiction{}) }

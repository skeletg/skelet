package conditions

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// channelIn matches when the current channel slug is in the configured list - Redundant with the rule's own channel_id scoping for the common single-store case, but useful when one rule should apply to a hand-picked subset of stores (e.g. all EU stores share a rule via the slug list rather than maintaining N rules)
type channelIn struct{}

type channelInParams struct {
	Slugs []string `json:"slugs"`
}

func (channelIn) Slug() string        { return "core.channel_in" }
func (channelIn) Label() string       { return "Channel in list" }
func (channelIn) Description() string { return "Current channel slug is in the list." }

func (channelIn) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["slugs"],
  "properties": {
    "slugs": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 1,
      "title": "Channel slugs"
    }
  },
  "additionalProperties": false
}`)
}

func (channelIn) ValidateParams(raw json.RawMessage) error {
	var p channelInParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Slugs) == 0 {
		return fmt.Errorf("slugs is required and must have at least one entry")
	}
	return nil
}

func (channelIn) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p channelInParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	have := env.ChannelSlug()
	if have == "" {
		return false, nil
	}
	if slices.Contains(p.Slugs, have) {
		return true, nil
	}
	return false, nil
}

func init() {
	add(channelIn{})
}

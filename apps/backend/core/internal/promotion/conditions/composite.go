package conditions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Composite conditions (all_of, any_of, not) combine other conditions into boolean trees without spawning separate promotion_conditions rows, stored as nested JSON on the parent's Params
// The dashboard widget nests one level, the runtime is fully recursive, and an unknown child slug evaluates false exactly as an unknown top-level type does

type compositeChild struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

type allOfParams struct {
	Conditions []compositeChild `json:"conditions"`
}

type allOf struct{}

func (allOf) Slug() string        { return "core.all_of" }
func (allOf) Label() string       { return "All of (AND)" }
func (allOf) Description() string { return "All of the nested conditions must match." }

func (allOf) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["conditions"],
  "properties": {
    "conditions": {
      "type": "array",
      "items": {"type": "object", "x-widget": "nested-condition"},
      "title": "Conditions",
      "minItems": 1
    }
  },
  "additionalProperties": false
}`)
}

func (allOf) ValidateParams(raw json.RawMessage) error {
	var p allOfParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Conditions) == 0 {
		return fmt.Errorf("conditions must have at least one entry")
	}
	return validateChildren(p.Conditions)
}

func (allOf) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p allOfParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	for _, child := range p.Conditions {
		ok, err := evaluateChild(ctx, env, child)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// ---- any_of ----

type anyOfParams struct {
	Conditions []compositeChild `json:"conditions"`
}

type anyOf struct{}

func (anyOf) Slug() string        { return "core.any_of" }
func (anyOf) Label() string       { return "Any of (OR)" }
func (anyOf) Description() string { return "At least one of the nested conditions must match." }

func (anyOf) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["conditions"],
  "properties": {
    "conditions": {
      "type": "array",
      "items": {"type": "object", "x-widget": "nested-condition"},
      "title": "Conditions",
      "minItems": 1
    }
  },
  "additionalProperties": false
}`)
}

func (anyOf) ValidateParams(raw json.RawMessage) error {
	var p anyOfParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if len(p.Conditions) == 0 {
		return fmt.Errorf("conditions must have at least one entry")
	}
	return validateChildren(p.Conditions)
}

func (anyOf) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p anyOfParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	for _, child := range p.Conditions {
		ok, err := evaluateChild(ctx, env, child)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// ---- not ----

type notParams struct {
	Condition compositeChild `json:"condition"`
}

type notCondition struct{}

func (notCondition) Slug() string        { return "core.not" }
func (notCondition) Label() string       { return "Not" }
func (notCondition) Description() string { return "Negates the nested condition." }

func (notCondition) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["condition"],
  "properties": {
    "condition": {"type": "object", "x-widget": "nested-condition", "title": "Condition"}
  },
  "additionalProperties": false
}`)
}

func (notCondition) ValidateParams(raw json.RawMessage) error {
	var p notParams
	if err := unmarshalParams(raw, &p); err != nil {
		return err
	}
	if p.Condition.Type == "" {
		return fmt.Errorf("condition.type is required")
	}
	return validateChildren([]compositeChild{p.Condition})
}

func (notCondition) Evaluate(ctx context.Context, env promotion.Environment, raw json.RawMessage) (bool, error) {
	var p notParams
	if err := unmarshalParams(raw, &p); err != nil {
		return false, err
	}
	ok, err := evaluateChild(ctx, env, p.Condition)
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// ---- shared child plumbing ----

func validateChildren(children []compositeChild) error {
	for i, child := range children {
		if child.Type == "" {
			return fmt.Errorf("conditions[%d].type is required", i)
		}
		impl, ok := promotion.DefaultRegistry.LookupCondition(child.Type)
		if !ok {
			return fmt.Errorf("conditions[%d]: unknown type %q", i, child.Type)
		}
		if err := impl.ValidateParams(child.Params); err != nil {
			return fmt.Errorf("conditions[%d]: %w", i, err)
		}
	}
	return nil
}

func evaluateChild(ctx context.Context, env promotion.Environment, child compositeChild) (bool, error) {
	impl, ok := promotion.RegistryFrom(ctx).LookupCondition(child.Type)
	if !ok {
		// Unknown slug at eval time → treat as false, matching the engine's handling of missing top-level types, so an
		// uninstalled plugin can't crash rules that reference it or turn a targeted rule into a global one
		return false, nil
	}
	return impl.Evaluate(ctx, env, child.Params)
}

func init() {
	add(allOf{})
	add(anyOf{})
	add(notCondition{})
}

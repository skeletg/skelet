package cms

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Enforces a write-time schema on a page's bag of custom field values, returning sanitized scalar and content data (unknown keys dropped, types coerced) plus verified selections and relationships
// nil inputs mean "no change", so update paths only validate the slices they supplied, and required-field enforcement runs only on create, when both maps are non-nil
func (s *pageService) validatePageData(
	pt PageType,
	scalarData, contentData json.RawMessage,
	selections []PageAttributeSelection,
	relationships []PageRelationship,
) (json.RawMessage, json.RawMessage, []PageAttributeSelection, []PageRelationship, error) {
	scalarMap := decodeJSONMap(scalarData)
	contentMap := decodeJSONMap(contentData)

	bySlug := make(map[string]PageTypeAttribute, len(pt.Attributes))
	byID := make(map[uuid.UUID]Attribute, len(pt.Attributes))
	for _, pta := range pt.Attributes {
		bySlug[pta.Attribute.Slug] = pta
		byID[pta.Attribute.ID] = pta.Attribute
	}

	// On create, with both maps supplied, enforce required scalar fields up front
	if scalarData != nil && contentData != nil {
		for slug, pta := range bySlug {
			req := pta.IsRequired || pta.Attribute.IsRequiredDefault
			if !req {
				continue
			}
			if pta.Attribute.InputType.AcceptsValues() || pta.Attribute.InputType.IsRelationship() {
				continue // checked below
			}
			target := scalarMap
			if pta.Attribute.IsTranslatable {
				target = contentMap
			}
			if v, ok := target[slug]; !ok || isEmpty(v) {
				return nil, nil, nil, nil, fmt.Errorf("%w: %q", ErrRequiredFieldMissing, slug)
			}
		}
	}

	// Coerce and validate every supplied scalar key, dropping unknowns
	cleanScalars := map[string]any{}
	for k, v := range scalarMap {
		pta, ok := bySlug[k]
		if !ok {
			continue
		}
		if pta.Attribute.IsTranslatable {
			// Translatable values belong in contentMap - ignore them here
			continue
		}
		coerced, err := coerceFieldValue(pta.Attribute.InputType, v)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		if err := validateAttributeRules(pta.Attribute, coerced); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		cleanScalars[k] = coerced
	}
	cleanContent := map[string]any{}
	for k, v := range contentMap {
		pta, ok := bySlug[k]
		if !ok {
			continue
		}
		if !pta.Attribute.IsTranslatable {
			continue
		}
		coerced, err := coerceFieldValue(pta.Attribute.InputType, v)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		if err := validateAttributeRules(pta.Attribute, coerced); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q: %w", ErrInvalidFieldValue, k, err)
		}
		cleanContent[k] = coerced
	}

	// Validate selections - the attribute must be on this page type and the value must belong to it - The repo enforces this at FK level too, but the errors here are friendlier
	for _, sl := range selections {
		attr, ok := byID[sl.AttributeID]
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("%w: attribute not on page type", ErrInvalidFieldValue)
		}
		if !attr.InputType.AcceptsValues() {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q", ErrValuesNotApplicable, attr.Slug)
		}
	}

	// Validate relationships - the attribute is on the type and target_type matches its relationship_target
	for _, rl := range relationships {
		attr, ok := byID[rl.AttributeID]
		if !ok {
			return nil, nil, nil, nil, fmt.Errorf("%w: relationship attribute not on page type", ErrInvalidFieldValue)
		}
		if !attr.InputType.IsRelationship() {
			return nil, nil, nil, nil, fmt.Errorf("%w: attribute %q is not relationship-typed", ErrInvalidFieldValue, attr.Slug)
		}
		if attr.RelationshipTarget != nil && *attr.RelationshipTarget != rl.TargetType {
			return nil, nil, nil, nil, fmt.Errorf("%w: relationship target mismatch on %q", ErrInvalidFieldValue, attr.Slug)
		}
	}

	scalarOut, _ := json.Marshal(cleanScalars)
	contentOut, _ := json.Marshal(cleanContent)
	return scalarOut, contentOut, selections, relationships, nil
}

func decodeJSONMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	if m == nil {
		return map[string]any{}
	}
	return m
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// Fills scalar and content data with each attribute's default_value for keys the page didn't supply, on create only
// Returns the inputs unchanged when nothing was seeded, preserving the required-field gate's nil-ness semantics so create-then-fill stays allowed
func applyAttributeDefaults(pt PageType, scalarData, contentData json.RawMessage) (json.RawMessage, json.RawMessage) {
	scalarMap := decodeJSONMap(scalarData)
	if scalarMap == nil {
		scalarMap = map[string]any{}
	}
	contentMap := decodeJSONMap(contentData)
	if contentMap == nil {
		contentMap = map[string]any{}
	}
	changed := false
	for _, pta := range pt.Attributes {
		a := pta.Attribute
		if len(a.DefaultValue) == 0 || a.InputType.AcceptsValues() || a.InputType.IsRelationship() {
			continue
		}
		var dv any
		if err := json.Unmarshal(a.DefaultValue, &dv); err != nil {
			continue
		}
		target := scalarMap
		if a.IsTranslatable {
			target = contentMap
		}
		if _, exists := target[a.Slug]; !exists {
			target[a.Slug] = dv
			changed = true
		}
	}
	if !changed {
		return scalarData, contentData
	}
	sb, _ := json.Marshal(scalarMap)
	cb, _ := json.Marshal(contentMap)
	return sb, cb
}

type attrValidationRules struct {
	Min       *float64 `json:"min"`
	Max       *float64 `json:"max"`
	MinLength *int     `json:"minLength"`
	MaxLength *int     `json:"maxLength"`
	Regex     string   `json:"regex"`
}

// Enforces an attribute's validation JSONB against an already type-coerced value - applied on every write, and malformed rules are ignored rather than blocking saves
func validateAttributeRules(a Attribute, v any) error {
	if len(a.Validation) == 0 || v == nil {
		return nil
	}
	var r attrValidationRules
	if err := json.Unmarshal(a.Validation, &r); err != nil {
		return nil
	}
	switch a.InputType {
	case InputNumber:
		var f float64
		switch n := v.(type) {
		case float64:
			f = n
		case int:
			f = float64(n)
		case int64:
			f = float64(n)
		default:
			return nil
		}
		if r.Min != nil && f < *r.Min {
			return fmt.Errorf("must be at least %v", *r.Min)
		}
		if r.Max != nil && f > *r.Max {
			return fmt.Errorf("must be at most %v", *r.Max)
		}
	case InputText, InputTextarea, InputURL, InputRichText:
		s, ok := v.(string)
		if !ok {
			return nil
		}
		if r.MinLength != nil && len(s) < *r.MinLength {
			return fmt.Errorf("must be at least %d characters", *r.MinLength)
		}
		if r.MaxLength != nil && len(s) > *r.MaxLength {
			return fmt.Errorf("must be at most %d characters", *r.MaxLength)
		}
		if r.Regex != "" {
			if re, err := regexp.Compile(r.Regex); err == nil && !re.MatchString(s) {
				return fmt.Errorf("does not match the required pattern")
			}
		}
	}
	return nil
}

// Normalizes a raw value to the canonical Go type for its input type, mirroring config.Service - "1" becomes true, "7" becomes 7
func coerceFieldValue(t InputType, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch t {
	case InputText, InputTextarea, InputURL:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", v)
		}
		return s, nil
	case InputNumber:
		switch n := v.(type) {
		case float64:
			return n, nil
		case int, int32, int64:
			return n, nil
		case string:
			var f float64
			if _, err := fmt.Sscanf(n, "%f", &f); err != nil {
				return nil, fmt.Errorf("not a number: %q", n)
			}
			return f, nil
		}
		return nil, fmt.Errorf("expected number, got %T", v)
	case InputBoolean:
		switch b := v.(type) {
		case bool:
			return b, nil
		case string:
			switch strings.ToLower(b) {
			case "true", "1", "yes", "on":
				return true, nil
			case "false", "0", "no", "off":
				return false, nil
			}
		}
		return nil, fmt.Errorf("expected boolean")
	case InputDate, InputDateTime:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected ISO-8601 date string")
		}
		return s, nil
	case InputAsset:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected asset id (uuid string)")
		}
		if _, err := uuid.Parse(s); err != nil {
			return nil, fmt.Errorf("invalid asset id: %w", err)
		}
		return s, nil
	case InputAssets:
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("expected asset id array")
		}
		ids := make([]string, 0, len(arr))
		for _, x := range arr {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("expected uuid string in array")
			}
			if _, err := uuid.Parse(s); err != nil {
				return nil, fmt.Errorf("invalid asset id: %w", err)
			}
			ids = append(ids, s)
		}
		return ids, nil
	case InputRichText, InputJSON:
		return v, nil
	}
	return v, nil
}

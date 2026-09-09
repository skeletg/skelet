package segment

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// The dynamic-segment rule engine - marketers author a structured condition tree and the server compiles it to ONE parameterized SQL query over customers
// Operators never supply SQL: every field, operator and join is allowlisted here, and every value travels as a bind arg
// The wire shape in customer_segments.rules is {logic, conditions[]}, where a condition is one of kind trait, lifecycle, order_count, total_spent_cents, last_order_days, consent or group
// days: 0 means all time - Trait conditions read customer_traits, recomputed daily, while order conditions read the orders ledger live

type RuleCondition struct {
	Kind    string  `json:"kind"`
	Field   string  `json:"field,omitempty"`
	Op      string  `json:"op,omitempty"`
	Value   float64 `json:"value,omitempty"`
	Days    int     `json:"days,omitempty"`
	Stage   string  `json:"stage,omitempty"`
	Channel string  `json:"channel,omitempty"`
	Granted bool    `json:"granted,omitempty"`
	Slug    string  `json:"slug,omitempty"`
}

type Rules struct {
	Logic      string          `json:"logic,omitempty"` // "and" (default) | "or"
	Conditions []RuleCondition `json:"conditions"`
}

var traitFields = map[string]string{
	"rfm_r": "rfm_r", "rfm_f": "rfm_f", "rfm_m": "rfm_m",
	"recency_days": "recency_days", "frequency": "frequency",
	"monetary_cents": "monetary_cents", "engagement_score": "engagement_score",
	// The predictive layer - lets rules target churn_risk or predicted_clv_cents directly
	"churn_risk": "churn_risk", "predicted_clv_cents": "predicted_clv_cents",
	"next_order_days": "next_order_days",
}

var numericOps = map[string]string{
	"eq": "=", "neq": "<>", "gt": ">", "gte": ">=", "lt": "<", "lte": "<=",
}

// Exactly the stages the trait recompute writes - "dormant" was accepted here and never produced, so a segment on it matched nobody, while "lapsed" was produced and could not be selected
var lifecycleStages = map[string]bool{
	"prospect": true, "new": true, "active": true, "at_risk": true, "lapsed": true,
}

// Decodes and validates the JSONB payload - unknown keys are rejected, so a typo'd condition fails at save time rather than silently at refresh time
func ParseRules(raw json.RawMessage) (*Rules, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var r Rules
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("invalid rules: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

func (r *Rules) Validate() error {
	if r.Logic != "" && r.Logic != "and" && r.Logic != "or" {
		return fmt.Errorf("logic must be \"and\" or \"or\"")
	}
	if len(r.Conditions) == 0 {
		return fmt.Errorf("rules need at least one condition")
	}
	for i, c := range r.Conditions {
		if err := c.validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i+1, err)
		}
	}
	return nil
}

func (c *RuleCondition) validate() error {
	switch c.Kind {
	case "trait":
		if _, ok := traitFields[c.Field]; !ok {
			return fmt.Errorf("unknown trait field %q", c.Field)
		}
		if _, ok := numericOps[c.Op]; !ok {
			return fmt.Errorf("unknown op %q", c.Op)
		}
	case "lifecycle":
		if c.Op != "eq" && c.Op != "neq" {
			return fmt.Errorf("lifecycle supports eq/neq")
		}
		if !lifecycleStages[c.Stage] {
			return fmt.Errorf("unknown lifecycle stage %q", c.Stage)
		}
	case "order_count", "total_spent_cents":
		if op := c.Op; op != "gte" && op != "lte" && op != "eq" {
			return fmt.Errorf("%s supports gte/lte/eq", c.Kind)
		}
		if c.Days < 0 {
			return fmt.Errorf("days must be >= 0")
		}
	case "last_order_days":
		if c.Op != "lte" && c.Op != "gte" {
			return fmt.Errorf("last_order_days supports lte/gte")
		}
	case "consent":
		if strings.TrimSpace(c.Channel) == "" {
			return fmt.Errorf("consent needs a channel")
		}
	case "group":
		if strings.TrimSpace(c.Slug) == "" {
			return fmt.Errorf("group needs a slug")
		}
	default:
		return fmt.Errorf("unknown condition kind %q", c.Kind)
	}
	return nil
}

// SignalsConditions returns the conditions that read customer_traits, keyed by their index in the rule set
// Those live in the signals database now, so they cannot be a subquery here and are resolved to an id set first
func (r *Rules) SignalsConditions() map[int]RuleCondition {
	out := map[int]RuleCondition{}
	for i, c := range r.Conditions {
		if synapseRuleKinds[c.Kind] {
			out[i] = c
		}
	}
	return out
}

// Turns validated rules into a customer-id query - all values are bind args, and the SQL text is assembled ONLY from the allowlisted fragments above, so operator input never reaches the SQL string
// resolved carries the customer ids for each trait condition, by index - an empty set matches nobody, which is what an unreachable signals database should mean
func (r *Rules) Compile(resolved map[int][]uuid.UUID) (string, []any) {
	joiner := " AND "
	if r.Logic == "or" {
		joiner = " OR "
	}
	var (
		preds []string
		args  []any
	)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	for i, c := range r.Conditions {
		switch c.Kind {
		case "trait", "lifecycle":
			// Already resolved against the signals database - the ids come in as one bind arg rather than a join
			ids := resolved[i]
			if len(ids) == 0 {
				preds = append(preds, "FALSE")
				continue
			}
			preds = append(preds, "c.id = ANY("+arg(pq.Array(ids))+")")
		case "order_count":
			window := ""
			if c.Days > 0 {
				window = fmt.Sprintf(" AND o.created_at > NOW() - make_interval(days => %s::int)", arg(c.Days))
			}
			preds = append(preds, fmt.Sprintf(
				"(SELECT COUNT(*) FROM orders o WHERE o.customer_id = c.id AND o.status NOT IN ('cancelled','draft')%s) %s %s",
				window, numericOps[c.Op], arg(c.Value)))
		case "total_spent_cents":
			window := ""
			if c.Days > 0 {
				window = fmt.Sprintf(" AND o.created_at > NOW() - make_interval(days => %s::int)", arg(c.Days))
			}
			preds = append(preds, fmt.Sprintf(
				"(SELECT COALESCE(SUM(o.total_amount),0) FROM orders o WHERE o.customer_id = c.id AND o.status NOT IN ('cancelled','draft')%s) %s %s",
				window, numericOps[c.Op], arg(c.Value)))
		case "last_order_days":
			cmp := "<="
			if c.Op == "gte" {
				cmp = ">="
			}
			preds = append(preds, fmt.Sprintf(
				"EXTRACT(EPOCH FROM NOW() - (SELECT MAX(o.created_at) FROM orders o WHERE o.customer_id = c.id AND o.status NOT IN ('cancelled','draft')))/86400 %s %s",
				cmp, arg(c.Value)))
		case "consent":
			preds = append(preds, fmt.Sprintf(
				"EXISTS (SELECT 1 FROM customer_consents cc WHERE cc.customer_id = c.id AND cc.kind = %s AND cc.opted_in = %s)",
				arg(c.Channel), arg(c.Granted)))
		case "group":
			preds = append(preds, fmt.Sprintf(
				"EXISTS (SELECT 1 FROM customer_groups g WHERE g.id = c.customer_group_id AND g.slug = %s)",
				arg(c.Slug)))
		}
	}
	return "SELECT c.id FROM customers c WHERE " + strings.Join(preds, joiner), args
}

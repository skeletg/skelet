package segment

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// The dynamic-segment rule engine - marketers author a structured condition tree and the server compiles it to ONE parameterized SQL query over the customer roster Operators never supply SQL: every field, operator and join is allowlisted here, and every value travels as a bind arg The wire shape in customer_segments.rules is {logic, conditions[]}, where a condition is one of kind trait, lifecycle, order_count, total_spent_cents, last_order_days, consent or group days: 0 means all time
//
// Everything a rule reads is the suite's own: customer_roster with its consents and group slug (pushed by core), customer_traits
// (recomputed daily) and customer_order_agg (lifetime orders, projected off the spine with the traits). A windowed order condition
// is the one thing those tables cannot answer, so it is resolved against the spine first (orders.go) and arrives as an id set

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

// A consent kind is core's registry slug - the roster carries every kind core holds, keyed by it
var consentKind = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

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
		if !consentKind.MatchString(c.Channel) {
			return fmt.Errorf("consent kind %q is not a registry slug", c.Channel)
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

// The conditions that need the spine: an order count or spend inside a window, keyed by their index in the rule set
func (r *Rules) WindowedConditions() map[int]RuleCondition {
	out := map[int]RuleCondition{}
	for i, c := range r.Conditions {
		if (c.Kind == "order_count" || c.Kind == "total_spent_cents") && c.Days > 0 {
			out[i] = c
		}
	}
	return out
}

// One windowed condition after the spine has answered it - either the customers it selects or, when a customer with no
// orders in the window satisfies it the way "at most 5 orders" does, the customers it rules out with everyone else in
type Resolved struct {
	IDs     []uuid.UUID
	Exclude bool
}

// Turns validated rules into a customer-id query - all values are bind args, and the SQL text is assembled ONLY from the allowlisted fragments above, so operator input never reaches the SQL string
// resolved carries the spine's answer for each windowed condition, by index - a condition it does not cover matches nobody, which is what an unreachable spine should mean: narrowing a segment is survivable, widening it to the whole roster is not
func (r *Rules) Compile(resolved map[int]Resolved) (string, []any) {
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
		case "trait":
			preds = append(preds, fmt.Sprintf(
				"c.customer_id IN (SELECT t.customer_id FROM customer_traits t WHERE t.%s %s %s)",
				traitFields[c.Field], numericOps[c.Op], arg(c.Value)))
		case "lifecycle":
			op := "="
			if c.Op == "neq" {
				op = "<>"
			}
			preds = append(preds, fmt.Sprintf(
				"c.customer_id IN (SELECT t.customer_id FROM customer_traits t WHERE t.lifecycle_stage %s %s)",
				op, arg(c.Stage)))
		case "order_count", "total_spent_cents":
			if c.Days > 0 {
				preds = append(preds, resolvedPredicate(resolved[i], arg))
				continue
			}
			// Lifetime, off the projection - a customer who never ordered has no row there, and reads as zero
			col := "order_count"
			if c.Kind == "total_spent_cents" {
				col = "monetary_cents"
			}
			preds = append(preds, fmt.Sprintf(
				"COALESCE((SELECT a.%s FROM customer_order_agg a WHERE a.customer_id = c.customer_id), 0) %s %s",
				col, numericOps[c.Op], arg(c.Value)))
		case "last_order_days":
			cmp := "<="
			if c.Op == "gte" {
				cmp = ">="
			}
			// NULL with no orders, so neither side of a recency rule ever selects someone who never bought
			preds = append(preds, fmt.Sprintf(
				"EXTRACT(EPOCH FROM NOW() - (SELECT a.last_order_at FROM customer_order_agg a WHERE a.customer_id = c.customer_id))/86400 %s %s",
				cmp, arg(c.Value)))
		case "consent":
			preds = append(preds, fmt.Sprintf("COALESCE((c.consents ->> %s)::boolean, false) = %s", arg(c.Channel), arg(c.Granted)))
		case "group":
			preds = append(preds, "c.customer_group_slug = "+arg(c.Slug))
		}
	}
	return "SELECT c.customer_id FROM customer_roster c WHERE " + strings.Join(preds, joiner), args
}

// The predicate for a spine-resolved condition - a zero-value Resolved is the unresolved case, and refuses
func resolvedPredicate(r Resolved, arg func(any) string) string {
	if r.Exclude {
		if len(r.IDs) == 0 {
			return "TRUE"
		}
		return "NOT (c.customer_id = ANY(" + arg(pq.Array(r.IDs)) + "))"
	}
	if len(r.IDs) == 0 {
		return "FALSE"
	}
	return "c.customer_id = ANY(" + arg(pq.Array(r.IDs)) + ")"
}

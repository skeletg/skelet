// Package tax implements the storefront's default tax computation as a predicate-to-effect rule engine, mirroring internal/promotion - a Rule owns an ordered set of typed Conditions, ALL of which must pass, and typed Effects applied in order
// Pluggable seam - order.placing carries a *TaxCents pointer, so a plugin wanting real tax from Avalara, TaxJar or MOSS writes through it and this engine never runs for that order - The order service falls back to Compute only when the pointer is still nil after the sync chain
// Rates are basis points, 875 being 8.75%, so integer math at compute time keeps rounding deterministic without a decimal library
package tax

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Rule struct {
	ID               uuid.UUID
	ChannelID        uuid.UUID
	Name             string
	Priority         int32  // lower = evaluated earlier
	ExclusivityGroup string // empty = always stacks - non-empty = at most one rule per group applies
	SortOrder        int32
	IsActive         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time

	Conditions []Condition
	Effects    []Effect
}

type Condition struct {
	ID            uuid.UUID
	RuleID        uuid.UUID
	Type          string
	Params        json.RawMessage
	ParamsVersion int32
	SortOrder     int32
}

type Effect struct {
	ID            uuid.UUID
	RuleID        uuid.UUID
	Type          string
	Params        json.RawMessage
	ParamsVersion int32
	SortOrder     int32
}

// The whole-order entry point for the order service, from which the engine builds an Environment - TaxInclusive flips the math, treating TaxableCents as gross and reporting the tax PORTION rather than an additive surcharge
type ComputeInput struct {
	ChannelID    uuid.UUID
	CountryCode  string
	RegionCode   string
	TaxableCents int64
	TaxInclusive bool
}

// The whole-order answer plus the first rule that applied, for audit and display - with nothing applying, RuleID is nil and Cents zero, which callers read as no tax
type ComputeResult struct {
	Cents   int64
	RuleID  *uuid.UUID
	Name    string
	RateBPS int32
}

type AppliedTax struct {
	RuleID  uuid.UUID
	Name    string
	RateBPS int32 // effective rate of the rule's percentage effect, for display - 0 if none
	Cents   int64
}

type EvalResult struct {
	TotalCents int64
	Applied    []AppliedTax
}

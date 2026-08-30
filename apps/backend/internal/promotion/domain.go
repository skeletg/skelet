// Package promotion is the v2 rule engine - nested condition trees, stackable actions and coupon batches
// A Rule is the discount intent (priority, stacking flags, validity window, usage limits); Codes hang off it with optional per-code overrides; Conditions are typed predicates ANDed together, with core.any_of for OR; Actions are typed mutations adding intent to the Ledger in sort order
// Both are discriminated by a slug - built-ins register themselves in init() under conditions/ and actions/, plugins register via the SDK, and an unknown slug is skipped at eval rather than panicking, so an uninstalled plugin can't crash rules that reference it
// The engine is stateless: build an Environment, call Engine.Evaluate, read the Ledger back out
package promotion

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type StopMode string

const (
	StopNone           StopMode = "none"
	StopSameActionType StopMode = "same_action_type"
	StopAll            StopMode = "all"
)

// schedule_json (recurring schedules) is kept as JSONB but the engine ignores it - a plugin can read it through a custom condition
// What the engine applied to one order - orders keep only an aggregate discount, so
// this is the only record of which rule (and which typed code) produced it
type OrderRedemption struct {
	ID            uuid.UUID
	RuleID        *uuid.UUID
	RuleName      string
	RuleSlug      string
	Code          string
	DiscountCents int64
	RedeemedAt    time.Time
}

type Rule struct {
	ID               uuid.UUID
	Slug             string
	Name             string
	Description      string
	PublicLabel      string
	ChannelID        *uuid.UUID // nil = all stores
	RequiresCode     bool
	Priority         int32
	StopAfterApply   StopMode
	ExclusivityGroup string // empty = always stack
	ValidFrom        *time.Time
	ValidTo          *time.Time
	ScheduleJSON     json.RawMessage
	MaxUses          *int32
	PerCustomerLimit *int32
	UsedCount        int32
	IsActive         bool
	CreatedByAdminID *uuid.UUID
	UpdatedByAdminID *uuid.UUID
	// A small corner-sticker label for listing cards and a larger banner for detail pages - nullable, so rules with no media render as text only
	LabelMediaID  *uuid.UUID
	BannerMediaID *uuid.UUID
	// An alternative to LabelMediaID where short text plus a colour swatch beats an uploaded image - empty text means no pill, and empty colours mean the storefront's default palette
	PillText      string
	PillTextColor string // CSS color (typically `#RRGGBB`)
	PillBgColor   string // CSS color
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Conditions    []Condition
	Actions       []Action
}

type Code struct {
	ID                uuid.UUID
	RuleID            uuid.UUID
	Code              string
	MaxUses           *int32
	PerCustomerLimit  *int32
	ValidFrom         *time.Time
	ValidTo           *time.Time
	RestrictedToEmail string
	UsedCount         int32
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Condition struct {
	ID            uuid.UUID
	RuleID        uuid.UUID
	Type          string
	Params        json.RawMessage
	ParamsVersion int32
	SortOrder     int32
}

type Action struct {
	ID            uuid.UUID
	RuleID        uuid.UUID
	Type          string
	Params        json.RawMessage
	ParamsVersion int32
	SortOrder     int32
}

// One element of Engine.Evaluate's return - which rule fired, which code triggered it, and how much it contributed, so order placement can write per-redemption audit rows
type AppliedRule struct {
	RuleID        uuid.UUID
	CodeID        *uuid.UUID
	Priority      int32
	DiscountCents int64
}

// PlaceOrder turns each into an order_line with unit_price=0 and is_gift=true, and reserves inventory for it
type FreeGift struct {
	ProductSKU string
	Quantity   float64
}

// The locale-specific storefront label and terms - the rule's own public_label is the fallback when the request's locale has no row
type RuleContent struct {
	ID        uuid.UUID
	RuleID    uuid.UUID
	Locale    string
	Label     string
	Terms     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AppliedCodeRecord struct {
	CartID            uuid.UUID
	RuleID            uuid.UUID
	CodeID            *uuid.UUID
	CodeText          string // empty when code_id is nil (auto-apply)
	AppliedAt         time.Time
	InvalidatedReason string
}

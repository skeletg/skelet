package sdk

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/google/uuid"
)

// Read-only cart/customer/channel view the host adapter builds from real domain types, so plugins never import internal/cart, internal/customer or internal/channel
// Richer than a remote provider's serialized context, which carries no ProductID, CategoryIDs or AttributeKVs - a category- or attribute-scoped condition is the case that needs to be in-process
// Deliberately minimal otherwise - the surface only ever grows, which is what lets a v2 host add accessors without breaking existing plugins
type PromotionEnvironment interface {
	SubtotalCents() int64
	Currency() string
	CountryCode() string
	ChannelID() *uuid.UUID
	ChannelSlug() string
	CustomerID() *uuid.UUID
	CustomerGroupSlug() string
	// The shopper's email, empty when unknown - conditions pinning a code to one recipient fail closed on empty
	CustomerEmail() string
	CustomerSegmentSlugs() []string // segment-gated conditions without duplicating membership state
	Lines() []PromotionCartLine
	AppliedCodes() []string
}

// CategoryIDs and AttributeKVs are pre-resolved by the host so plugins don't round-trip the catalog at eval time
type PromotionCartLine struct {
	LineID         uuid.UUID
	ProductID      uuid.UUID
	ProductSKU     string
	ProductName    string
	Quantity       float64
	UnitPriceCents int64
	ProductKind    string // "simple" | "configurable" | "bundle" | "grouped"
	CategoryIDs    []uuid.UUID
	AttributeKVs   map[string]string
}

// One per Evaluate - the snapshot getters expose running state to actions that coordinate, e.g. a cap-total action reading SubtotalDiscountCents and stopping early
type PromotionLedger interface {
	AddSubtotalDiscount(cents int64)
	AddLineAdjustment(lineID uuid.UUID, cents int64)
	SetFreeShipping(methodSlugs []string)
	SetShippingOverride(cents int64)
	AddFreeGift(productSKU string, qty float64)
	CapSubtotalDiscount(maxCents int64)

	SubtotalDiscountCents() int64
	LineAdjustment(lineID uuid.UUID) int64
	FreeShipping() bool
	ShippingOverride() *int64
}

// Lets plugin code stamp the originating rule and code id onto its own records
type PromotionAppliedContext struct {
	RuleID   uuid.UUID
	CodeID   *uuid.UUID
	Priority int32
}

// Slug is the discriminator on promotion_conditions.type rows - avoid a "core." prefix, which collides with built-ins
// ValidateParams runs on the admin write path so a malformed rule never reaches Evaluate, and ParamsSchema returns Draft-7 JSON Schema the dashboard renders the editor form from
type PromotionCondition interface {
	Slug() string
	Label() string
	Description() string
	ParamsSchema() json.RawMessage
	ValidateParams(params json.RawMessage) error
	Evaluate(ctx context.Context, env PromotionEnvironment, params json.RawMessage) (bool, error)
}

// Apply gets the running ledger, so a plugin action stacks with built-in ones
type PromotionAction interface {
	Slug() string
	Label() string
	Description() string
	ParamsSchema() json.RawMessage
	ValidateParams(params json.RawMessage) error
	Apply(ctx context.Context, env PromotionEnvironment, params json.RawMessage, ledger PromotionLedger, applied PromotionAppliedContext) error
}

// Fires inside the order-placement tx for any applied rule whose action slug matches, so a loyalty or gift-card plugin debits its own ledger in the same tx as the order write and a failed order rolls both back
type PromotionRedemptionHook interface {
	Slug() string
	Redeem(ctx context.Context, tx *sql.Tx, env PromotionEnvironment, ledger PromotionLedger, applied PromotionAppliedContext) error
}

// Panics on duplicate slug
func (pl *Plugin) RegisterPromotionCondition(c PromotionCondition) {
	withHost(func(h Host) error { return h.RegisterPromotionCondition(pl.slug, c) })
}

func (pl *Plugin) RegisterPromotionAction(a PromotionAction) {
	withHost(func(h Host) error { return h.RegisterPromotionAction(pl.slug, a) })
}

// A disabled plugin's hook is skipped but the discount its action already computed still lands, so pair a ledger-debiting hook with an action from the SAME plugin and both go dark together
func (pl *Plugin) RegisterPromotionRedemptionHook(h PromotionRedemptionHook) {
	withHost(func(host Host) error { return host.RegisterPromotionRedemptionHook(pl.slug, h) })
}

package promotion

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/money"
)

// The read-only view conditions and actions receive at evaluation time - an interface so plugins talk through accessor methods instead of importing internal/cart and other domain packages, matching the SDK's pattern for the PaymentProvider config reader
// BuildEnvironment holds the concrete implementation, taking the real domain types and exposing them as this slim view
type Environment interface {
	SubtotalCents() int64
	Currency() string
	CountryCode() string
	ChannelID() *uuid.UUID
	ChannelSlug() string
	CustomerID() *uuid.UUID
	CustomerGroupSlug() string
	// The shopper's email, for codes pinned to one recipient - empty when unknown, which fails a restricted code closed
	CustomerEmail() string
	// The active customer-segment slugs the customer belongs to, empty for guests - backs the core.customer_in_segment condition
	CustomerSegmentSlugs() []string
	Lines() []CartLineView
	AppliedCodes() []string
}

// The slim cart-line view passed to conditions and actions - category memberships and attribute KVs are pre-resolved, so a condition needn't round-trip the catalog at eval time
type CartLineView struct {
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

// The default implementation BuildEnvironment populates - conditions and actions never see it directly, going through the Environment interface
type envImpl struct {
	subtotalCents        int64
	currency             string
	countryCode          string
	channelID            *uuid.UUID
	channelSlug          string
	customerID           *uuid.UUID
	customerGroupSlug    string
	customerEmail        string
	customerSegmentSlugs []string
	lines                []CartLineView
	appliedCodes         []string
}

func (e *envImpl) SubtotalCents() int64           { return e.subtotalCents }
func (e *envImpl) Currency() string               { return e.currency }
func (e *envImpl) CountryCode() string            { return e.countryCode }
func (e *envImpl) ChannelID() *uuid.UUID          { return e.channelID }
func (e *envImpl) ChannelSlug() string            { return e.channelSlug }
func (e *envImpl) CustomerID() *uuid.UUID         { return e.customerID }
func (e *envImpl) CustomerGroupSlug() string      { return e.customerGroupSlug }
func (e *envImpl) CustomerEmail() string          { return e.customerEmail }
func (e *envImpl) CustomerSegmentSlugs() []string { return e.customerSegmentSlugs }
func (e *envImpl) Lines() []CartLineView          { return e.lines }
func (e *envImpl) AppliedCodes() []string         { return e.appliedCodes }

// The construction input for BuildEnvironment - callers populate it once and reuse the resulting Environment across rule evaluation
type EnvironmentInput struct {
	SubtotalCents        int64
	Currency             string
	CountryCode          string
	ChannelID            *uuid.UUID
	ChannelSlug          string
	CustomerID           *uuid.UUID
	CustomerGroupSlug    string
	CustomerEmail        string
	CustomerSegmentSlugs []string
	Lines                []CartLineView
	AppliedCodes         []string
}

// A copy of env seeing only the given lines, with the subtotal recomputed from them
// The matching actions need it because conditions answer against a whole cart, not one line, so "10% off any widget" asks each line in isolation
// Every other field carries over by construction - hand-rolling the copy at each call site silently dropped CustomerEmail and CustomerSegmentSlugs, which made a segment-gated matching condition impossible to satisfy
func ProjectLines(env Environment, lines []CartLineView) Environment {
	var subtotal int64
	for _, l := range lines {
		subtotal += money.LineTotal(l.UnitPriceCents, l.Quantity)
	}
	return &envImpl{
		subtotalCents:        subtotal,
		currency:             env.Currency(),
		countryCode:          env.CountryCode(),
		channelID:            env.ChannelID(),
		channelSlug:          env.ChannelSlug(),
		customerID:           env.CustomerID(),
		customerGroupSlug:    env.CustomerGroupSlug(),
		customerEmail:        env.CustomerEmail(),
		customerSegmentSlugs: env.CustomerSegmentSlugs(),
		lines:                lines,
		appliedCodes:         env.AppliedCodes(),
	}
}

// Constructs the concrete environment - the order package, cart resolver and others fold their domain objects into EnvironmentInput then call this
func BuildEnvironment(in EnvironmentInput) Environment {
	return &envImpl{
		subtotalCents:        in.SubtotalCents,
		currency:             in.Currency,
		countryCode:          in.CountryCode,
		channelID:            in.ChannelID,
		channelSlug:          in.ChannelSlug,
		customerID:           in.CustomerID,
		customerGroupSlug:    in.CustomerGroupSlug,
		customerEmail:        in.CustomerEmail,
		customerSegmentSlugs: in.CustomerSegmentSlugs,
		lines:                in.Lines,
		appliedCodes:         in.AppliedCodes,
	}
}

package promotion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// A rule's condition or action delegated to a merchant HTTP endpoint (remoteprovider kind=promotion_condition / promotion_action) instead of a compile-time evaluator - the rule stores the provider slug in its params, and evaluation invokes that provider with the cart context It lets a merchant express bespoke eligibility, like a loyalty-tier lookup or an external risk gate, and bespoke discounts from a dynamic-pricing service, without a Go plugin Hot path - cart evaluation runs on every cart render, so set a cache TTL and keep it fail_open, the default: an evaluator error otherwise aborts the ENTIRE cart price calculation. fail_open makes a down condition "no match" and a down action a no-op, while fail_closed propagates the error as the merchant's explicit "don't price the cart if my service is down" Wire contracts: a condition takes {provider, params, cart} and returns {match}; an action takes the same and returns {subtotalDiscountCents?, lineAdjustments?, freeShipping?, shippingOverrideCents?, freeGifts?}

// Wires the remote.condition and remote.action types into the promotion registry - call from main.go once the remoteprovider stack exists
func RegisterRemoteProviderTypes(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) {
	RegisterConditionType(&remoteCondition{svc: svc, invoker: invoker})
	RegisterActionType(&remoteAction{svc: svc, invoker: invoker})
}

// The shared param shape - which provider to call, plus an opaque params blob forwarded to it
type remoteTypeParams struct {
	Provider string          `json:"provider"`
	Params   json.RawMessage `json:"params,omitempty"`
}

func parseRemoteTypeParams(raw json.RawMessage) (remoteTypeParams, error) {
	var p remoteTypeParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return p, err
		}
	}
	if p.Provider == "" {
		return p, fmt.Errorf("provider is required")
	}
	return p, nil
}

var remoteParamsSchema = json.RawMessage(`{
  "type": "object",
  "required": ["provider"],
  "properties": {
    "provider": {"type": "string", "title": "Remote provider slug"},
    "params": {"type": "object", "title": "Extra params forwarded to the provider"}
  },
  "additionalProperties": false
}`)

type remoteCartContext struct {
	SubtotalCents int64                   `json:"subtotalCents"`
	Currency      string                  `json:"currency"`
	CountryCode   string                  `json:"countryCode"`
	ChannelID     string                  `json:"channelId,omitempty"`
	ChannelSlug   string                  `json:"channelSlug,omitempty"`
	CustomerID    string                  `json:"customerId,omitempty"`
	CustomerGroup string                  `json:"customerGroup,omitempty"`
	CustomerEmail string                  `json:"customerEmail,omitempty"`
	Segments      []string                `json:"segments,omitempty"`
	AppliedCodes  []string                `json:"appliedCodes,omitempty"`
	Lines         []remoteCartContextLine `json:"lines"`
}

type remoteCartContextLine struct {
	LineID         string  `json:"lineId"`
	ProductID      string  `json:"productId,omitempty"`
	SKU            string  `json:"sku"`
	ProductName    string  `json:"productName"`
	Quantity       float64 `json:"quantity"`
	UnitPriceCents int64   `json:"unitPriceCents"`
	Kind           string  `json:"kind"`
	// Pre-resolved by the host so a remote condition can scope by category or attribute without calling back into the API per evaluation - a callback would land on the checkout hot path, behind this provider's cache TTL, on every cart mutation Without these, "20% off Outerwear" and "buy 2 of brand=Acme" were expressible as an in-process plugin condition and NOT as a remote one
	CategoryIDs []string          `json:"categoryIds,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

func buildRemoteCartContext(env Environment) remoteCartContext {
	lines := make([]remoteCartContextLine, len(env.Lines()))
	for i, l := range env.Lines() {
		lines[i] = remoteCartContextLine{
			LineID: l.LineID.String(), ProductID: l.ProductID.String(), SKU: l.ProductSKU,
			ProductName: l.ProductName, Quantity: l.Quantity, UnitPriceCents: l.UnitPriceCents,
			Kind: l.ProductKind, Attributes: l.AttributeKVs,
		}
		if len(l.CategoryIDs) > 0 {
			cats := make([]string, len(l.CategoryIDs))
			for j, c := range l.CategoryIDs {
				cats[j] = c.String()
			}
			lines[i].CategoryIDs = cats
		}
	}
	ctx := remoteCartContext{
		SubtotalCents: env.SubtotalCents(),
		Currency:      env.Currency(),
		CountryCode:   env.CountryCode(),
		ChannelSlug:   env.ChannelSlug(),
		CustomerGroup: env.CustomerGroupSlug(),
		CustomerEmail: env.CustomerEmail(),
		Segments:      env.CustomerSegmentSlugs(),
		AppliedCodes:  env.AppliedCodes(),
		Lines:         lines,
	}
	if env.ChannelID() != nil {
		ctx.ChannelID = env.ChannelID().String()
	}
	if env.CustomerID() != nil {
		ctx.CustomerID = env.CustomerID().String()
	}
	return ctx
}

// Finds the active provider the params reference, scoped to the environment's channel - ok=false when it's missing or scoped elsewhere, and the caller then applies fail-open
func resolveProvider(ctx context.Context, svc *remoteprovider.Service, kind remoteprovider.Kind, env Environment, slug string) (remoteprovider.Provider, bool) {
	p, err := svc.GetBySlug(ctx, kind, slug)
	if err != nil || !p.IsActive {
		return remoteprovider.Provider{}, false
	}
	if p.ChannelID != nil {
		if env.ChannelID() == nil || *p.ChannelID != *env.ChannelID() {
			return remoteprovider.Provider{}, false
		}
	}
	return p, true
}

// --- condition ---

type remoteCondition struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func (*remoteCondition) Slug() string  { return "remote.condition" }
func (*remoteCondition) Label() string { return "Remote condition (HTTP)" }
func (*remoteCondition) Description() string {
	return "Delegates eligibility to a remote promotion_condition provider. Set a cache TTL - this runs on every cart render."
}
func (*remoteCondition) ParamsSchema() json.RawMessage { return remoteParamsSchema }

func (*remoteCondition) ValidateParams(raw json.RawMessage) error {
	_, err := parseRemoteTypeParams(raw)
	return err
}

type remoteConditionRequest struct {
	Provider string            `json:"provider"`
	Params   json.RawMessage   `json:"params,omitempty"`
	Cart     remoteCartContext `json:"cart"`
}

type remoteConditionResponse struct {
	Match bool `json:"match"`
}

func (c *remoteCondition) Evaluate(ctx context.Context, env Environment, raw json.RawMessage) (bool, error) {
	params, err := parseRemoteTypeParams(raw)
	if err != nil {
		return false, err
	}
	p, ok := resolveProvider(ctx, c.svc, remoteprovider.KindPromotionCondition, env, params.Provider)
	if !ok {
		return false, nil // missing provider = no match (fail-open)
	}
	req := remoteConditionRequest{Provider: p.Slug, Params: params.Params, Cart: buildRemoteCartContext(env)}
	var resp remoteConditionResponse
	if err := c.invoker.Invoke(ctx, p, req, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return false, fmt.Errorf("remote condition %q: %w", p.Slug, err)
		}
		log.Printf("[promotion] remote condition %q failed (no match): %v", p.Slug, err)
		return false, nil
	}
	return resp.Match, nil
}

// --- action ---

type remoteAction struct {
	svc     *remoteprovider.Service
	invoker *remoteprovider.Invoker
}

func (*remoteAction) Slug() string  { return "remote.action" }
func (*remoteAction) Label() string { return "Remote action (HTTP)" }
func (*remoteAction) Description() string {
	return "Delegates discount computation to a remote promotion_action provider. Set a cache TTL - this runs on every cart render."
}
func (*remoteAction) ParamsSchema() json.RawMessage { return remoteParamsSchema }

func (*remoteAction) ValidateParams(raw json.RawMessage) error {
	_, err := parseRemoteTypeParams(raw)
	return err
}

type remoteActionRequest struct {
	Provider string            `json:"provider"`
	Params   json.RawMessage   `json:"params,omitempty"`
	Cart     remoteCartContext `json:"cart"`
}

type remoteActionResponse struct {
	SubtotalDiscountCents int64            `json:"subtotalDiscountCents"`
	LineAdjustments       map[string]int64 `json:"lineAdjustments"`
	FreeShipping          bool             `json:"freeShipping"`
	FreeShippingMethods   []string         `json:"freeShippingMethods"`
	ShippingOverrideCents *int64           `json:"shippingOverrideCents"`
	FreeGifts             []remoteFreeGift `json:"freeGifts"`
}

type remoteFreeGift struct {
	SKU      string  `json:"sku"`
	Quantity float64 `json:"quantity"`
}

func (a *remoteAction) Apply(ctx context.Context, env Environment, raw json.RawMessage, ledger Ledger, _ AppliedRuleContext) error {
	params, err := parseRemoteTypeParams(raw)
	if err != nil {
		return err
	}
	p, ok := resolveProvider(ctx, a.svc, remoteprovider.KindPromotionAction, env, params.Provider)
	if !ok {
		return nil // missing provider = no discount (fail-open)
	}
	req := remoteActionRequest{Provider: p.Slug, Params: params.Params, Cart: buildRemoteCartContext(env)}
	var resp remoteActionResponse
	if err := a.invoker.Invoke(ctx, p, req, &resp); err != nil {
		if p.FailurePolicy == remoteprovider.FailClosed {
			return fmt.Errorf("remote action %q: %w", p.Slug, err)
		}
		log.Printf("[promotion] remote action %q failed (no discount): %v", p.Slug, err)
		return nil
	}
	// Fold the response into the ledger - the ledger clamps non-positive values itself, so a malformed response can't add a negative discount
	if resp.SubtotalDiscountCents > 0 {
		ledger.AddSubtotalDiscount(resp.SubtotalDiscountCents)
	}
	for lineStr, cents := range resp.LineAdjustments {
		lineID, err := uuid.Parse(lineStr)
		if err != nil {
			continue // ignore an unparseable line id rather than failing the cart
		}
		ledger.AddLineAdjustment(lineID, cents)
	}
	if resp.FreeShipping {
		ledger.SetFreeShipping(resp.FreeShippingMethods)
	}
	if resp.ShippingOverrideCents != nil {
		ledger.SetShippingOverride(*resp.ShippingOverrideCents)
	}
	for _, g := range resp.FreeGifts {
		ledger.AddFreeGift(g.SKU, g.Quantity)
	}
	return nil
}

package example

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Redeem fires inside the placement tx, so the test reads this instead of committing an order
var redemptionRan = make(chan struct{}, 4)

// The smallest real condition: a schema the dashboard renders, admin-write validation, and one environment read
type minSubtotalCondition struct{}

func (minSubtotalCondition) Slug() string  { return Slug + ".min-subtotal" }
func (minSubtotalCondition) Label() string { return "Example: minimum subtotal" }
func (minSubtotalCondition) Description() string {
	return "Matches when the cart subtotal is at least min_cents."
}

func (minSubtotalCondition) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"min_cents":{"type":"integer","minimum":0}},"required":["min_cents"]}`)
}

func (minSubtotalCondition) ValidateParams(raw json.RawMessage) error {
	var p struct {
		MinCents *int64 `json:"min_cents"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.MinCents == nil || *p.MinCents < 0 {
		return errors.New("min_cents must be a non-negative integer")
	}
	return nil
}

func (minSubtotalCondition) Evaluate(_ context.Context, env sdk.PromotionEnvironment, raw json.RawMessage) (bool, error) {
	var p struct {
		MinCents int64 `json:"min_cents"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return false, err
	}
	return env.SubtotalCents() >= p.MinCents, nil
}

// Discounts through the shared ledger, so it stacks with built-in actions and the cap rules still hold
type flatDiscountAction struct{}

func (flatDiscountAction) Slug() string  { return Slug + ".flat-discount" }
func (flatDiscountAction) Label() string { return "Example: flat discount" }
func (flatDiscountAction) Description() string {
	return "Subtracts a fixed number of cents from the subtotal."
}

func (flatDiscountAction) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"cents":{"type":"integer","minimum":1}},"required":["cents"]}`)
}

func (flatDiscountAction) ValidateParams(raw json.RawMessage) error {
	var p struct {
		Cents *int64 `json:"cents"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	if p.Cents == nil || *p.Cents <= 0 {
		return errors.New("cents must be a positive integer")
	}
	return nil
}

func (flatDiscountAction) Apply(_ context.Context, _ sdk.PromotionEnvironment, raw json.RawMessage, ledger sdk.PromotionLedger, _ sdk.PromotionAppliedContext) error {
	var p struct {
		Cents int64 `json:"cents"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	ledger.AddSubtotalDiscount(p.Cents)
	return nil
}

// Slug matches the action's, so it fires for every applied flat-discount rule - pairing them on one plugin is what lets the kill switch take both dark together
type recordRedemptionHook struct{}

func (recordRedemptionHook) Slug() string { return Slug + ".flat-discount" }

func (recordRedemptionHook) Redeem(context.Context, *sql.Tx, sdk.PromotionEnvironment, sdk.PromotionLedger, sdk.PromotionAppliedContext) error {
	select {
	case redemptionRan <- struct{}{}:
	default:
	}
	return nil
}

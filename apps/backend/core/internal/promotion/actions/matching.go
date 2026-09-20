package actions

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// Walks the cart lines, evaluates the child condition against each, and accumulates per-unit discounts up to maxQty - unitFn computes the discount cents for `qty` units of a line, so callers like percentOffMatching apply % or fixed-cents formulas without duplicating the iteration scaffolding
// There's no "matches this specific line" hook on conditions today - they answer against the whole cart - so we build a one-line projection of the env per line and ask about that - It keeps the condition contract uniform while still letting "10% off any widget" pick out only the widget lines
func applyToMatchingLines(
	ctx context.Context,
	env promotion.Environment,
	cond matchingChild,
	maxQty *float64,
	unitFn func(line promotion.CartLineView, qty float64) int64,
) (map[uuid.UUID]int64, error) {
	impl, ok := promotion.RegistryFrom(ctx).LookupCondition(cond.Type)
	if !ok {
		// An unknown condition slug means no lines match, mirroring the engine's behaviour for missing top-level types
		return map[uuid.UUID]int64{}, nil
	}
	out := make(map[uuid.UUID]int64)
	// Tracks the unallocated portion of maxQty, with -1 meaning no cap - equivalent to maxQty being nil
	remaining := float64(-1)
	if maxQty != nil {
		remaining = *maxQty
	}
	for _, line := range env.Lines() {
		if remaining == 0 {
			break
		}
		// Per-line environment projection - the condition sees a cart containing only this line, so a SKU condition fires for matching lines rather than the whole cart at once
		singleLineEnv := promotion.ProjectLines(env, []promotion.CartLineView{line})
		matches, err := impl.Evaluate(ctx, singleLineEnv, cond.Params)
		if err != nil {
			return nil, err
		}
		if !matches {
			continue
		}
		qty := line.Quantity
		if remaining > 0 && qty > remaining {
			qty = remaining
		}
		if qty <= 0 {
			continue
		}
		discount := unitFn(line, qty)
		if discount > 0 {
			out[line.LineID] += discount
		}
		if remaining > 0 {
			remaining -= qty
		}
	}
	return out, nil
}

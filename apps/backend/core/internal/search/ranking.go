package search

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const (
	pathRankingSalesWeight  = "search/products/ranking_sales_weight"
	pathRankingInStockBoost = "search/products/ranking_in_stock_boost"
)

func cfgFloatOr(ctx context.Context, cfg *config.Service, path string, def float64) float64 {
	raw := cfgStringOr(ctx, cfg, path, "")
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return def
	}
	return f
}

// Folds business signals into ordering: log-scaled trailing-30d sales, so a bestseller outranks a text-equal shelf-warmer without one viral SKU drowning relevance, plus an optional flat in-stock boost
// Both weights are operator-tunable - 0 disables a signal and the query comes back untouched with both at 0
func applyBusinessRanking(ctx context.Context, cfg *config.Service, query map[string]any) map[string]any {
	salesWeight := cfgFloatOr(ctx, cfg, pathRankingSalesWeight, 0.5)
	inStockBoost := cfgFloatOr(ctx, cfg, pathRankingInStockBoost, 0)
	return applyBusinessRankingWeights(query, salesWeight, inStockBoost)
}

// The pure core, split out for tests
func applyBusinessRankingWeights(query map[string]any, salesWeight, inStockBoost float64) map[string]any {
	if salesWeight <= 0 && inStockBoost <= 0 {
		return query
	}
	functions := []any{}
	if salesWeight > 0 {
		functions = append(functions, map[string]any{
			"field_value_factor": map[string]any{
				"field":    "sales_30d",
				"modifier": "log1p",
				"factor":   salesWeight,
				"missing":  0,
			},
		})
	}
	if inStockBoost > 0 {
		functions = append(functions, map[string]any{
			"filter": map[string]any{"term": map[string]any{"is_in_stock": true}},
			"weight": inStockBoost,
		})
	}
	return map[string]any{
		"function_score": map[string]any{
			"query":      query,
			"functions":  functions,
			"score_mode": "sum",
			"boost_mode": "sum",
		},
	}
}

// Wrapped OUTSIDE the business ranking so an explicit merchandising decision always dominates the automatic sales signal
// Weights are MULTIPLICATIVE (boost w -> (10+w)/10, bury w -> 10/(10-w)) because additive burial can drive a score negative, which OpenSearch rejects outright
func applyMerchBoosts(query map[string]any, boosts map[uuid.UUID]float64) map[string]any {
	if len(boosts) == 0 {
		return query
	}
	functions := make([]any, 0, len(boosts))
	for id, w := range boosts {
		factor := 1.0
		if w > 0 {
			factor = (10 + w) / 10
		} else if w < 0 {
			factor = 10 / (10 - w)
		}
		functions = append(functions, map[string]any{
			"filter": map[string]any{"term": map[string]any{"product_id": id.String()}},
			"weight": factor,
		})
	}
	return map[string]any{
		"function_score": map[string]any{
			"query":      query,
			"functions":  functions,
			"score_mode": "multiply",
			"boost_mode": "multiply",
		},
	}
}

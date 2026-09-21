package search

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestApplyBusinessRankingWeights(t *testing.T) {
	base := map[string]any{"bool": map[string]any{}}

	// Both signals off: query passes through untouched
	if out := applyBusinessRankingWeights(base, 0, 0); out["function_score"] != nil {
		t.Fatal("zero weights must not wrap the query")
	}

	// Sales weight only: one field_value_factor function
	out := applyBusinessRankingWeights(base, 0.5, 0)
	fs, ok := out["function_score"].(map[string]any)
	if !ok {
		t.Fatal("expected function_score wrapper")
	}
	fns := fs["functions"].([]any)
	if len(fns) != 1 {
		t.Fatalf("want 1 function, got %d", len(fns))
	}
	raw, _ := json.Marshal(fns[0])
	if string(raw) == "" || !json.Valid(raw) {
		t.Fatal("function not serializable")
	}
	var f map[string]any
	_ = json.Unmarshal(raw, &f)
	fvf := f["field_value_factor"].(map[string]any)
	if fvf["field"] != "sales_30d" || fvf["modifier"] != "log1p" || fvf["factor"].(float64) != 0.5 {
		t.Fatalf("bad field_value_factor: %v", fvf)
	}

	// Both: two functions, sum modes
	out = applyBusinessRankingWeights(base, 1, 2)
	fs = out["function_score"].(map[string]any)
	if len(fs["functions"].([]any)) != 2 || fs["score_mode"] != "sum" || fs["boost_mode"] != "sum" {
		t.Fatalf("bad combined wrapper: %v", fs)
	}
}

func TestApplyMerchBoostsMultiplicative(t *testing.T) {
	base := map[string]any{"bool": map[string]any{}}
	if out := applyMerchBoosts(base, nil); out["function_score"] != nil {
		t.Fatal("nil boosts must not wrap")
	}
	id1, id2 := uuidMust("11111111-1111-4111-8111-111111111111"), uuidMust("22222222-2222-4222-8222-222222222222")
	out := applyMerchBoosts(base, map[uuid.UUID]float64{id1: 80, id2: -80})
	fs := out["function_score"].(map[string]any)
	if fs["score_mode"] != "multiply" || fs["boost_mode"] != "multiply" {
		t.Fatalf("must be multiplicative: %v", fs)
	}
	for _, f := range fs["functions"].([]any) {
		w := f.(map[string]any)["weight"].(float64)
		if w <= 0 {
			t.Fatalf("factor must stay positive, got %v", w)
		}
	}
}

func uuidMust(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

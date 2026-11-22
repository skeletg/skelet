package analytics

import (
	"errors"
	"testing"
)

// The one statement of what an Insights read and its filters may be - the engines and a saved board both hold to it
func TestInsightsQueriesAreRefusedOnlyForWhatCantRun(t *testing.T) {
	sku := PropertyFilter{Key: "sku", Op: FilterEq, Values: []string{"X"}}
	for name, q := range map[string]InsightsQuery{
		"a count":                  {Measure: "total"},
		"a count, by its old name": {Measure: "count"},
		"people":                   {Measure: "unique"},
		"a sum":                    {Measure: "sum", SumKey: "net_base_cents", Filters: []PropertyFilter{sku}},
		"one of several":           {Filters: []PropertyFilter{{Key: "sku", Op: FilterIn, Values: []string{"X", "Y"}}}},
	} {
		if err := q.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, q := range map[string]InsightsQuery{
		"an unknown measure":  {Measure: "median"},
		"a sum of nothing":    {Measure: "sum"},
		"too many events":     {Events: make([]string, MaxEvents+1)},
		"no property":         {Filters: []PropertyFilter{{Op: FilterEq, Values: []string{"X"}}}},
		"no operator":         {Filters: []PropertyFilter{{Key: "sku", Values: []string{"X"}}}},
		"= with two values":   {Filters: []PropertyFilter{{Key: "sku", Op: FilterEq, Values: []string{"X", "Y"}}}},
		"one of nothing":      {Filters: []PropertyFilter{{Key: "sku", Op: FilterIn}}},
		"too many conditions": {Filters: make([]PropertyFilter, MaxFilters+1)},
	} {
		if err := q.Validate(); !errors.Is(err, ErrBadFilter) {
			t.Errorf("%s: err = %v, want ErrBadFilter", name, err)
		}
	}
}

// A step filter from before operators reads as the EQ it meant
func TestAStepFilterFromBeforeOperatorsIsEq(t *testing.T) {
	if f := StepFilter("device", "", "mobile", nil); f.Op != FilterEq || len(f.Values) != 1 || f.Values[0] != "mobile" {
		t.Fatalf("= %+v", f)
	}
	if f := StepFilter("sku", FilterIn, "", []string{"X", "Y"}); f.Op != FilterIn || len(f.Values) != 2 {
		t.Fatalf("= %+v", f)
	}
}

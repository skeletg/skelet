package board

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDefinitionRefusesWhatItsKindCantRun(t *testing.T) {
	for name, d := range map[string]Definition{
		"unknown kind":       {Kind: "pie"},
		"one-step funnel":    {Kind: "funnel", Steps: []string{"page_view"}},
		"filter off a step":  {Kind: "funnel", Steps: []string{"a", "b"}, Filters: []Filter{{Step: 2, Key: "device", Value: "x"}}},
		"retention, no end":  {Kind: "retention", StartEvent: "order_placed"},
		"flow without focus": {Kind: "flow"},
		"bad unit":           {Kind: "insights", Unit: "hour"},
		"period too long":    {Kind: "insights", SinceDays: 400},
		"window too long":    {Kind: "stickiness", WindowDays: 400},
		"a sum of nothing":   {Kind: "insights", Measure: "sum"},
		"a bad operator":     {Kind: "insights", Where: []Condition{{Key: "sku", Op: "like", Values: []string{"X"}}}},
		"= with two values":  {Kind: "stickiness", Where: []Condition{{Key: "sku", Op: "eq", Values: []string{"X", "Y"}}}},
	} {
		if err := d.validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	funnel := Definition{Kind: "funnel", Steps: []string{"page_view", "order_placed"}, Filters: []Filter{{Step: 0, Key: "device", Value: "mobile"}}, WindowHours: 24, Breakdown: "device"}
	if err := funnel.validate(); err != nil {
		t.Fatalf("a valid funnel: %v", err)
	}
	netSales := Definition{Kind: "insights", Events: []string{"order_line", "order_line_adjusted", "refund_line"}, Measure: "sum", SumKey: "net_base_cents",
		Where: []Condition{{Key: "sku", Op: "in", Values: []string{"X", "Y"}}}}
	dropOff := Definition{Kind: "flow", Event: "view_item", NotThen: "cart_add", NotThenWithinHours: 24}
	for _, d := range []Definition{{Kind: "stickiness", WindowDays: 14}, netSales, dropOff} {
		if err := d.validate(); err != nil {
			t.Fatalf("%+v: %v", d, err)
		}
	}
}

// CreateStarters stores them as they are, so each must already be what normalized makes of it
func TestEveryStarterBoardIsValid(t *testing.T) {
	if len(Starters) != 4 {
		t.Fatalf("%d starter boards, want the four areas", len(Starters))
	}
	for _, b := range Starters {
		n, err := b.normalized()
		if err != nil {
			t.Errorf("%s: %v", b.Name, err)
		} else if !reflect.DeepEqual(n, b) {
			t.Errorf("%s isn't stored as written: %+v", b.Name, n)
		}
	}
}

func TestNameLimitCountsCharacters(t *testing.T) {
	if _, err := (Board{Name: strings.Repeat("ж", maxName)}).normalized(); err != nil {
		t.Errorf("a %d-letter Cyrillic name was refused: %v", maxName, err)
	}
	if _, err := (Board{Name: strings.Repeat("ж", maxName+1)}).normalized(); !errors.Is(err, ErrInvalid) {
		t.Errorf("a name over the limit: %v, want ErrInvalid", err)
	}
}

func TestReportSizeIsSmallMediumOrFull(t *testing.T) {
	board := func(size string) Board {
		return Board{Name: "Mine", Reports: []Report{{Title: "Buyers", Size: size, Definition: Definition{Kind: "stickiness"}}}}
	}
	if _, err := board("huge").normalized(); !errors.Is(err, ErrInvalid) {
		t.Errorf("size huge: err = %v, want ErrInvalid", err)
	}
	for _, size := range []string{"", "small", "medium", "full"} {
		if _, err := board(size).normalized(); err != nil {
			t.Errorf("size %q: %v", size, err)
		}
	}
}

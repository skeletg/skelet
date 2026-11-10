package signalsgrpc

import (
	"reflect"
	"testing"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/board"
)

// A board report keeps every setting its engine reads across the contract - a field one direction forgets saves as unset
func TestABoardReportKeepsItsFiltersSumAndDropOff(t *testing.T) {
	in := board.Report{Title: "Net sales of two", Size: "full", Definition: board.Definition{
		Kind: "insights", Events: []string{"order_line", "order_line_adjusted", "refund_line"}, Measure: "sum", SumKey: "net_base_cents",
		Where:       []board.Condition{{Key: "sku", Op: "in", Values: []string{"X", "Y"}}},
		StartWhere:  []board.Condition{{Key: "device", Op: "eq", Values: []string{"mobile"}}},
		ReturnWhere: []board.Condition{{Key: "country", Op: "neq", Values: []string{"RS"}}},
		Filters:     []board.Filter{{Step: 1, Key: "sku", Op: "in", Values: []string{"X", "Y"}}},
		NotThen:     "cart_add", NotThenWithinHours: 24,
	}}
	got := reportsFrom([]*signalsv1.BoardReport{reportProto(in)})
	if len(got) != 1 || !reflect.DeepEqual(got[0], in) {
		t.Fatalf("round trip = %+v\nwant %+v", got, in)
	}
}

package admin

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
)

// What the dashboard saves on a board comes back as it was - a definition field the relay drops saves as unset
func TestABoardReportRelaysItsFiltersSumAndDropOff(t *testing.T) {
	sum, net, notThen, within, in := "sum", "net_base_cents", "cart_add", int32(24), model.PropertyFilterOpIn
	out := reportOut(reportIn(&model.BoardReportInput{Title: "Net sales", Definition: &model.BoardReportDefinitionInput{
		Kind: model.BoardReportKindInsights, Events: []string{"order_line", "refund_line"}, Measure: &sum, SumKey: &net,
		Where:       []*model.SynapsePropertyFilterInput{{Key: "sku", Op: model.PropertyFilterOpIn, Values: []string{"X", "Y"}}},
		ReturnWhere: []*model.SynapsePropertyFilterInput{{Key: "device", Op: model.PropertyFilterOpEq, Values: []string{"mobile"}}},
		Filters:     []*model.SynapseFunnelFilterInput{{Step: 1, Key: "sku", Op: &in, Values: []string{"X", "Y"}}},
		NotThen:     &notThen, NotThenWithinHours: &within,
	}})).Definition
	switch {
	case len(out.Events) != 2 || *out.SumKey != net || *out.Measure != sum:
		t.Errorf("the sum = %v %v %v", out.Events, out.SumKey, out.Measure)
	case len(out.Where) != 1 || out.Where[0].Op != model.PropertyFilterOpIn || len(out.Where[0].Values) != 2 || len(out.StartWhere) != 0 || len(out.ReturnWhere) != 1:
		t.Errorf("the conditions = %+v / %+v / %+v", out.Where, out.StartWhere, out.ReturnWhere)
	case *out.Filters[0].Op != model.PropertyFilterOpIn || len(out.Filters[0].Values) != 2:
		t.Errorf("the funnel filter = %+v", out.Filters[0])
	case *out.NotThen != notThen || *out.NotThenWithinHours != within:
		t.Errorf("the drop-off = %v within %v", out.NotThen, out.NotThenWithinHours)
	}
}

package signalsgrpc

import (
	"math"
	"testing"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

// Every measure goes whole as an amount - a sum in cents and a count past int32 alike - beside the clamped value
func TestInsightsCarryEachMeasureWholeAsAnAmount(t *testing.T) {
	big := int64(math.MaxInt32) + 5
	res := insightsResponse(analytics.Insights{Total: big, PrevTotal: 7, Series: []analytics.InsightSeriesTotal{{Key: "X", Total: big}}})
	if res.GetTotal() != math.MaxInt32 || res.GetTotalAmount() != big || res.GetPrevTotalAmount() != 7 {
		t.Errorf("total = %d, amount %d / %d", res.GetTotal(), res.GetTotalAmount(), res.GetPrevTotalAmount())
	}
	if s := res.GetSeries()[0]; s.GetTotal() != math.MaxInt32 || s.GetAmount() != big {
		t.Errorf("series = %d, amount %d", s.GetTotal(), s.GetAmount())
	}
}

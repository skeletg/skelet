package plugin

import (
	"slices"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The SDK can't import core, so its list of statuses a stage sits in is a copy - this is what keeps it one
func TestTheSDKKnowsEveryStatusButDraft(t *testing.T) {
	want := slices.DeleteFunc(order.Statuses(), func(s string) bool { return s == order.OrderStatusDraft })
	if got := slices.Sorted(slices.Values(sdk.StagedStatuses())); !slices.Equal(got, want) {
		t.Errorf("sdk statuses %v, core's %v", got, want)
	}
}

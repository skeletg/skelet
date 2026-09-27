package wiring

import (
	"testing"
	"time"
)

func TestTheSuitesChannelSweepsOutliveTheSchedulerDefault(t *testing.T) {
	for _, name := range []string{"trait-compute", "segment-refresh", "synapse-segment-refresh"} {
		if got := signalsJobTimeout(name); got < 30*time.Minute {
			t.Errorf("%s runs under %v, want the maintenance call's 30m - at many channels it is cut off every run", name, got)
		}
	}
	if got := signalsJobTimeout("event-destinations-forward"); got != 0 {
		t.Errorf("a minute-cadence job got %v, want the scheduler default", got)
	}
}

package wiring

import (
	"testing"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/health"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
)

// A queue that isn't drained outranks failures, which outrank a quiet day - and nothing switched on reads as a choice
func TestTrackingHealthNamesTheWorstFirst(t *testing.T) {
	for name, tc := range map[string]struct {
		p    tracking.Pipeline
		want health.Status
	}{
		"nothing on":           {tracking.Pipeline{Dead24h: 3, Late: time.Hour}, health.StatusOff},
		"quiet":                {tracking.Pipeline{Enabled: 2}, health.StatusOK},
		"a burst still clears": {tracking.Pipeline{Enabled: 2, Late: trackingLagLimit}, health.StatusOK},
		"failures":             {tracking.Pipeline{Enabled: 2, Dead24h: 4}, health.StatusWarn},
		"undrained":            {tracking.Pipeline{Enabled: 2, Dead24h: 4, Late: trackingLagLimit + time.Second}, health.StatusBad},
	} {
		t.Run(name, func(t *testing.T) {
			if got := trackingHealth(tc.p); got.Status != tc.want {
				t.Errorf("%+v = %s (%s), want %s", tc.p, got.Status, got.Detail, tc.want)
			}
		})
	}
}

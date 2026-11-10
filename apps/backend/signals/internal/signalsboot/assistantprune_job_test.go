package signalsboot

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type daysSettings map[string]int64

func (d daysSettings) Int(_ context.Context, path string, _ *uuid.UUID) (int64, error) {
	return d[path], nil
}
func (daysSettings) String(context.Context, string, *uuid.UUID) (string, error) { return "", nil }

type cutoff struct{ before time.Time }

func (c *cutoff) Prune(_ context.Context, before time.Time) (int64, int64, error) {
	c.before = before
	return 2, 3, nil
}

func TestAssistantTranscriptsNeverOutliveTheEventLog(t *testing.T) {
	for name, tc := range map[string]struct {
		assistant, spine int64
		want             int
	}{
		"its own window":           {assistant: 30, spine: 365, want: 30},
		"the event log is shorter": {assistant: 90, spine: 14, want: 14},
		"kept for good, log isn't": {assistant: 0, spine: 60, want: 60},
		"both kept for good":       {assistant: 0, spine: 0, want: 0},
	} {
		got := &cutoff{}
		job := AssistantPruneJob(daysSettings{"signals/assistant/retention_days": tc.assistant, "signals/analytics/spine_retention_days": tc.spine}, got)
		conversations, turns, err := job(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == 0 {
			if !got.before.IsZero() || conversations != 0 || turns != 0 {
				t.Errorf("%s: pruned before %v", name, got.before)
			}
			continue
		}
		if days := int(time.Since(got.before).Hours()/24 + 0.5); days != tc.want || conversations != 2 || turns != 3 {
			t.Errorf("%s: cut at %d days, %d conversations %d turns - want %d days, the counts apart", name, days, conversations, turns, tc.want)
		}
	}
}

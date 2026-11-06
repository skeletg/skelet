package leasequeue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryWalksTheLadderThenGivesUp(t *testing.T) {
	ladder := []time.Duration{time.Second, time.Minute}
	for attempts, want := range map[int32]time.Duration{0: time.Second, 1: time.Second, 2: time.Minute} {
		if got, ok := Retry(attempts, ladder); !ok || got != want {
			t.Errorf("attempt %d: %v %v, want %v", attempts, got, ok, want)
		}
	}
	if _, ok := Retry(3, ladder); ok {
		t.Error("the first attempt plus one per rung is the most a row gets")
	}
}

// Full batches are claimed again at once, every row is sent, and a panicking send costs only its own row
func TestRunOnceDrainsFullBatchesAndSurvivesAPanic(t *testing.T) {
	queue := []int{1, 2, 3, 4, 5}
	var sent atomic.Int32
	r := Runner[int]{
		Name: "test", Batch: 2, Lease: 60, Workers: 2,
		Claim: func(_ context.Context, batch, _ int32) ([]int, error) {
			n := min(int(batch), len(queue))
			out := queue[:n]
			queue = queue[n:]
			return out, nil
		},
		Prepare: func(context.Context, []int) func(context.Context, int) {
			return func(_ context.Context, item int) {
				if item == 3 {
					panic("mapping bug")
				}
				sent.Add(1)
			}
		},
	}
	if n := r.RunOnce(context.Background()); n != 5 || sent.Load() != 4 {
		t.Errorf("took %d and sent %d, want all 5 taken and the 4 that didn't panic sent", n, sent.Load())
	}
}

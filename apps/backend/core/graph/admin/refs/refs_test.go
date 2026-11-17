package refs

import (
	"context"
	"errors"
	"testing"
)

type rec struct{ ID string }

// Each distinct id is asked once, a hundred at a time, and the answers come back in the ids' order with nil for one
// that named nothing
func TestAlignAnswersInTheIdsOrder(t *testing.T) {
	var calls [][]string
	load := func(_ context.Context, ids []string) ([]*rec, error) {
		calls = append(calls, ids)
		var out []*rec
		for _, id := range ids {
			if id != "gone" {
				out = append([]*rec{{ID: id}}, out...) // the loader's own order, which Align mustn't trust
			}
		}
		return out, nil
	}
	ids := []string{"b", "gone", "a", "b", ""}
	got, err := Align(context.Background(), ids, load, func(r *rec) string { return r.ID })
	if err != nil || len(got) != 5 || got[0].ID != "b" || got[1] != nil || got[2].ID != "a" || got[3].ID != "b" || got[4] != nil {
		t.Fatalf("aligned %+v, %v", got, err)
	}
	if len(calls) != 1 || len(calls[0]) != 3 {
		t.Errorf("asked %v - want each distinct id once", calls)
	}

	many := make([]string, 250)
	for i := range many {
		many[i] = string(rune('a'+i%26)) + string(rune('a'+i/26))
	}
	calls = nil
	if _, err := Align(context.Background(), many, load, func(r *rec) string { return r.ID }); err != nil || len(calls) != 3 {
		t.Errorf("250 ids in %d calls (%v) - want three of at most a hundred", len(calls), err)
	}
	if _, err := Align[rec](context.Background(), ids, nil, nil); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("no loader: %v", err)
	}
}

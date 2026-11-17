package refs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

type rec struct{ ID string }

// Each distinct id is asked once, the loader's batch at a time, and the answers come back in the ids' order with nil
// for one that named nothing
func TestAlignAnswersInTheIdsOrder(t *testing.T) {
	a, b, gone := uuid.NewString(), uuid.NewString(), uuid.NewString()
	var calls [][]string
	load := func(_ context.Context, ids []string) ([]*rec, error) {
		calls = append(calls, ids)
		var out []*rec
		for _, id := range ids {
			if id != gone {
				out = append([]*rec{{ID: id}}, out...) // the loader's own order, which Align mustn't trust
			}
		}
		return out, nil
	}
	l := Loader[rec]{Load: load, Batch: 100}
	// An upper-case or braced id is the same record, and one that isn't a uuid names nothing and is never asked
	ids := []string{b, gone, strings.ToUpper(a), "{" + b + "}", "", "not-an-id"}
	got, err := Align(context.Background(), ids, l, func(r *rec) string { return r.ID })
	if err != nil || len(got) != 6 || got[0].ID != b || got[1] != nil || got[2].ID != a || got[3].ID != b || got[4] != nil || got[5] != nil {
		t.Fatalf("aligned %+v, %v", got, err)
	}
	if len(calls) != 1 || len(calls[0]) != 3 {
		t.Errorf("asked %v - want each distinct id once, and nothing that isn't an id", calls)
	}
	if _, err := Align(context.Background(), ids, Loader[rec]{}, nil); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("no loader: %v", err)
	}
}

// A loader that takes 24 ids is asked 24 at a time - handed more, core's own query truncates and the rest would come
// back as nothing there
func TestAlignKeepsToTheLoadersBatch(t *testing.T) {
	many := make([]string, 60)
	for i := range many {
		many[i] = uuid.NewString()
	}
	biggest, calls := 0, 0
	capped := func(_ context.Context, ids []string) ([]*rec, error) {
		calls++
		biggest = max(biggest, len(ids))
		if len(ids) > 24 {
			ids = ids[:24]
		}
		out := make([]*rec, len(ids))
		for i, id := range ids {
			out[i] = &rec{ID: id}
		}
		return out, nil
	}
	got, err := Align(context.Background(), many, Loader[rec]{Load: capped, Batch: 24}, func(r *rec) string { return r.ID })
	if err != nil || calls != 3 || biggest > 24 {
		t.Fatalf("%d calls, the biggest of %d ids, %v - want three of at most 24", calls, biggest, err)
	}
	for i, r := range got {
		if r == nil || r.ID != many[i] {
			t.Fatalf("id %d of 60 came back %+v", i, r)
		}
	}
}

// Gone and not-yours are nothing there; a database error or a deadline is a failure, or a page of nulls gets cached
func TestEachOnlyLeavesOutWhatIsntThere(t *testing.T) {
	gone, hidden, broken, fine := "gone", "hidden", "broken", "fine"
	one := func(_ context.Context, id string) (*rec, error) {
		switch id {
		case gone:
			return nil, apperr.New("NOT_FOUND", apperr.ClassNotFound, "category not found")
		case hidden:
			return nil, apperr.New("FORBIDDEN", apperr.ClassForbidden, "not yours")
		case broken:
			return nil, errors.New("get category: connection reset")
		case "empty":
			return nil, nil
		}
		return &rec{ID: id}, nil
	}
	got, err := Each(one)(context.Background(), []string{gone, fine, hidden, "empty"})
	if err != nil || len(got) != 1 || got[0].ID != fine {
		t.Errorf("got %+v, %v - want the one record that's there", got, err)
	}
	if _, err := Each(one)(context.Background(), []string{fine, broken}); err == nil {
		t.Error("a database error was answered as nothing there")
	}
	late := func(context.Context, string) (*rec, error) { return nil, context.DeadlineExceeded }
	if _, err := Each(late)(context.Background(), []string{fine}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a deadline was answered as nothing there: %v", err)
	}
}

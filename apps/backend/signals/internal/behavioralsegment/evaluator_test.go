package behavioralsegment

import (
	"testing"

	"github.com/google/uuid"
)

// AND, OR and NOT come down to these three
func TestSetAlgebra(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()

	t.Run("intersect keeps only what both hold", func(t *testing.T) {
		got := intersectSets(sliceToSet([]uuid.UUID{a, b}), sliceToSet([]uuid.UUID{b, c}))
		if len(got) != 1 || !got[b] {
			t.Errorf("intersect gave %d ids, want just b", len(got))
		}
	})

	t.Run("intersect is the same either way round", func(t *testing.T) {
		x, y := sliceToSet([]uuid.UUID{a}), sliceToSet([]uuid.UUID{a, b, c})
		if len(intersectSets(x, y)) != len(intersectSets(y, x)) {
			t.Error("intersect depends on argument order, but it picks the smaller side to walk")
		}
	})

	t.Run("union adds into the accumulator", func(t *testing.T) {
		dst := sliceToSet([]uuid.UUID{a})
		unionInto(dst, sliceToSet([]uuid.UUID{b, c}))
		if len(dst) != 3 {
			t.Errorf("union gave %d ids, want 3", len(dst))
		}
	})

	t.Run("complement is the universe minus the matches", func(t *testing.T) {
		got := complementSet(sliceToSet([]uuid.UUID{a, b, c}), sliceToSet([]uuid.UUID{b}))
		if len(got) != 2 || got[b] {
			t.Errorf("complement gave %d ids and b=%v, want 2 without b", len(got), got[b])
		}
	})

	t.Run("an empty universe complements to nothing", func(t *testing.T) {
		if got := complementSet(map[uuid.UUID]bool{}, sliceToSet([]uuid.UUID{a})); len(got) != 0 {
			t.Errorf("complement of an empty universe gave %d ids", len(got))
		}
	})
}

func TestSliceSetRoundTrip(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	got := setToSlice(sliceToSet(ids))
	if len(got) != len(ids) {
		t.Fatalf("round trip gave %d ids, want %d", len(got), len(ids))
	}
	seen := sliceToSet(got)
	for _, id := range ids {
		if !seen[id] {
			t.Errorf("%s went missing in the round trip", id)
		}
	}
}

// A duplicate id is one customer, not two - the sets are what dedupe a member list
func TestSliceToSetDedupes(t *testing.T) {
	id := uuid.New()
	if got := sliceToSet([]uuid.UUID{id, id, id}); len(got) != 1 {
		t.Errorf("three copies of one id gave %d entries", len(got))
	}
}

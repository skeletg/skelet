package search

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubEngine struct {
	name  string
	err   error
	hits  []ProductHit
	total int64
	calls int
}

func (s *stubEngine) Name() string { return s.name }
func (s *stubEngine) SearchProducts(context.Context, string, uuid.UUID, string, int, int, map[uuid.UUID]float64) ([]ProductHit, int64, error) {
	s.calls++
	return s.hits, s.total, s.err
}
func (s *stubEngine) BrowseProducts(context.Context, uuid.UUID, string, string, BrowseFilter, string, int, int, map[uuid.UUID]float64) (BrowseResult, error) {
	s.calls++
	return BrowseResult{Total: s.total}, s.err
}
func (s *stubEngine) ListSortOptions(context.Context) []SortOption {
	return []SortOption{{Slug: s.name}}
}

func TestTieredEngine(t *testing.T) {
	boom := errors.New("cluster down")

	t.Run("primary answers and the backup is never called", func(t *testing.T) {
		primary := &stubEngine{name: "primary", total: 7}
		backup := &stubEngine{name: "backup", total: 1}
		var degraded bool
		e := NewTieredEngine(primary, backup, func(context.Context, string, error) { degraded = true })

		if _, total, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil); err != nil || total != 7 {
			t.Fatalf("got total=%d err=%v, want 7/nil", total, err)
		}
		if backup.calls != 0 {
			t.Fatal("backup was called while the primary was healthy")
		}
		if degraded {
			t.Fatal("reported degraded on a healthy primary")
		}
	})

	t.Run("primary failure falls back and reports degraded", func(t *testing.T) {
		primary := &stubEngine{name: "primary", err: boom}
		backup := &stubEngine{name: "backup", total: 3}
		var op string
		e := NewTieredEngine(primary, backup, func(_ context.Context, o string, _ error) { op = o })

		res, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil)
		if err != nil || res.Total != 3 {
			t.Fatalf("got total=%d err=%v, want 3/nil", res.Total, err)
		}
		if backup.calls != 1 {
			t.Fatalf("backup calls = %d, want 1", backup.calls)
		}
		if op != "browse" {
			t.Fatalf("degraded op = %q, want browse", op)
		}
	})

	// The shipped production default: no cluster configured at all
	t.Run("nil primary runs entirely on the backup", func(t *testing.T) {
		backup := &stubEngine{name: "backup", total: 5}
		var degraded bool
		e := NewTieredEngine(nil, backup, func(context.Context, string, error) { degraded = true })

		if _, total, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil); err != nil || total != 5 {
			t.Fatalf("got total=%d err=%v, want 5/nil", total, err)
		}
		if _, err := e.BrowseProducts(context.Background(), uuid.New(), "en", "", BrowseFilter{}, "", 10, 0, nil); err != nil {
			t.Fatalf("browse: %v", err)
		}
		// Never configured is a steady state, not a degradation - reporting it marked every browse and search response
		// uncacheable forever, which disabled the front response cache on the two hottest paths for the shipped default
		if degraded {
			t.Fatal("a tier with no primary reported degraded, which keeps every response out of the cache")
		}
		if got := e.Name(); got != "backup" {
			t.Fatalf("Name() = %q, want backup", got)
		}
		if opts := e.ListSortOptions(context.Background()); len(opts) != 1 || opts[0].Slug != "backup" {
			t.Fatalf("sort options came from the wrong engine: %+v", opts)
		}
	})

	// Both down is a real error, not a silent empty page
	t.Run("both failing surfaces the error", func(t *testing.T) {
		e := NewTieredEngine(&stubEngine{name: "primary", err: boom}, &stubEngine{name: "backup", err: boom}, nil)
		if _, _, err := e.SearchProducts(context.Background(), "q", uuid.New(), "en", 10, 0, nil); err == nil {
			t.Fatal("want an error when every tier fails")
		}
	})
}

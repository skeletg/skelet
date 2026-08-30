package search

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubLifecycle struct {
	count        int
	recreateErr  error
	reindexErr   error
	recreates    int
	reindexes    int
	ensures      int
	ensureErr    error
	lastRecreate bool // reindex must come after recreate
}

func (s *stubLifecycle) EnsureIndex(context.Context) error { s.ensures++; return s.ensureErr }
func (s *stubLifecycle) Recreate(context.Context) error {
	s.recreates++
	s.lastRecreate = true
	return s.recreateErr
}
func (s *stubLifecycle) ReindexAll(context.Context) (int, error) {
	s.reindexes++
	return s.count, s.reindexErr
}

func TestIndexRegistryRefusesDuplicatesAndEmpties(t *testing.T) {
	r := NewIndexRegistry()
	if err := r.Register(RegisteredIndex{Slug: "products", Lifecycle: &stubLifecycle{}}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(RegisteredIndex{Slug: "products", Lifecycle: &stubLifecycle{}}); err == nil {
		t.Fatal("duplicate slug registered silently - two writers would interleave one index")
	}
	if err := r.Register(RegisteredIndex{Slug: "", Lifecycle: &stubLifecycle{}}); err == nil {
		t.Fatal("empty slug accepted")
	}
	if err := r.Register(RegisteredIndex{Slug: "x"}); err == nil {
		t.Fatal("nil lifecycle accepted")
	}
}

func TestReindexTargetsRunsEveryIndexPastAFailure(t *testing.T) {
	r := NewIndexRegistry()
	boom := errors.New("cluster hiccup")
	failing := &stubLifecycle{count: 3, reindexErr: boom}
	healthy := &stubLifecycle{count: 7}
	_ = r.Register(RegisteredIndex{Slug: "blog", Lifecycle: failing})
	_ = r.Register(RegisteredIndex{Slug: "products", Lifecycle: healthy})

	results, err := r.ReindexTargets(context.Background(), []string{"all"})
	if err != nil {
		t.Fatalf("reindex all: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	// One index failing must not stop the other - "products failed so blog was never
	// tried" helps nobody mid-incident
	if healthy.reindexes != 1 || healthy.recreates != 1 {
		t.Fatalf("healthy index skipped after the failing one: reindexes=%d recreates=%d", healthy.reindexes, healthy.recreates)
	}
	byName := map[string]ReindexResult{}
	for _, res := range results {
		byName[res.Slug] = res
	}
	if byName["blog"].Err == nil || byName["blog"].Count != 3 {
		t.Fatalf("failing index result = %+v, want its partial count and error", byName["blog"])
	}
	if byName["products"].Err != nil || byName["products"].Count != 7 {
		t.Fatalf("healthy index result = %+v", byName["products"])
	}
}

func TestReindexTargetsSkipsRebuildWhenRecreateFails(t *testing.T) {
	r := NewIndexRegistry()
	broken := &stubLifecycle{recreateErr: errors.New("mapping rejected")}
	_ = r.Register(RegisteredIndex{Slug: "products", Lifecycle: broken})

	results, err := r.ReindexTargets(context.Background(), []string{"products"})
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if !results[0].Skipped || results[0].Err == nil {
		t.Fatalf("result = %+v, want skipped with error", results[0])
	}
	// Rebuilding into an index whose recreate failed would write into the OLD mapping
	if broken.reindexes != 0 {
		t.Fatal("reindex ran after a failed recreate")
	}
}

func TestReindexTargetsNamesUnknownTargets(t *testing.T) {
	r := NewIndexRegistry()
	_ = r.Register(RegisteredIndex{Slug: "products", Lifecycle: &stubLifecycle{}})
	_, err := r.ReindexTargets(context.Background(), []string{"posts"})
	if err == nil {
		t.Fatal("unknown target accepted")
	}
	// The message must name what IS registered, or the operator is left guessing
	if got := err.Error(); !strings.Contains(got, "products") {
		t.Fatalf("error %q does not list registered slugs", got)
	}
}

type blueGreenStub struct {
	stubLifecycle
	swaps int
}

func (b *blueGreenStub) RebuildAndSwap(context.Context) (int, error) {
	b.swaps++
	return 42, nil
}

// Blue-green must win over recreate-in-place - falling back would silently reopen
// the index-empty-while-rebuilding window the interface exists to close
func TestReindexTargetsPrefersBlueGreen(t *testing.T) {
	r := NewIndexRegistry()
	bg := &blueGreenStub{}
	_ = r.Register(RegisteredIndex{Slug: "products", Lifecycle: bg})

	results, err := r.ReindexTargets(context.Background(), []string{"products"})
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}
	if bg.swaps != 1 || bg.recreates != 0 || bg.reindexes != 0 {
		t.Fatalf("swaps=%d recreates=%d reindexes=%d, want the blue-green path only", bg.swaps, bg.recreates, bg.reindexes)
	}
	if results[0].Count != 42 {
		t.Fatalf("count = %d, want the swap's count", results[0].Count)
	}
}

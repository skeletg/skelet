package search

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// IndexLifecycle is what every search index owes the operator tooling: boot-time creation, a drop-and-recreate for mapping changes, and a full rebuild from the source of truth. It deliberately abstracts the LIFECYCLE and not the query DSL - engine.go explains why query contracts stay domain-phrased, and the same logic runs the other way here: reindexing is inherently engine-specific, so this is the one shape products, blog posts and plugin-owned indexes genuinely share
type IndexLifecycle interface {
	EnsureIndex(ctx context.Context) error
	Recreate(ctx context.Context) error
	// Returns how many source records were indexed - on error the count says how far
	// it got, since "0 or everything" hides whether 10 or 10,000 made it
	ReindexAll(ctx context.Context) (int, error)
}

// RegisteredIndex pairs a lifecycle with the slug operators address it by - a slug rather than the lifecycle naming
// itself, because ProductIndexer.Name() is already taken by the Engine contract (it answers "opensearch")
type RegisteredIndex struct {
	// Operator-facing: `bin/cli search reindex <slug>` and the search.reindex action
	Slug string
	// Empty for core indexes; a plugin's manifest slug for registered ones, so the
	// reindex tooling can say WHOSE index just failed
	PluginSlug string
	Lifecycle  IndexLifecycle
}

// IndexRegistry is the set of live indexes. An instance rather than a package global so a test (or a process wiring a different subset)
// can't inherit another's entries - core indexes register at wiring time, plugin ones through the SDK host
type IndexRegistry struct {
	mu      sync.Mutex
	entries []RegisteredIndex
}

func NewIndexRegistry() *IndexRegistry {
	return &IndexRegistry{}
}

// Register refuses duplicate slugs loudly - two writers rebuilding the same index
// name would silently interleave their docs
func (r *IndexRegistry) Register(entry RegisteredIndex) error {
	if entry.Slug == "" {
		return fmt.Errorf("search index registration needs a slug")
	}
	if entry.Lifecycle == nil {
		return fmt.Errorf("search index %q registered without a lifecycle", entry.Slug)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.Slug == entry.Slug {
			return fmt.Errorf("search index %q is already registered", entry.Slug)
		}
	}
	r.entries = append(r.entries, entry)
	return nil
}

// List returns entries sorted by slug, so CLI output and action option order stay
// stable across boots regardless of init ordering
func (r *IndexRegistry) List() []RegisteredIndex {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]RegisteredIndex(nil), r.entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func (r *IndexRegistry) Get(slug string) (RegisteredIndex, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.Slug == slug {
			return e, true
		}
	}
	return RegisteredIndex{}, false
}

// Slugs is List reduced to names, for error messages and argument options
func (r *IndexRegistry) Slugs() []string {
	entries := r.List()
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Slug
	}
	return out
}

// ReindexResult is one index's outcome inside a multi-index run
type ReindexResult struct {
	Slug    string
	Count   int
	Err     error
	Skipped bool // recreate failed, so the rebuild never started
}

// ReindexTargets drops and rebuilds each named index ("all" or nil = every one). One index failing doesn't stop the
// others - the operator asked for a rebuild, and "products failed so blog was never tried" helps nobody
func (r *IndexRegistry) ReindexTargets(ctx context.Context, targets []string) ([]ReindexResult, error) {
	var run []RegisteredIndex
	if len(targets) == 0 || (len(targets) == 1 && targets[0] == "all") {
		run = r.List()
	} else {
		for _, t := range targets {
			e, ok := r.Get(t)
			if !ok {
				return nil, fmt.Errorf("unknown search index %q (registered: %s)", t, strings.Join(r.Slugs(), ", "))
			}
			run = append(run, e)
		}
	}
	if len(run) == 0 {
		return nil, fmt.Errorf("no search indexes registered")
	}
	out := make([]ReindexResult, 0, len(run))
	for _, e := range run {
		res := ReindexResult{Slug: e.Slug}
		// Blue-green when the index supports it: the live index serves untouched until an atomic alias flip.
		// Recreate-in-place is the fallback (plugin indexes), where the index IS empty for the duration
		if bg, ok := e.Lifecycle.(BlueGreen); ok {
			res.Count, res.Err = bg.RebuildAndSwap(ctx)
			out = append(out, res)
			continue
		}
		if err := e.Lifecycle.Recreate(ctx); err != nil {
			res.Err = fmt.Errorf("recreate: %w", err)
			res.Skipped = true
			out = append(out, res)
			continue
		}
		res.Count, res.Err = e.Lifecycle.ReindexAll(ctx)
		out = append(out, res)
	}
	return out, nil
}

// EnsureAll creates whatever is missing - the boot path, where one failing index
// must not stop the rest from getting theirs
func (r *IndexRegistry) EnsureAll(ctx context.Context) map[string]error {
	errs := map[string]error{}
	for _, e := range r.List() {
		if err := e.Lifecycle.EnsureIndex(ctx); err != nil {
			errs[e.Slug] = err
		}
	}
	return errs
}

// Compile-time proof the two core indexers fit the lifecycle
var (
	_ IndexLifecycle = (*ProductIndexer)(nil)
	_ IndexLifecycle = (*BlogPostIndexer)(nil)
)

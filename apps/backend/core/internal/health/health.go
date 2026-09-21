// Package health collects operator-facing status for every subsystem behind one admin panel.
package health

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"sync"
	"time"
)

type Status string

const (
	StatusOK   Status = "OK"
	StatusWarn Status = "WARN"
	StatusBad  Status = "BAD"
	// Not configured. A deployment running none of the optional tiers should read
	// as calm, not broken - "off" is a choice someone made, not a fault
	StatusOff Status = "OFF"
)

// Detail states what IS - short and factual. Hint says what to do about it, and is
// left empty when there is nothing to do, so the UI can render it as the exception
type Result struct {
	Status Status
	Detail string
	Hint   string
}

func OK(detail string) Result         { return Result{Status: StatusOK, Detail: detail} }
func Off(detail, hint string) Result  { return Result{Status: StatusOff, Detail: detail, Hint: hint} }
func Warn(detail, hint string) Result { return Result{Status: StatusWarn, Detail: detail, Hint: hint} }
func Bad(detail, hint string) Result  { return Result{Status: StatusBad, Detail: detail, Hint: hint} }

type CheckFunc func(ctx context.Context) Result

type Check struct {
	Group string
	Key   string
	Label string
	Order int
	Run   CheckFunc
}

type Group struct {
	Key   string
	Label string
	Order int
}

// One probe's worth of time. The panel is a page load, not a monitoring system -
// a wedged dependency must report "timed out" rather than hold the response open
const probeTimeout = 3 * time.Second

type Registry struct {
	mu     sync.Mutex
	groups map[string]Group
	checks []Check
	// One probe sweep per memoTTL no matter how many admin tabs poll - the panel refreshes every 5s per tab, and without
	// this each tab fires every probe (an S3 Head, an OpenSearch ping, one query per webhook) on its own
	memoMu  sync.Mutex
	memo    Report
	memoAt  time.Time
	gen     int // bumped per Register, so a fresh registration invalidates the memo
	memoGen int
}

func NewRegistry() *Registry {
	return &Registry{groups: map[string]Group{}}
}

// RegisterGroup names a section and fixes its position. Registering a check into an
// unnamed group still works - it just sorts last under its own key
func (r *Registry) RegisterGroup(g Group) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groups[g.Key] = g
}

func (r *Registry) Register(c Check) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks = append(r.checks, c)
	r.gen++
}

// CheckResult pairs a registration with what its probe said
type CheckResult struct {
	Key    string
	Label  string
	Order  int
	Result Result
}

type GroupResult struct {
	Key    string
	Label  string
	Order  int
	Checks []CheckResult
}

type Report struct {
	Status    Status
	CheckedAt time.Time
	Groups    []GroupResult
}

// Run probes everything concurrently. One slow subsystem costs the slowest single probe rather than the sum, and a panicking one reports as failed instead of taking the whole panel
// an operator opens this page precisely when something is already wrong, so it has to survive its own subjects.
func (r *Registry) Run(ctx context.Context, now time.Time) Report {
	r.memoMu.Lock()
	defer r.memoMu.Unlock()
	r.mu.Lock()
	gen := r.gen
	r.mu.Unlock()
	if !r.memoAt.IsZero() && gen == r.memoGen && now.Sub(r.memoAt) < memoTTL {
		return r.memo
	}
	rep := r.run(context.WithoutCancel(ctx), now)
	r.memo, r.memoAt, r.memoGen = rep, now, gen
	return rep
}

const memoTTL = 3 * time.Second

func (r *Registry) run(ctx context.Context, now time.Time) Report {
	r.mu.Lock()
	checks := append([]Check(nil), r.checks...)
	groups := make(map[string]Group, len(r.groups))
	maps.Copy(groups, r.groups)
	r.mu.Unlock()

	results := make([]Result, len(checks))
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Go(func() {
			results[i] = runOne(ctx, c)
		})
	}
	wg.Wait()

	byGroup := map[string]*GroupResult{}
	for i, c := range checks {
		g, ok := byGroup[c.Group]
		if !ok {
			meta := groups[c.Group]
			label := meta.Label
			if label == "" {
				label = c.Group
			}
			g = &GroupResult{Key: c.Group, Label: label, Order: meta.Order}
			byGroup[c.Group] = g
		}
		g.Checks = append(g.Checks, CheckResult{Key: c.Key, Label: c.Label, Order: c.Order, Result: results[i]})
	}

	out := make([]GroupResult, 0, len(byGroup))
	for _, g := range byGroup {
		sort.SliceStable(g.Checks, func(i, j int) bool {
			if g.Checks[i].Order != g.Checks[j].Order {
				return g.Checks[i].Order < g.Checks[j].Order
			}
			return g.Checks[i].Key < g.Checks[j].Key
		})
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Key < out[j].Key
	})

	return Report{Status: worst(results), CheckedAt: now, Groups: out}
}

func runOne(ctx context.Context, c Check) (res Result) {
	defer func() {
		if p := recover(); p != nil {
			res = Bad("probe panicked", fmt.Sprintf("%v - this is a bug in the %s check, not necessarily in the subsystem", p, c.Key))
		}
	}()
	pctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	done := make(chan Result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- Bad("probe panicked", fmt.Sprintf("%v", p))
			}
		}()
		done <- c.Run(pctx)
	}()
	select {
	case r := <-done:
		return r
	case <-pctx.Done():
		// The probe goroutine is left to finish into the buffered channel rather than
		// leaked forever - it holds one slot and exits when its own ctx unblocks it
		return Bad("timed out", "The subsystem did not answer within "+probeTimeout.String()+", which usually means it is reachable but wedged rather than down")
	}
}

// OFF never escalates - a subsystem nobody deployed is not a fault. Everything off
// still reports off, so an unconfigured install reads calm rather than green
func worst(results []Result) Status {
	out := StatusOff
	for _, r := range results {
		switch r.Status {
		case StatusBad:
			return StatusBad
		case StatusWarn:
			out = StatusWarn
		case StatusOK:
			if out != StatusWarn {
				out = StatusOK
			}
		}
	}
	return out
}

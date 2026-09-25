package signalsboot

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
)

type fixedOrder struct {
	order  []uuid.UUID
	marked []uuid.UUID
}

func (f *fixedOrder) TraitComputeChannels(context.Context) ([]uuid.UUID, error)       { return f.order, nil }
func (f *fixedOrder) MarkTraitsAttempted(context.Context, uuid.UUID, time.Time) error { return nil }
func (f *fixedOrder) MarkTraitsComputed(_ context.Context, id uuid.UUID, _ time.Time) error {
	f.marked = append(f.marked, id)
	return nil
}

type failingOn struct {
	bad  uuid.UUID
	done []uuid.UUID
}

func (f *failingOn) ComputeTraits(_ context.Context, id uuid.UUID, _ time.Time, _ analytics.LifecycleWindows) (int, error) {
	if id == f.bad {
		return 0, errors.New("spine read failed")
	}
	f.done = append(f.done, id)
	return 1, nil
}

type noSettings struct{}

func (noSettings) Int(context.Context, string, *uuid.UUID) (int64, error)     { return 0, nil }
func (noSettings) String(context.Context, string, *uuid.UUID) (string, error) { return "", nil }

func TestOneFailingChannelDoesNotCostTheOthersTheirTraits(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	roster := &fixedOrder{order: []uuid.UUID{a, b, c}}
	traits := &failingOn{bad: a}
	if err := TraitComputeJob(roster, traits, noSettings{})(context.Background()); err == nil {
		t.Error("the failing channel went unreported")
	}
	if !slices.Equal(traits.done, []uuid.UUID{b, c}) {
		t.Errorf("computed %v, want the two channels after the failing one", traits.done)
	}
	if !slices.Equal(roster.marked, []uuid.UUID{b, c}) {
		t.Errorf("marked %v, want only the channels that finished", roster.marked)
	}
}

type attemptLog struct {
	fixedOrder
	attempted []uuid.UUID
}

func (a *attemptLog) MarkTraitsAttempted(_ context.Context, id uuid.UUID, _ time.Time) error {
	a.attempted = append(a.attempted, id)
	return nil
}

func TestAFailedChannelIsStillMarkedAttempted(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	roster := &attemptLog{order: []uuid.UUID{a, b}}
	_ = TraitComputeJob(roster, &failingOn{bad: a}, noSettings{})(context.Background())
	if !slices.Equal(roster.attempted, []uuid.UUID{a, b}) {
		t.Errorf("attempted %v, want both - an unmarked failure leads every run and starves the rest", roster.attempted)
	}
}

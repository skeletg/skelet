package config

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// askedScopesRepo records the scope each read was asked for, which is the only thing the adapter decides
type askedScopesRepo struct{ asked []Scope }

func (r *askedScopesRepo) Upsert(_ context.Context, scope Scope, path, value string) (StoredValue, error) {
	return StoredValue{Scope: scope, Path: path, Value: value}, nil
}
func (r *askedScopesRepo) Delete(context.Context, Scope, string) error { return nil }
func (r *askedScopesRepo) GetCandidates(_ context.Context, scope Scope, _ string) ([]StoredValue, error) {
	r.asked = append(r.asked, scope)
	return nil, nil
}
func (r *askedScopesRepo) ListInScopes(context.Context, Scope) ([]StoredValue, error) {
	return nil, nil
}
func (r *askedScopesRepo) ListAll(context.Context) ([]StoredValue, error) { return nil, nil }

// The suite spells scope as a channel id, so this adapter is the single place that can send a
// per-channel read to the store-wide value or the other way round - every Signals setting reads through it
func TestChannelScopedMapsANilChannelToTheStoreWideScope(t *testing.T) {
	repo := &askedScopesRepo{}
	svc := NewService(&askedScopesRepo{})
	svc.SetSignalsRepo(repo) // signals/lens/* is a Signals path, so its reads go to the suite's store
	s := NewChannelScoped(svc)

	if _, err := s.Bool(context.Background(), "signals/lens/enabled", nil); err != nil {
		t.Fatalf("Bool: %v", err)
	}
	if len(repo.asked) == 0 {
		t.Fatal("no scope reached the repository")
	}
	if got := repo.asked[0]; got.Kind != DefaultScope.Kind || got.ChannelID != nil {
		t.Errorf("asked for %+v, want the store-wide scope", got)
	}
}

func TestChannelScopedMapsAChannelIDToThatChannelsScope(t *testing.T) {
	repo := &askedScopesRepo{}
	svc := NewService(&askedScopesRepo{})
	svc.SetSignalsRepo(repo)
	s := NewChannelScoped(svc)
	ch := uuid.New()

	if _, err := s.Int(context.Background(), "signals/lens/sampling_rate", &ch); err != nil {
		t.Fatalf("Int: %v", err)
	}
	var seen bool
	for _, sc := range repo.asked {
		if sc.Kind == ScopeChannel && sc.ChannelID != nil && *sc.ChannelID == ch {
			seen = true
		}
	}
	if !seen {
		t.Errorf("asked for %+v, none of them scoped to channel %s", repo.asked, ch)
	}
}

// The path says which store holds it, and getting that wrong reads a Signals knob out of core's
// table - where it is not, so the suite would quietly run on defaults whatever an operator set
func TestASignalsPathReadsTheSuitesStoreAndACorePathDoesNot(t *testing.T) {
	core, signals := &askedScopesRepo{}, &askedScopesRepo{}
	svc := NewService(core)
	svc.SetSignalsRepo(signals)
	ctx := context.Background()

	if _, err := svc.Get(ctx, "signals/lens/enabled", DefaultScope); err != nil {
		t.Fatalf("read a signals path: %v", err)
	}
	if len(signals.asked) == 0 || len(core.asked) != 0 {
		t.Errorf("signals path hit core=%d signals=%d, want it in the suite's store alone", len(core.asked), len(signals.asked))
	}

	core.asked, signals.asked = nil, nil
	if _, err := svc.Get(ctx, "sales/cart/retention_days", DefaultScope); err != nil {
		t.Fatalf("read a core path: %v", err)
	}
	if len(core.asked) == 0 || len(signals.asked) != 0 {
		t.Errorf("core path hit core=%d signals=%d, want it in core's store alone", len(core.asked), len(signals.asked))
	}
}

// A store this process cannot reach must refuse the write rather than drop it - an operator saving a
// Signals setting with the suite unreachable would otherwise see it succeed and change nothing
func TestAWriteToAnUnreachableStoreIsRefused(t *testing.T) {
	svc := NewService(&askedScopesRepo{}) // no signals store wired

	if _, err := svc.Set(context.Background(), "signals/lens/enabled", "false", DefaultScope); !errors.Is(err, ErrStoreUnavailable) {
		t.Errorf("Set = %v, want ErrStoreUnavailable", err)
	}
}

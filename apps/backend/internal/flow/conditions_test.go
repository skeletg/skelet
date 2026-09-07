package flow

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
)

type stubTraits struct {
	t     CustomerTraits
	found bool
}

func (s stubTraits) CustomerTraits(context.Context, uuid.UUID) (CustomerTraits, bool, error) {
	return s.t, s.found, nil
}

// The point of the registry: a condition whose suite isn't installed is absent, not silently false
func TestUnregisteredKindIsRejectedRatherThanEvaluated(t *testing.T) {
	s := &Service{conditions: NewConditionRegistry()}
	if s.validConditionKind("lifecycle") {
		t.Error("lifecycle validated with nothing registered to answer it")
	}
	if s.evalConditionBase(context.Background(), &Condition{Kind: "lifecycle"}, uuid.New()) {
		t.Error("an unregistered condition matched")
	}

	s.RegisterCondition(lifecycleCondition{traits: stubTraits{t: CustomerTraits{Lifecycle: "active"}, found: true}})
	if !s.validConditionKind("lifecycle") {
		t.Error("lifecycle is registered but did not validate")
	}
	if !s.evalConditionBase(context.Background(), &Condition{Kind: "lifecycle", Lifecycle: []string{"active"}}, uuid.New()) {
		t.Error("registered lifecycle condition did not match a matching customer")
	}
}

// A customer with no traits row must not satisfy an rfm condition whose minimums are all zero
func TestRFMDoesNotMatchACustomerWithNoTraits(t *testing.T) {
	c := rfmCondition{traits: stubTraits{found: false}}
	if c.Matches(context.Background(), uuid.New(), &Condition{}) {
		t.Error("rfm matched a customer that has no traits row, so every zero minimum passed")
	}
}

// Core answers these two without the analytics stack, so they are registered by the constructor rather than by a suite
func TestCoreConditionsNeedNoSuite(t *testing.T) {
	for _, e := range []ConditionEvaluator{consentCondition{}, emailActivityCondition{}} {
		if e.Kind() == "" {
			t.Fatal("a core condition has no kind")
		}
		if e.Matches(context.Background(), uuid.New(), &Condition{}) {
			t.Errorf("%s matched with no reader wired, want a refusal", e.Kind())
		}
	}
}

func TestRegistryKindsAreSortedAndDeduped(t *testing.T) {
	r := NewConditionRegistry()
	r.Register(rfmCondition{})
	r.Register(consentCondition{})
	r.Register(rfmCondition{}) // last registration wins rather than colliding
	got := r.Kinds()
	if !slices.Equal(got, []string{"consent", "rfm"}) {
		t.Errorf("Kinds() = %v, want [consent rfm]", got)
	}
}

// A Service built without the constructor must refuse conditions, not panic on a nil registry
func TestNilRegistryRefusesRatherThanPanics(t *testing.T) {
	var r *ConditionRegistry
	if _, ok := r.Lookup("consent"); ok {
		t.Error("nil registry reported a hit")
	}
	if got := r.Kinds(); got != nil {
		t.Errorf("nil registry Kinds() = %v, want nil", got)
	}
}

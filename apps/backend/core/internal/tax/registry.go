package tax

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// The runtime contract for a tax condition type - ValidateParams runs on the admin write path, and ParamsSchema is Draft-07 JSON Schema the dashboard renders a form from
type ConditionEvaluator interface {
	Slug() string
	Label() string
	Description() string
	ParamsSchema() json.RawMessage
	ValidateParams(params json.RawMessage) error
	Evaluate(ctx context.Context, env Environment, params json.RawMessage) (bool, error)
}

// The runtime contract for a tax effect type - Apply writes tax into the ledger, and may read prior tax back out, which is how compound effects work
type EffectApplier interface {
	Slug() string
	Label() string
	Description() string
	ParamsSchema() json.RawMessage
	ValidateParams(params json.RawMessage) error
	Apply(ctx context.Context, env Environment, params json.RawMessage, ledger *Ledger) error
}

type Registry struct {
	mu         sync.RWMutex
	conditions map[string]ConditionEvaluator
	effects    map[string]EffectApplier
}

func NewRegistry() *Registry {
	return &Registry{
		conditions: map[string]ConditionEvaluator{},
		effects:    map[string]EffectApplier{},
	}
}

// The package-level singleton built-in types self-register into through their init()
var DefaultRegistry = NewRegistry()

func (r *Registry) RegisterCondition(c ConditionEvaluator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.conditions[c.Slug()]; dup {
		panic("tax: duplicate condition type " + c.Slug())
	}
	r.conditions[c.Slug()] = c
}

func (r *Registry) RegisterEffect(e EffectApplier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.effects[e.Slug()]; dup {
		panic("tax: duplicate effect type " + e.Slug())
	}
	r.effects[e.Slug()] = e
}

func (r *Registry) LookupCondition(slug string) (ConditionEvaluator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conditions[slug]
	return c, ok
}

func (r *Registry) LookupEffect(slug string) (EffectApplier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.effects[slug]
	return e, ok
}

type TypeDescriptor struct {
	Slug         string
	Label        string
	Description  string
	ParamsSchema json.RawMessage
}

func (r *Registry) AllConditions() []TypeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TypeDescriptor, 0, len(r.conditions))
	for _, c := range r.conditions {
		out = append(out, TypeDescriptor{c.Slug(), c.Label(), c.Description(), c.ParamsSchema()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func (r *Registry) AllEffects() []TypeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]TypeDescriptor, 0, len(r.effects))
	for _, e := range r.effects {
		out = append(out, TypeDescriptor{e.Slug(), e.Label(), e.Description(), e.ParamsSchema()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

// ValidateCondition and ValidateEffect check a type exists and its params parse - used by the service on the write path
func (r *Registry) ValidateCondition(typ string, params json.RawMessage) error {
	c, ok := r.LookupCondition(typ)
	if !ok {
		return fmt.Errorf("unknown tax condition type %q", typ)
	}
	return c.ValidateParams(params)
}

func (r *Registry) ValidateEffect(typ string, params json.RawMessage) error {
	e, ok := r.LookupEffect(typ)
	if !ok {
		return fmt.Errorf("unknown tax effect type %q", typ)
	}
	return e.ValidateParams(params)
}

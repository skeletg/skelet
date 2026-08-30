package promotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// The runtime contract for a condition type, one implementation per slug - built-ins live in conditions/*.go and register in init(), while plugins go through the SDK
// ValidateParams runs at the admin write path so a malformed rule never reaches Evaluate, and ParamsSchema is Draft-7 JSON Schema for the dashboard's form engine
type ConditionEvaluator interface {
	Slug() string
	Label() string
	Description() string
	ParamsSchema() json.RawMessage
	ValidateParams(params json.RawMessage) error
	Evaluate(ctx context.Context, env Environment, params json.RawMessage) (bool, error)
}

// The runtime contract for an action type - Apply receives the running Ledger so it can add discounts and mutate cart state, and the AppliedRuleContext lets it stamp the applied-rule id onto the audit trail
type ActionApplier interface {
	Slug() string
	Label() string
	Description() string
	ParamsSchema() json.RawMessage
	ValidateParams(params json.RawMessage) error
	Apply(ctx context.Context, env Environment, params json.RawMessage, ledger Ledger, rule AppliedRuleContext) error
}

// Invoked inside the order-placement tx for any rule whose action type slug matches - loyalty and gift-card plugins use it to debit their own ledger in the same transaction as the order write - Built-in actions don't need hooks
type RedemptionHook interface {
	Slug() string
	Redeem(ctx context.Context, tx *sql.Tx, env Environment, ledger Ledger, applied AppliedRuleContext) error
}

// Passed to actions and redemption hooks so they know which rule and code triggered them
type AppliedRuleContext struct {
	RuleID   uuid.UUID
	CodeID   *uuid.UUID
	Priority int32
}

// Indexes condition and action implementations by slug - built-ins register at package init() through the global DefaultRegistry, and plugins register through the SDK adapter
type Registry struct {
	mu         sync.RWMutex
	conditions map[string]ConditionEvaluator
	actions    map[string]ActionApplier
	hooks      map[string]RedemptionHook
}

func NewRegistry() *Registry {
	return &Registry{
		conditions: make(map[string]ConditionEvaluator),
		actions:    make(map[string]ActionApplier),
		hooks:      make(map[string]RedemptionHook),
	}
}

// The package-level singleton both built-ins and the SDK adapter write to, and the engine reads from
var DefaultRegistry = NewRegistry()

type registryCtxKey struct{}

// Carries the evaluating engine's registry down to nested lookups - the composite conditions' children and the matching actions' per-line conditions
// Engine.Evaluate sets it, so an engine built with a non-default registry keeps its nested types instead of falling back to the global singleton
func WithRegistry(ctx context.Context, r *Registry) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, registryCtxKey{}, r)
}

// The registry in play for this evaluation, falling back to the global default - so a nested lookup outside an Evaluate call (an admin validate path, a plugin calling in directly) still resolves
func RegistryFrom(ctx context.Context) *Registry {
	if r, ok := ctx.Value(registryCtxKey{}).(*Registry); ok && r != nil {
		return r
	}
	return DefaultRegistry
}

func (r *Registry) RegisterCondition(c ConditionEvaluator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c == nil {
		panic("promotion: nil condition registered")
	}
	slug := c.Slug()
	if slug == "" {
		panic("promotion: condition with empty slug")
	}
	if _, exists := r.conditions[slug]; exists {
		panic(fmt.Sprintf("promotion: condition slug %q registered twice", slug))
	}
	r.conditions[slug] = c
}

func (r *Registry) RegisterAction(a ActionApplier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a == nil {
		panic("promotion: nil action registered")
	}
	slug := a.Slug()
	if slug == "" {
		panic("promotion: action with empty slug")
	}
	if _, exists := r.actions[slug]; exists {
		panic(fmt.Sprintf("promotion: action slug %q registered twice", slug))
	}
	r.actions[slug] = a
}

func (r *Registry) RegisterHook(h RedemptionHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h == nil {
		panic("promotion: nil hook registered")
	}
	slug := h.Slug()
	if _, exists := r.hooks[slug]; exists {
		panic(fmt.Sprintf("promotion: hook slug %q registered twice", slug))
	}
	r.hooks[slug] = h
}

// The implementation for a slug, or nil with ok=false when unknown - the engine treats that as a silent skip, logged in dev rather than a panic, so a plugin uninstalled mid-flight doesn't kill rules referencing it
func (r *Registry) LookupCondition(slug string) (ConditionEvaluator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conditions[slug]
	return c, ok
}

func (r *Registry) LookupAction(slug string) (ActionApplier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.actions[slug]
	return a, ok
}

func (r *Registry) LookupHook(slug string) (RedemptionHook, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.hooks[slug]
	return h, ok
}

// The catalog for admin GraphQL introspection, sorted by slug for stable output
func (r *Registry) AllConditions() []ConditionEvaluator {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConditionEvaluator, 0, len(r.conditions))
	for _, c := range r.conditions {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug() < out[j].Slug() })
	return out
}

func (r *Registry) AllActions() []ActionApplier {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ActionApplier, 0, len(r.actions))
	for _, a := range r.actions {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug() < out[j].Slug() })
	return out
}

// Clears the registry so global state doesn't leak between test scenarios - never called from production paths
func (r *Registry) ResetForTest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conditions = make(map[string]ConditionEvaluator)
	r.actions = make(map[string]ActionApplier)
	r.hooks = make(map[string]RedemptionHook)
}

// Package-level convenience wrappers - the plugin-facing SDK and built-in init() funcs both call these

func RegisterConditionType(c ConditionEvaluator) { DefaultRegistry.RegisterCondition(c) }
func RegisterActionType(a ActionApplier)         { DefaultRegistry.RegisterAction(a) }
func RegisterRedemptionHook(h RedemptionHook)    { DefaultRegistry.RegisterHook(h) }

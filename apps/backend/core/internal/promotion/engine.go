package promotion

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/google/uuid"
)

// Stateless from the caller's view, but internally holds a per-store rules cache so high-frequency Evaluate calls don't hammer the DB
type Engine struct {
	repo     Repository
	registry *Registry
	cache    *rulesCache
	// Fans invalidations out to the other replicas - nil leaves the TTL as the only staleness bound
	bus *CacheBus
}

func NewEngine(repo Repository, registry *Registry) *Engine {
	if registry == nil {
		registry = DefaultRegistry
	}
	return &Engine{
		repo:     repo,
		registry: registry,
		cache:    newRulesCache(rulesCacheTTL),
	}
}

// Wires cross-replica invalidation and relaxes the TTL, since the broadcast now carries the fast path and the TTL is only the fallback for a missed message
// Call before serving traffic, and start RunCacheInvalidation alongside it
func (e *Engine) SetCacheBus(b *CacheBus) {
	e.bus = b
	if b != nil {
		e.cache.ttl = rulesCacheTTLBroadcast
	}
}

// Called from Service CRUD and Reorder, so the next Evaluate reads fresh state from the DB - locally at once, and on every other replica as soon as the broadcast lands
func (e *Engine) InvalidateCache() {
	e.cache.EvictAll()
	e.bus.publish(context.Background())
}

// Exposes the lookup tables for admin GraphQL introspection and plugin host-adapter wiring
func (e *Engine) Registry() *Registry { return e.registry }

func (e *Engine) Repo() Repository { return e.repo }

// Callers split the ledger out for order totals and the applied list for audit rows
// Trace is only populated when the caller asks via WithTrace(ctx) - left nil in the hot path so per-cart Evaluate stays allocation-light
type EvalResult struct {
	Ledger  Ledger
	Applied []AppliedRule
	Trace   []TraceEntry
}

// One row of the "why did or didn't this rule fire" report, returned to operators through the simulatePromotion admin query
type TraceEntry struct {
	RuleID         uuid.UUID
	RuleSlug       string
	Priority       int32
	Matched        bool
	SkipReason     string // empty when Matched=true
	DiscountCents  int64  // contribution to the running subtotal discount
	StopAfterApply StopMode
}

type traceCtxKey struct{}

// Signals Evaluate to populate EvalResult.Trace - the admin simulate path sets it, the cart and order placement paths don't
func WithTrace(ctx context.Context) context.Context {
	return context.WithValue(ctx, traceCtxKey{}, true)
}

func tracing(ctx context.Context) bool {
	v, _ := ctx.Value(traceCtxKey{}).(bool)
	return v
}

// Loads active rules, filters by code requirement, sorts by priority, and applies each matching rule's actions in order while respecting stacking flags
func (e *Engine) Evaluate(ctx context.Context, env Environment) (EvalResult, error) {
	// Nested lookups (composite children, the matching actions' per-line conditions) resolve through the context, so they see THIS engine's registry rather than the global default
	ctx = WithRegistry(ctx, e.registry)
	rules, err := e.loadCandidates(ctx, env)
	if err != nil {
		return EvalResult{}, err
	}
	// Stable sort on (priority asc, id asc) so ties are deterministic across reloads, which matters for replay testing
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID.String() < rules[j].ID.String()
	})

	lconcrete := newLedger()
	var ledger Ledger = lconcrete
	var applied []AppliedRule
	// Deferred state - a rule with StopSameActionType, StopAll or an exclusivity group records intent during its tier, but the intent only bites at the next priority tier That way two peers at priority=10 both fire and their directives apply from priority>=11, the expected "same priority = peers, lower priority = blocked" model Claiming an exclusivity group the instant a rule fires contradicts that model: two peers in one group would have the tie broken by rule uuid instead of both firing
	skipActionTypes := map[string]bool{}
	pendingSkipActionTypes := map[string]bool{}
	seenGroups := map[string]bool{}
	pendingSeenGroups := map[string]bool{}
	haltAfterTier := false

	codeIndex, codeRejected := buildCodeIndex(ctx, e.repo, env)
	traceOn := tracing(ctx)
	var trace []TraceEntry
	if traceOn {
		trace = make([]TraceEntry, 0, len(rules))
	}
	addTrace := func(rule Rule, matched bool, reason string, delta int64) {
		if !traceOn {
			return
		}
		trace = append(trace, TraceEntry{
			RuleID:         rule.ID,
			RuleSlug:       rule.Slug,
			Priority:       rule.Priority,
			Matched:        matched,
			SkipReason:     reason,
			DiscountCents:  delta,
			StopAfterApply: rule.StopAfterApply,
		})
	}

	// Re-applies any discount cap, clamps to the cart subtotal, allocates per-line discounts and packages the result - pulled out because there are now two return points, a tier-boundary halt and the natural end of list
	finalize := func() (EvalResult, error) {
		lconcrete.Finalize(env.Lines(), env.SubtotalCents())
		// The clamp can shrink the aggregate, so rescale each rule's recorded contribution by the same ratio - otherwise the per-rule audit rows sum to more than the order's discount_amount
		rescaleAppliedContributions(applied, lconcrete.SubtotalDiscountCents())
		return EvalResult{Ledger: ledger, Applied: applied, Trace: trace}, nil
	}

	currentTier := int32(-1)
	tierInitialized := false
	for _, rule := range rules {
		// Tier boundary - rules are pre-sorted by priority asc, so any change marks a new tier - Apply the stop directives accumulated in the previous one
		if !tierInitialized || rule.Priority != currentTier {
			if tierInitialized {
				if haltAfterTier {
					return finalize()
				}
				// Promote tier-local same-action-type blocks and exclusivity claims into the active sets so later tiers honour them
				for t := range pendingSkipActionTypes {
					skipActionTypes[t] = true
				}
				pendingSkipActionTypes = map[string]bool{}
				for g := range pendingSeenGroups {
					seenGroups[g] = true
				}
				pendingSeenGroups = map[string]bool{}
			}
			currentTier = rule.Priority
			tierInitialized = true
		}

		if rule.ExclusivityGroup != "" && seenGroups[rule.ExclusivityGroup] {
			addTrace(rule, false, "exclusivity_group_taken:"+rule.ExclusivityGroup, 0)
			continue
		}
		if rule.RequiresCode {
			// Skip rules whose code isn't in the cart's applied set
			if _, ok := codeIndex[rule.ID]; !ok {
				reason := "code_not_applied"
				if r, rejected := codeRejected[rule.ID]; rejected {
					reason = r
				}
				addTrace(rule, false, reason, 0)
				continue
			}
		}
		if anyActionSkipped(rule.Actions, skipActionTypes) {
			addTrace(rule, false, "action_type_shadowed", 0)
			continue
		}

		match, skipReason, err := e.evalAllConditions(ctx, env, rule.Conditions)
		if err != nil {
			return EvalResult{}, fmt.Errorf("eval conditions for rule %s: %w", rule.Slug, err)
		}
		if !match {
			addTrace(rule, false, skipReason, 0)
			continue
		}

		ruleCtx := AppliedRuleContext{
			RuleID:   rule.ID,
			CodeID:   codeIndex[rule.ID],
			Priority: rule.Priority,
		}
		before := ledger.SubtotalDiscountCents()
		for _, action := range rule.Actions {
			if err := e.applyAction(ctx, env, action, ledger, ruleCtx); err != nil {
				return EvalResult{}, fmt.Errorf("apply action %s on rule %s: %w", action.Type, rule.Slug, err)
			}
		}
		delta := ledger.SubtotalDiscountCents() - before

		applied = append(applied, AppliedRule{
			RuleID:        rule.ID,
			CodeID:        ruleCtx.CodeID,
			Priority:      rule.Priority,
			DiscountCents: delta,
		})
		addTrace(rule, true, "", delta)

		if rule.ExclusivityGroup != "" {
			// Deferred to the tier boundary so same-priority peers in one group both fire
			pendingSeenGroups[rule.ExclusivityGroup] = true
		}
		switch rule.StopAfterApply {
		case StopAll:
			// Defer the halt to the end of the tier so same-priority peers still get their turn
			haltAfterTier = true
		case StopSameActionType:
			// Defer the action-type block to later tiers so peers at this priority sharing an action type still fire
			for _, a := range rule.Actions {
				pendingSkipActionTypes[a.Type] = true
			}
		}
	}

	return finalize()
}

// Pulls active rules for the store and hydrates conditions and actions in two batched queries
// The base list comes from the engine's TTL cache; conditions and actions stay DB-fetched because they're two cheap batched queries and rule mutations invalidate the rules cache anyway
func (e *Engine) loadCandidates(ctx context.Context, env Environment) ([]Rule, error) {
	rules, err := loadCachedActiveRules(ctx, e.cache, e.repo, env.ChannelID())
	if err != nil {
		return nil, fmt.Errorf("load active rules: %w", err)
	}
	if len(rules) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	conds, err := e.repo.ListConditionsByRules(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load conditions: %w", err)
	}
	actions, err := e.repo.ListActionsByRules(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load actions: %w", err)
	}
	byRule := make(map[uuid.UUID]*Rule, len(rules))
	for i := range rules {
		byRule[rules[i].ID] = &rules[i]
	}
	for _, c := range conds {
		if r, ok := byRule[c.RuleID]; ok {
			r.Conditions = append(r.Conditions, c)
		}
	}
	for _, a := range actions {
		if r, ok := byRule[a.RuleID]; ok {
			r.Actions = append(r.Actions, a)
		}
	}
	return rules, nil
}

// Maps rule_id to code_id for the codes the cart has applied, which the engine consults for code-gated rules An unknown code (typo, deleted) silently misses - the applyCoupon mutation's earlier validation is what surfaces the failure This is the only place a code is consumed at eval time, so the code-level window and email restriction are enforced here rather than only at apply time - reconcileAppliedCodes turns the miss into shopper-facing copy The second return maps rule_id to the gate a resolved-but-rejected code tripped, so the trace says "code_expired" instead of the misleading "code_not_applied"
func buildCodeIndex(ctx context.Context, repo Repository, env Environment) (map[uuid.UUID]*uuid.UUID, map[uuid.UUID]string) {
	out := make(map[uuid.UUID]*uuid.UUID)
	rejected := map[uuid.UUID]string{}
	now := nowFunc()
	email := env.CustomerEmail()
	for _, code := range env.AppliedCodes() {
		row, err := repo.GetCodeByCode(ctx, code)
		if err != nil {
			continue
		}
		if !codeWindowOpen(row, now) {
			rejected[row.RuleID] = "code_outside_validity_window"
			continue
		}
		if !codeEmailAllows(row, email) {
			rejected[row.RuleID] = "code_restricted_to_another_email"
			continue
		}
		// The cap has to bite HERE and not only at redemption - a code that filled up between apply and checkout would otherwise still discount the cart, and then IncrementCodeUsage's guard would refuse and fail the whole placement with nothing the shopper could do about it
		if row.MaxUses != nil && row.UsedCount >= *row.MaxUses {
			rejected[row.RuleID] = "code_usage_limit_reached"
			continue
		}
		id := row.ID
		out[row.RuleID] = &id
	}
	return out, rejected
}

// Scales each rule's recorded contribution so the per-rule sum matches the (possibly clamped) aggregate
// The contributions are audit rows and analytics input, so they have to add up to what the order actually discounted
func rescaleAppliedContributions(applied []AppliedRule, finalTotal int64) {
	var recorded int64
	for _, a := range applied {
		recorded += a.DiscountCents
	}
	if recorded <= 0 || recorded == finalTotal {
		return
	}
	var allocated int64
	last := -1
	for i := range applied {
		if applied[i].DiscountCents <= 0 {
			// A rule that only set free shipping or queued a gift contributed nothing to the subtotal and mustn't absorb the rounding remainder
			continue
		}
		applied[i].DiscountCents = (applied[i].DiscountCents * finalTotal) / recorded
		allocated += applied[i].DiscountCents
		last = i
	}
	if last >= 0 {
		applied[last].DiscountCents += finalTotal - allocated
	}
}

// True when every condition row matches - AND semantics, so use core.any_of or core.not for boolean composition The second return is the trace's skip reason, so an operator can tell "the cart didn't qualify" from "the plugin that owns this gate isn't loaded" An unknown condition slug FAILS THE RULE, matching core.all_of's nested children and the matching actions' per-line conditions Contributing true to the AND would mean uninstalling a plugin silently promotes its rules to everyone: a "gold tier only, 50% off" rule became a storewide 50% off
func (e *Engine) evalAllConditions(ctx context.Context, env Environment, conds []Condition) (bool, string, error) {
	for _, c := range conds {
		impl, ok := e.registry.LookupCondition(c.Type)
		if !ok {
			slog.WarnContext(ctx, "promotion: unknown condition type, rule cannot match",
				"slug", c.Type, "rule_id", c.RuleID)
			return false, "unknown_condition_type:" + c.Type, nil
		}
		match, err := impl.Evaluate(ctx, env, c.Params)
		if err != nil {
			return false, "", fmt.Errorf("evaluate %s: %w", c.Type, err)
		}
		if !match {
			return false, "conditions_unmet", nil
		}
	}
	return true, "", nil
}

// Unknown slugs are silently skipped, same rationale as conditions
func (e *Engine) applyAction(ctx context.Context, env Environment, a Action, ledger Ledger, ruleCtx AppliedRuleContext) error {
	impl, ok := e.registry.LookupAction(a.Type)
	if !ok {
		slog.WarnContext(ctx, "promotion: unknown action type, skipping",
			"slug", a.Type, "rule_id", a.RuleID)
		return nil
	}
	return impl.Apply(ctx, env, a.Params, ledger, ruleCtx)
}

// True when any of the rule's actions shares a type with a previously stopped one, for the StopSameActionType mode
func anyActionSkipped(actions []Action, skipped map[string]bool) bool {
	for _, a := range actions {
		if skipped[a.Type] {
			return true
		}
	}
	return false
}

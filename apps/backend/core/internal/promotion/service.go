package promotion

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Order placement talks to Engine directly; admin resolvers come here for CRUD, storefront resolvers for apply/remove and to Engine for eval
type Service struct {
	repo   Repository
	engine *Engine
	// Optional enrichment capabilities EvaluateCart applies so no caller has to remember to - nil means the corresponding conditions just don't match
	productAttrs ProductAttrsLookup
	segments     CustomerSegmentsLookup
}

func NewService(repo Repository) *Service {
	s := &Service{repo: repo}
	s.engine = NewEngine(repo, DefaultRegistry)
	// The repo already answers this - defaulting it here means no wiring site can forget and quietly leave the product-scoped conditions dead
	s.productAttrs = repo
	return s
}

// The catalog facts the product-scoped conditions gate on, resolved per cart line
// Cart lines carry sku, price and quantity only, so core.product_kind, core.product_in_category and core.product_has_attribute need this to see anything - without it they silently never match, while the admin simulator (which is handed kind and category ids directly) makes them look like they work
type ProductAttrsLookup interface {
	PromotionLineAttrs(ctx context.Context, skus []string) (map[string]LineProductAttrs, error)
}

// One SKU's catalog facts - AttributeKVs merges selectable values (comma-joined when multi-valued) with free-text scalars, both keyed by attribute slug
type LineProductAttrs struct {
	ProductID    uuid.UUID
	Kind         string
	CategoryIDs  []uuid.UUID
	AttributeKVs map[string]string
}

// Resolves a customer's active segment slugs for core.customer_in_segment
type CustomerSegmentsLookup interface {
	SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error)
}

// Optional, nil leaves the product-scoped conditions unable to match
func (s *Service) SetProductAttrs(l ProductAttrsLookup) { s.productAttrs = l }

// Optional, nil leaves core.customer_in_segment unable to match
func (s *Service) SetCustomerSegments(l CustomerSegmentsLookup) { s.segments = l }

// Direct access for adjacent services - admin resolvers reach into repo for CRUD, order placement reaches into engine for evaluation
func (s *Service) Repo() Repository { return s.repo }

func (s *Service) RedemptionsForOrder(ctx context.Context, orderID uuid.UUID) ([]OrderRedemption, error) {
	return s.repo.ListRedemptionsByOrder(ctx, orderID)
}
func (s *Service) Engine() *Engine { return s.engine }

// ---- Rule CRUD (admin) ----

// Conditions and actions are passed inline and written transactionally, so a partial rule never lands
type CreateRuleInput struct {
	Rule       Rule        // sans ID + UsedCount
	Conditions []Condition // sans ID + RuleID
	Actions    []Action    // sans ID + RuleID
}

func (s *Service) CreateRule(ctx context.Context, in CreateRuleInput) (Rule, error) {
	if err := s.validateRuleInputs(in.Conditions, in.Actions); err != nil {
		return Rule{}, err
	}
	normalizeRule(&in.Rule)
	created, err := s.repo.CreateRule(ctx, in.Rule)
	if err != nil {
		return Rule{}, err
	}
	if err := s.repo.ReplaceRuleConditions(ctx, created.ID, in.Conditions); err != nil {
		return Rule{}, err
	}
	if err := s.repo.ReplaceRuleActions(ctx, created.ID, in.Actions); err != nil {
		return Rule{}, err
	}
	s.engine.InvalidateCache()
	return s.GetRuleHydrated(ctx, created.ID)
}

// Fills zero values with domain defaults so the DB CHECK constraints accept the row - an unset StopAfterApply becomes StopNone, matching the migration default and the GraphQL enum
func normalizeRule(r *Rule) {
	if r.StopAfterApply == "" {
		r.StopAfterApply = StopNone
	}
}

// Conditions and Actions are pointer-to-slice so callers can distinguish "don't touch" (nil) from "replace with this set, even if empty"
// Drag-and-drop reorder passes nil for both; the rule editor passes non-nil even when the operator deleted every row
type UpdateRuleInput struct {
	Rule       Rule
	Conditions *[]Condition
	Actions    *[]Action
}

func (s *Service) UpdateRule(ctx context.Context, in UpdateRuleInput) (Rule, error) {
	normalizeRule(&in.Rule)
	var conds []Condition
	if in.Conditions != nil {
		conds = *in.Conditions
	}
	var acts []Action
	if in.Actions != nil {
		acts = *in.Actions
	}
	if err := s.validateRuleInputs(conds, acts); err != nil {
		return Rule{}, err
	}
	if _, err := s.repo.UpdateRule(ctx, in.Rule); err != nil {
		return Rule{}, err
	}
	if in.Conditions != nil {
		if err := s.repo.ReplaceRuleConditions(ctx, in.Rule.ID, conds); err != nil {
			return Rule{}, err
		}
	}
	if in.Actions != nil {
		if err := s.repo.ReplaceRuleActions(ctx, in.Rule.ID, acts); err != nil {
			return Rule{}, err
		}
	}
	s.engine.InvalidateCache()
	return s.GetRuleHydrated(ctx, in.Rule.ID)
}

func (s *Service) DeleteRule(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.DeleteRule(ctx, id); err != nil {
		return err
	}
	s.engine.InvalidateCache()
	return nil
}

// Priorities are spaced 10, 20, 30 so a later single-row insert can slot between two rules (admin types priority=15) without a full renumber
func (s *Service) Reorder(ctx context.Context, orderedIDs []uuid.UUID) error {
	if err := s.repo.ReorderRules(ctx, orderedIDs); err != nil {
		return err
	}
	s.engine.InvalidateCache()
	return nil
}

func (s *Service) GetRuleHydrated(ctx context.Context, id uuid.UUID) (Rule, error) {
	rule, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	conds, err := s.repo.ListConditionsByRule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	rule.Conditions = conds
	actions, err := s.repo.ListActionsByRule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	rule.Actions = actions
	return rule, nil
}

// Doesn't hydrate conditions or actions - list rows only show a summary
func (s *Service) ListRules(ctx context.Context, limit, offset int32) ([]Rule, int32, error) {
	return s.repo.ListRules(ctx, limit, offset)
}

// Asks each registered implementation to validate its own params - built-ins and plugin types both pass through here, so a malformed rule never reaches Evaluate
func (s *Service) validateRuleInputs(conds []Condition, actions []Action) error {
	for _, c := range conds {
		impl, ok := s.engine.registry.LookupCondition(c.Type)
		if !ok {
			return fmt.Errorf("unknown condition type %q", c.Type)
		}
		if err := impl.ValidateParams(c.Params); err != nil {
			return fmt.Errorf("condition %s: %w", c.Type, err)
		}
	}
	for _, a := range actions {
		impl, ok := s.engine.registry.LookupAction(a.Type)
		if !ok {
			return fmt.Errorf("unknown action type %q", a.Type)
		}
		if err := impl.ValidateParams(a.Params); err != nil {
			return fmt.Errorf("action %s: %w", a.Type, err)
		}
	}
	return nil
}

// ---- Code management ----

func (s *Service) GenerateCodes(ctx context.Context, ruleID uuid.UUID, pattern string, count int, prefix, suffix string) ([]string, error) {
	if count <= 0 || count > 100000 {
		return nil, fmt.Errorf("count must be 1..100000")
	}
	if _, err := s.repo.GetRule(ctx, ruleID); err != nil {
		return nil, err
	}
	gen := NewCodeGenerator(pattern, prefix, suffix)
	if err := gen.Validate(); err != nil {
		return nil, err
	}
	// The bulk insert's ON CONFLICT DO NOTHING only commits unique codes, so generate a surplus and retry the shortfall
	want := count
	all := make([]string, 0, want)
	for attempt := 0; attempt < 5 && want > 0; attempt++ {
		batch := gen.Batch(want)
		inserted, err := s.repo.BulkInsertCodes(ctx, ruleID, batch)
		if err != nil {
			return nil, err
		}
		// We don't know WHICH codes inserted - collisions are vanishingly rare for any reasonable pattern
		all = append(all, batch[:inserted]...)
		want -= inserted
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("generate codes: 0 inserted after retries; check pattern uniqueness")
	}
	return all, nil
}

// ---- Cart code apply / remove (storefront) ----

// The engine is re-evaluated after every apply, so the cart's totals reflect every stackable rule that fires
type ApplyCodeResult struct {
	Code           Code
	Rule           Rule
	AlreadyApplied bool
}

// Returns a typed error so the storefront can translate it into a UX message - another channel's code reads as unknown
func (s *Service) ApplyCode(ctx context.Context, cartID, channelID uuid.UUID, codeText string, customerID *uuid.UUID) (ApplyCodeResult, error) {
	// Codes are case-insensitive to the customer but stored uppercase, so normalise at every entry point and "welcome10" still matches
	codeText = strings.ToUpper(strings.TrimSpace(codeText))
	if codeText == "" {
		return ApplyCodeResult{}, ErrCodeNotFound
	}
	code, err := s.repo.GetCodeByCode(ctx, codeText)
	if err != nil {
		return ApplyCodeResult{}, err
	}
	rule, err := s.repo.GetRule(ctx, code.RuleID)
	if err != nil {
		return ApplyCodeResult{}, err
	}
	if rule.ChannelID != nil && *rule.ChannelID != channelID {
		return ApplyCodeResult{}, ErrCodeNotFound
	}
	if !rule.IsActive {
		return ApplyCodeResult{}, ErrRuleInactive
	}
	if !rule.RequiresCode {
		// Edge case - the code was migrated onto an auto-apply rule
		return ApplyCodeResult{}, ErrCodeOnAutoApplyRule
	}
	// Code-level validity window overrides rule-level when set
	if err := checkValidityWindow(rule, code); err != nil {
		return ApplyCodeResult{}, err
	}
	if rule.MaxUses != nil && rule.UsedCount >= *rule.MaxUses {
		return ApplyCodeResult{}, ErrRuleMaxUses
	}
	if code.MaxUses != nil && code.UsedCount >= *code.MaxUses {
		return ApplyCodeResult{}, ErrCodeMaxUses
	}
	// A code pinned to one recipient only works for that address - the engine re-checks at eval time, this is just the early, explainable rejection
	if code.RestrictedToEmail != "" {
		email, err := s.repo.GetCartEmail(ctx, cartID)
		if err != nil {
			return ApplyCodeResult{}, err
		}
		if !codeEmailAllows(code, email) {
			return ApplyCodeResult{}, ErrCodeEmailRestricted
		}
	}
	if customerID != nil {
		if rule.PerCustomerLimit != nil {
			used, err := s.repo.CountRuleUsageByCustomer(ctx, rule.ID, *customerID)
			if err != nil {
				return ApplyCodeResult{}, err
			}
			if used >= *rule.PerCustomerLimit {
				return ApplyCodeResult{}, ErrPerCustomerLimitRule
			}
		}
		if code.PerCustomerLimit != nil {
			used, err := s.repo.CountCodeUsageByCustomer(ctx, code.ID, *customerID)
			if err != nil {
				return ApplyCodeResult{}, err
			}
			if used >= *code.PerCustomerLimit {
				return ApplyCodeResult{}, ErrPerCustomerLimitCode
			}
		}
	}
	// ApplyCode only handles gates that don't depend on cart contents - per-rule conditions including min subtotal are evaluated inside the engine, which the cart UI calls right after this returns
	if err := s.repo.ApplyCodeToCart(ctx, cartID, rule.ID, &code.ID); err != nil {
		return ApplyCodeResult{}, err
	}
	return ApplyCodeResult{Code: code, Rule: rule}, nil
}

// No-op for codes that aren't applied
func (s *Service) RemoveCode(ctx context.Context, cartID uuid.UUID, codeText string) error {
	codeText = strings.ToUpper(strings.TrimSpace(codeText))
	if codeText == "" {
		return nil
	}
	return s.repo.RemoveCodeFromCartByCode(ctx, cartID, codeText)
}

// Invalidation reasons are populated where applicable
func (s *Service) ListCartAppliedCodes(ctx context.Context, cartID uuid.UUID) ([]AppliedCodeRecord, error) {
	return s.repo.ListCartAppliedCodes(ctx, cartID)
}

// ---- Redemption (order placement, called inside the tx) ----

// Runs inside the order placement transaction - increments the rule's and code's used_count, writes a redemption audit row, and fires the action's RedemptionHook so loyalty plugins debit their own ledger in the same tx
func (s *Service) Redeem(ctx context.Context, tx *sql.Tx, env Environment, applied AppliedRule, customerID *uuid.UUID, orderID uuid.UUID, actionsForHooks []Action) error {
	repo := s.repo.WithTx(tx)
	rule, err := repo.GetRule(ctx, applied.RuleID)
	if err != nil {
		return fmt.Errorf("load rule for redemption check: %w", err)
	}
	var code Code
	if applied.CodeID != nil {
		code, err = repo.GetCode(ctx, *applied.CodeID)
		if err != nil {
			return fmt.Errorf("load code for redemption check: %w", err)
		}
		// The cart_applied_codes row was written when the shopper typed the code - re-check the window where it's actually consumed, so a code that expired between apply and checkout can't still discount
		// Auto-apply rules are deliberately exempt: they were live when the engine ran, and lapsing mid-checkout shouldn't fail the whole order
		if err := checkValidityWindow(rule, code); err != nil {
			return err
		}
	}
	// Cap-guarded increments that refuse once used_count hits max_uses The UPDATEs row-lock the rule and code, so concurrent redemptions serialize here and the per-customer re-checks below are race-free - the second tx blocks, then counts the first's committed row Eval already filters used-up rules and codes, so reaching the cap here means the last slot went to a concurrent checkout within the cache window - evict so the shopper's retry prices without the rule instead of failing forever on a discount it can never redeem
	if err := repo.IncrementRuleUsage(ctx, applied.RuleID); err != nil {
		if errors.Is(err, ErrRuleMaxUses) {
			s.engine.InvalidateCache()
		}
		return err
	}
	if applied.CodeID != nil {
		if err := repo.IncrementCodeUsage(ctx, *applied.CodeID); err != nil {
			if errors.Is(err, ErrCodeMaxUses) {
				s.engine.InvalidateCache()
			}
			return err
		}
	}
	// ApplyCode checked per-customer limits already, but that's read-then-act - re-check inside the redemption tx, where the counts land after the locking UPDATEs above
	if customerID != nil {
		if rule.PerCustomerLimit != nil {
			used, err := repo.CountRuleUsageByCustomer(ctx, applied.RuleID, *customerID)
			if err != nil {
				return err
			}
			if used >= *rule.PerCustomerLimit {
				return ErrPerCustomerLimitRule
			}
		}
		if applied.CodeID != nil && code.PerCustomerLimit != nil {
			used, err := repo.CountCodeUsageByCustomer(ctx, *applied.CodeID, *customerID)
			if err != nil {
				return err
			}
			if used >= *code.PerCustomerLimit {
				return ErrPerCustomerLimitCode
			}
		}
	}
	if err := repo.RecordRedemption(ctx, applied.RuleID, applied.CodeID, customerID, orderID, applied.DiscountCents); err != nil {
		return err
	}
	// Loyalty and gift-card plugins debit their own ledger here, so a failed order rolls the debit back with it
	for _, action := range actionsForHooks {
		hook, ok := s.engine.registry.LookupHook(action.Type)
		if !ok {
			continue
		}
		if err := hook.Redeem(ctx, tx, env, newReadOnlyLedger(applied.DiscountCents), AppliedRuleContext{
			RuleID:   applied.RuleID,
			CodeID:   applied.CodeID,
			Priority: applied.Priority,
		}); err != nil {
			return fmt.Errorf("hook %s: %w", action.Type, err)
		}
	}
	return nil
}

// Called by PlaceOrder after the order tx succeeds
func (s *Service) ClearCartCodes(ctx context.Context, tx *sql.Tx, cartID uuid.UUID) error {
	return s.repo.WithTx(tx).ClearCartAppliedCodes(ctx, cartID)
}

// ---- Validation error sentinels ----

type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string { return "promotion: " + e.Reason }

var (
	ErrRuleInactive         = &ValidationError{Reason: "rule_inactive"}
	ErrRuleMaxUses          = &ValidationError{Reason: "rule_max_uses"}
	ErrRuleNotYetValid      = &ValidationError{Reason: "rule_not_yet_valid"}
	ErrRuleExpired          = &ValidationError{Reason: "rule_expired"}
	ErrCodeInactive         = &ValidationError{Reason: "code_inactive"}
	ErrCodeMaxUses          = &ValidationError{Reason: "code_max_uses"}
	ErrCodeNotYetValid      = &ValidationError{Reason: "code_not_yet_valid"}
	ErrCodeExpired          = &ValidationError{Reason: "code_expired"}
	ErrCodeOnAutoApplyRule  = &ValidationError{Reason: "code_belongs_to_auto_rule"}
	ErrCodeEmailRestricted  = &ValidationError{Reason: "code_email_restricted"}
	ErrPerCustomerLimitRule = &ValidationError{Reason: "per_customer_limit_rule"}
	ErrPerCustomerLimitCode = &ValidationError{Reason: "per_customer_limit_code"}
)

// Register the sentinels so the GraphQL presenter classifies them as ClassValidation - without this the storefront sees "internal error" instead of "code expired"
func init() {
	apperr.RegisterSentinel(ErrCodeNotFound, "PROMOTION_CODE_NOT_FOUND", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrSlugTaken, "PROMOTION_SLUG_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrCodeTaken, "PROMOTION_CODE_TAKEN", apperr.ClassConflict)
	apperr.RegisterSentinel(ErrRuleNotFound, "PROMOTION_RULE_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrRuleInactive, "PROMOTION_RULE_INACTIVE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrRuleMaxUses, "PROMOTION_RULE_MAX_USES", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrRuleNotYetValid, "PROMOTION_RULE_NOT_YET_VALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrRuleExpired, "PROMOTION_RULE_EXPIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCodeInactive, "PROMOTION_CODE_INACTIVE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCodeMaxUses, "PROMOTION_CODE_MAX_USES", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCodeNotYetValid, "PROMOTION_CODE_NOT_YET_VALID", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCodeExpired, "PROMOTION_CODE_EXPIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCodeOnAutoApplyRule, "PROMOTION_CODE_ON_AUTO_APPLY_RULE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrCodeEmailRestricted, "PROMOTION_CODE_EMAIL_RESTRICTED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPerCustomerLimitRule, "PROMOTION_PER_CUSTOMER_LIMIT_RULE", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrPerCustomerLimitCode, "PROMOTION_PER_CUSTOMER_LIMIT_CODE", apperr.ClassValidation)
}

// The code-level window only - the rule-level one is already filtered out by ListActiveRulesForChannel, so eval-time code resolution needn't re-check it
func codeWindowOpen(c Code, now time.Time) bool {
	if c.ValidFrom != nil && now.Before(*c.ValidFrom) {
		return false
	}
	if c.ValidTo != nil && now.After(*c.ValidTo) {
		return false
	}
	return true
}

// An empty restriction means anyone - an unknown shopper email can't satisfy a restriction, so a pinned code fails closed
func codeEmailAllows(c Code, email string) bool {
	if c.RestrictedToEmail == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(c.RestrictedToEmail))
}

func checkValidityWindow(rule Rule, code Code) error {
	now := nowFunc()
	// Code-level wins, and a NULL field means no bound
	from := rule.ValidFrom
	if code.ValidFrom != nil {
		from = code.ValidFrom
	}
	to := rule.ValidTo
	if code.ValidTo != nil {
		to = code.ValidTo
	}
	if from != nil && now.Before(*from) {
		return ErrCodeNotYetValid
	}
	if to != nil && now.After(*to) {
		return ErrCodeExpired
	}
	return nil
}

// ---- i18n rule contents ----

func (s *Service) UpsertRuleContent(ctx context.Context, ruleID uuid.UUID, locale, label, terms string) (RuleContent, error) {
	if locale == "" {
		return RuleContent{}, fmt.Errorf("locale is required")
	}
	return s.repo.UpsertRuleContent(ctx, RuleContent{
		RuleID: ruleID,
		Locale: locale,
		Label:  label,
		Terms:  terms,
	})
}

func (s *Service) ListRuleContents(ctx context.Context, ruleID uuid.UUID) ([]RuleContent, error) {
	return s.repo.ListRuleContents(ctx, ruleID)
}

func (s *Service) DeleteRuleContent(ctx context.Context, ruleID uuid.UUID, locale string) error {
	return s.repo.DeleteRuleContent(ctx, ruleID, locale)
}

// Batched form for DataLoader paths, scoped to a single per-request locale - a missing key means no row, and callers fall back to the rule's public_label
func (s *Service) GetRuleContentsByIDs(ctx context.Context, ruleIDs []uuid.UUID, locale string) (map[uuid.UUID]RuleContent, error) {
	return s.repo.GetRuleContentsByIDs(ctx, ruleIDs, locale)
}

// Batched form for DataLoader paths - one Condition per row for the caller to group by RuleID, one SQL roundtrip no matter how many rules
func (s *Service) ListConditionsByRules(ctx context.Context, ruleIDs []uuid.UUID) ([]Condition, error) {
	return s.repo.ListConditionsByRules(ctx, ruleIDs)
}

// Reads the engine's in-process cache, exposed so storefront resolvers can iterate rules themselves and route per-rule lookups through DataLoaders
func (s *Service) ActiveRulesForChannel(ctx context.Context, channelID *uuid.UUID) ([]Rule, error) {
	return loadCachedActiveRules(ctx, s.engine.cache, s.repo, channelID)
}

// Falls back to the rule's own public_label when the locale has no content row - storefront cart helpers use it when projecting AppliedPromotionCode
func (s *Service) ResolvePublicLabel(ctx context.Context, ruleID uuid.UUID, locale, fallback string) string {
	if locale == "" {
		return fallback
	}
	row, err := s.repo.GetRuleContent(ctx, ruleID, locale)
	if err != nil || row.Label == "" {
		return fallback
	}
	return row.Label
}

// ---- Analytics ----

func (s *Service) AnalyticsByRule(ctx context.Context, start, end time.Time) ([]RuleAnalytics, error) {
	return s.repo.AnalyticsByRule(ctx, start, end)
}

func (s *Service) AnalyticsTotals(ctx context.Context, start, end time.Time) (AnalyticsTotals, error) {
	return s.repo.AnalyticsTotals(ctx, start, end)
}

func (s *Service) AnalyticsDaily(ctx context.Context, start, end time.Time, ruleID *uuid.UUID) ([]AnalyticsDailyPoint, error) {
	return s.repo.AnalyticsDaily(ctx, start, end, ruleID)
}

// ---- Simulate ----

// Evaluates against a synthetic environment and returns the full trace - every rule walked with match status, skip reason and discount contribution - so operators can answer "why didn't X fire?" without touching real carts
func (s *Service) Simulate(ctx context.Context, env Environment) (EvalResult, error) {
	return s.engine.Evaluate(WithTrace(ctx), env)
}

// ---- Cart evaluation ----

// Transport-agnostic input callers populate from whatever cart shape they hold The customer's group slug is pre-resolved by the caller - promotion deliberately doesn't import customer, keeping the layering one-way Everything derivable from CustomerID and the line SKUs is resolved HERE rather than by each caller, because it kept drifting: the storefront passed segments and no country while order placement passed country and no segments, so a segment-gated promo discounted the cart and then vanished at checkout, and a country-gated one did the reverse
type CartEvalInput struct {
	CartID            uuid.UUID
	SubtotalCents     int64
	Currency          string
	CountryCode       string // optional - shipping country, "" when unknown
	ChannelID         *uuid.UUID
	ChannelSlug       string
	CustomerID        *uuid.UUID
	CustomerGroupSlug string
	// The shopper's checkout email, for codes pinned to one recipient - left empty, the cart's own email is looked up instead
	CustomerEmail string
	// Leave nil to have the wired segment lookup resolve them from CustomerID - pass a non-nil slice only to override, which the admin simulator does
	CustomerSegmentSlugs []string
	Lines                []CartLineView
}

// Callers building a display reconcile the engine result with the cart_applied_codes rows; callers stamping totals read only Result.Ledger
type CartEvalResult struct {
	Result          EvalResult
	ManuallyApplied []AppliedCodeRecord
}

// Centralising the env build is what stops the storefront and order placement drifting over which fields they fill - the segment and catalog enrichment below is here for the same reason
func (s *Service) EvaluateCart(ctx context.Context, in CartEvalInput) (CartEvalResult, error) {
	s.enrichCartEvalInput(ctx, &in)
	applied, err := s.repo.ListCartAppliedCodes(ctx, in.CartID)
	if err != nil {
		return CartEvalResult{}, fmt.Errorf("load cart applied codes: %w", err)
	}
	codeTexts := make([]string, 0, len(applied))
	for _, a := range applied {
		if a.CodeText != "" && a.InvalidatedReason == "" {
			codeTexts = append(codeTexts, a.CodeText)
		}
	}
	// Only a manually-applied code can be email-restricted, so carts with none skip the lookup entirely
	email := in.CustomerEmail
	if email == "" && len(codeTexts) > 0 {
		email, err = s.repo.GetCartEmail(ctx, in.CartID)
		if err != nil {
			return CartEvalResult{}, fmt.Errorf("load cart email: %w", err)
		}
	}
	env := BuildEnvironment(EnvironmentInput{
		SubtotalCents:        in.SubtotalCents,
		Currency:             in.Currency,
		CountryCode:          in.CountryCode,
		ChannelID:            in.ChannelID,
		ChannelSlug:          in.ChannelSlug,
		CustomerID:           in.CustomerID,
		CustomerGroupSlug:    in.CustomerGroupSlug,
		CustomerEmail:        email,
		CustomerSegmentSlugs: in.CustomerSegmentSlugs,
		Lines:                in.Lines,
		AppliedCodes:         codeTexts,
	})
	res, err := s.engine.Evaluate(ctx, env)
	if err != nil {
		return CartEvalResult{}, fmt.Errorf("promotion engine: %w", err)
	}
	s.reconcileAppliedCodes(ctx, in.CartID, email, applied, res)
	return CartEvalResult{Result: res, ManuallyApplied: applied}, nil
}

// Fills in whatever the caller left for us: the customer's segment slugs and each line's catalog facts. Best-effort throughout - a lookup
// failure leaves the field empty, so the affected conditions simply don't match rather than failing the whole cart price
func (s *Service) enrichCartEvalInput(ctx context.Context, in *CartEvalInput) {
	if in.CustomerSegmentSlugs == nil && in.CustomerID != nil && s.segments != nil {
		if slugs, err := s.segments.SlugsForCustomer(ctx, *in.CustomerID); err == nil {
			in.CustomerSegmentSlugs = slugs
		} else {
			slog.WarnContext(ctx, "promotion: resolve customer segments", "customer", *in.CustomerID, "err", err)
		}
	}
	if s.productAttrs == nil || len(in.Lines) == 0 {
		return
	}
	skus := make([]string, 0, len(in.Lines))
	for _, l := range in.Lines {
		if l.ProductSKU != "" {
			skus = append(skus, l.ProductSKU)
		}
	}
	if len(skus) == 0 {
		return
	}
	attrs, err := s.productAttrs.PromotionLineAttrs(ctx, skus)
	if err != nil {
		slog.WarnContext(ctx, "promotion: resolve line product attrs", "err", err)
		return
	}
	for i := range in.Lines {
		a, ok := attrs[in.Lines[i].ProductSKU]
		if !ok {
			continue
		}
		// A caller that already knows a field wins - the admin simulator hands in synthetic kinds and categories
		if in.Lines[i].ProductID == uuid.Nil {
			in.Lines[i].ProductID = a.ProductID
		}
		if in.Lines[i].ProductKind == "" {
			in.Lines[i].ProductKind = a.Kind
		}
		if len(in.Lines[i].CategoryIDs) == 0 {
			in.Lines[i].CategoryIDs = a.CategoryIDs
		}
		if len(in.Lines[i].AttributeKVs) == 0 {
			in.Lines[i].AttributeKVs = a.AttributeKVs
		}
	}
}

// reconcileAppliedCodes stamps a reason onto applied codes whose rule no longer fires, so the shopper sees WHY the discount vanished instead of a silent zero Terminal states (expired, disabled, used up) persist and stop refiring; a transient conditions miss stays in-memory only so the code refires when the cart qualifies again
func (s *Service) reconcileAppliedCodes(ctx context.Context, cartID uuid.UUID, email string, applied []AppliedCodeRecord, res EvalResult) {
	fired := make(map[uuid.UUID]bool, len(res.Applied))
	for _, ar := range res.Applied {
		fired[ar.RuleID] = true
	}
	for i := range applied {
		a := &applied[i]
		if a.CodeText == "" || a.InvalidatedReason != "" || fired[a.RuleID] {
			continue
		}
		reason, terminal := s.appliedFailureReason(ctx, *a, email)
		a.InvalidatedReason = reason
		if terminal {
			// Best-effort - a failed write just re-derives the reason on the next eval
			if err := s.repo.MarkCartCodeInvalidated(ctx, cartID, a.RuleID, reason); err != nil {
				slog.WarnContext(ctx, "promotion: mark code invalidated", "cart", cartID, "rule", a.RuleID, "err", err)
			}
		}
	}
}

// The reasons render verbatim on the storefront ("No longer valid: ..."), so they read as UX copy
func (s *Service) appliedFailureReason(ctx context.Context, a AppliedCodeRecord, email string) (reason string, terminal bool) {
	rule, err := s.repo.GetRule(ctx, a.RuleID)
	if err != nil {
		return "no longer available", true
	}
	if !rule.IsActive {
		return "no longer available", true
	}
	var code Code
	if a.CodeID != nil {
		if c, err := s.repo.GetCode(ctx, *a.CodeID); err == nil {
			code = c
		}
	}
	if err := checkValidityWindow(rule, code); err != nil {
		return "expired", true
	}
	if (rule.MaxUses != nil && rule.UsedCount >= *rule.MaxUses) ||
		(code.MaxUses != nil && code.UsedCount >= *code.MaxUses) {
		return "usage limit reached", true
	}
	if code.ID != uuid.Nil && !code.IsActive {
		return "no longer available", true
	}
	// Not terminal - the shopper can still put the right address in the checkout email field and have the code refire
	if !codeEmailAllows(code, email) {
		return "reserved for a different email address", false
	}
	return "cart no longer qualifies", false
}

// One row per applied rule - SubtotalDiscount is that rule's contribution, and FreeShipping repeats the whole-cart flag for caller convenience
type AppliedRuleSummary struct {
	Code              string // empty for auto-apply rules
	RuleID            uuid.UUID
	RuleName          string
	PublicLabel       string
	SubtotalDiscount  int64
	FreeShipping      bool
	InvalidatedReason string
	// False means the rule auto-applied, so the storefront hides "Remove" - auto rules can only be invalidated by changing the cart
	RequiresCode bool
	// SKU and quantity from each core.add_free_gift action - the resolver resolves SKUs to product info and computes the in-cart flag, keeping this service catalog-agnostic
	FreeGiftOffers []FreeGiftOffer
}

// Raw (SKU, qty) from one core.add_free_gift action's params, enriched by the resolver before it reaches the storefront
type FreeGiftOffer struct {
	ProductSKU string
	Quantity   float64
}

// Manually-applied rows first in cart-row order, then auto-apply rules that fired without a code
// A rule that fails to look up is dropped silently - rare, and it shouldn't break the rest of the display
func (s *Service) BuildAppliedSummary(ctx context.Context, in CartEvalResult) []AppliedRuleSummary {
	ruleDiscount := map[uuid.UUID]int64{}
	for _, ar := range in.Result.Applied {
		ruleDiscount[ar.RuleID] = ar.DiscountCents
	}
	freeShip := in.Result.Ledger.FreeShipping()
	seen := map[uuid.UUID]bool{}
	out := make([]AppliedRuleSummary, 0, len(in.Result.Applied))
	for _, a := range in.ManuallyApplied {
		rule, err := s.repo.GetRule(ctx, a.RuleID)
		if err != nil {
			continue
		}
		seen[rule.ID] = true
		out = append(out, AppliedRuleSummary{
			Code:              a.CodeText,
			RuleID:            rule.ID,
			RuleName:          rule.Name,
			PublicLabel:       rule.PublicLabel,
			SubtotalDiscount:  ruleDiscount[rule.ID],
			FreeShipping:      freeShip,
			InvalidatedReason: a.InvalidatedReason,
			RequiresCode:      rule.RequiresCode,
			FreeGiftOffers:    s.freeGiftOffersForRule(ctx, rule.ID),
		})
	}
	for _, ar := range in.Result.Applied {
		if seen[ar.RuleID] {
			continue
		}
		rule, err := s.repo.GetRule(ctx, ar.RuleID)
		if err != nil {
			continue
		}
		out = append(out, AppliedRuleSummary{
			RuleID:           rule.ID,
			RuleName:         rule.Name,
			PublicLabel:      rule.PublicLabel,
			SubtotalDiscount: ar.DiscountCents,
			FreeShipping:     freeShip,
			RequiresCode:     rule.RequiresCode,
			FreeGiftOffers:   s.freeGiftOffersForRule(ctx, rule.ID),
		})
	}
	return out
}

// Other action types are ignored, and a lookup error returns an empty slice - a missing action listing only costs the offer text, PlaceOrder still picks it up from the engine's ledger
func (s *Service) freeGiftOffersForRule(ctx context.Context, ruleID uuid.UUID) []FreeGiftOffer {
	actions, err := s.repo.ListActionsByRule(ctx, ruleID)
	if err != nil {
		return nil
	}
	var out []FreeGiftOffer
	for _, a := range actions {
		if a.Type != "core.add_free_gift" {
			continue
		}
		var p struct {
			ProductSKU string  `json:"product_sku"`
			Qty        float64 `json:"qty"`
		}
		if err := json.Unmarshal(a.Params, &p); err != nil {
			continue
		}
		if p.ProductSKU == "" || p.Qty <= 0 {
			continue
		}
		out = append(out, FreeGiftOffer{ProductSKU: p.ProductSKU, Quantity: p.Qty})
	}
	return out
}

// ---- Display ----

// Rules whose product-level conditions match the candidate AND that carry a label or banner, priority ascending - the storefront's PLP/PDP promotional creative Cart-level conditions are NOT evaluated: badges are informational, not enforcement - Validity windows and is_active ARE honoured so expired or paused rules stay hidden Hits the engine's active-rules cache, so after the first request warms it the per-product call is essentially free
func (s *Service) DisplaysForProduct(ctx context.Context, channelID *uuid.UUID, p ProductCandidate) ([]Rule, error) {
	rules, err := loadCachedActiveRules(ctx, s.engine.cache, s.repo, channelID)
	if err != nil {
		return nil, err
	}
	// The cache only stores rules, so load conditions fresh - MatchesProduct needs them, and one batched query for every active rule is a single cheap roundtrip
	if len(rules) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(rules))
	for i, r := range rules {
		ids[i] = r.ID
	}
	conds, err := s.repo.ListConditionsByRules(ctx, ids)
	if err != nil {
		return nil, err
	}
	byRule := make(map[uuid.UUID][]Condition, len(rules))
	for _, c := range conds {
		byRule[c.RuleID] = append(byRule[c.RuleID], c)
	}
	out := make([]Rule, 0, len(rules))
	for i := range rules {
		r := rules[i]
		// A rule qualifies for display with at least one of: media label (PLP sticker), banner (PDP hero), or pill text
		if r.LabelMediaID == nil && r.BannerMediaID == nil && r.PillText == "" {
			continue
		}
		r.Conditions = byRule[r.ID]
		if !MatchesProduct(r, p) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Hooks observe what the engine emitted but can't mutate it - the totals are already stamped on the order row, so mutation here would just be confusing
// A degenerate Ledger keeps the hook signature aligned with action.Apply
type readOnlyLedger struct{ discount int64 }

func newReadOnlyLedger(discount int64) Ledger {
	return &readOnlyLedger{discount: discount}
}

func (l *readOnlyLedger) AddSubtotalDiscount(int64)            {}
func (l *readOnlyLedger) AddLineAdjustment(uuid.UUID, int64)   {}
func (l *readOnlyLedger) SetFreeShipping([]string)             {}
func (l *readOnlyLedger) SetShippingOverride(int64)            {}
func (l *readOnlyLedger) AddFreeGift(string, float64)          {}
func (l *readOnlyLedger) CapSubtotalDiscount(int64)            {}
func (l *readOnlyLedger) SubtotalDiscountCents() int64         { return l.discount }
func (l *readOnlyLedger) LineAdjustments() map[uuid.UUID]int64 { return nil }
func (l *readOnlyLedger) LineAdjustment(uuid.UUID) int64       { return 0 }
func (l *readOnlyLedger) FreeShipping() bool                   { return false }
func (l *readOnlyLedger) FreeShippingMethods() []string        { return nil }
func (l *readOnlyLedger) ShippingOverride() *int64             { return nil }
func (l *readOnlyLedger) FreeGifts() []FreeGift                { return nil }

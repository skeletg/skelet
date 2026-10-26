package catalog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A product's option rules: when the customer's answers match a condition, the rule's actions apply to the cart line -
// an option becomes required or optional, the line quantity is fixed, or the price moves. Actions never change answers,
// so a condition may read any option and evaluation has no order to respect, unlike visible_if
type ProductOptionRuleActionType string

const (
	RuleActionRequire     ProductOptionRuleActionType = "require"
	RuleActionOptional    ProductOptionRuleActionType = "optional"
	RuleActionSetQuantity ProductOptionRuleActionType = "set_quantity"
	RuleActionAdjustPrice ProductOptionRuleActionType = "adjust_price"
)

func (t ProductOptionRuleActionType) IsKnown() bool {
	switch t {
	case RuleActionRequire, RuleActionOptional, RuleActionSetQuantity, RuleActionAdjustPrice:
		return true
	}
	return false
}

type RuleChannelPrice struct {
	ChannelID  uuid.UUID `json:"channel_id"`
	PriceValue int32     `json:"price_value"`
}

type ProductOptionRuleAction struct {
	Type ProductOptionRuleActionType `json:"type"`
	// require / optional
	OptionID *uuid.UUID `json:"option_id,omitempty"`
	// set_quantity
	Quantity *float64 `json:"quantity,omitempty"`
	// adjust_price - FIXED in the product currency's minor units, PERCENT in whole points of the line's unit price; either may be negative
	PriceType     ProductOptionPriceType `json:"price_type,omitempty"`
	PriceValue    int32                  `json:"price_value,omitempty"`
	ChannelPrices []RuleChannelPrice     `json:"channel_prices,omitempty"`
}

type ProductOptionRule struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	// Set instead of ProductID on a template's rule - TemplateName is resolved only in a product's list
	TemplateID   uuid.UUID
	TemplateName string
	SortOrder    int32
	// Label is resolved to the request locale with a default-locale fallback - it names the cart line row a price change adds
	Label     string
	Labels    map[string]string
	Condition OptionConditionGroup
	Actions   []ProductOptionRuleAction
	// The owning product's default currency, which a FIXED amount is in
	Currency  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ProductOptionRuleInput struct {
	// Keeps the rule's identity across saves - nil adds one
	ID        *uuid.UUID
	Labels    map[string]string
	Condition OptionConditionGroup
	Actions   []ProductOptionRuleAction
}

var (
	ErrOptionRuleInvalid = errors.New("invalid option rule")
	ErrOptionInUse       = errors.New("an option rule or condition still uses this")
)

const (
	MaxOptionRules       = 50
	MaxOptionRuleActions = 20
	maxRuleLabelRunes    = 120
)

// The FIXED amount an adjust_price action charges on one channel - same resolution as an option's surcharge
func (a ProductOptionRuleAction) ChannelPriceValue(channelID uuid.UUID, channelCurrency, baseCurrency string) (int32, bool) {
	if a.PriceType == OptionPricePercent {
		return a.PriceValue, true
	}
	prices := make([]OptionChannelPrice, len(a.ChannelPrices))
	for i, p := range a.ChannelPrices {
		prices[i] = OptionChannelPrice(p)
	}
	return ResolveChannelFixed(a.PriceValue, prices, channelID, channelCurrency, baseCurrency)
}

// The request locale's label, else the default locale's, else whichever is stored - a name in some language beats none
func ResolveRuleLabel(labels map[string]string, locale, defaultLocale string) string {
	for _, loc := range append([]string{locale, defaultLocale}, slices.Sorted(maps.Keys(labels))...) {
		if l := strings.TrimSpace(labels[loc]); l != "" {
			return l
		}
	}
	return ""
}

// ==================== Service ====================

func (s *optionService) GetProductOptionRules(ctx context.Context, productID uuid.UUID, locale string) ([]ProductOptionRule, error) {
	byProduct, err := s.GetProductOptionRulesByProductIDs(ctx, []uuid.UUID{productID}, locale)
	if err != nil {
		return nil, err
	}
	if rules := byProduct[productID]; rules != nil {
		return rules, nil
	}
	return []ProductOptionRule{}, nil
}

// A product's list leads with the rules of the templates it uses, in their order, then its own
func (s *optionService) GetProductOptionRulesByProductIDs(ctx context.Context, productIDs []uuid.UUID, locale string) (map[uuid.UUID][]ProductOptionRule, error) {
	productIDs = distinctIDs(productIDs)
	own, err := s.repo.ListProductOptionRulesByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	uses, err := s.repo.ListEffectiveOptionTemplates(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	assigned := make(map[uuid.UUID][]uuid.UUID, len(uses))
	var templateIDs []uuid.UUID
	for pid, list := range uses {
		assigned[pid] = templateIDsOf(list)
		templateIDs = append(templateIDs, assigned[pid]...)
	}
	templateIDs = distinctIDs(templateIDs)
	byTemplate := map[uuid.UUID][]ProductOptionRule{}
	templates := map[uuid.UUID]OptionTemplate{}
	if len(templateIDs) > 0 {
		if byTemplate, err = s.repo.ListProductOptionRulesByTemplateIDs(ctx, templateIDs); err != nil {
			return nil, err
		}
		if templates, err = s.repo.ListOptionTemplatesByIDs(ctx, templateIDs); err != nil {
			return nil, err
		}
	}
	if len(own) == 0 && len(byTemplate) == 0 {
		return map[uuid.UUID][]ProductOptionRule{}, nil
	}
	owners, err := s.repo.ListProductRowsByIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("option rule owners: %w", err)
	}
	def := s.resolveDefaultLocale(ctx)
	if strings.TrimSpace(locale) == "" {
		locale = def
	}
	out := make(map[uuid.UUID][]ProductOptionRule, len(productIDs))
	for _, pid := range productIDs {
		var list []ProductOptionRule
		for _, tid := range assigned[pid] {
			for _, r := range byTemplate[tid] {
				r.Currency, r.TemplateName = templates[tid].CurrencyCode, templates[tid].Name
				list = append(list, r)
			}
		}
		for _, r := range own[pid] {
			r.Currency = owners[pid].DefaultCurrencyCode
			list = append(list, r)
		}
		for i := range list {
			list[i].Label = ResolveRuleLabel(list[i].Labels, locale, def)
		}
		if len(list) > 0 {
			out[pid] = list
		}
	}
	return out, nil
}

// SetProductOptionRules replaces the product's own rules with the given list, in order - an input carrying an id
// updates that rule in place, so carts holding its price row still find it, and a rule left out is deleted. They may
// read and change the options of the templates the product uses too
func (s *optionService) SetProductOptionRules(ctx context.Context, productID uuid.UUID, input []ProductOptionRuleInput) ([]ProductOptionRule, error) {
	if len(input) > MaxOptionRules {
		return nil, fmt.Errorf("%w: at most %d rules per product", ErrOptionRuleInvalid, MaxOptionRules)
	}
	if _, err := s.repo.GetProductByID(ctx, productID); err != nil {
		return nil, err
	}
	owner := OptionOwner{ProductID: productID}
	options, err := s.ownerOptions(ctx, owner)
	if err != nil {
		return nil, err
	}
	listings, err := s.core.repo.ListProductChannelListingsByProduct(ctx, productID)
	if err != nil {
		return nil, err
	}
	listed := make(map[uuid.UUID]bool, len(listings))
	for _, l := range listings {
		listed[l.ChannelID] = true
	}
	for i := range input {
		if err := validateOptionRule(&input[i], options, listed); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	if err := s.announced(ctx, owner, s.replaceRules(ctx, owner, input)); err != nil {
		return nil, err
	}
	return s.GetProductOptionRules(ctx, productID, "")
}

func (s *optionService) replaceRules(ctx context.Context, owner OptionOwner, input []ProductOptionRuleInput) error {
	return s.inTx(ctx, func(r Repository) error {
		existing, err := s.ownedRules(ctx, r, owner)
		if err != nil {
			return err
		}
		kept := make(map[uuid.UUID]bool, len(input))
		for i, in := range input {
			if in.ID == nil {
				if _, err := r.CreateProductOptionRule(ctx, owner, int32(i), in); err != nil {
					return err
				}
				continue
			}
			kept[*in.ID] = true
			if _, err := r.UpdateProductOptionRule(ctx, *in.ID, owner, int32(i), in); err != nil {
				return fmt.Errorf("rule %d: %w", i+1, err)
			}
		}
		for _, old := range existing {
			if !kept[old.ID] {
				if err := r.DeleteProductOptionRule(ctx, old.ID, owner); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Only the owner's own rules - a product's list without its templates'
func (s *optionService) ownedRules(ctx context.Context, r OptionRepository, owner OptionOwner) ([]ProductOptionRule, error) {
	if owner.IsTemplate() {
		byTemplate, err := r.ListProductOptionRulesByTemplateIDs(ctx, []uuid.UUID{owner.TemplateID})
		return byTemplate[owner.TemplateID], err
	}
	byProduct, err := r.ListProductOptionRulesByProductIDs(ctx, []uuid.UUID{owner.ProductID})
	return byProduct[owner.ProductID], err
}

// The rule using an option or one of its values, if any - deleting either would leave the rule pointing at nothing
func (s *optionService) ruleUsing(ctx context.Context, owner OptionOwner, optionID, valueID *uuid.UUID) (*ProductOptionRule, error) {
	rules, err := s.ownedRules(ctx, s.repo, owner)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		if optionID != nil && (groupReferencesOption(&rule.Condition, *optionID) || ruleTargets(rule, *optionID)) {
			return &rule, nil
		}
		if valueID != nil && groupReferencesValue(&rule.Condition, *valueID) {
			return &rule, nil
		}
	}
	return nil, nil
}

func ruleTargets(rule ProductOptionRule, optionID uuid.UUID) bool {
	for _, a := range rule.Actions {
		if a.OptionID != nil && *a.OptionID == optionID {
			return true
		}
	}
	return false
}

func ruleInUseError(rule *ProductOptionRule) error {
	if l := ResolveRuleLabel(rule.Labels, "", ""); l != "" {
		return fmt.Errorf("%w: the rule %q uses it - change or remove that rule first", ErrOptionInUse, l)
	}
	return fmt.Errorf("%w: rule %d uses it - change or remove that rule first", ErrOptionInUse, rule.SortOrder+1)
}

// validateOptionRule checks the condition the way visible_if is checked, minus the order rule, and each action against
// the product's own options - it trims labels and drops blank ones in place
func validateOptionRule(in *ProductOptionRuleInput, options []ProductOption, listed map[uuid.UUID]bool) error {
	if len(in.Condition.Rules) == 0 && len(in.Condition.Groups) == 0 {
		return fmt.Errorf("%w: a rule needs a condition", ErrOptionRuleInvalid)
	}
	// MaxInt32 as its own position lets every option be a parent, and the nil id matches none of them
	if err := validateOptionVisibility(&in.Condition, math.MaxInt32, uuid.Nil, options); err != nil {
		return fmt.Errorf("%w: %w", ErrOptionRuleInvalid, err)
	}
	if len(in.Actions) == 0 {
		return fmt.Errorf("%w: a rule needs at least one action", ErrOptionRuleInvalid)
	}
	if len(in.Actions) > MaxOptionRuleActions {
		return fmt.Errorf("%w: at most %d actions per rule", ErrOptionRuleInvalid, MaxOptionRuleActions)
	}
	byID := make(map[uuid.UUID]bool, len(options))
	for _, o := range options {
		byID[o.ID] = true
	}
	labels := make(map[string]string, len(in.Labels))
	for loc, l := range in.Labels {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if len([]rune(l)) > maxRuleLabelRunes {
			return fmt.Errorf("%w: a label is at most %d characters", ErrOptionRuleInvalid, maxRuleLabelRunes)
		}
		labels[strings.TrimSpace(loc)] = l
	}
	in.Labels = labels
	quantities := 0
	for i, a := range in.Actions {
		switch a.Type {
		case RuleActionRequire, RuleActionOptional:
			if a.OptionID == nil || !byID[*a.OptionID] {
				return fmt.Errorf("%w: action %d names an option not on this product", ErrOptionRuleInvalid, i+1)
			}
			in.Actions[i] = ProductOptionRuleAction{Type: a.Type, OptionID: a.OptionID}
		case RuleActionSetQuantity:
			if a.Quantity == nil {
				return fmt.Errorf("%w: action %d: %w", ErrOptionRuleInvalid, i+1, ErrInvalidForcedLineQty)
			}
			if err := validateForcedLineQty(a.Quantity); err != nil {
				return fmt.Errorf("%w: action %d: %w", ErrOptionRuleInvalid, i+1, err)
			}
			if quantities++; quantities > 1 {
				return fmt.Errorf("%w: one rule can set the quantity once", ErrOptionRuleInvalid)
			}
			in.Actions[i] = ProductOptionRuleAction{Type: a.Type, Quantity: a.Quantity}
		case RuleActionAdjustPrice:
			if !a.PriceType.IsKnown() {
				return fmt.Errorf("%w: action %d: %w", ErrOptionRuleInvalid, i+1, ErrInvalidProductOptionPriceType)
			}
			if a.PriceType == OptionPricePercent {
				if a.PriceValue < MinOptionPercent || a.PriceValue > MaxOptionPercent {
					return fmt.Errorf("%w: action %d: a percentage runs %d to %d", ErrOptionRuleInvalid, i+1, MinOptionPercent, MaxOptionPercent)
				}
				if len(a.ChannelPrices) > 0 {
					return fmt.Errorf("%w: action %d: %w", ErrOptionRuleInvalid, i+1, ErrOptionChannelPriceNotFixed)
				}
			}
			seen := make(map[uuid.UUID]bool, len(a.ChannelPrices))
			for _, p := range a.ChannelPrices {
				if !listed[p.ChannelID] {
					return fmt.Errorf("%w: action %d: %w", ErrOptionRuleInvalid, i+1, ErrOptionChannelNotListed)
				}
				if seen[p.ChannelID] {
					return fmt.Errorf("%w: action %d sets one channel twice", ErrOptionRuleInvalid, i+1)
				}
				seen[p.ChannelID] = true
				if p.PriceValue > MaxOptionChannelSurcharge || p.PriceValue < -MaxOptionChannelSurcharge {
					return fmt.Errorf("%w: action %d: %w", ErrOptionRuleInvalid, i+1, ErrOptionChannelPriceRange)
				}
			}
			in.Actions[i] = ProductOptionRuleAction{Type: a.Type, PriceType: a.PriceType, PriceValue: a.PriceValue, ChannelPrices: a.ChannelPrices}
		default:
			return fmt.Errorf("%w: action %d has an unknown type %q", ErrOptionRuleInvalid, i+1, a.Type)
		}
	}
	return nil
}

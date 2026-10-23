package catalog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A named set of options and rules kept once and assigned to many products, directly or through their product type -
// every product using it shows its options first, before its own, and an edit reaches all of them
type OptionTemplate struct {
	ID          uuid.UUID
	Name        string
	Description string
	// What a bare FIXED amount on the template's options and rules is in - per-store amounts cover the other currencies
	CurrencyCode string
	OptionCount  int
	RuleCount    int
	// Products that picked it themselves - ProductTypeCount counts the types handing it to theirs
	ProductCount     int
	ProductTypeCount int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// One template a product shows, and whether its product type hands it over rather than the product picking it
type OptionTemplateUse struct {
	TemplateID      uuid.UUID
	FromProductType bool
}

func templateIDsOf(uses []OptionTemplateUse) []uuid.UUID {
	out := make([]uuid.UUID, len(uses))
	for i, u := range uses {
		out[i] = u.TemplateID
	}
	return out
}

type OptionTemplateInput struct {
	Name         string
	Description  string
	CurrencyCode string
}

// How the product page asks a product's options - every one at once, or a step at a time
type ConfiguratorLayout string

const (
	ConfiguratorList  ConfiguratorLayout = "list"
	ConfiguratorSteps ConfiguratorLayout = "steps"
)

// The product or template an option or rule belongs to - exactly one id is set
type OptionOwner struct {
	ProductID  uuid.UUID
	TemplateID uuid.UUID
}

func (o OptionOwner) IsTemplate() bool { return o.TemplateID != uuid.Nil }

func (o ProductOption) Owner() OptionOwner {
	return OptionOwner{ProductID: o.ProductID, TemplateID: o.TemplateID}
}

var (
	ErrOptionTemplateNotFound  = errors.New("configurator template not found")
	ErrOptionTemplateInvalid   = errors.New("invalid configurator template")
	ErrOptionTemplateNameTaken = errors.New("a configurator template with that name already exists")
	ErrOptionTemplateInUse     = errors.New("configurator template is in use")
)

const (
	// One list - a product's own picks, or a product type's. A product shows both, so up to twice this many
	MaxOptionTemplatesPerList  = 10
	maxOptionTemplateNameRunes = 120
	maxOptionTemplateDescRunes = 2000
	// A product's template options sort below every option of its own, so anything ordering by sort_order - the
	// storefront, the cart's variant merge, the cart console - shows them first
	templateSortBand = math.MinInt32 / 2
)

var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

func normalizeTemplateInput(in OptionTemplateInput) (OptionTemplateInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.CurrencyCode = strings.ToUpper(strings.TrimSpace(in.CurrencyCode))
	switch {
	case in.Name == "":
		return in, fmt.Errorf("%w: a template needs a name", ErrOptionTemplateInvalid)
	case len([]rune(in.Name)) > maxOptionTemplateNameRunes:
		return in, fmt.Errorf("%w: a name is at most %d characters", ErrOptionTemplateInvalid, maxOptionTemplateNameRunes)
	case len([]rune(in.Description)) > maxOptionTemplateDescRunes:
		return in, fmt.Errorf("%w: a description is at most %d characters", ErrOptionTemplateInvalid, maxOptionTemplateDescRunes)
	case !currencyCodePattern.MatchString(in.CurrencyCode):
		return in, fmt.Errorf("%w: currency must be a three-letter code like USD", ErrOptionTemplateInvalid)
	}
	return in, nil
}

// ==================== Service ====================

func (s *optionService) ListOptionTemplates(ctx context.Context) ([]OptionTemplate, error) {
	return s.repo.ListOptionTemplates(ctx)
}

// With its option, rule and product counts
func (s *optionService) GetOptionTemplate(ctx context.Context, id uuid.UUID) (OptionTemplate, error) {
	byID, err := s.repo.ListOptionTemplatesByIDs(ctx, []uuid.UUID{id})
	if err != nil {
		return OptionTemplate{}, err
	}
	t, ok := byID[id]
	if !ok {
		return OptionTemplate{}, ErrOptionTemplateNotFound
	}
	return t, nil
}

func (s *optionService) CreateOptionTemplate(ctx context.Context, in OptionTemplateInput) (OptionTemplate, error) {
	in, err := normalizeTemplateInput(in)
	if err != nil {
		return OptionTemplate{}, err
	}
	return s.repo.CreateOptionTemplate(ctx, in)
}

// A currency change re-reads every bare FIXED amount in the new currency, the same way it would on a product
func (s *optionService) UpdateOptionTemplate(ctx context.Context, id uuid.UUID, in OptionTemplateInput) (OptionTemplate, error) {
	in, err := normalizeTemplateInput(in)
	if err != nil {
		return OptionTemplate{}, err
	}
	updated, err := s.repo.UpdateOptionTemplate(ctx, id, in)
	if err != nil {
		return OptionTemplate{}, err
	}
	return updated, s.announced(ctx, OptionOwner{TemplateID: id}, nil)
}

func (s *optionService) DeleteOptionTemplate(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetOptionTemplate(ctx, id); err != nil {
		return err
	}
	n, err := s.repo.CountProductsUsingOptionTemplate(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %d %s still use it - remove it from them first", ErrOptionTemplateInUse, n, plural(n, "product", "products"))
	}
	if n, err = s.repo.CountProductTypesUsingOptionTemplate(ctx, id); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("%w: %d product %s still hand it to their products - remove it from them first", ErrOptionTemplateInUse, n, plural(n, "type", "types"))
	}
	return s.repo.DeleteOptionTemplate(ctx, id)
}

func (s *optionService) ListProductIDsUsingOptionTemplate(ctx context.Context, templateID uuid.UUID, limit, offset int32) ([]uuid.UUID, int64, error) {
	ids, err := s.repo.ListProductIDsUsingOptionTemplate(ctx, templateID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountProductsUsingOptionTemplate(ctx, templateID)
	return ids, total, err
}

// The templates a product picked itself, in the order their options show
func (s *optionService) GetProductOptionTemplates(ctx context.Context, productID uuid.UUID) ([]OptionTemplate, error) {
	assigned, err := s.repo.ListOptionTemplateAssignments(ctx, []uuid.UUID{productID})
	if err != nil {
		return nil, err
	}
	return s.templatesInOrder(ctx, assigned[productID])
}

func checkTemplateList(templateIDs []uuid.UUID) error {
	if len(templateIDs) > MaxOptionTemplatesPerList {
		return fmt.Errorf("%w: at most %d templates in one list", ErrOptionTemplateInvalid, MaxOptionTemplatesPerList)
	}
	seen := make(map[uuid.UUID]bool, len(templateIDs))
	for _, id := range templateIDs {
		if seen[id] {
			return fmt.Errorf("%w: a template is listed twice", ErrOptionTemplateInvalid)
		}
		seen[id] = true
	}
	return nil
}

func (s *optionService) templatesInOrder(ctx context.Context, ids []uuid.UUID) ([]OptionTemplate, error) {
	if len(ids) == 0 {
		return []OptionTemplate{}, nil
	}
	byID, err := s.repo.ListOptionTemplatesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]OptionTemplate, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// SetProductOptionTemplates replaces the templates a product uses with the given list, in order - a template left out
// must not be something the product's own options or rules still read
func (s *optionService) SetProductOptionTemplates(ctx context.Context, productID uuid.UUID, templateIDs []uuid.UUID) error {
	if err := checkTemplateList(templateIDs); err != nil {
		return err
	}
	seen := make(map[uuid.UUID]bool, len(templateIDs))
	for _, id := range templateIDs {
		seen[id] = true
	}
	if err := s.checkTemplateTarget(ctx, productID); err != nil {
		return err
	}
	found, err := s.repo.ListOptionTemplatesByIDs(ctx, templateIDs)
	if err != nil {
		return err
	}
	if len(found) != len(templateIDs) {
		return ErrOptionTemplateNotFound
	}
	current, err := s.repo.ListOptionTemplateAssignments(ctx, []uuid.UUID{productID})
	if err != nil {
		return err
	}
	viaType, err := s.fromProductType(ctx, []uuid.UUID{productID})
	if err != nil {
		return err
	}
	for _, old := range current[productID] {
		if !seen[old] && !viaType[productID][old] {
			if err := s.refuseLeaningOnTemplate(ctx, []uuid.UUID{productID}, old); err != nil {
				return err
			}
		}
	}
	err = s.inTx(ctx, func(r Repository) error {
		return r.ReplaceOptionTemplateAssignments(ctx, productID, templateIDs)
	})
	return s.announced(ctx, OptionOwner{ProductID: productID}, err)
}

// AssignOptionTemplate adds the template to the end of each product's list - one already using it stays as it is
func (s *optionService) AssignOptionTemplate(ctx context.Context, templateID uuid.UUID, productIDs []uuid.UUID) error {
	if _, err := s.repo.GetOptionTemplate(ctx, templateID); err != nil {
		return err
	}
	assigned, err := s.repo.ListOptionTemplateAssignments(ctx, productIDs)
	if err != nil {
		return err
	}
	for _, pid := range productIDs {
		if err := s.checkTemplateTarget(ctx, pid); err != nil {
			return err
		}
		if !slices.Contains(assigned[pid], templateID) && len(assigned[pid]) >= MaxOptionTemplatesPerList {
			return fmt.Errorf("%w: a product uses at most %d templates", ErrOptionTemplateInvalid, MaxOptionTemplatesPerList)
		}
	}
	err = s.inTx(ctx, func(r Repository) error {
		for _, pid := range productIDs {
			if err := r.AppendOptionTemplateAssignment(ctx, pid, templateID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.publishProductsUpdated(ctx, productIDs, ProductFieldOptions)
	return nil
}

func (s *optionService) UnassignOptionTemplate(ctx context.Context, templateID uuid.UUID, productIDs []uuid.UUID) error {
	viaType, err := s.fromProductType(ctx, productIDs)
	if err != nil {
		return err
	}
	losing := slices.DeleteFunc(slices.Clone(productIDs), func(pid uuid.UUID) bool { return viaType[pid][templateID] })
	if err := s.refuseLeaningOnTemplate(ctx, losing, templateID); err != nil {
		return err
	}
	err = s.inTx(ctx, func(r Repository) error {
		for _, pid := range productIDs {
			if err := r.DeleteOptionTemplateAssignment(ctx, pid, templateID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.publishProductsUpdated(ctx, productIDs, ProductFieldOptions)
	return nil
}

// DetachOptionTemplate turns a template the product uses into options and rules of its own, so they can be changed for
// this product alone - its own options and rules that read the template's are pointed at the copies
func (s *optionService) DetachOptionTemplate(ctx context.Context, productID, templateID uuid.UUID) error {
	product, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return err
	}
	template, err := s.repo.GetOptionTemplate(ctx, templateID)
	if err != nil {
		return err
	}
	assigned, err := s.repo.ListOptionTemplateAssignments(ctx, []uuid.UUID{productID})
	if err != nil {
		return err
	}
	viaType, err := s.fromProductType(ctx, []uuid.UUID{productID})
	if err != nil {
		return err
	}
	// Copies next to a template the type keeps handing over would ask every option twice
	if viaType[productID][templateID] {
		return fmt.Errorf("%w: it comes from the product's type - take it off the type, or detach it from every product of the type", ErrOptionTemplateInvalid)
	}
	if !slices.Contains(assigned[productID], templateID) {
		return fmt.Errorf("%w: the product doesn't use this template", ErrOptionTemplateInvalid)
	}
	source, err := s.GetOptionTemplateOptions(ctx, templateID, "")
	if err != nil {
		return err
	}
	if template.CurrencyCode != product.DefaultCurrencyCode && carriesBareFixed(source) {
		return fmt.Errorf("%w: its amounts are in %s and the product's are in %s - copying them would change what they charge", ErrOptionTemplateInvalid, template.CurrencyCode, product.DefaultCurrencyCode)
	}
	templateRules, err := s.repo.ListProductOptionRulesByTemplateIDs(ctx, []uuid.UUID{templateID})
	if err != nil {
		return err
	}
	listings, err := s.core.repo.ListProductChannelListingsByProduct(ctx, productID)
	if err != nil {
		return err
	}
	listed := make(map[uuid.UUID]bool, len(listings))
	for _, l := range listings {
		listed[l.ChannelID] = true
	}
	def := s.resolveDefaultLocale(ctx)

	err = s.inTx(ctx, func(r Repository) error {
		own, err := r.ListProductOptionsByProductID(ctx, productID)
		if err != nil {
			return err
		}
		first := int32(0)
		for _, o := range own {
			first = min(first, o.SortOrder)
		}
		ids := map[uuid.UUID]uuid.UUID{}
		for i, o := range source {
			created, valueIDs, err := cloneOption(ctx, r, o.ID, OptionOwner{ProductID: productID}, first-int32(len(source)-i), def, listed)
			if err != nil {
				return err
			}
			ids[o.ID] = created.ID
			maps.Copy(ids, valueIDs)
		}
		remap := func(id uuid.UUID) uuid.UUID {
			if to, ok := ids[id]; ok {
				return to
			}
			return id
		}
		copies, err := r.ListProductOptionsByProductID(ctx, productID)
		if err != nil {
			return err
		}
		for _, o := range copies {
			if o.VisibleIf == nil || !groupReferencesAny(o.VisibleIf, ids) {
				continue
			}
			g := remapGroup(*o.VisibleIf, remap)
			if _, err := r.UpdateProductOption(ctx, o.ID, fieldsOf(o, &g)); err != nil {
				return err
			}
		}
		ownRules, err := r.ListProductOptionRulesByProductIDs(ctx, []uuid.UUID{productID})
		if err != nil {
			return err
		}
		owner := OptionOwner{ProductID: productID}
		for i, rule := range templateRules[templateID] {
			if _, err := r.CreateProductOptionRule(ctx, owner, int32(i), remapRuleInput(rule, remap, listed)); err != nil {
				return err
			}
		}
		for i, rule := range ownRules[productID] {
			if _, err := r.UpdateProductOptionRule(ctx, rule.ID, owner, int32(len(templateRules[templateID])+i), remapRuleInput(rule, remap, nil)); err != nil {
				return err
			}
		}
		return r.DeleteOptionTemplateAssignment(ctx, productID, templateID)
	})
	return s.announced(ctx, OptionOwner{ProductID: productID}, err)
}

func (s *optionService) GetOptionTemplateOptions(ctx context.Context, templateID uuid.UUID, locale string) ([]ProductOption, error) {
	template, err := s.repo.GetOptionTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	byTemplate, err := s.repo.ListProductOptionsByTemplateIDs(ctx, []uuid.UUID{templateID})
	if err != nil {
		return nil, err
	}
	options := byTemplate[templateID]
	if err := s.hydrateOptions(ctx, options, locale, map[uuid.UUID]OptionTemplate{templateID: template}); err != nil {
		return nil, err
	}
	if options == nil {
		options = []ProductOption{}
	}
	return options, nil
}

func (s *optionService) GetOptionTemplateRules(ctx context.Context, templateID uuid.UUID, locale string) ([]ProductOptionRule, error) {
	template, err := s.repo.GetOptionTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	byTemplate, err := s.repo.ListProductOptionRulesByTemplateIDs(ctx, []uuid.UUID{templateID})
	if err != nil {
		return nil, err
	}
	rules := byTemplate[templateID]
	def := s.resolveDefaultLocale(ctx)
	if strings.TrimSpace(locale) == "" {
		locale = def
	}
	for i := range rules {
		rules[i].Label = ResolveRuleLabel(rules[i].Labels, locale, def)
		rules[i].Currency = template.CurrencyCode
		rules[i].TemplateName = template.Name
	}
	if rules == nil {
		rules = []ProductOptionRule{}
	}
	return rules, nil
}

// SetOptionTemplateRules replaces a template's rules - they read and change only the template's own options
func (s *optionService) SetOptionTemplateRules(ctx context.Context, templateID uuid.UUID, input []ProductOptionRuleInput) ([]ProductOptionRule, error) {
	if len(input) > MaxOptionRules {
		return nil, fmt.Errorf("%w: at most %d rules per template", ErrOptionRuleInvalid, MaxOptionRules)
	}
	options, err := s.GetOptionTemplateOptions(ctx, templateID, "")
	if err != nil {
		return nil, err
	}
	listed, err := s.knownChannels(ctx, ruleChannelIDs(input))
	if err != nil {
		return nil, err
	}
	for i := range input {
		if err := validateOptionRule(&input[i], options, listed); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
	}
	err = s.replaceRules(ctx, OptionOwner{TemplateID: templateID}, input)
	if err := s.announced(ctx, OptionOwner{TemplateID: templateID}, err); err != nil {
		return nil, err
	}
	return s.GetOptionTemplateRules(ctx, templateID, "")
}

func (s *optionService) GetProductConfiguratorLayout(ctx context.Context, productID uuid.UUID) (ConfiguratorLayout, error) {
	return s.repo.GetProductConfiguratorLayout(ctx, productID)
}

func (s *optionService) SetProductConfiguratorLayout(ctx context.Context, productID uuid.UUID, layout ConfiguratorLayout) error {
	if layout != ConfiguratorList && layout != ConfiguratorSteps {
		return fmt.Errorf("%w: layout must be list or steps", ErrOptionTemplateInvalid)
	}
	if err := s.checkTemplateTarget(ctx, productID); err != nil {
		return err
	}
	return s.announced(ctx, OptionOwner{ProductID: productID}, s.repo.SetProductConfiguratorLayout(ctx, productID, layout))
}

// ==================== Helpers ====================

func (s *optionService) checkTemplateTarget(ctx context.Context, productID uuid.UUID) error {
	p, err := s.repo.GetProductByID(ctx, productID)
	if err != nil {
		return err
	}
	if !slices.Contains(optionCarryingKinds, p.Kind) {
		return fmt.Errorf("%w: product %q is kind %q", ErrOptionInputKindNotPurchasable, p.SKU, string(p.Kind))
	}
	return nil
}

// The products' own options and rules that read or change one of the template's options - taking the template away
// would leave them pointing at nothing
func (s *optionService) refuseLeaningOnTemplate(ctx context.Context, productIDs []uuid.UUID, templateID uuid.UUID) error {
	if len(productIDs) == 0 {
		return nil
	}
	byTemplate, err := s.repo.ListProductOptionsByTemplateIDs(ctx, []uuid.UUID{templateID})
	if err != nil {
		return err
	}
	opts := byTemplate[templateID]
	optionIDs := make([]uuid.UUID, len(opts))
	for i, o := range opts {
		optionIDs[i] = o.ID
	}
	values, err := s.repo.ListProductOptionValuesByOptionIDs(ctx, optionIDs)
	if err != nil {
		return err
	}
	for i := range opts {
		opts[i].Values = values[opts[i].ID]
	}
	if err := s.refuseProductLeaning(ctx, productIDs, opts...); err != nil {
		return fmt.Errorf("%w (remove that first, or detach the template to keep its options)", err)
	}
	return nil
}

// refuseProductLeaning names the first of these products whose own option or rule reads or changes one of the
// template options or one of their values
func (s *optionService) refuseProductLeaning(ctx context.Context, productIDs []uuid.UUID, opts ...ProductOption) error {
	if len(productIDs) == 0 || len(opts) == 0 {
		return nil
	}
	own, err := s.repo.ListProductOptionsByProductIDs(ctx, productIDs)
	if err != nil {
		return err
	}
	rules, err := s.repo.ListProductOptionRulesByProductIDs(ctx, productIDs)
	if err != nil {
		return err
	}
	touches := func(g *OptionConditionGroup, opt ProductOption) bool {
		if groupReferencesOption(g, opt.ID) {
			return true
		}
		for _, v := range opt.Values {
			if groupReferencesValue(g, v.ID) {
				return true
			}
		}
		return false
	}
	// A type can reach thousands of products - only the one in the way is looked up, for its SKU
	sku := func(pid uuid.UUID) string {
		if p, err := s.repo.GetProductByID(ctx, pid); err == nil {
			return p.SKU
		}
		return pid.String()
	}
	for _, pid := range productIDs {
		for _, opt := range opts {
			for _, o := range own[pid] {
				if touches(o.VisibleIf, opt) {
					return fmt.Errorf("%w: an option on product %q shows only for an answer to it", ErrOptionInUse, sku(pid))
				}
			}
			for _, r := range rules[pid] {
				if touches(&r.Condition, opt) || ruleTargets(r, opt.ID) {
					return fmt.Errorf("%w: a rule on product %q uses it", ErrOptionInUse, sku(pid))
				}
			}
		}
	}
	return nil
}

// Channels a template's per-store amounts may name - any that exists, since a template isn't listed anywhere itself
func (s *optionService) knownChannels(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if s.channelSvc == nil {
			out[id] = true
			continue
		}
		if _, err := s.channelSvc.GetByID(ctx, id); err == nil {
			out[id] = true
		}
	}
	return out, nil
}

func ruleChannelIDs(input []ProductOptionRuleInput) []uuid.UUID {
	var out []uuid.UUID
	for _, r := range input {
		for _, a := range r.Actions {
			for _, p := range a.ChannelPrices {
				if !slices.Contains(out, p.ChannelID) {
					out = append(out, p.ChannelID)
				}
			}
		}
	}
	return out
}

func carriesBareFixed(options []ProductOption) bool {
	for _, o := range options {
		if o.PriceValue != nil && *o.PriceValue != 0 && o.PriceType != nil && *o.PriceType == OptionPriceFixed {
			return true
		}
		for _, v := range o.Values {
			if v.PriceValue != 0 && (v.PriceType == OptionPriceFixed || v.SourceProductID != nil) {
				return true
			}
		}
	}
	return false
}

func groupReferencesAny(g *OptionConditionGroup, ids map[uuid.UUID]uuid.UUID) bool {
	for id := range ids {
		if groupReferencesOption(g, id) || groupReferencesValue(g, id) {
			return true
		}
	}
	return false
}

func remapGroup(g OptionConditionGroup, remap func(uuid.UUID) uuid.UUID) OptionConditionGroup {
	out := OptionConditionGroup{Logic: g.Logic}
	for _, r := range g.Rules {
		values := make([]uuid.UUID, len(r.ValueIDs))
		for i, v := range r.ValueIDs {
			values[i] = remap(v)
		}
		out.Rules = append(out.Rules, OptionRule{OptionID: remap(r.OptionID), Op: r.Op, ValueIDs: values})
	}
	for _, sub := range g.Groups {
		out.Groups = append(out.Groups, remapGroup(sub, remap))
	}
	return out
}

// A rule as input again, pointed at the copies - store amounts for channels the product isn't listed on are dropped
// when listed is given, since a product's own amounts only name its own channels
func remapRuleInput(rule ProductOptionRule, remap func(uuid.UUID) uuid.UUID, listed map[uuid.UUID]bool) ProductOptionRuleInput {
	actions := make([]ProductOptionRuleAction, len(rule.Actions))
	for i, a := range rule.Actions {
		if a.OptionID != nil {
			id := remap(*a.OptionID)
			a.OptionID = &id
		}
		if listed != nil {
			a.ChannelPrices = slices.DeleteFunc(slices.Clone(a.ChannelPrices), func(p RuleChannelPrice) bool { return !listed[p.ChannelID] })
		}
		actions[i] = a
	}
	return ProductOptionRuleInput{Labels: rule.Labels, Condition: remapGroup(rule.Condition, remap), Actions: actions}
}

func fieldsOf(o ProductOption, visibleIf *OptionConditionGroup) UpdateProductOptionFields {
	return UpdateProductOptionFields{
		InputType:      o.InputType,
		IsRequired:     o.IsRequired,
		SortOrder:      o.SortOrder,
		PriceValue:     o.PriceValue,
		PriceType:      o.PriceType,
		MaxChars:       o.MaxChars,
		MaxFileBytes:   o.MaxFileBytes,
		FileExtensions: o.FileExtensions,
		VisibleIf:      visibleIf,
		ImageAssetID:   o.ImageAssetID,
	}
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

package tax

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// The tax-domain entry point - rule CRUD plus the Compute seam the order service calls, stateless beyond the repo and engine
type Service struct {
	repo   Repository
	engine *Engine
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, engine: NewEngine(repo, DefaultRegistry)}
}

func (s *Service) Repo() Repository { return s.repo }

func (s *Service) Engine() *Engine { return s.engine }

func (s *Service) Registry() *Registry { return DefaultRegistry }

// The whole-order tax seam for the order service - builds an Environment, runs the engine, and returns the total plus the first applied rule - A zero result with no error means nothing applied
func (s *Service) Compute(ctx context.Context, in ComputeInput) (ComputeResult, error) {
	if in.TaxableCents <= 0 || in.ChannelID == uuid.Nil {
		return ComputeResult{}, nil
	}
	env := BuildEnvironment(EnvironmentInput{
		ChannelID:    in.ChannelID,
		CountryCode:  in.CountryCode,
		RegionCode:   in.RegionCode,
		TaxableCents: in.TaxableCents,
		TaxInclusive: in.TaxInclusive,
	})
	res, err := s.engine.Evaluate(ctx, env)
	if err != nil {
		return ComputeResult{}, err
	}
	out := ComputeResult{Cents: res.TotalCents}
	if len(res.Applied) > 0 {
		first := res.Applied[0]
		out.RuleID = &first.RuleID
		out.Name = first.Name
		out.RateBPS = first.RateBPS
	}
	return out, nil
}

// One order line's taxable amount and product tax class, for per-line evaluation
type LineTaxInput struct {
	ProductTaxClass string
	TaxableCents    int64
}

// The per-line entry point - the engine runs once per line, and rounding is round-per-line-then-sum, so each line's tax is integer cents
type ComputeOrderInput struct {
	ChannelID         uuid.UUID
	CountryCode       string
	RegionCode        string
	TaxInclusive      bool
	CustomerGroupSlug string
	Lines             []LineTaxInput
}

type OrderTaxResult struct {
	TotalCents int64
	Lines      []EvalResult
}

// Evaluates tax per line and sums - a single-class cart gives the same total as whole-order Compute, so it's parity-preserving until tax classes are used
func (s *Service) ComputeOrder(ctx context.Context, in ComputeOrderInput) (OrderTaxResult, error) {
	if in.ChannelID == uuid.Nil {
		return OrderTaxResult{}, nil
	}
	out := OrderTaxResult{Lines: make([]EvalResult, 0, len(in.Lines))}
	for _, line := range in.Lines {
		if line.TaxableCents <= 0 {
			out.Lines = append(out.Lines, EvalResult{})
			continue
		}
		env := BuildEnvironment(EnvironmentInput{
			ChannelID:         in.ChannelID,
			CountryCode:       in.CountryCode,
			RegionCode:        in.RegionCode,
			TaxableCents:      line.TaxableCents,
			TaxInclusive:      in.TaxInclusive,
			ProductTaxClass:   line.ProductTaxClass,
			CustomerGroupSlug: in.CustomerGroupSlug,
		})
		res, err := s.engine.Evaluate(ctx, env)
		if err != nil {
			return OrderTaxResult{}, err
		}
		out.TotalCents += res.TotalCents
		out.Lines = append(out.Lines, res)
	}
	return out, nil
}

type CreateRuleInput struct {
	Rule       Rule
	Conditions []Condition
	Effects    []Effect
}

// Patches a rule - nil Conditions and Effects mean leave unchanged, while a non-nil, possibly empty, slice replaces them
type UpdateRuleInput struct {
	Rule       Rule
	Conditions *[]Condition
	Effects    *[]Effect
}

func (s *Service) CreateRule(ctx context.Context, in CreateRuleInput) (Rule, error) {
	if err := s.validate(in.Conditions, in.Effects); err != nil {
		return Rule{}, err
	}
	rule, err := s.repo.CreateRule(ctx, in.Rule)
	if err != nil {
		return Rule{}, err
	}
	if err := s.repo.ReplaceConditions(ctx, rule.ID, in.Conditions); err != nil {
		return Rule{}, err
	}
	if err := s.repo.ReplaceEffects(ctx, rule.ID, in.Effects); err != nil {
		return Rule{}, err
	}
	return s.GetRuleHydrated(ctx, rule.ID)
}

func (s *Service) UpdateRule(ctx context.Context, in UpdateRuleInput) (Rule, error) {
	conds := derefSlice(in.Conditions)
	effs := derefSlice(in.Effects)
	if err := s.validate(conds, effs); err != nil {
		return Rule{}, err
	}
	rule, err := s.repo.UpdateRule(ctx, in.Rule)
	if err != nil {
		return Rule{}, err
	}
	if in.Conditions != nil {
		if err := s.repo.ReplaceConditions(ctx, rule.ID, *in.Conditions); err != nil {
			return Rule{}, err
		}
	}
	if in.Effects != nil {
		if err := s.repo.ReplaceEffects(ctx, rule.ID, *in.Effects); err != nil {
			return Rule{}, err
		}
	}
	return s.GetRuleHydrated(ctx, rule.ID)
}

func (s *Service) DeleteRule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteRule(ctx, id)
}

func (s *Service) GetRuleHydrated(ctx context.Context, id uuid.UUID) (Rule, error) {
	rule, err := s.repo.GetRule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	if rule.Conditions, err = s.repo.ListConditionsByRule(ctx, id); err != nil {
		return Rule{}, err
	}
	if rule.Effects, err = s.repo.ListEffectsByRule(ctx, id); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// Every rule across all channels, hydrated - the admin GLOBAL cross-channel view
func (s *Service) ListAllRulesHydrated(ctx context.Context) ([]Rule, error) {
	return s.repo.ListAllRulesHydrated(ctx)
}

// The channel's rules, each with its conditions and effects, for the admin list
func (s *Service) ListRulesByChannelHydrated(ctx context.Context, channelID uuid.UUID) ([]Rule, error) {
	rules, err := s.repo.ListRulesByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].Conditions, err = s.repo.ListConditionsByRule(ctx, rules[i].ID); err != nil {
			return nil, err
		}
		if rules[i].Effects, err = s.repo.ListEffectsByRule(ctx, rules[i].ID); err != nil {
			return nil, err
		}
	}
	return rules, nil
}

func (s *Service) validate(conditions []Condition, effects []Effect) error {
	for i, c := range conditions {
		if err := DefaultRegistry.ValidateCondition(c.Type, c.Params); err != nil {
			return fmt.Errorf("condition %d: %w", i, err)
		}
	}
	for i, e := range effects {
		if err := DefaultRegistry.ValidateEffect(e.Type, e.Params); err != nil {
			return fmt.Errorf("effect %d: %w", i, err)
		}
	}
	return nil
}

func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

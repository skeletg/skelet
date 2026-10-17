package tax

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The tax-domain entry point - rule CRUD plus the Compute seam the order service calls, stateless beyond the repo and engine
type Service struct {
	repo   Repository
	engine *Engine
	tx     *txrunner.Manager
}

// SetTx makes a rule save atomic with its conditions and effects - without it each write commits on its own
func (s *Service) SetTx(tx *txrunner.Manager) { s.tx = tx }

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
	order, err := s.ComputeOrder(ctx, ComputeOrderInput{
		ChannelID: in.ChannelID, CountryCode: in.CountryCode, RegionCode: in.RegionCode, TaxInclusive: in.TaxInclusive,
		Lines: []LineTaxInput{{TaxableCents: in.TaxableCents}},
	})
	if err != nil {
		return ComputeResult{}, err
	}
	res := order.Lines[0]
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

// Evaluates tax per line and sums - a single-class cart gives the same total as whole-order Compute, so it's parity-preserving until tax classes are used. The channel's rules load once for the whole order
func (s *Service) ComputeOrder(ctx context.Context, in ComputeOrderInput) (OrderTaxResult, error) {
	if in.ChannelID == uuid.Nil {
		return OrderTaxResult{}, nil
	}
	rules, err := s.engine.activeRules(ctx, in.ChannelID)
	if err != nil {
		return OrderTaxResult{}, err
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
		res, err := s.engine.evaluate(ctx, env, rules)
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
	var id uuid.UUID
	err := s.inTx(ctx, func(r Repository) error {
		rule, err := r.CreateRule(ctx, in.Rule)
		if err != nil {
			return err
		}
		id = rule.ID
		if err := r.ReplaceConditions(ctx, rule.ID, in.Conditions); err != nil {
			return err
		}
		return r.ReplaceEffects(ctx, rule.ID, in.Effects)
	})
	if err != nil {
		return Rule{}, err
	}
	return s.GetRuleHydrated(ctx, id)
}

func (s *Service) UpdateRule(ctx context.Context, in UpdateRuleInput) (Rule, error) {
	conds := derefSlice(in.Conditions)
	effs := derefSlice(in.Effects)
	if err := s.validate(conds, effs); err != nil {
		return Rule{}, err
	}
	err := s.inTx(ctx, func(r Repository) error {
		rule, err := r.UpdateRule(ctx, in.Rule)
		if err != nil {
			return err
		}
		if in.Conditions != nil {
			if err := r.ReplaceConditions(ctx, rule.ID, *in.Conditions); err != nil {
				return err
			}
		}
		if in.Effects != nil {
			return r.ReplaceEffects(ctx, rule.ID, *in.Effects)
		}
		return nil
	})
	if err != nil {
		return Rule{}, err
	}
	return s.GetRuleHydrated(ctx, in.Rule.ID)
}

// inTx runs fn on a transactional repository, or on the plain one when no tx manager is wired
func (s *Service) inTx(ctx context.Context, fn func(Repository) error) error {
	if s.tx == nil {
		return fn(s.repo)
	}
	return s.tx.InTransaction(ctx, func(t *sql.Tx) error { return fn(s.repo.WithTx(t)) })
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
	return s.repo.ListRulesByChannelHydrated(ctx, channelID)
}

func (s *Service) validate(conditions []Condition, effects []Effect) error {
	for i, c := range conditions {
		if err := DefaultRegistry.ValidateCondition(c.Type, c.Params); err != nil {
			return invalidRule(fmt.Sprintf("condition %d: %v", i+1, err))
		}
	}
	for i, e := range effects {
		if err := DefaultRegistry.ValidateEffect(e.Type, e.Params); err != nil {
			return invalidRule(fmt.Sprintf("effect %d: %v", i+1, err))
		}
	}
	return nil
}

// invalidRule is a validation error carrying the registry's reason, so the admin sees what's wrong instead of a 500
func invalidRule(msg string) error {
	return &apperr.Error{Code: "TAX_RULE_INVALID", Class: apperr.ClassValidation, Message: msg}
}

func derefSlice[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

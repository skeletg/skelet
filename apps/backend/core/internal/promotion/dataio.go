package promotion

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

const (
	colCode      = "code"
	colUsedCount = "used_count"
	colCodeRule  = "rule"
	colCodeValid = "is_active"

	optRuleID = "rule_id"
	// Resolved by a provider registered at wiring time - the frontend must never need to know what a rule is
	SourceRules = "promotion_rules"
)

var codeFields = []dataio.Field{
	{Name: colCode, Label: "Code", Type: dataio.FieldText, Identifier: true, Required: true, Description: "Uppercased on import - re-importing an existing code is a no-op rather than an error", Example: "WELCOME10"},
	{Name: colCodeRule, Label: "Rule", Type: dataio.FieldText, ReadOnly: true},
	{Name: colUsedCount, Label: "Times used", Type: dataio.FieldInteger, ReadOnly: true},
	{Name: colCodeValid, Label: "Active", Type: dataio.FieldBool, ReadOnly: true},
}

// CodeResource is the coupon-code list under one promotion rule. Dumping codes is effectively minting unlimited
// discounts, so both directions gate on promotions.manage rather than the view grant
func CodeResource(svc *Service) dataio.Resource {
	return dataio.Resource{
		Slug:             "promotion_codes",
		Label:            "Promotion codes",
		Group:            "Marketing",
		Description:      "Coupon codes belonging to one promotion rule. Pick the rule below - a code list means nothing without it.",
		ViewPermission:   "promotions.manage",
		ImportPermission: "promotions.manage",
		Fields:           codeFields,
		Options: []dataio.OptionDef{
			{Name: optRuleID, Label: "Promotion rule", Type: dataio.OptSelect, Required: true, Source: SourceRules, Description: "Which rule these codes belong to"},
		},
		NewExporter: func(ctx context.Context, o dataio.Opts) (dataio.Exporter, error) {
			rule, err := ruleFor(ctx, svc, o)
			if err != nil {
				return nil, err
			}
			return &codeExporter{svc: svc, rule: rule}, nil
		},
		NewImporter: func(ctx context.Context, o dataio.Opts) (dataio.Importer, error) {
			rule, err := ruleFor(ctx, svc, o)
			if err != nil {
				return nil, err
			}
			return &codeImporter{svc: svc, rule: rule, opts: o}, nil
		},
	}
}

func ruleFor(ctx context.Context, svc *Service, o dataio.Opts) (Rule, error) {
	raw := o.Str(optRuleID)
	if raw == "" {
		return Rule{}, errors.New("pick the promotion rule these codes belong to")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return Rule{}, fmt.Errorf("invalid rule id %q", raw)
	}
	rule, err := svc.Repo().GetRule(ctx, id)
	if err != nil {
		return Rule{}, err
	}
	if !ruleInRunScope(rule, o.ChannelID) {
		return Rule{}, errors.New("that promotion rule belongs to another channel than this run")
	}
	return rule, nil
}

// The rule has to sit inside what the run was gated on: its channel, or everywhere when it names none
func ruleInRunScope(rule Rule, runChannel uuid.UUID) bool {
	if runChannel == uuid.Nil {
		return true
	}
	return rule.ChannelID != nil && *rule.ChannelID == runChannel
}

type codeExporter struct {
	svc  *Service
	rule Rule
}

func (e *codeExporter) Columns(context.Context) ([]dataio.Field, error) { return nil, nil }

// Paged rather than one big read - a rule can hold hundreds of thousands of generated codes
func (e *codeExporter) Stream(ctx context.Context, emit func(*dataio.Row) error) error {
	const pageSize = int32(5000)
	var offset int32
	for {
		codes, _, err := e.svc.Repo().ListCodesByRule(ctx, e.rule.ID, "", nil, pageSize, offset)
		if err != nil {
			return err
		}
		for _, c := range codes {
			r := dataio.NewRow()
			r.Ref = c.Code
			r.Set(colCode, c.Code)
			r.Set(colCodeRule, e.rule.Name)
			r.Set(colUsedCount, c.UsedCount)
			r.Set(colCodeValid, c.IsActive)
			if err := emit(r); err != nil {
				return err
			}
		}
		if int32(len(codes)) < pageSize {
			return nil
		}
		offset += pageSize
	}
}

// Batches so a 100k-code file is a few hundred inserts rather than 100k - Finish flushes the tail
type codeImporter struct {
	svc     *Service
	rule    Rule
	opts    dataio.Opts
	pending []pendingCode
	seen    map[string]bool
	taken   []dataio.RowError
}

type pendingCode struct {
	code string
	line int
}

const codeBatchSize = 1000

func (im *codeImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	code := normalizeCode(r.Str(colCode))
	r.Ref = code
	if code == "" || strings.HasPrefix(code, "#") {
		return dataio.OutcomeSkipped, nil // '#' lines let admins annotate their files
	}
	if im.seen == nil {
		im.seen = map[string]bool{}
	}
	if im.seen[code] {
		return dataio.OutcomeSkipped, nil
	}
	im.seen[code] = true
	if im.opts.DryRun {
		return dataio.OutcomeCreated, nil
	}
	im.pending = append(im.pending, pendingCode{code: code, line: r.Line})
	if len(im.pending) >= codeBatchSize {
		if err := im.flush(ctx); err != nil {
			return dataio.OutcomeSkipped, err
		}
	}
	return dataio.OutcomeCreated, nil
}

// A code another rule already owns is skipped by the insert - it was counted as created when its row was read, so it comes back here as that row's error
func (im *codeImporter) Finish(ctx context.Context) ([]dataio.RowError, error) {
	if err := im.flush(ctx); err != nil {
		return nil, err
	}
	return im.taken, nil
}

func (im *codeImporter) flush(ctx context.Context) error {
	if len(im.pending) == 0 {
		return nil
	}
	codes := make([]string, len(im.pending))
	for i, p := range im.pending {
		codes[i] = p.code
	}
	inserted, err := im.svc.Repo().BulkInsertCodes(ctx, im.rule.ID, codes)
	if err != nil {
		return err
	}
	landed := make(map[string]bool, len(inserted))
	for _, c := range inserted {
		landed[c] = true
	}
	for _, p := range im.pending {
		if !landed[p.code] {
			im.taken = append(im.taken, dataio.RowError{Line: p.line, Ref: p.code, Message: "code already exists"})
		}
	}
	im.pending = im.pending[:0]
	return nil
}

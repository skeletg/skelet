package tax

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/dataio"
)

const (
	colTaxName        = "name"
	colTaxCountry     = "country_code"
	colTaxRegion      = "region_code"
	colTaxRateBPS     = "rate_bps"
	colTaxPriority    = "priority"
	colTaxClass       = "tax_class"
	colTaxExclusivity = "exclusivity_group"
)

var rateFields = []dataio.Field{
	{Name: colTaxName, Label: "Rule name", Type: dataio.FieldText, Identifier: true, Required: true, Example: "Germany standard"},
	{Name: colTaxCountry, Label: "Country", Type: dataio.FieldText, Required: true, Description: "ISO-2 country code", Example: "DE"},
	{Name: colTaxRegion, Label: "Region", Type: dataio.FieldText, Description: "Blank means the whole country", Example: "BY"},
	{Name: colTaxRateBPS, Label: "Rate (bps)", Type: dataio.FieldInteger, Required: true, Description: "Basis points, so 1900 is 19%", Example: "1900"},
	{Name: colTaxPriority, Label: "Priority", Type: dataio.FieldInteger, Description: "Lower is evaluated earlier - defaults to 100", Example: "100"},
	{Name: colTaxClass, Label: "Tax class", Type: dataio.FieldText, Description: "Blank means every product"},
	{Name: colTaxExclusivity, Label: "Exclusivity group", Type: dataio.FieldText, Description: `Rules sharing a group don't stack - defaults to "import" so a country fallback and its state rows can't double up`},
}

// RateResource is the bulk on-ramp for jurisdictions. Each row compiles to one ordinary editable rule, so a
// rate table imported here is indistinguishable afterwards from one built by hand
func RateResource(svc *Service) dataio.Resource {
	return dataio.Resource{
		Slug:                    "tax_rates",
		Label:                   "Tax rates",
		Group:                   "Configuration",
		Description:             "A jurisdiction rate table. Import creates one rule per row; export emits the rules that still have that simple shape.",
		ViewPermission:          "settings.view",
		ImportPermission:        "settings.manage",
		RequireGlobalPermission: false,
		Fields:                  rateFields,
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			return &rateExporter{svc: svc, opts: o}, nil
		},
		NewImporter: func(_ context.Context, o dataio.Opts) (dataio.Importer, error) {
			if o.ChannelID == uuid.Nil {
				return nil, errors.New("a rate table is imported into one channel - pick one for this run")
			}
			return &rateImporter{svc: svc, opts: o}, nil
		},
	}
}

type rateExporter struct {
	svc   *Service
	opts  dataio.Opts
	rules []Rule
}

func (e *rateExporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	var err error
	if e.opts.ChannelID != uuid.Nil {
		e.rules, err = e.svc.ListRulesByChannelHydrated(ctx, e.opts.ChannelID)
	} else {
		e.rules, err = e.svc.ListAllRulesHydrated(ctx)
	}
	return nil, err
}

// Only rules that still match the import shape round-trip - anything hand-edited into something richer is skipped
// rather than flattened into a row that would re-import as a different rule
func (e *rateExporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	for _, rule := range e.rules {
		row, ok := rateRow(rule)
		if !ok {
			continue
		}
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

func rateRow(rule Rule) (*dataio.Row, bool) {
	var bps int32
	found := false
	for _, eff := range rule.Effects {
		if eff.Type != "core.percentage" {
			continue
		}
		var p struct {
			BPS int32 `json:"bps"`
		}
		if json.Unmarshal(eff.Params, &p) == nil {
			bps, found = p.BPS, true
		}
	}
	if !found {
		return nil, false
	}
	var country, region, class string
	for _, cond := range rule.Conditions {
		switch cond.Type {
		case "core.jurisdiction":
			var p struct {
				CountryCode string `json:"country_code"`
				RegionCode  string `json:"region_code"`
			}
			if json.Unmarshal(cond.Params, &p) == nil {
				country, region = p.CountryCode, p.RegionCode
			}
		case "core.product_tax_class":
			var p struct {
				Classes []string `json:"classes"`
			}
			if json.Unmarshal(cond.Params, &p) == nil && len(p.Classes) == 1 {
				class = p.Classes[0]
			}
		default:
			return nil, false // a condition the importer can't reproduce
		}
	}
	if country == "" {
		return nil, false
	}
	r := dataio.NewRow()
	r.Ref = rule.Name
	r.Set(colTaxName, rule.Name)
	r.Set(colTaxCountry, country)
	r.Set(colTaxRegion, region)
	r.Set(colTaxRateBPS, bps)
	r.Set(colTaxPriority, rule.Priority)
	r.Set(colTaxClass, class)
	r.Set(colTaxExclusivity, rule.ExclusivityGroup)
	return r, true
}

type rateImporter struct {
	svc  *Service
	opts dataio.Opts
}

// Every row creates a rule - there is no natural key on the rate table, so re-running a file doubles it up
func (im *rateImporter) Row(ctx context.Context, r *dataio.Row) (dataio.Outcome, error) {
	r.Ref = r.Str(colTaxName)
	if !r.RequireDeclared() {
		return dataio.OutcomeSkipped, r.Err()
	}
	country := r.Upper(colTaxCountry)
	if len(country) != 2 {
		r.Fail("%s must be an ISO-2 code", colTaxCountry)
	}
	bps := r.Int(colTaxRateBPS)
	if bps < 0 || bps > 100000 {
		r.Fail("%s must be between 0 and 100000", colTaxRateBPS)
	}
	priority := int32(100)
	if p := r.Int32(colTaxPriority); p != nil {
		priority = *p
	}
	if err := r.Err(); err != nil {
		return dataio.OutcomeSkipped, err
	}
	if im.opts.DryRun {
		return dataio.OutcomeCreated, nil
	}

	group := r.Str(colTaxExclusivity)
	if group == "" {
		group = "import"
	}
	jur, _ := json.Marshal(map[string]string{"country_code": country, "region_code": r.Upper(colTaxRegion)})
	conds := []Condition{{Type: "core.jurisdiction", Params: jur, ParamsVersion: 1, SortOrder: 0}}
	if class := r.Str(colTaxClass); class != "" {
		cls, _ := json.Marshal(map[string][]string{"classes": {class}})
		conds = append(conds, Condition{Type: "core.product_tax_class", Params: cls, ParamsVersion: 1, SortOrder: 1})
	}
	rate, _ := json.Marshal(map[string]int32{"bps": int32(bps)})

	if _, err := im.svc.CreateRule(ctx, CreateRuleInput{
		Rule: Rule{
			ChannelID:        im.opts.ChannelID,
			Name:             r.Str(colTaxName),
			Priority:         priority,
			ExclusivityGroup: group,
			IsActive:         true,
		},
		Conditions: conds,
		Effects:    []Effect{{Type: "core.percentage", Params: rate, ParamsVersion: 1, SortOrder: 0}},
	}); err != nil {
		return dataio.OutcomeSkipped, fmt.Errorf("create rule: %w", err)
	}
	return dataio.OutcomeCreated, nil
}

package tax

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// CSV rate-table import, the bulk on-ramp for jurisdictions - each row compiles to one Rule (a core.jurisdiction condition, an optional core.product_tax_class, and a core.percentage effect) that stays an ordinary editable rule afterwards A header row is required and column order doesn't matter: name and country_code and rate_bps in basis points are required, region_code empty means the whole country, priority defaults to 100, tax_class empty means all products exclusivity_group defaults to "import" so a country fallback and its state rows don't stack - pass distinct values when you do want them to

type RateImportRow struct {
	Name             string
	CountryCode      string
	RegionCode       string
	RateBPS          int32
	Priority         int32
	TaxClass         string
	ExclusivityGroup string
}

// Creates one active rule per row - NOT transactional across rows, since each CreateRule commits, so a mid-file error returns the created count alongside it and the admin knows where it stopped
func (s *Service) ImportRateTable(ctx context.Context, channelID uuid.UUID, rows []RateImportRow) (int, error) {
	created := 0
	for i, row := range rows {
		jur, _ := json.Marshal(map[string]string{
			"country_code": row.CountryCode,
			"region_code":  row.RegionCode,
		})
		conds := []Condition{{Type: "core.jurisdiction", Params: jur, ParamsVersion: 1, SortOrder: 0}}
		if row.TaxClass != "" {
			cls, _ := json.Marshal(map[string][]string{"classes": {row.TaxClass}})
			conds = append(conds, Condition{Type: "core.product_tax_class", Params: cls, ParamsVersion: 1, SortOrder: 1})
		}
		rate, _ := json.Marshal(map[string]int32{"bps": row.RateBPS})
		_, err := s.CreateRule(ctx, CreateRuleInput{
			Rule: Rule{
				ChannelID:        channelID,
				Name:             row.Name,
				Priority:         row.Priority,
				ExclusivityGroup: row.ExclusivityGroup,
				IsActive:         true,
			},
			Conditions: conds,
			Effects:    []Effect{{Type: "core.percentage", Params: rate, ParamsVersion: 1, SortOrder: 0}},
		})
		if err != nil {
			return created, fmt.Errorf("tax import: row %d (%s): %w", i+1, row.Name, err)
		}
		created++
	}
	return created, nil
}

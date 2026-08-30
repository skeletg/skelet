package tax

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// CSV rate-table import, the bulk on-ramp for jurisdictions - each row compiles to one Rule (a core.jurisdiction condition, an optional core.product_tax_class, and a core.percentage effect) that stays an ordinary editable rule afterwards
// A header row is required and column order doesn't matter: name and country_code and rate_bps in basis points are required, region_code empty means the whole country, priority defaults to 100, tax_class empty means all products
// exclusivity_group defaults to "import" so a country fallback and its state rows don't stack - pass distinct values when you do want them to

type RateImportRow struct {
	Name             string
	CountryCode      string
	RegionCode       string
	RateBPS          int32
	Priority         int32
	TaxClass         string
	ExclusivityGroup string
}

// Parses and validates the CSV - errors carry 1-based data-row numbers so the admin can fix the file
func ParseRateTableCSV(r io.Reader) ([]RateImportRow, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("tax import: read header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, required := range []string{"name", "country_code", "rate_bps"} {
		if _, ok := col[required]; !ok {
			return nil, fmt.Errorf("tax import: missing required column %q", required)
		}
	}
	field := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	var rows []RateImportRow
	for n := 1; ; n++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tax import: row %d: %w", n, err)
		}
		row := RateImportRow{
			Name:             field(rec, "name"),
			CountryCode:      strings.ToUpper(field(rec, "country_code")),
			RegionCode:       strings.ToUpper(field(rec, "region_code")),
			TaxClass:         field(rec, "tax_class"),
			ExclusivityGroup: field(rec, "exclusivity_group"),
			Priority:         100,
		}
		if row.Name == "" {
			return nil, fmt.Errorf("tax import: row %d: name is required", n)
		}
		if len(row.CountryCode) != 2 {
			return nil, fmt.Errorf("tax import: row %d: country_code must be ISO-2", n)
		}
		bps, err := strconv.Atoi(field(rec, "rate_bps"))
		if err != nil || bps < 0 || bps > 100000 {
			return nil, fmt.Errorf("tax import: row %d: rate_bps must be an integer in 0..100000", n)
		}
		row.RateBPS = int32(bps)
		if p := field(rec, "priority"); p != "" {
			pr, err := strconv.Atoi(p)
			if err != nil {
				return nil, fmt.Errorf("tax import: row %d: priority must be an integer", n)
			}
			row.Priority = int32(pr)
		}
		if row.ExclusivityGroup == "" {
			row.ExclusivityGroup = "import"
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("tax import: no data rows")
	}
	return rows, nil
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

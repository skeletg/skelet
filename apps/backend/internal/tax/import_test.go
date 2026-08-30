package tax

import (
	"strings"
	"testing"
)

func TestParseRateTableCSV(t *testing.T) {
	csvData := `name,country_code,region_code,rate_bps,priority,tax_class,exclusivity_group
NY state,US,NY,875,10,,
US fallback,US,,0,100,,
DE VAT,DE,,1900,10,,
DE reduced (books),DE,,700,5,reduced,
FR stacking eco-tax,FR,,200,10,,eco`
	rows, err := ParseRateTableCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d", len(rows))
	}
	ny := rows[0]
	if ny.CountryCode != "US" || ny.RegionCode != "NY" || ny.RateBPS != 875 || ny.Priority != 10 {
		t.Errorf("NY row parsed wrong: %+v", ny)
	}
	if rows[1].Priority != 100 && rows[1].RateBPS != 0 {
		t.Errorf("fallback row: %+v", rows[1])
	}
	if rows[3].TaxClass != "reduced" {
		t.Errorf("tax_class lost: %+v", rows[3])
	}
	if rows[0].ExclusivityGroup != "import" {
		t.Errorf("default exclusivity_group should be 'import', got %q", rows[0].ExclusivityGroup)
	}
	if rows[4].ExclusivityGroup != "eco" {
		t.Errorf("explicit exclusivity_group lost: %+v", rows[4])
	}
}

func TestParseRateTableCSVColumnOrderInsensitive(t *testing.T) {
	rows, err := ParseRateTableCSV(strings.NewReader("rate_bps,name,country_code\n2000,SE VAT,se\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rows[0].CountryCode != "SE" || rows[0].RateBPS != 2000 {
		t.Errorf("reordered columns parsed wrong: %+v", rows[0])
	}
}

func TestParseRateTableCSVErrors(t *testing.T) {
	cases := map[string]string{
		"missing column":   "name,region_code\nx,NY\n",
		"bad bps":          "name,country_code,rate_bps\nx,US,eight\n",
		"bps out of range": "name,country_code,rate_bps\nx,US,200000\n",
		"bad country":      "name,country_code,rate_bps\nx,USA,100\n",
		"empty name":       "name,country_code,rate_bps\n,US,100\n",
		"no rows":          "name,country_code,rate_bps\n",
	}
	for label, data := range cases {
		if _, err := ParseRateTableCSV(strings.NewReader(data)); err == nil {
			t.Errorf("%s: expected error", label)
		}
	}
}

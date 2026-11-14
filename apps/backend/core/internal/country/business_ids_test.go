package country

import (
	"errors"
	"testing"
)

func TestNormalizeBusinessIDFields(t *testing.T) {
	got, err := NormalizeBusinessIDFields([]BusinessIDField{
		{Label: " USt-IdNr. ", Required: true, Pattern: `DE\d{9}`, Hint: "DE and 9 digits"},
		{Label: "Numéro SIRET", Pattern: `^\d{14}$`},
	})
	if err != nil {
		t.Fatalf("NormalizeBusinessIDFields: %v", err)
	}
	if got[0].Key != "ust_idnr" || got[0].Label != "USt-IdNr." || got[1].Key != "numero_siret" {
		t.Fatalf("got %+v, want keys derived from the labels", got)
	}

	for name, fields := range map[string][]BusinessIDField{
		"no label":      {{Key: "vat"}},
		"duplicate key": {{Label: "VAT"}, {Label: "vat"}},
		"bad pattern":   {{Label: "VAT", Pattern: `(\d`}},
		"bad key":       {{Label: "VAT", Key: "Has Space"}},
	} {
		if _, err := NormalizeBusinessIDFields(fields); !errors.Is(err, ErrBusinessIDFieldInvalid) {
			t.Errorf("%s: err = %v, want ErrBusinessIDFieldInvalid", name, err)
		}
	}
}

func TestBusinessIDFieldMatchesTheWholeValue(t *testing.T) {
	f := BusinessIDField{Pattern: `\d{9}`}
	if !f.Matches("123456789") || f.Matches("1234567890") || f.Matches("12345678a") {
		t.Error("a pattern must match the whole value, anchored or not")
	}
	if !(BusinessIDField{}).Matches("anything") {
		t.Error("no pattern accepts any value")
	}
}

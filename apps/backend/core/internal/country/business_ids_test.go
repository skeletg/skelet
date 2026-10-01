package country

import (
	"errors"
	"testing"
)

func TestNormalizeBusinessIDFields(t *testing.T) {
	got, err := NormalizeBusinessIDFields([]BusinessIDField{
		{Label: " PIB ", Required: true, Pattern: `\d{9}`, Hint: "9 digits"},
		{Label: "Matični broj", Pattern: `^\d{8}$`},
	})
	if err != nil {
		t.Fatalf("NormalizeBusinessIDFields: %v", err)
	}
	if got[0].Key != "pib" || got[0].Label != "PIB" || got[1].Key != "maticni_broj" {
		t.Fatalf("got %+v, want keys derived from the labels", got)
	}

	for name, fields := range map[string][]BusinessIDField{
		"no label":      {{Key: "pib"}},
		"duplicate key": {{Label: "PIB"}, {Label: "pib"}},
		"bad pattern":   {{Label: "PIB", Pattern: `(\d`}},
		"bad key":       {{Label: "PIB", Key: "Has Space"}},
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

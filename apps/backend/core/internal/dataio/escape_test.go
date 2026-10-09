package dataio

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"unicode/utf8"
)

// A shopper-supplied value starting with a formula character must reach the spreadsheet inert, and come back unchanged on import
func TestCSVQuotesFormulaCellsAndRoundTripsThem(t *testing.T) {
	cols := []Field{{Name: "name", Type: FieldText}, {Name: "price", Type: FieldMoney}, {Name: "tags", Type: FieldList}}
	var buf bytes.Buffer
	w, err := csvCodec{}.Writer(&buf, cols)
	if err != nil {
		t.Fatal(err)
	}
	values := []string{`=HYPERLINK("http://evil","x")`, "+1 555", "-gift", "@sum", "\tsneaky", "plain", "'already quoted", "'=kept quote"}
	for _, v := range values {
		r := NewRow()
		r.Set("name", v)
		r.Set("price", -500)
		r.Set("tags", "=a|b")
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for i, rec := range recs[1:] {
		if formulaLike(rec[0]) || formulaLike(rec[2]) {
			t.Errorf("row %d left a live formula: %q / %q", i, rec[0], rec[2])
		}
		if rec[1] != "-500" {
			t.Errorf("row %d quoted a negative amount: %q", i, rec[1])
		}
	}

	rr, err := csvCodec{}.Reader(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range values {
		row, err := rr.Next()
		if err != nil {
			t.Fatal(err)
		}
		got := row.Str("name")
		if strings.TrimSpace(want) != got {
			t.Errorf("value %d came back as %q, want %q", i, got, strings.TrimSpace(want))
		}
		if row.Str("tags") != "=a|b" {
			t.Errorf("list cell came back as %q", row.Str("tags"))
		}
	}
}

func TestJSONLKeepsBigNumbersAndJoinsArrays(t *testing.T) {
	rr, err := jsonlCodec{}.Reader(strings.NewReader(`{"sku": 12345678901234567890, "price": 9007199254740993, "options": ["color", "size", 3]}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	row, err := rr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if got := row.Str("sku"); got != "12345678901234567890" {
		t.Errorf("sku = %q, want every digit", got)
	}
	if got := row.Str("price"); got != "9007199254740993" {
		t.Errorf("price = %q, want every digit", got)
	}
	if got := row.Str("options"); got != "color|size|3" {
		t.Errorf("options = %q, want a pipe-joined list", got)
	}
}

func TestJSONLRefusesANestedObject(t *testing.T) {
	for _, line := range []string{`{"sku": "A", "meta": {"x": 1}}`, `{"sku": "A", "list": [{"x": 1}]}`} {
		rr, _ := jsonlCodec{}.Reader(strings.NewReader(line + "\n"))
		if _, err := rr.Next(); err == nil {
			t.Errorf("%s was accepted, want a nested-object error", line)
		}
	}
}

func TestFloatCellsRefuseNaNAndInf(t *testing.T) {
	for _, v := range []string{"NaN", "Inf", "-Inf", "+Inf"} {
		r := NewRow()
		r.Set("qty", v)
		if r.OptFloat("qty") != nil || r.Err() == nil {
			t.Errorf("%s was accepted as a number", v)
		}
	}
}

func TestRowMessagesAreCutOnACharacter(t *testing.T) {
	msg := trimMessage("a" + strings.Repeat("ž", maxRowMessageRunes+10)) // the odd byte puts the 400th byte mid-character
	if !utf8.ValidString(msg) {
		t.Fatal("trimmed message is not valid UTF-8")
	}
	if !strings.HasSuffix(msg, "...") {
		t.Error("a long message lost its ellipsis")
	}
	if got := trimMessage("short"); got != "short" {
		t.Errorf("a short message changed to %q", got)
	}
}

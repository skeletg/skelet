package sqlnull

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUUIDVariantsDisagreeOnlyAboutNil(t *testing.T) {
	id, zero := uuid.New(), uuid.Nil
	cases := []struct {
		name string
		got  uuid.NullUUID
		want bool
	}{
		{"UUID nil", UUID(nil), false},
		{"UUID Nil keeps it", UUID(&zero), true},
		{"UUID set", UUID(&id), true},
		{"UUIDNonZero nil", UUIDNonZero(nil), false},
		{"UUIDNonZero Nil is NULL", UUIDNonZero(&zero), false},
		{"UUIDNonZero set", UUIDNonZero(&id), true},
		{"UUIDValueNonZero Nil is NULL", UUIDValueNonZero(zero), false},
		{"UUIDValueNonZero set", UUIDValueNonZero(id), true},
	}
	for _, c := range cases {
		if c.got.Valid != c.want {
			t.Errorf("%s: Valid = %v, want %v", c.name, c.got.Valid, c.want)
		}
	}
	if UUID(&id).UUID != id || UUIDNonZero(&id).UUID != id || UUIDValueNonZero(id).UUID != id {
		t.Error("a set id was not carried through")
	}
}

func TestStringVariantsDisagreeOnlyAboutEmpty(t *testing.T) {
	empty, word := "", "x"
	cases := []struct {
		name string
		got  sql.NullString
		want bool
	}{
		{"String nil", String(nil), false},
		{"String empty keeps it", String(&empty), true},
		{"StringNonEmpty nil", StringNonEmpty(nil), false},
		{"StringNonEmpty empty is NULL", StringNonEmpty(&empty), false},
		{"StringNonEmpty set", StringNonEmpty(&word), true},
		{"StringValueNonEmpty empty is NULL", StringValueNonEmpty(""), false},
		{"StringValueNonEmpty set", StringValueNonEmpty("x"), true},
	}
	for _, c := range cases {
		if c.got.Valid != c.want {
			t.Errorf("%s: Valid = %v, want %v", c.name, c.got.Valid, c.want)
		}
	}
}

func TestZeroValuesStayValid(t *testing.T) {
	var (
		tm  time.Time
		i16 int16
		i32 int32
		i64 int64
		b   bool
		f   float64
	)
	if !Time(&tm).Valid || !Int16(&i16).Valid || !Int32(&i32).Valid || !Int64(&i64).Valid || !Bool(&b).Valid || !Float64(&f).Valid {
		t.Error("a pointer to a zero value must be stored, not turned into NULL")
	}
	if Time(nil).Valid || Int16(nil).Valid || Int32(nil).Valid || Int64(nil).Valid || Bool(nil).Valid || Float64(nil).Valid {
		t.Error("a nil pointer must be NULL")
	}
}

func TestRawTreatsEmptyAsNull(t *testing.T) {
	if Raw(nil).Valid || Raw(json.RawMessage{}).Valid {
		t.Error("empty JSON must be NULL")
	}
	if got := Raw(json.RawMessage(`{}`)); !got.Valid || string(got.RawMessage) != "{}" {
		t.Errorf("Raw({}) = %+v", got)
	}
	if RawMessage(Raw(nil)) != nil || string(RawMessage(Raw([]byte(`[1]`)))) != "[1]" {
		t.Error("RawMessage did not round-trip")
	}
}

func TestReadsCopyAndNullIsNil(t *testing.T) {
	id := uuid.New()
	n := uuid.NullUUID{UUID: id, Valid: true}
	p := UUIDPtr(n)
	n.UUID = uuid.Nil
	if p == nil || *p != id {
		t.Error("UUIDPtr must copy, not alias the row")
	}
	if UUIDPtr(uuid.NullUUID{}) != nil || StringPtr(sql.NullString{}) != nil || TimePtr(sql.NullTime{}) != nil ||
		Int32Ptr(sql.NullInt32{}) != nil || Int64Ptr(sql.NullInt64{}) != nil || Float64Ptr(sql.NullFloat64{}) != nil {
		t.Error("NULL must read as nil")
	}
	if s := StringPtr(sql.NullString{String: "", Valid: true}); s == nil || *s != "" {
		t.Error("a stored empty string must read as a pointer to empty, not nil")
	}
	if StringValue(sql.NullString{}) != "" || StringValue(sql.NullString{String: "x", Valid: true}) != "x" {
		t.Error("StringValue")
	}
}

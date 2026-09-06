package admin

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
)

// Stop mode decides whether a matched rule lets the next one run, so a value that changes meaning across the hop silently stacks or blocks discounts
func TestStopModeRoundTrips(t *testing.T) {
	for _, m := range []promotion.StopMode{promotion.StopNone, promotion.StopAll, promotion.StopSameActionType} {
		gql := stopModeToGQL(m)
		if back := stopModeFromGQL(gql); back != m {
			t.Errorf("%q round-tripped through %q to %q", m, gql, back)
		}
	}
	// each domain mode needs its own enum, or two collapse into one
	seen := map[model.PromotionStopMode]promotion.StopMode{}
	for _, m := range []promotion.StopMode{promotion.StopNone, promotion.StopAll, promotion.StopSameActionType} {
		gql := stopModeToGQL(m)
		if prev, dup := seen[gql]; dup {
			t.Errorf("%q and %q both map to %q", m, prev, gql)
		}
		seen[gql] = m
	}
}

// An unrecognized value has to land on NONE, the only mode that can't suppress a rule the operator expected to run
func TestStopModeFallsBackToNone(t *testing.T) {
	if got := stopModeToGQL(promotion.StopMode("halt_everything")); got != model.PromotionStopModeNone {
		t.Errorf("an unknown domain mode = %q, want NONE", got)
	}
	if got := stopModeFromGQL(model.PromotionStopMode("HALT")); got != promotion.StopNone {
		t.Errorf("an unknown GraphQL mode = %q, want StopNone", got)
	}
	if got := stopModeFromGQLPtr(nil); got != promotion.StopNone {
		t.Errorf("a nil mode = %q, want StopNone so an omitted field never suppresses a rule", got)
	}
	all := model.PromotionStopModeAll
	if got := stopModeFromGQLPtr(&all); got != promotion.StopAll {
		t.Errorf("stopModeFromGQLPtr(&ALL) = %q, want StopAll", got)
	}
}

func TestUUIDPtrToStringPtr(t *testing.T) {
	if got := uuidPtrToStringPtr(nil); got != nil {
		t.Errorf("uuidPtrToStringPtr(nil) = %v, want nil", got)
	}
	id := uuid.New()
	got := uuidPtrToStringPtr(&id)
	if got == nil || *got != id.String() {
		t.Errorf("uuidPtrToStringPtr = %v, want %q", got, id)
	}
	nilID := uuid.Nil
	if got := uuidPtrToStringPtr(&nilID); got == nil || *got != uuid.Nil.String() {
		t.Errorf("a pointer to uuid.Nil = %v, want the zero uuid rendered rather than dropped", got)
	}
}

// An empty string from an admin form means "not set", and treating it as a parse failure would reject a legitimate clear
func TestOptionalUUID(t *testing.T) {
	id := uuid.New()
	s := id.String()
	got, err := optionalUUID(&s)
	if err != nil || got == nil || *got != id {
		t.Errorf("optionalUUID = (%v, %v), want %v", got, err, id)
	}

	empty := ""
	for name, in := range map[string]*string{"nil": nil, "empty": &empty} {
		got, err := optionalUUID(in)
		if err != nil {
			t.Errorf("%s: optionalUUID = %v, want no error", name, err)
		}
		if got != nil {
			t.Errorf("%s: optionalUUID = %v, want nil for an unset field", name, got)
		}
	}

	bad := "not-a-uuid"
	if got, err := optionalUUID(&bad); err == nil {
		t.Errorf("optionalUUID(%q) = %v, want a parse error", bad, got)
	}
}

// Nil means "not supplied" everywhere in these inputs, and each deref has to fall back to the value the field's own semantics call for
func TestDerefHelpers(t *testing.T) {
	b, i := true, int32(7)

	if got := derefBoolDefault(nil, true); !got {
		t.Error("derefBoolDefault(nil, true) = false, want the default honored")
	}
	if got := derefBoolDefault(nil, false); got {
		t.Error("derefBoolDefault(nil, false) = true, want the default honored")
	}
	if got := derefBoolDefault(&b, false); !got {
		t.Error("an explicit true was overridden by the default")
	}
	no := false
	if got := derefBoolDefault(&no, true); got {
		t.Error("an explicit false was overridden by the default - the whole point of the pointer")
	}

	if got := derefInt32Default(nil, 42); got != 42 {
		t.Errorf("derefInt32Default(nil, 42) = %d, want 42", got)
	}
	if got := derefInt32Default(&i, 42); got != 7 {
		t.Errorf("derefInt32Default = %d, want the supplied 7", got)
	}
	zero := int32(0)
	if got := derefInt32Default(&zero, 42); got != 0 {
		t.Errorf("an explicit 0 became %d, want 0 rather than the default", got)
	}
}

// Falling back to the list index is what keeps a batch of conditions in the order the admin arranged them when the client omits sortOrder
func TestDerefInt32WithIndex(t *testing.T) {
	for idx := range 4 {
		if got := derefInt32WithIndex(nil, idx); got != int32(idx) {
			t.Errorf("derefInt32WithIndex(nil, %d) = %d, want %d", idx, got, idx)
		}
	}
	explicit := int32(99)
	if got := derefInt32WithIndex(&explicit, 0); got != 99 {
		t.Errorf("derefInt32WithIndex = %d, want the supplied 99", got)
	}
	zero := int32(0)
	if got := derefInt32WithIndex(&zero, 3); got != 0 {
		t.Errorf("an explicit 0 at index 3 became %d, want 0", got)
	}
}

// Condition params round-trip through the JSON scalar, so a value that changes shape on the way out breaks the rule editor's re-load
func TestAnyToRawMessageAndBack(t *testing.T) {
	for name, in := range map[string]any{
		"object": map[string]any{"kinds": []any{"bundle", "simple"}},
		"array":  []any{float64(1), float64(2)},
		"string": "plain",
		"number": float64(42),
		"bool":   true,
	} {
		raw, err := anyToRawMessage(in)
		if err != nil {
			t.Errorf("%s: anyToRawMessage = %v", name, err)
			continue
		}
		if got := rawMessageToAny(raw); !reflect.DeepEqual(got, in) {
			t.Errorf("%s: round-tripped to %#v, want %#v", name, got, in)
		}
	}
}

// Nil has to become the JSON literal null rather than empty bytes, which would make the column invalid JSON
func TestAnyToRawMessageEncodesNilAsJSONNull(t *testing.T) {
	raw, err := anyToRawMessage(nil)
	if err != nil {
		t.Fatalf("anyToRawMessage(nil): %v", err)
	}
	if string(raw) != "null" {
		t.Errorf("anyToRawMessage(nil) = %q, want %q", raw, "null")
	}
	if !json.Valid(raw) {
		t.Errorf("anyToRawMessage(nil) = %q, which is not valid JSON", raw)
	}
}

// A caller that already holds raw JSON must not get it double-encoded into a string
func TestAnyToRawMessagePassesRawJSONThrough(t *testing.T) {
	in := json.RawMessage(`{"kinds":["bundle"]}`)
	raw, err := anyToRawMessage(in)
	if err != nil {
		t.Fatalf("anyToRawMessage: %v", err)
	}
	if string(raw) != string(in) {
		t.Errorf("anyToRawMessage = %q, want it passed through as %q", raw, in)
	}
}

// Both empty bytes and the null literal are GraphQL null - a stored empty column must not read as the string ""
func TestRawMessageToAnyTreatsEmptyAndNullAsNil(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"nil":     nil,
		"empty":   json.RawMessage(""),
		"literal": json.RawMessage("null"),
	} {
		if got := rawMessageToAny(raw); got != nil {
			t.Errorf("%s: rawMessageToAny = %#v, want nil", name, got)
		}
	}
}

// A column holding something that isn't JSON must surface as a readable string rather than crashing the query
func TestRawMessageToAnyFallsBackToTheRawString(t *testing.T) {
	raw := json.RawMessage("{not json")
	got := rawMessageToAny(raw)
	if got != "{not json" {
		t.Errorf("rawMessageToAny = %#v, want the raw string back", got)
	}
}

// Codes are pasted in bulk out of a spreadsheet, so the parser absorbs blank lines, comments and case while refusing to import the same code twice
func TestParseCSVCodes(t *testing.T) {
	got := parseCSVCodes("SAVE10\nsave20\n\n  SAVE30  \n# a comment\nSAVE10\nsave10\n")
	want := []string{"SAVE10", "SAVE20", "SAVE30"}
	if len(got) != len(want) {
		t.Fatalf("parseCSVCodes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseCSVCodes = %v, want %v", got, want)
		}
	}
}

// Codes are matched uppercase at redemption, so a lowercase paste has to normalize or the shopper's code never applies
func TestParseCSVCodesUppercasesAndDeduplicates(t *testing.T) {
	got := parseCSVCodes("save10\nSAVE10\nSaVe10")
	if len(got) != 1 || got[0] != "SAVE10" {
		t.Errorf("parseCSVCodes = %v, want one normalized SAVE10", got)
	}
}

func TestParseCSVCodesSkipsCommentsAndBlanks(t *testing.T) {
	for name, in := range map[string]string{
		"empty":         "",
		"only blanks":   "\n\n   \n\t\n",
		"only comments": "# header\n#SAVE10\n",
	} {
		if got := parseCSVCodes(in); len(got) != 0 {
			t.Errorf("%s: parseCSVCodes = %v, want nothing", name, got)
		}
	}
	// a code that merely contains a hash is not a comment
	if got := parseCSVCodes("SAVE#10"); len(got) != 1 || got[0] != "SAVE#10" {
		t.Errorf("parseCSVCodes = %v, want the code kept - only a leading hash is a comment", got)
	}
}

// The result feeds a bulk insert, so it must never be nil
func TestParseCSVCodesNeverReturnsNil(t *testing.T) {
	if got := parseCSVCodes(""); got == nil {
		t.Error("parseCSVCodes(\"\") = nil, want an empty slice")
	}
}

// Both analytics bounds are inclusive, so leaving them blank has to mean all-time rather than an empty window
func TestDefaultAnalyticsRangeSpansAllTimeWhenUnset(t *testing.T) {
	before := time.Now()
	s, e := defaultAnalyticsRange(nil, nil)
	if !s.Equal(time.Unix(0, 0)) {
		t.Errorf("start = %v, want the epoch", s)
	}
	if e.Before(before) {
		t.Errorf("end = %v, want it at or after now (%v)", e, before)
	}
	if !s.Before(e) {
		t.Errorf("range = %v..%v, want a non-empty window", s, e)
	}
}

func TestDefaultAnalyticsRangeHonorsSuppliedBounds(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC)

	s, e := defaultAnalyticsRange(&start, &end)
	if !s.Equal(start) || !e.Equal(end) {
		t.Errorf("range = %v..%v, want %v..%v", s, e, start, end)
	}

	// one side supplied leaves the other at its default
	s, e = defaultAnalyticsRange(&start, nil)
	if !s.Equal(start) {
		t.Errorf("start = %v, want %v", s, start)
	}
	if e.Before(start) {
		t.Errorf("end = %v, want it after the supplied start", e)
	}
	s, e = defaultAnalyticsRange(nil, &end)
	if !s.Equal(time.Unix(0, 0)) || !e.Equal(end) {
		t.Errorf("range = %v..%v, want epoch..%v", s, e, end)
	}
}

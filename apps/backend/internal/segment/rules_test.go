package segment

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseRulesValidation(t *testing.T) {
	bad := []string{
		`{"conditions":[]}`,
		`{"conditions":[{"kind":"nope"}]}`,
		`{"conditions":[{"kind":"trait","field":"password","op":"eq","value":1}]}`,
		`{"conditions":[{"kind":"trait","field":"rfm_r","op":"like","value":1}]}`,
		`{"conditions":[{"kind":"lifecycle","op":"eq","stage":"vip"}]}`,
		`{"conditions":[{"kind":"group"}]}`,
		`{"logic":"xor","conditions":[{"kind":"group","slug":"x"}]}`,
		`{"conditions":[{"kind":"trait","field":"rfm_r","op":"eq","value":1,"extra":"nope"}]}`,
	}
	for _, b := range bad {
		if _, err := ParseRules(json.RawMessage(b)); err == nil {
			t.Errorf("accepted invalid rules: %s", b)
		}
	}
	good := `{"logic":"or","conditions":[
		{"kind":"trait","field":"rfm_m","op":"gte","value":4},
		{"kind":"order_count","op":"gte","value":3,"days":90},
		{"kind":"consent","channel":"marketing_email","granted":true},
		{"kind":"group","slug":"wholesale"}
	]}`
	if _, err := ParseRules(json.RawMessage(good)); err != nil {
		t.Fatalf("rejected valid rules: %v", err)
	}
}

func TestCompileParameterizes(t *testing.T) {
	r, err := ParseRules(json.RawMessage(`{"conditions":[
		{"kind":"trait","field":"rfm_r","op":"gte","value":4},
		{"kind":"total_spent_cents","op":"gte","value":50000,"days":365},
		{"kind":"lifecycle","op":"eq","stage":"active"},
		{"kind":"group","slug":"wholesale"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	// The two trait conditions resolve against the signals database first, so their ids arrive as bind args
	traitIDs := []uuid.UUID{uuid.New(), uuid.New()}
	sqlStr, args := r.Compile(map[int][]uuid.UUID{0: traitIDs, 2: traitIDs})
	// Every operator value must be a bind arg, never inlined
	if strings.Contains(sqlStr, "50000") || strings.Contains(sqlStr, "wholesale") || strings.Contains(sqlStr, "active") {
		t.Fatalf("value leaked into SQL text: %s", sqlStr)
	}
	// customer_traits moved to the signals database, so a trait condition can no longer be a subquery here
	for _, gone := range []string{"customer_traits", "rfm_r", "lifecycle_stage"} {
		if strings.Contains(sqlStr, gone) {
			t.Errorf("trait condition still reaches across databases via %q: %s", gone, sqlStr)
		}
	}
	for _, frag := range []string{"c.id = ANY(", "SUM(o.total_amount)", "make_interval", "customer_groups"} {
		if !strings.Contains(sqlStr, frag) {
			t.Fatalf("missing fragment %q in: %s", frag, sqlStr)
		}
	}
	if len(args) != 5 { // 365, 50000, wholesale, and one id array per trait condition
		t.Fatalf("want 5 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(sqlStr, " AND ") {
		t.Fatal("default logic must be AND")
	}

	// An unresolved trait condition must match nobody rather than everybody - an unreachable signals database
	// narrowing a segment is survivable, silently widening it to the whole customer base is not
	unresolved, _ := r.Compile(nil)
	if !strings.Contains(unresolved, "FALSE") {
		t.Errorf("an unresolved trait condition did not refuse: %s", unresolved)
	}

	// OR logic joins with OR
	r2, _ := ParseRules(json.RawMessage(`{"logic":"or","conditions":[
		{"kind":"group","slug":"a"},{"kind":"group","slug":"b"}]}`))
	s2, _ := r2.Compile(nil)
	if !strings.Contains(s2, " OR ") {
		t.Fatal("or logic must join with OR")
	}
}

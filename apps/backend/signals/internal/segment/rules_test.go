package segment

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

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
		`{"conditions":[{"kind":"consent","channel":"Not A Slug","granted":true}]}`,
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
	// The windowed spend condition was answered by the spine first, so its ids arrive as one bind arg
	spineIDs := []uuid.UUID{uuid.New(), uuid.New()}
	sqlStr, args := r.Compile(map[int]Resolved{1: {IDs: spineIDs}})
	// Every operator value must be a bind arg, never inlined
	if strings.Contains(sqlStr, "50000") || strings.Contains(sqlStr, "wholesale") || strings.Contains(sqlStr, "active") {
		t.Fatalf("value leaked into SQL text: %s", sqlStr)
	}
	// Everything reads the suite's own projections - the commerce tables the rules once joined are not reachable from here
	for _, gone := range []string{"FROM customers", "orders o", "customer_groups", "customer_consents"} {
		if strings.Contains(sqlStr, gone) {
			t.Errorf("rule still reaches into commerce via %q: %s", gone, sqlStr)
		}
	}
	for _, frag := range []string{"FROM customer_roster c", "FROM customer_traits t", "c.customer_id = ANY(", "c.customer_group_slug = "} {
		if !strings.Contains(sqlStr, frag) {
			t.Fatalf("missing fragment %q in: %s", frag, sqlStr)
		}
	}
	if len(args) != 4 { // 4, the id array, active, wholesale
		t.Fatalf("want 4 args, got %d: %v", len(args), args)
	}
	if !strings.Contains(sqlStr, " AND ") {
		t.Fatal("default logic must be AND")
	}

	// A windowed condition the spine did not answer must match nobody rather than everybody - an unreachable spine
	// narrowing a segment is survivable, silently widening it to the whole roster is not
	unresolved, _ := r.Compile(nil)
	if !strings.Contains(unresolved, "FALSE") {
		t.Errorf("an unresolved windowed condition did not refuse: %s", unresolved)
	}

	// "At most 5 orders" is true of everyone the spine has no row for, so it compiles as the exceptions - and with no
	// exceptions, as everyone
	atMost, _ := ParseRules(json.RawMessage(`{"conditions":[{"kind":"order_count","op":"lte","value":5,"days":30}]}`))
	if s, _ := atMost.Compile(map[int]Resolved{0: {Exclude: true, IDs: spineIDs}}); !strings.Contains(s, "NOT (c.customer_id = ANY(") {
		t.Errorf("an exclusion did not compile as NOT ANY: %s", s)
	}
	if s, _ := atMost.Compile(map[int]Resolved{0: {Exclude: true}}); !strings.Contains(s, "TRUE") {
		t.Errorf("an exclusion with nobody to exclude did not compile as TRUE: %s", s)
	}

	// OR logic joins with OR
	r2, _ := ParseRules(json.RawMessage(`{"logic":"or","conditions":[
		{"kind":"group","slug":"a"},{"kind":"group","slug":"b"}]}`))
	s2, _ := r2.Compile(nil)
	if !strings.Contains(s2, " OR ") {
		t.Fatal("or logic must join with OR")
	}
}

// The spine as a fixture - what each channel's customers ordered from the asked-for instant on
type fakeOrders struct {
	byChannel map[uuid.UUID][]CustomerOrderStat
	calls     int
}

func (f *fakeOrders) CustomerOrdersSince(_ context.Context, channelID uuid.UUID, _ time.Time) ([]CustomerOrderStat, error) {
	f.calls++
	return f.byChannel[channelID], nil
}

// Only RosterChannels is reached on the way to the spine - the embedded nil interface answers for the rest
type rosterOnly struct {
	Repository
	channels []uuid.UUID
}

func (r rosterOnly) RosterChannels(context.Context) ([]uuid.UUID, error) { return r.channels, nil }

func TestWindowedConditionsResolveAgainstTheSpine(t *testing.T) {
	chA, chB := uuid.New(), uuid.New()
	heavy, light, other := uuid.New(), uuid.New(), uuid.New()
	spine := &fakeOrders{byChannel: map[uuid.UUID][]CustomerOrderStat{
		chA: {{CustomerID: heavy, OrderCount: 3, MonetaryCents: 9000}, {CustomerID: light, OrderCount: 1, MonetaryCents: 500}},
		chB: {{CustomerID: other, OrderCount: 7, MonetaryCents: 100}},
	}}
	svc := &Service{repo: rosterOnly{channels: []uuid.UUID{chA, chB}}, orders: spine}
	ctx := context.Background()

	resolve := func(raw string) map[int]Resolved {
		t.Helper()
		r, err := ParseRules(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		out, err := svc.resolveWindowedConditions(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	same := func(got, want []uuid.UUID) bool {
		slices.SortFunc(got, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		return slices.Equal(got, want)
	}

	// "At least 3 orders" selects those the spine shows clearing it, across every channel
	got := resolve(`{"conditions":[{"kind":"order_count","op":"gte","value":3,"days":30}]}`)[0]
	if got.Exclude || !same(got.IDs, []uuid.UUID{heavy, other}) {
		t.Errorf("at least 3 orders = %+v, want heavy and other selected", got)
	}
	// "At most 5 orders" is true of everyone with no orders in the window, so it comes back as who exceeded it
	got = resolve(`{"conditions":[{"kind":"order_count","op":"lte","value":5,"days":30}]}`)[0]
	if !got.Exclude || !same(got.IDs, []uuid.UUID{other}) {
		t.Errorf("at most 5 orders = %+v, want only other excluded", got)
	}
	// Spend reads the other column
	got = resolve(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":1000,"days":7}]}`)[0]
	if got.Exclude || !same(got.IDs, []uuid.UUID{heavy}) {
		t.Errorf("spend >= 1000 = %+v, want heavy alone", got)
	}
	// Zero spend is what a customer with no row has, and none of these three qualify
	got = resolve(`{"conditions":[{"kind":"total_spent_cents","op":"eq","value":0,"days":7}]}`)[0]
	if !got.Exclude || !same(got.IDs, []uuid.UUID{heavy, light, other}) {
		t.Errorf("spend == 0 = %+v, want all three excluded", got)
	}

	// Two conditions on one window share the read - one call per channel, not per condition
	spine.calls = 0
	out := resolve(`{"conditions":[
		{"kind":"order_count","op":"gte","value":1,"days":90},
		{"kind":"total_spent_cents","op":"gte","value":1,"days":90},
		{"kind":"group","slug":"wholesale"}]}`)
	if spine.calls != 2 {
		t.Errorf("the spine was read %d times for two conditions on one window over two channels, want 2", spine.calls)
	}
	if _, ok := out[2]; ok {
		t.Error("a group condition was resolved against the spine")
	}

	// Without a spine the condition cannot be answered, and says so rather than matching nobody in silence
	r, _ := ParseRules(json.RawMessage(`{"conditions":[{"kind":"order_count","op":"gte","value":1,"days":30}]}`))
	if _, err := (&Service{repo: rosterOnly{}}).resolveWindowedConditions(ctx, r); err == nil {
		t.Error("a windowed condition resolved with no spine wired")
	}
	// And a rule set with no windowed condition never reaches for one
	static, _ := ParseRules(json.RawMessage(`{"conditions":[{"kind":"order_count","op":"gte","value":1}]}`))
	if out, err := (&Service{repo: rosterOnly{}}).resolveWindowedConditions(ctx, static); err != nil || out != nil {
		t.Errorf("a lifetime condition touched the spine: %v %v", out, err)
	}
}

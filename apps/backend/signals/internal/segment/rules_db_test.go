package segment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Compile hand-assembles SQL text from allowlisted fragments, and nothing else in the package ever runs the result
// so a renamed column, a trait added to the allowlist without its migration, or a bad cast compiles fine, passes the string-fragment assertions, and only fails at refresh time, where it reads as "the segment is empty" rather than as an error
// Every table a rule reads lives in the signals database, so that is the only one these tests open

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.RequireSignals(t)
}

func mustRules(t *testing.T, raw string) *Rules {
	t.Helper()
	r, err := ParseRules(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("ParseRules(%s): %v", raw, err)
	}
	return r
}

// Runs the compiled query and returns the customer ids it selects - resolved stands in for the spine's answer to any windowed condition
func run(t *testing.T, conn *sql.DB, r *Rules, resolved map[int]Resolved) []uuid.UUID {
	t.Helper()
	query, args := r.Compile(resolved)
	rows, err := conn.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("Postgres rejected the compiled query: %v\n%s\nargs: %v", err, query, args)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read rows: %v", err)
	}
	return out
}

func selects(ids []uuid.UUID, want uuid.UUID) bool { return slices.Contains(ids, want) }

// A customer as core pushes them: on a channel, in a group or not, with both consents already resolved
type rosterRow struct {
	channelID uuid.UUID
	groupSlug string
	marketing bool
	sms       bool
	consents  map[string]bool
}

func seedRoster(t *testing.T, conn *sql.DB, row rosterRow) uuid.UUID {
	t.Helper()
	id := uuid.New()
	var groupID any
	if row.groupSlug != "" {
		groupID = uuid.New()
	}
	consents := map[string]bool{"marketing_email": row.marketing, "marketing_sms": row.sms}
	maps.Copy(consents, row.consents)
	consentsJSON, _ := json.Marshal(consents)
	if _, err := conn.Exec(
		`INSERT INTO customer_roster (customer_id, channel_id, email, is_active, customer_group_id, customer_group_slug, marketing_opted_in, sms_opted_in, consents)
		 VALUES ($1, $2, $3, true, $4, $5, $6, $7, $8)`,
		id, row.channelID, "segrule-"+id.String()[:8]+"@example.invalid", groupID, row.groupSlug, row.marketing, row.sms, consentsJSON); err != nil {
		t.Fatalf("seed roster row: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_roster WHERE customer_id = $1`, id) })
	return id
}

func seedTraits(t *testing.T, conn *sql.DB, customerID, channelID uuid.UUID, stage string, monetary int64, rfmM int) {
	t.Helper()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_traits WHERE customer_id = $1`, customerID) })
	if _, err := conn.Exec(
		`INSERT INTO customer_traits (customer_id, channel_id, lifecycle_stage, monetary_cents, rfm_m, frequency)
		 VALUES ($1, $2, $3, $4, $5, 3)`,
		customerID, channelID, stage, monetary, rfmM); err != nil {
		t.Fatalf("seed traits: %v", err)
	}
}

// The lifetime projection trait compute lands off the spine - count and spend already netted against cancellations
func seedOrders(t *testing.T, conn *sql.DB, customerID, channelID uuid.UUID, count int, cents int64, lastAgeDays int) {
	t.Helper()
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_order_agg WHERE customer_id = $1`, customerID) })
	if _, err := conn.Exec(
		`INSERT INTO customer_order_agg (customer_id, channel_id, order_count, monetary_cents, first_order_at, last_order_at)
		 VALUES ($1, $2, $3, $4, NOW() - make_interval(days => $5::int + 100), NOW() - make_interval(days => $5::int))`,
		customerID, channelID, count, cents, lastAgeDays); err != nil {
		t.Fatalf("seed order aggregate: %v", err)
	}
}

// Answers every windowed condition with a throwaway id, so the compiled text of each branch reaches Postgres
func stubResolved(r *Rules) map[int]Resolved {
	out := map[int]Resolved{}
	for i := range r.WindowedConditions() {
		out[i] = Resolved{IDs: []uuid.UUID{uuid.New()}}
	}
	return out
}

func TestEveryAllowlistedTraitFieldHasAColumnBehindIt(t *testing.T) {
	// traitFields is extended by hand whenever a predictive column lands - an entry without its
	// migration, or a typo in either, makes every segment using it silently select nobody
	conn := testDB(t)
	if len(traitFields) == 0 {
		t.Fatal("the allowlist is empty, so this check would pass vacuously")
	}
	for field := range traitFields {
		t.Run(field, func(t *testing.T) {
			run(t, conn, mustRules(t, fmt.Sprintf(`{"conditions":[{"kind":"trait","field":%q,"op":"gte","value":1,"channelId":%q}]}`, field, uuid.NewString())), nil)
		})
	}
}

func TestEveryConditionKindAndOperatorExecutes(t *testing.T) {
	// Each kind is its own hand-written fragment, and the windowed variants build a second one - so
	// every branch needs Postgres to have parsed it at least once
	conn := testDB(t)
	anyChannel := uuid.NewString()
	for name, raw := range map[string]string{
		"trait, every numeric op": `{"conditions":[
			{"kind":"trait","field":"rfm_r","op":"eq","value":1},
			{"kind":"trait","field":"rfm_f","op":"neq","value":1},
			{"kind":"trait","field":"rfm_m","op":"gt","value":1},
			{"kind":"trait","field":"frequency","op":"gte","value":1},
			{"kind":"trait","field":"monetary_cents","op":"lt","value":1,"channelId":"` + anyChannel + `"},
			{"kind":"trait","field":"engagement_score","op":"lte","value":1}]}`,
		"lifecycle eq":          `{"conditions":[{"kind":"lifecycle","op":"eq","stage":"active"}]}`,
		"lifecycle neq":         `{"conditions":[{"kind":"lifecycle","op":"neq","stage":"lapsed"}]}`,
		"order count, all time": `{"conditions":[{"kind":"order_count","op":"gte","value":1}]}`,
		"order count, windowed": `{"conditions":[{"kind":"order_count","op":"lte","value":5,"days":90}]}`,
		"spend, all time":       `{"conditions":[{"kind":"total_spent_cents","op":"eq","value":0,"channelId":"` + anyChannel + `"}]}`,
		"spend, windowed":       `{"conditions":[{"kind":"total_spent_cents","op":"gte","value":1,"days":365,"channelId":"` + anyChannel + `"}]}`,
		"last order, at most":   `{"conditions":[{"kind":"last_order_days","op":"lte","value":30}]}`,
		"last order, at least":  `{"conditions":[{"kind":"last_order_days","op":"gte","value":30}]}`,
		"email consent granted": `{"conditions":[{"kind":"consent","channel":"marketing_email","granted":true}]}`,
		"sms consent withheld":  `{"conditions":[{"kind":"consent","channel":"marketing_sms","granted":false}]}`,
		"group":                 `{"conditions":[{"kind":"group","slug":"wholesale"}]}`,
		"everything joined by and": `{"conditions":[
			{"kind":"trait","field":"rfm_m","op":"gte","value":1},
			{"kind":"lifecycle","op":"eq","stage":"active"},
			{"kind":"order_count","op":"gte","value":1,"days":30},
			{"kind":"total_spent_cents","op":"gte","value":1,"days":30,"channelId":"` + anyChannel + `"},
			{"kind":"last_order_days","op":"lte","value":30},
			{"kind":"consent","channel":"marketing_email","granted":true},
			{"kind":"group","slug":"wholesale"}]}`,
		"everything joined by or": `{"logic":"or","conditions":[
			{"kind":"trait","field":"churn_risk","op":"gte","value":80},
			{"kind":"group","slug":"wholesale"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := mustRules(t, raw)
			run(t, conn, r, stubResolved(r))
		})
	}
	// The exclusion form a windowed "at most" compiles to, with and without exceptions
	atMost := mustRules(t, `{"conditions":[{"kind":"order_count","op":"lte","value":5,"days":90}]}`)
	run(t, conn, atMost, map[int]Resolved{0: {Exclude: true, IDs: []uuid.UUID{uuid.New()}}})
	run(t, conn, atMost, map[int]Resolved{0: {Exclude: true}})
}

func TestHostileValuesStayInsideBindArgs(t *testing.T) {
	// The group slug is the one free-text field a marketer types that reaches the query at all - inlined
	// instead of bound, each of these ends the predicate early
	conn := testDB(t)
	for name, raw := range map[string]string{
		"a quote-breaking group slug": `{"conditions":[{"kind":"group","slug":"' OR 1=1 --"}]}`,
		"a statement in a slug":       `{"conditions":[{"kind":"group","slug":"x'; DROP TABLE customer_roster; --"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := run(t, conn, mustRules(t, raw), nil); len(got) != 0 {
				t.Errorf("the rule selected %d customers, want none - the value was interpreted rather than compared", len(got))
			}
		})
	}
	// A consent kind is a registry slug or nothing - anything else fails validation before it could reach the query
	if _, err := ParseRules(json.RawMessage(`{"conditions":[{"kind":"consent","channel":"' UNION SELECT customer_id FROM customer_roster --","granted":true}]}`)); err == nil {
		t.Error("a consent kind that is not a slug passed validation")
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM customer_roster`).Scan(&n); err != nil {
		t.Fatalf("customer_roster is gone after the injection attempts: %v", err)
	}
}

func TestTraitAndGroupRulesSelectTheMatchingCustomer(t *testing.T) {
	conn := testDB(t)
	channelID, otherChannel := uuid.New(), uuid.New()
	groupSlug := "wholesale-" + uuid.NewString()[:8]
	matching := seedRoster(t, conn, rosterRow{channelID: channelID, groupSlug: groupSlug})
	other := seedRoster(t, conn, rosterRow{channelID: otherChannel})
	seedTraits(t, conn, matching, channelID, "active", 50000, 5)
	seedTraits(t, conn, other, otherChannel, "lapsed", 100, 1)

	for name, tc := range map[string]struct {
		raw       string
		wantMatch bool
		wantOther bool
	}{
		"a trait threshold the customer clears": {
			`{"conditions":[{"kind":"trait","field":"rfm_m","op":"gte","value":5}]}`, true, false,
		},
		"a trait threshold nobody clears": {
			`{"conditions":[{"kind":"trait","field":"rfm_m","op":"gte","value":99}]}`, false, false,
		},
		"a lifecycle stage": {
			`{"conditions":[{"kind":"lifecycle","op":"eq","stage":"active"}]}`, true, false,
		},
		"a negated lifecycle stage": {
			`{"conditions":[{"kind":"lifecycle","op":"neq","stage":"active"}]}`, false, true,
		},
		"both, joined by and": {
			`{"conditions":[
				{"kind":"trait","field":"rfm_m","op":"gte","value":5},
				{"kind":"lifecycle","op":"eq","stage":"lapsed"}]}`, false, false,
		},
		"either, joined by or": {
			`{"logic":"or","conditions":[
				{"kind":"trait","field":"rfm_m","op":"gte","value":5},
				{"kind":"lifecycle","op":"eq","stage":"lapsed"}]}`, true, true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := run(t, conn, mustRules(t, tc.raw), nil)
			if selects(got, matching) != tc.wantMatch {
				t.Errorf("selected the matching customer = %v, want %v", selects(got, matching), tc.wantMatch)
			}
			if selects(got, other) != tc.wantOther {
				t.Errorf("selected the other customer = %v, want %v", selects(got, other), tc.wantOther)
			}
		})
	}

	inGroup := run(t, conn, mustRules(t, fmt.Sprintf(`{"conditions":[{"kind":"group","slug":%q}]}`, groupSlug)), nil)
	if !selects(inGroup, matching) || selects(inGroup, other) {
		t.Error("the group rule did not select exactly the member of that group")
	}
	notInGroup := run(t, conn, mustRules(t, `{"conditions":[{"kind":"group","slug":"no-such-group"}]}`), nil)
	if selects(notInGroup, matching) {
		t.Error("an unknown group slug still selected somebody")
	}
}

func TestOrderRulesReadTheProjection(t *testing.T) {
	// Lifetime order conditions read customer_order_agg, which trait compute projects off the spine with
	// cancellations already netted out - so what the rule sees is what the RFM scores beside it were built from
	conn := testDB(t)
	channelID := uuid.New()
	shopper := seedRoster(t, conn, rosterRow{channelID: channelID})
	bystander := seedRoster(t, conn, rosterRow{channelID: channelID})
	seedOrders(t, conn, shopper, channelID, 2, 100000, 5)

	for name, tc := range map[string]struct {
		raw  string
		want bool
	}{
		"two orders all time":             {`{"conditions":[{"kind":"order_count","op":"eq","value":2}]}`, true},
		"not three":                       {`{"conditions":[{"kind":"order_count","op":"gte","value":3}]}`, false},
		"spend at the projected total":    {`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":100000,"channelId":"` + channelID.String() + `"}]}`, true},
		"spend one cent over it":          {`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":100001,"channelId":"` + channelID.String() + `"}]}`, false},
		"spend on another channel":        {`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":1,"channelId":"` + uuid.NewString() + `"}]}`, false},
		"ordered within the last 30 days": {`{"conditions":[{"kind":"last_order_days","op":"lte","value":30}]}`, true},
		"has not ordered for 30 days":     {`{"conditions":[{"kind":"last_order_days","op":"gte","value":30}]}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := run(t, conn, mustRules(t, tc.raw), nil)
			if selects(got, shopper) != tc.want {
				t.Errorf("selected the shopper = %v, want %v", selects(got, shopper), tc.want)
			}
			if selects(got, bystander) {
				t.Error("a customer with no orders was selected by a positive order rule")
			}
		})
	}

	// A windowed condition composes with the projection ones through the spine's answer - here, who it says ordered recently
	recentAndBig := mustRules(t, `{"conditions":[
		{"kind":"order_count","op":"gte","value":1,"days":30},
		{"kind":"total_spent_cents","op":"gte","value":100000,"channelId":"`+channelID.String()+`"}]}`)
	if got := run(t, conn, recentAndBig, map[int]Resolved{0: {IDs: []uuid.UUID{shopper}}}); !selects(got, shopper) {
		t.Error("the shopper the spine named was not selected alongside their lifetime spend")
	}
	if got := run(t, conn, recentAndBig, map[int]Resolved{0: {IDs: []uuid.UUID{bystander}}}); selects(got, shopper) {
		t.Error("the shopper was selected although the spine did not name them")
	}
	// The exclusion form: everyone but who the spine says exceeded the window
	atMostOne := mustRules(t, `{"conditions":[{"kind":"order_count","op":"lte","value":1,"days":30}]}`)
	if got := run(t, conn, atMostOne, map[int]Resolved{0: {Exclude: true, IDs: []uuid.UUID{shopper}}}); selects(got, shopper) || !selects(got, bystander) {
		t.Error("an exclusion did not keep everyone but the named customer")
	}
}

func TestACustomerWhoNeverOrderedIsNotDormantByAge(t *testing.T) {
	// last_order_days reads the projection's last_order_at, which is NULL with no row, so the predicate is NULL rather than true - a "hasn't ordered in 90 days" win-back segment therefore covers lapsed buyers only, never people who never bought, and the empty-set surprise belongs here rather than in a marketer's send report
	conn := testDB(t)
	neverOrdered := seedRoster(t, conn, rosterRow{channelID: uuid.New()})

	for name, raw := range map[string]string{
		"at least 90 days since the last order": `{"conditions":[{"kind":"last_order_days","op":"gte","value":90}]}`,
		"within 90 days of the last order":      `{"conditions":[{"kind":"last_order_days","op":"lte","value":90}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if selects(run(t, conn, mustRules(t, raw), nil), neverOrdered) {
				t.Error("a customer with no orders was selected by an order-recency rule")
			}
		})
	}
	// The rule that does reach them is a zero-order count, so a win-back segment has to say so
	if !selects(run(t, conn, mustRules(t, `{"conditions":[{"kind":"order_count","op":"eq","value":0}]}`), nil), neverOrdered) {
		t.Error("a zero order_count rule missed the customer who never ordered")
	}
}

func TestConsentRulesSplitOnTheResolvedFlag(t *testing.T) {
	conn := testDB(t)
	ch := uuid.New()
	emailIn := seedRoster(t, conn, rosterRow{channelID: ch, marketing: true})
	smsIn := seedRoster(t, conn, rosterRow{channelID: ch, sms: true})
	neither := seedRoster(t, conn, rosterRow{channelID: ch})

	granted := run(t, conn, mustRules(t, `{"conditions":[{"kind":"consent","channel":"marketing_email","granted":true}]}`), nil)
	if !selects(granted, emailIn) || selects(granted, smsIn) || selects(granted, neither) {
		t.Error("the email opt-in rule did not select exactly the customer who opted in to email")
	}
	withheld := run(t, conn, mustRules(t, `{"conditions":[{"kind":"consent","channel":"marketing_email","granted":false}]}`), nil)
	if selects(withheld, emailIn) || !selects(withheld, smsIn) || !selects(withheld, neither) {
		t.Error("the email opt-out rule did not select everyone without email consent")
	}
	sms := run(t, conn, mustRules(t, `{"conditions":[{"kind":"consent","channel":"marketing_sms","granted":true}]}`), nil)
	if !selects(sms, smsIn) || selects(sms, emailIn) {
		t.Error("the SMS rule read the email flag")
	}

	optedOut := seedRoster(t, conn, rosterRow{channelID: ch, consents: map[string]bool{"transactional_email": false}})
	txn := run(t, conn, mustRules(t, `{"conditions":[{"kind":"consent","channel":"transactional_email","granted":true}]}`), nil)
	if selects(txn, optedOut) || !selects(txn, emailIn) || !selects(txn, neither) {
		t.Error("transactional_email did not split on the stored answer with the opt-in default for everyone else")
	}
	declined := run(t, conn, mustRules(t, `{"conditions":[{"kind":"consent","channel":"transactional_email","granted":false}]}`), nil)
	if !selects(declined, optedOut) || selects(declined, neither) {
		t.Error("a customer with no transactional answer read as opted out, want the registry's opt-in default")
	}
}

func TestTheDynamicSweepStartsWithTheLongestUnrefreshed(t *testing.T) {
	conn := testDB(t)
	ctx := context.Background()
	repo := NewRepository(db.New(conn))
	mk := func(slug string) uuid.UUID {
		var id uuid.UUID
		if err := conn.QueryRow(`INSERT INTO customer_segments (slug, name, rules) VALUES ($1, $1, '{"conditions":[{"kind":"group","slug":"x"}]}') RETURNING id`, slug).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customer_segments WHERE id = $1`, id) })
		return id
	}
	refreshed := mk("sweep-a-" + uuid.NewString()[:8])
	missed := mk("sweep-b-" + uuid.NewString()[:8])
	if err := repo.RecordDailySnapshot(ctx, refreshed); err != nil {
		t.Fatal(err)
	}
	dyn, err := repo.ListDynamic(ctx)
	if err != nil {
		t.Fatal(err)
	}
	at := map[uuid.UUID]int{}
	for i, s := range dyn {
		at[s.ID] = i
	}
	if at[missed] > at[refreshed] {
		t.Error("the segment refreshed today comes first, so a sweep that times out starves the one it missed every time")
	}
}

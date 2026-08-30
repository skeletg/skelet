package segment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/internal/testdb"
)

// Compile hand-assembles SQL text from allowlisted fragments, and nothing else in the package ever
// runs the result - so a renamed column, a trait added to the allowlist without its migration, or a
// bad cast compiles fine, passes the string-fragment assertions, and only fails at refresh time,
// where it reads as "the segment is empty" rather than as an error
//
// These execute every fragment against Postgres, then check the rows a rule actually selects

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

func mustRules(t *testing.T, raw string) *Rules {
	t.Helper()
	r, err := ParseRules(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("ParseRules(%s): %v", raw, err)
	}
	return r
}

// Runs the compiled query and returns the customer ids it selects
func run(t *testing.T, conn *sql.DB, r *Rules) []uuid.UUID {
	t.Helper()
	query, args := r.Compile()
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

func seedCustomer(t *testing.T, conn *sql.DB) (customerID, channelID, groupID uuid.UUID) {
	t.Helper()
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRow(`SELECT id FROM channels LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	if err := conn.QueryRow(
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`,
		"segrule-"+uuid.NewString()+"@example.invalid", groupID, channelID).Scan(&customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, customerID) }) // traits, consents and orders cascade
	return customerID, channelID, groupID
}

func seedTraits(t *testing.T, conn *sql.DB, customerID, channelID uuid.UUID, stage string, monetary int64, rfmM int) {
	t.Helper()
	if _, err := conn.Exec(
		`INSERT INTO customer_traits (customer_id, channel_id, lifecycle_stage, monetary_cents, rfm_m, frequency)
		 VALUES ($1, $2, $3, $4, $5, 3)`,
		customerID, channelID, stage, monetary, rfmM); err != nil {
		t.Fatalf("seed traits: %v", err)
	}
}

func seedOrder(t *testing.T, conn *sql.DB, customerID, channelID uuid.UUID, cents int64, status string, ageDays int) {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO orders (channel_id, customer_id, currency_code, total_amount, status, created_at)
		 VALUES ($1, $2, 'USD', $3, $4, NOW() - make_interval(days => $5::int)) RETURNING id`,
		channelID, customerID, cents, status, ageDays).Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, id) })
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
			run(t, conn, mustRules(t, fmt.Sprintf(`{"conditions":[{"kind":"trait","field":%q,"op":"gte","value":1}]}`, field)))
		})
	}
}

func TestEveryConditionKindAndOperatorExecutes(t *testing.T) {
	// Each kind is its own hand-written fragment, and the windowed variants build a second one - so
	// every branch needs Postgres to have parsed it at least once
	conn := testDB(t)
	for name, raw := range map[string]string{
		"trait, every numeric op": `{"conditions":[
			{"kind":"trait","field":"rfm_r","op":"eq","value":1},
			{"kind":"trait","field":"rfm_f","op":"neq","value":1},
			{"kind":"trait","field":"rfm_m","op":"gt","value":1},
			{"kind":"trait","field":"frequency","op":"gte","value":1},
			{"kind":"trait","field":"monetary_cents","op":"lt","value":1},
			{"kind":"trait","field":"engagement_score","op":"lte","value":1}]}`,
		"lifecycle eq":          `{"conditions":[{"kind":"lifecycle","op":"eq","stage":"active"}]}`,
		"lifecycle neq":         `{"conditions":[{"kind":"lifecycle","op":"neq","stage":"dormant"}]}`,
		"order count, all time": `{"conditions":[{"kind":"order_count","op":"gte","value":1}]}`,
		"order count, windowed": `{"conditions":[{"kind":"order_count","op":"lte","value":5,"days":90}]}`,
		"spend, all time":       `{"conditions":[{"kind":"total_spent_cents","op":"eq","value":0}]}`,
		"spend, windowed":       `{"conditions":[{"kind":"total_spent_cents","op":"gte","value":1,"days":365}]}`,
		"last order, at most":   `{"conditions":[{"kind":"last_order_days","op":"lte","value":30}]}`,
		"last order, at least":  `{"conditions":[{"kind":"last_order_days","op":"gte","value":30}]}`,
		"consent granted":       `{"conditions":[{"kind":"consent","channel":"marketing_email","granted":true}]}`,
		"consent withheld":      `{"conditions":[{"kind":"consent","channel":"marketing_email","granted":false}]}`,
		"group":                 `{"conditions":[{"kind":"group","slug":"wholesale"}]}`,
		"everything joined by and": `{"conditions":[
			{"kind":"trait","field":"rfm_m","op":"gte","value":1},
			{"kind":"lifecycle","op":"eq","stage":"active"},
			{"kind":"order_count","op":"gte","value":1,"days":30},
			{"kind":"total_spent_cents","op":"gte","value":1,"days":30},
			{"kind":"last_order_days","op":"lte","value":30},
			{"kind":"consent","channel":"marketing_email","granted":true},
			{"kind":"group","slug":"wholesale"}]}`,
		"everything joined by or": `{"logic":"or","conditions":[
			{"kind":"trait","field":"churn_risk","op":"gte","value":80},
			{"kind":"group","slug":"wholesale"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			run(t, conn, mustRules(t, raw))
		})
	}
}

func TestHostileValuesStayInsideBindArgs(t *testing.T) {
	// These are the free-text fields a marketer types, so they are the only operator input that
	// reaches the query at all - inlined instead of bound, each of these ends the predicate early
	conn := testDB(t)
	for name, raw := range map[string]string{
		"a quote-breaking group slug": `{"conditions":[{"kind":"group","slug":"' OR 1=1 --"}]}`,
		"a statement in a slug":       `{"conditions":[{"kind":"group","slug":"x'; DROP TABLE customers; --"}]}`,
		"a consent channel injection": `{"conditions":[{"kind":"consent","channel":"' UNION SELECT id FROM customers --","granted":true}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := run(t, conn, mustRules(t, raw)); len(got) != 0 {
				t.Errorf("the rule selected %d customers, want none - the value was interpreted rather than compared", len(got))
			}
		})
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM customers`).Scan(&n); err != nil {
		t.Fatalf("customers table is gone after the injection attempts: %v", err)
	}
}

func TestTraitAndGroupRulesSelectTheMatchingCustomer(t *testing.T) {
	conn := testDB(t)
	matching, channelID, groupID := seedCustomer(t, conn)
	other, otherChannel, _ := seedCustomer(t, conn)
	seedTraits(t, conn, matching, channelID, "active", 50000, 5)
	seedTraits(t, conn, other, otherChannel, "dormant", 100, 1)

	var groupSlug string
	if err := conn.QueryRow(`SELECT slug FROM customer_groups WHERE id = $1`, groupID).Scan(&groupSlug); err != nil {
		t.Fatalf("read group slug: %v", err)
	}

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
				{"kind":"lifecycle","op":"eq","stage":"dormant"}]}`, false, false,
		},
		"either, joined by or": {
			`{"logic":"or","conditions":[
				{"kind":"trait","field":"rfm_m","op":"gte","value":5},
				{"kind":"lifecycle","op":"eq","stage":"dormant"}]}`, true, true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := run(t, conn, mustRules(t, tc.raw))
			if selects(got, matching) != tc.wantMatch {
				t.Errorf("selected the matching customer = %v, want %v", selects(got, matching), tc.wantMatch)
			}
			if selects(got, other) != tc.wantOther {
				t.Errorf("selected the other customer = %v, want %v", selects(got, other), tc.wantOther)
			}
		})
	}

	inGroup := run(t, conn, mustRules(t, fmt.Sprintf(`{"conditions":[{"kind":"group","slug":%q}]}`, groupSlug)))
	if !selects(inGroup, matching) {
		t.Error("the group rule missed a member of that group")
	}
	notInGroup := run(t, conn, mustRules(t, `{"conditions":[{"kind":"group","slug":"no-such-group"}]}`))
	if selects(notInGroup, matching) {
		t.Error("an unknown group slug still selected somebody")
	}
}

func TestOrderRulesReadTheLedgerRatherThanTheTraitSnapshot(t *testing.T) {
	// Trait rules run off a daily recompute while order rules are live, which is the whole reason both
	// exist - and the ledger fragment has to exclude the statuses that never became revenue
	conn := testDB(t)
	shopper, channelID, _ := seedCustomer(t, conn)
	seedOrder(t, conn, shopper, channelID, 30000, "completed", 5)
	seedOrder(t, conn, shopper, channelID, 70000, "completed", 200)
	seedOrder(t, conn, shopper, channelID, 500000, "cancelled", 5)
	seedOrder(t, conn, shopper, channelID, 900000, "draft", 5)

	for name, tc := range map[string]struct {
		raw  string
		want bool
	}{
		"two countable orders all time":       {`{"conditions":[{"kind":"order_count","op":"eq","value":2}]}`, true},
		"cancelled and draft are not counted": {`{"conditions":[{"kind":"order_count","op":"gte","value":3}]}`, false},
		"one order inside a 30 day window":    {`{"conditions":[{"kind":"order_count","op":"eq","value":1,"days":30}]}`, true},
		"spend sums the countable orders":     {`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":100000}]}`, true},
		"spend excludes the cancelled one":    {`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":100001}]}`, false},
		"windowed spend sees only the recent": {`{"conditions":[{"kind":"total_spent_cents","op":"eq","value":30000,"days":30}]}`, true},
		"ordered within the last 30 days":     {`{"conditions":[{"kind":"last_order_days","op":"lte","value":30}]}`, true},
		"has not ordered for 30 days":         {`{"conditions":[{"kind":"last_order_days","op":"gte","value":30}]}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := selects(run(t, conn, mustRules(t, tc.raw)), shopper); got != tc.want {
				t.Errorf("selected the shopper = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestACustomerWhoNeverOrderedIsNotDormantByAge(t *testing.T) {
	// last_order_days compares against MAX(created_at), which is NULL with no orders, so the predicate
	// is NULL rather than true - a "hasn't ordered in 90 days" win-back segment therefore covers
	// lapsed buyers only, never people who never bought, and the empty-set surprise belongs here
	// rather than in a marketer's send report
	conn := testDB(t)
	neverOrdered, _, _ := seedCustomer(t, conn)

	for name, raw := range map[string]string{
		"at least 90 days since the last order": `{"conditions":[{"kind":"last_order_days","op":"gte","value":90}]}`,
		"within 90 days of the last order":      `{"conditions":[{"kind":"last_order_days","op":"lte","value":90}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if selects(run(t, conn, mustRules(t, raw)), neverOrdered) {
				t.Error("a customer with no orders was selected by an order-recency rule")
			}
		})
	}
	// The rule that does reach them is a zero-order count, so a win-back segment has to say so
	if !selects(run(t, conn, mustRules(t, `{"conditions":[{"kind":"order_count","op":"eq","value":0}]}`)), neverOrdered) {
		t.Error("a zero order_count rule missed the customer who never ordered")
	}
}

func TestConsentRulesSplitOnTheStoredFlag(t *testing.T) {
	// Campaign audiences gate on this, so an inverted or missing-row case sends marketing to someone
	// who opted out
	conn := testDB(t)
	optedIn, _, _ := seedCustomer(t, conn)
	optedOut, _, _ := seedCustomer(t, conn)
	noRecord, _, _ := seedCustomer(t, conn)
	kind := "seg-rule-" + uuid.NewString()[:8]
	for id, granted := range map[uuid.UUID]bool{optedIn: true, optedOut: false} {
		if _, err := conn.Exec(
			`INSERT INTO customer_consents (customer_id, kind, opted_in) VALUES ($1, $2, $3)`,
			id, kind, granted); err != nil {
			t.Fatalf("seed consent: %v", err)
		}
	}

	granted := run(t, conn, mustRules(t, fmt.Sprintf(`{"conditions":[{"kind":"consent","channel":%q,"granted":true}]}`, kind)))
	withheld := run(t, conn, mustRules(t, fmt.Sprintf(`{"conditions":[{"kind":"consent","channel":%q,"granted":false}]}`, kind)))

	if !selects(granted, optedIn) || selects(granted, optedOut) || selects(granted, noRecord) {
		t.Error("the opted-in rule did not select exactly the customer who opted in")
	}
	if !selects(withheld, optedOut) || selects(withheld, optedIn) {
		t.Error("the opted-out rule did not select exactly the customer who opted out")
	}
	if selects(withheld, noRecord) {
		t.Error("a customer with no consent row was selected by an explicit opted-out rule; absence is not a stored 'false'")
	}
}

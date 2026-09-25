package referral

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// A referral payout mints real store credit out of nothing on a customer-triggered event, which makes it the most abusable path in the program: register throwaway accounts under your own code, place an order each and cancel, and every loop is free money unless the reversal claws it back

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Require(t)
}

type fakeOrderLookup struct {
	currency string
	err      error
}

func (f fakeOrderLookup) GetByID(context.Context, uuid.UUID) (order.Order, error) {
	return order.Order{CurrencyCode: f.currency}, f.err
}

// The services wired the way main.go does, plus the channel the program config is scoped to
func newFixture(t *testing.T, conn *sql.DB) (*Service, *storecredit.Service, uuid.UUID) {
	t.Helper()
	queries, txMgr := dbsqlc.New(conn), txrunner.New(conn)
	credit := storecredit.NewService(queries, txMgr)
	svc := NewService(queries, txMgr, config.NewService(configstore.New(queries, txMgr)), credit)
	svc.SetOrderLookup(fakeOrderLookup{currency: "USD"})

	var channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to scope the program to: %v", err)
	}
	return svc, credit, channelID
}

func seedCustomer(t *testing.T, conn *sql.DB) uuid.UUID {
	t.Helper()
	var groupID, channelID uuid.UUID
	if err := conn.QueryRow(`SELECT id FROM customer_groups LIMIT 1`).Scan(&groupID); err != nil {
		t.Skipf("no customer group to seed against: %v", err)
	}
	if err := conn.QueryRow(`SELECT id FROM channels ORDER BY created_at LIMIT 1`).Scan(&channelID); err != nil {
		t.Skipf("no channel to seed against: %v", err)
	}
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO customers (email, password_hash, customer_group_id, channel_id)
		 VALUES ($1, 'x', $2, $3) RETURNING id`,
		"referral-"+uuid.NewString()+"@example.invalid", groupID, channelID).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(`DELETE FROM store_credit_entries WHERE customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM referrals WHERE referrer_customer_id = $1 OR referee_customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM referral_codes WHERE customer_id = $1`, id)
		_, _ = conn.Exec(`DELETE FROM customers WHERE id = $1`, id)
	})
	return id
}

// referrals.order_id points at a real order, and the reversal reads the order to learn the currency
func seedOrder(t *testing.T, conn *sql.DB, channelID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := conn.QueryRow(
		`INSERT INTO orders (channel_id, currency_code) VALUES ($1, 'USD') RETURNING id`,
		channelID).Scan(&id); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, id) })
	return id
}

func setProgram(t *testing.T, conn *sql.DB, channelID uuid.UUID, enabled bool, referrerCents, refereeCents string) {
	t.Helper()
	on := "false"
	if enabled {
		on = "true"
	}
	for path, value := range map[string]string{
		"sales/referrals/enabled":               on,
		"sales/referrals/referrer_reward_cents": referrerCents,
		"sales/referrals/referee_reward_cents":  refereeCents,
	} {
		if _, err := conn.Exec(
			`INSERT INTO config_values (path, scope, channel_id, value) VALUES ($1, 'channel', $2, $3)
			 ON CONFLICT (scope, channel_id, COALESCE(locale, ''), path) DO UPDATE SET value = excluded.value`,
			path, channelID, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
		p, c := path, channelID
		t.Cleanup(func() {
			_, _ = conn.Exec(`DELETE FROM config_values WHERE path = $1 AND channel_id = $2`, p, c)
		})
	}
}

// A referrer holding a code and a referee attached to it, the state the first order rewards
func seedPendingReferral(t *testing.T, conn *sql.DB, svc *Service) (referrer, referee uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	referrer, referee = seedCustomer(t, conn), seedCustomer(t, conn)
	code, err := svc.CodeFor(ctx, referrer)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}
	if err := svc.Attach(ctx, referee, code); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return referrer, referee
}

func statusOf(t *testing.T, conn *sql.DB, referee uuid.UUID) string {
	t.Helper()
	var status string
	if err := conn.QueryRow(`SELECT status FROM referrals WHERE referee_customer_id = $1`, referee).Scan(&status); err != nil {
		t.Fatalf("read referral status: %v", err)
	}
	return status
}

func balance(t *testing.T, credit *storecredit.Service, customerID uuid.UUID) int64 {
	t.Helper()
	bal, err := credit.Balance(context.Background(), customerID, "USD")
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return bal
}

func TestCodeForMintsOnceAndStaysStable(t *testing.T) {
	// The code goes into share links and word of mouth, so re-minting would orphan every invite
	// already sent - and a second row would make the code-to-owner lookup ambiguous
	conn := testDB(t)
	svc, _, _ := newFixture(t, conn)
	ctx := context.Background()
	customerID := seedCustomer(t, conn)

	first, err := svc.CodeFor(ctx, customerID)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}
	for range 3 {
		again, err := svc.CodeFor(ctx, customerID)
		if err != nil {
			t.Fatalf("repeat CodeFor: %v", err)
		}
		if again != first {
			t.Fatalf("code changed from %q to %q", first, again)
		}
	}
	var rows int
	if err := conn.QueryRow(`SELECT count(*) FROM referral_codes WHERE customer_id = $1`, customerID).Scan(&rows); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if rows != 1 {
		t.Errorf("code rows = %d, want 1", rows)
	}
}

func TestAttachRefusesTheCodesThatWouldMintFreeCredit(t *testing.T) {
	// Self-referral is the one-account version of the fraud loop, and it reaches Attach through the
	// ordinary register flow whenever someone pastes their own code
	conn := testDB(t)
	svc, _, _ := newFixture(t, conn)
	ctx := context.Background()
	owner, referee := seedCustomer(t, conn), seedCustomer(t, conn)
	code, err := svc.CodeFor(ctx, owner)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}

	if err := svc.Attach(ctx, owner, code); err == nil || !strings.Contains(err.Error(), "self-referral") {
		t.Errorf("self-referral err = %v, want the refusal", err)
	}
	if err := svc.Attach(ctx, referee, "ZZZZZZZZ"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("unknown code err = %v, want the refusal", err)
	}
	if err := svc.Attach(ctx, referee, strings.ToLower("  "+code+"  ")); err != nil {
		t.Fatalf("Attach with the code as a shopper would paste it: %v", err)
	}
	var attached uuid.UUID
	if err := conn.QueryRow(`SELECT referrer_customer_id FROM referrals WHERE referee_customer_id = $1`, referee).Scan(&attached); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if attached != owner {
		t.Errorf("referral points at %v, want the code's owner %v", attached, owner)
	}
}

func TestASecondAttachCannotRepointAnExistingReferral(t *testing.T) {
	// One referral per referee, ever - the repeat is silently ignored rather than an error, since the
	// register flow calls this on every signup, but it must never move the payout to another code
	conn := testDB(t)
	svc, _, _ := newFixture(t, conn)
	ctx := context.Background()
	first, second, referee := seedCustomer(t, conn), seedCustomer(t, conn), seedCustomer(t, conn)
	firstCode, err := svc.CodeFor(ctx, first)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}
	secondCode, err := svc.CodeFor(ctx, second)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}

	if err := svc.Attach(ctx, referee, firstCode); err != nil {
		t.Fatalf("first Attach: %v", err)
	}
	for _, code := range []string{firstCode, secondCode} {
		if err := svc.Attach(ctx, referee, code); err != nil {
			t.Errorf("repeat Attach with %q: err = %v, want a silent no-op", code, err)
		}
	}

	var rows int
	if err := conn.QueryRow(`SELECT count(*) FROM referrals WHERE referee_customer_id = $1`, referee).Scan(&rows); err != nil {
		t.Fatalf("count referrals: %v", err)
	}
	var attached uuid.UUID
	if err := conn.QueryRow(
		`SELECT referrer_customer_id FROM referrals WHERE referee_customer_id = $1`, referee,
	).Scan(&attached); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if rows != 1 {
		t.Errorf("referral rows = %d, want exactly one", rows)
	}
	if attached != first {
		t.Errorf("referral now points at %v, want the original referrer %v", attached, first)
	}
}

func TestTheFirstOrderPaysBothSidesOnce(t *testing.T) {
	// OrderPlaced is at-least-once through the outbox, so the status flip is what stops a redelivery
	// from granting the reward again
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	placed := &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"}
	for range 3 {
		_ = svc.onOrderPlaced(ctx, placed)
	}

	if got := balance(t, credit, referrer); got != 1000 {
		t.Errorf("referrer balance = %d, want 1000 granted exactly once", got)
	}
	if got := balance(t, credit, referee); got != 500 {
		t.Errorf("referee balance = %d, want 500 granted exactly once", got)
	}
	if got := statusOf(t, conn, referee); got != "rewarded" {
		t.Errorf("referral status = %q, want rewarded", got)
	}
}

func TestNoRewardWhileTheProgramIsOff(t *testing.T) {
	// The program ships disabled, so an order placed before an operator turns it on must not pay out -
	// and must leave the referral pending so it still can later
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, false, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})

	if got := balance(t, credit, referrer); got != 0 {
		t.Errorf("referrer balance = %d with the program off", got)
	}
	if got := statusOf(t, conn, referee); got != "pending" {
		t.Errorf("referral status = %q, want it left pending", got)
	}
}

func TestOnlyTheRefereesOwnFirstOrderPaysOut(t *testing.T) {
	// The reward is for bringing a new customer, so a REFERRER placing their own order must not trigger
	// it, and neither must a second order by the referee
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{
		OrderID: seedOrder(t, conn, channelID), CustomerID: &referrer, ChannelID: channelID, Currency: "USD",
	})
	if got := balance(t, credit, referrer); got != 0 {
		t.Fatalf("the referrer's own order paid out %d", got)
	}
	if got := statusOf(t, conn, referee); got != "pending" {
		t.Fatalf("referral status = %q, want it still pending", got)
	}

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{
		OrderID: seedOrder(t, conn, channelID), CustomerID: &referee, ChannelID: channelID, Currency: "USD",
	})
	before := balance(t, credit, referrer)
	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{
		OrderID: seedOrder(t, conn, channelID), CustomerID: &referee, ChannelID: channelID, Currency: "USD",
	})
	if got := balance(t, credit, referrer); got != before {
		t.Errorf("a second order by the referee paid out again: %d then %d", before, got)
	}
}

func TestAGuestOrderIsNotAReferralTrigger(t *testing.T) { // a nil customer id has no referral to look up
	conn := testDB(t)
	svc, _, channelID := newFixture(t, conn)
	setProgram(t, conn, channelID, true, "1000", "500")

	_ = svc.onOrderPlaced(context.Background(), &order.OrderPlaced{
		OrderID: seedOrder(t, conn, channelID), ChannelID: channelID, Currency: "USD",
	})
}

func TestCancellingTheQualifyingOrderClawsBothRewardsBack(t *testing.T) {
	// This is the loop the reversal exists for: without it, throwaway accounts under one code placing and cancelling an
	// order each mint credit forever, and the referral stays rewarded so nothing ever notices
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})
	if balance(t, credit, referrer) != 1000 {
		t.Fatal("the reward never landed, so the reversal proves nothing")
	}

	for range 3 { // the cancel event is at-least-once too, and a replayed clawback would overdraw
		_ = svc.onOrderCancelled(ctx, &order.OrderCancelled{OrderID: orderID, ChannelID: channelID})
	}

	if got := balance(t, credit, referrer); got != 0 {
		t.Errorf("referrer balance = %d after the cancel, want it clawed back to 0", got)
	}
	if got := balance(t, credit, referee); got != 0 {
		t.Errorf("referee balance = %d after the cancel, want it clawed back to 0", got)
	}
	if got := statusOf(t, conn, referee); got == "rewarded" {
		t.Error("the referral is still marked rewarded, so nothing records that it was undone")
	}
}

func TestAFullRefundReversesLikeACancel(t *testing.T) { // else the cancel loop just becomes a refund loop
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})
	_ = svc.onOrderRefunded(ctx, &order.OrderRefunded{OrderID: orderID, ChannelID: channelID, FullyRefunded: true})

	if got := balance(t, credit, referrer); got != 0 {
		t.Errorf("referrer balance = %d after a full refund, want 0", got)
	}
}

func TestAPartialRefundLeavesThePayoutAlone(t *testing.T) {
	// The reward is all-or-nothing, so a genuine customer returning one item of several must not cost
	// the referrer their payout
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})
	_ = svc.onOrderRefunded(ctx, &order.OrderRefunded{
		OrderID: orderID, ChannelID: channelID, MerchandiseFraction: 0.4, FullyRefunded: false,
	})

	if got := balance(t, credit, referrer); got != 1000 {
		t.Errorf("referrer balance = %d after a partial refund, want the payout kept", got)
	}
	if got := statusOf(t, conn, referee); got != "rewarded" {
		t.Errorf("referral status = %q, want it still rewarded", got)
	}
}

func TestCancellingAnUnrelatedOrderIsANoOp(t *testing.T) { // most cancellations have no referral behind them
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	qualifying := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: qualifying, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})
	_ = svc.onOrderCancelled(ctx, &order.OrderCancelled{OrderID: seedOrder(t, conn, channelID), ChannelID: channelID})

	if got := balance(t, credit, referrer); got != 1000 {
		t.Errorf("referrer balance = %d, want the unrelated cancel ignored", got)
	}
	if got := statusOf(t, conn, referee); got != "rewarded" {
		t.Errorf("referral status = %q, want it untouched", got)
	}
}

func TestASpentRewardIsClawedBackToZeroNotIntoDebt(t *testing.T) {
	// The clawback is clamped at the current balance - taking the full reward back off a customer who
	// already spent it would leave them owing the store money with no record of why
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})
	if _, err := credit.Adjust(ctx, referrer, -600, "USD", "spent on an order", nil); err != nil {
		t.Fatalf("spend part of the reward: %v", err)
	}
	if got := balance(t, credit, referrer); got != 400 {
		t.Fatalf("balance before the reversal = %d, want 400", got)
	}

	_ = svc.onOrderCancelled(ctx, &order.OrderCancelled{OrderID: orderID, ChannelID: channelID})

	if got := balance(t, credit, referrer); got != 0 {
		t.Errorf("referrer balance = %d after clawing back a spent reward, want 0 rather than a negative", got)
	}
}

func TestTheReversalRefusesToGuessTheCurrency(t *testing.T) {
	// Clawing back in the wrong currency would take a different amount of real money than was granted,
	// so an unreadable order has to abort and leave the payout in place for a retry
	conn := testDB(t)
	svc, credit, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)
	orderID := seedOrder(t, conn, channelID)

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{OrderID: orderID, CustomerID: &referee, ChannelID: channelID, Currency: "USD"})
	svc.SetOrderLookup(fakeOrderLookup{currency: ""})
	_ = svc.onOrderCancelled(ctx, &order.OrderCancelled{OrderID: orderID, ChannelID: channelID})

	if got := balance(t, credit, referrer); got != 1000 {
		t.Errorf("referrer balance = %d, want the payout untouched when the currency is unknown", got)
	}
	if got := statusOf(t, conn, referee); got != "rewarded" {
		t.Errorf("referral status = %q, want it left rewarded so a retry can still reverse it", got)
	}
}

func TestStatsCountWhatTheCustomerEarned(t *testing.T) {
	conn := testDB(t)
	svc, _, channelID := newFixture(t, conn)
	ctx := context.Background()
	setProgram(t, conn, channelID, true, "1000", "500")
	referrer, referee := seedPendingReferral(t, conn, svc)

	pending, err := svc.StatsFor(ctx, referrer)
	if err != nil {
		t.Fatalf("StatsFor: %v", err)
	}
	if pending.Referred != 1 || pending.Rewarded != 0 || pending.EarnedCents != 0 {
		t.Errorf("before the order stats = %+v, want one referred and nothing earned", pending)
	}

	_ = svc.onOrderPlaced(ctx, &order.OrderPlaced{
		OrderID: seedOrder(t, conn, channelID), CustomerID: &referee, ChannelID: channelID, Currency: "USD",
	})
	rewarded, err := svc.StatsFor(ctx, referrer)
	if err != nil {
		t.Fatalf("StatsFor: %v", err)
	}
	if rewarded.Referred != 1 || rewarded.Rewarded != 1 || rewarded.EarnedCents != 1000 {
		t.Errorf("after the order stats = %+v, want one rewarded referral worth 1000", rewarded)
	}
}

func TestACodeOnlyRefersOnItsOwnersChannel(t *testing.T) {
	conn := testDB(t)
	svc, _, _ := newFixture(t, conn)
	ctx := context.Background()
	owner, referee := seedCustomer(t, conn), seedCustomer(t, conn)
	var other uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'other', 'USD') RETURNING id`, "ref-"+uuid.NewString()[:8]).Scan(&other); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, other) })
	if _, err := conn.Exec(`UPDATE customers SET channel_id = $1 WHERE id = $2`, other, referee); err != nil {
		t.Fatalf("move referee: %v", err)
	}
	code, err := svc.CodeFor(ctx, owner)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}
	if err := svc.Attach(ctx, referee, code); err == nil {
		t.Fatal("a code from another channel attached, so that channel's program pays the other channel's referrer")
	}
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM referrals WHERE referee_customer_id = $1`, referee).Scan(&n); err != nil || n != 0 {
		t.Fatalf("referrals for the cross-channel referee = %d, %v", n, err)
	}
}

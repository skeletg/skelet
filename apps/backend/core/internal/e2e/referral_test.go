package e2e

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// enableReferrals turns the give-get program on for one channel at the reward amounts the scenario asserts
func (h *harness) enableReferrals(t *testing.T, f *store, referrerCents, refereeCents string) {
	t.Helper()
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &f.Channel.ID}
	for path, val := range map[string]string{
		"sales/referrals/enabled":               "true",
		"sales/referrals/referrer_reward_cents": referrerCents,
		"sales/referrals/referee_reward_cents":  refereeCents,
	} {
		if _, err := h.D.Config.Set(h.Ctx, path, val, scope); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
}

func (h *harness) newReferralPair(t *testing.T, f *store) (referrer, referee uuid.UUID, refereeEmail string) {
	t.Helper()
	rEmail := "referrer-" + uuid.NewString()[:8] + "@e2e.test"
	eEmail := "referee-" + uuid.NewString()[:8] + "@e2e.test"
	rc, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, rEmail, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register referrer: %v", err)
	}
	ec, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, eEmail, "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register referee: %v", err)
	}
	code, err := h.D.Referral.CodeFor(h.Ctx, rc.ID)
	if err != nil {
		t.Fatalf("mint referral code: %v", err)
	}
	if err := h.D.Referral.Attach(h.Ctx, ec.ID, code); err != nil {
		t.Fatalf("attach referral: %v", err)
	}
	return rc.ID, ec.ID, eEmail
}

func (h *harness) placeFor(t *testing.T, f *store, customerID uuid.UUID, email string, qty float64) order.Order {
	t.Helper()
	cartID := h.NewCustomerCart(t, f, customerID, f.Product.SKU, qty)
	o, err := h.TryPlace(t, f.Channel, cartID, func(in *order.PlaceOrderInput) {
		in.CustomerID = &customerID
		in.Email = email
	})
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	return o
}

// Scenario: the give-get cycle - both sides get store credit on the referee's first order, the reward is idempotent under replay, and a cancellation claws both back
func TestReferralRewardsBothSidesAndReversesOnCancel(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "500")
	referrer, referee, refereeEmail := h.newReferralPair(t, f)

	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referrer, f.Channel.CurrencyCode); bal != 0 {
		t.Fatalf("referrer credit before the first order = %d, want 0", bal)
	}

	o := h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)

	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referrer, f.Channel.CurrencyCode); bal != 1000 {
		t.Errorf("referrer credit after the referee's first order = %d, want 1000", bal)
	}
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referee, f.Channel.CurrencyCode); bal != 500 {
		t.Errorf("referee credit after their first order = %d, want 500", bal)
	}

	// OrderPlaced is at-least-once, so a replay must not pay the reward twice
	h.RearmOutbox(t, "order.placed", o.ID.String(), 1)
	h.Settle(t)
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referrer, f.Channel.CurrencyCode); bal != 1000 {
		t.Errorf("referrer credit after a replayed placement = %d, want still 1000", bal)
	}
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referee, f.Channel.CurrencyCode); bal != 500 {
		t.Errorf("referee credit after a replayed placement = %d, want still 500", bal)
	}

	// Cancelling the qualifying order takes both rewards back
	h.Cancel(t, o.ID)
	h.Settle(t)
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referrer, f.Channel.CurrencyCode); bal != 0 {
		t.Errorf("referrer credit after cancellation = %d, want it clawed back to 0", bal)
	}
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referee, f.Channel.CurrencyCode); bal != 0 {
		t.Errorf("referee credit after cancellation = %d, want it clawed back to 0", bal)
	}
}

// A second cancellation reversal would claw the credit back twice, taking the customer negative
func TestReferralReversalIsIdempotent(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "500")
	referrer, referee, refereeEmail := h.newReferralPair(t, f)

	o := h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)
	h.Cancel(t, o.ID)
	h.Settle(t)

	// re-arm the cancellation so the reversal subscriber runs a second time
	h.RearmOutbox(t, "order.cancelled", o.ID.String(), 1)
	h.Settle(t)

	for name, id := range map[string]uuid.UUID{"referrer": referrer, "referee": referee} {
		bal, err := h.D.StoreCredit.Balance(h.Ctx, id, f.Channel.CurrencyCode)
		if err != nil {
			t.Fatalf("%s balance: %v", name, err)
		}
		if bal != 0 {
			t.Errorf("%s credit after a replayed reversal = %d, want 0 rather than a double clawback", name, bal)
		}
	}
}

// The program off means no credit moves at all - an operator who never enabled referrals must not be paying rewards out of a default
func TestReferralPaysNothingWhenTheProgramIsOff(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	// deliberately NOT enabling the program
	referrer, referee, refereeEmail := h.newReferralPair(t, f)

	h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)

	for name, id := range map[string]uuid.UUID{"referrer": referrer, "referee": referee} {
		if bal, _ := h.D.StoreCredit.Balance(h.Ctx, id, f.Channel.CurrencyCode); bal != 0 {
			t.Errorf("%s credit with the program disabled = %d, want 0", name, bal)
		}
	}
}

// Only the FIRST order qualifies - the referral is marked rewarded, so a second order pays nothing more
func TestReferralRewardsOnlyTheFirstOrder(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "500")
	referrer, referee, refereeEmail := h.newReferralPair(t, f)

	h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)
	h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)

	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referrer, f.Channel.CurrencyCode); bal != 1000 {
		t.Errorf("referrer credit after the referee's second order = %d, want still 1000", bal)
	}
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referee, f.Channel.CurrencyCode); bal != 500 {
		t.Errorf("referee credit after their second order = %d, want still 500", bal)
	}
}

// A zero reward on one side is a legitimate config (give-only or get-only), and it must not block the other side's credit or write an empty ledger row
func TestReferralHonorsAOneSidedProgram(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "0") // referrer only
	referrer, referee, refereeEmail := h.newReferralPair(t, f)

	h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)

	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referrer, f.Channel.CurrencyCode); bal != 1000 {
		t.Errorf("referrer credit = %d, want 1000", bal)
	}
	if bal, _ := h.D.StoreCredit.Balance(h.Ctx, referee, f.Channel.CurrencyCode); bal != 0 {
		t.Errorf("referee credit under a give-only program = %d, want 0", bal)
	}
}

// Codes are the public handle, so they have to be stable per customer and refuse the obvious abuses
func TestReferralCodeAndAttachGuards(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "500")

	a, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, "code-a-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register a: %v", err)
	}
	b, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, "code-b-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register b: %v", err)
	}

	code, err := h.D.Referral.CodeFor(h.Ctx, a.ID)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}
	if code == "" {
		t.Fatal("CodeFor returned an empty code")
	}
	// stable, so a shared link keeps working
	again, err := h.D.Referral.CodeFor(h.Ctx, a.ID)
	if err != nil || again != code {
		t.Errorf("CodeFor = (%q, %v) on the second call, want the stable %q", again, err, code)
	}
	// and distinct per customer
	otherCode, err := h.D.Referral.CodeFor(h.Ctx, b.ID)
	if err != nil {
		t.Fatalf("CodeFor b: %v", err)
	}
	if otherCode == code {
		t.Errorf("both customers got code %q, want distinct codes", code)
	}

	if err := h.D.Referral.Attach(h.Ctx, a.ID, code); err == nil {
		t.Error("a self-referral was accepted")
	}
	if err := h.D.Referral.Attach(h.Ctx, b.ID, "NOSUCHCODE"); err == nil {
		t.Error("an unknown code was accepted")
	}
	// an empty code is "no referral", not an error - the signup form posts it blank
	if err := h.D.Referral.Attach(h.Ctx, b.ID, ""); err != nil {
		t.Errorf("an empty code = %v, want nil", err)
	}
}

// The code is typed off a card or pasted from a link, so lookup has to survive case and padding
func TestReferralAttachNormalizesTheCode(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "500")

	owner, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, "norm-o-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	joiner, err := h.D.Customer.Register(h.Ctx, f.Channel.ID, "norm-j-"+uuid.NewString()[:8]+"@e2e.test", "password123", "", "", nil)
	if err != nil {
		t.Fatalf("register joiner: %v", err)
	}
	code, err := h.D.Referral.CodeFor(h.Ctx, owner.ID)
	if err != nil {
		t.Fatalf("CodeFor: %v", err)
	}

	if err := h.D.Referral.Attach(h.Ctx, joiner.ID, "  "+lower(code)+"  "); err != nil {
		t.Fatalf("attach a lowercased and padded code: %v", err)
	}
	if got, _ := h.D.Referral.StatsFor(h.Ctx, owner.ID); got.Referred != 1 {
		t.Errorf("owner referred count = %d, want 1 - the normalized code did not resolve", got.Referred)
	}
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}

// Stats back the account page, so they have to move with the reward rather than staying at zero
func TestReferralStatsTrackRewards(t *testing.T) {
	h := newHarness(t)
	f := h.NewStore(t, 20, 2500)
	h.enableReferrals(t, f, "1000", "500")
	referrer, referee, refereeEmail := h.newReferralPair(t, f)

	before, err := h.D.Referral.StatsFor(h.Ctx, referrer)
	if err != nil {
		t.Fatalf("StatsFor: %v", err)
	}
	if before.Referred != 1 {
		t.Errorf("referred = %d before the order, want the attached 1", before.Referred)
	}
	if before.Rewarded != 0 {
		t.Errorf("rewarded = %d before the order, want 0", before.Rewarded)
	}

	h.placeFor(t, f, referee, refereeEmail, 1)
	h.Settle(t)

	after, err := h.D.Referral.StatsFor(h.Ctx, referrer)
	if err != nil {
		t.Fatalf("StatsFor: %v", err)
	}
	if after.Rewarded != 1 {
		t.Errorf("rewarded = %d after the qualifying order, want 1", after.Rewarded)
	}
	if after.EarnedCents != 1000 {
		t.Errorf("earned = %d, want the 1000 granted", after.EarnedCents)
	}
}

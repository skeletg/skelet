package promotion_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
)

// The code-level window and restricted_to_email are only checked by applyCoupon, which runs once A cart keeps its cart_applied_codes row forever, so both have to be re-checked where the code is actually consumed - eval time - or a code that expired (or was never for this shopper) still discounts
func TestEvaluateCart_CodeGates(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	tests := []struct {
		name       string
		code       promotion.Code // ID/RuleID/Code filled in by the harness
		cartEmail  string
		inputEmail string
		wantCents  int64
		wantReason string
	}{
		{
			name:      "no restrictions fires",
			wantCents: 1000,
		},
		{
			name:      "open window fires",
			code:      promotion.Code{ValidFrom: &past, ValidTo: &future},
			wantCents: 1000,
		},
		{
			name:       "code-level valid_to in the past drops the discount",
			code:       promotion.Code{ValidTo: &past},
			wantReason: "expired",
		},
		{
			name:       "code-level valid_from in the future drops the discount",
			code:       promotion.Code{ValidFrom: &future},
			wantReason: "expired",
		},
		{
			name:      "matching email fires",
			code:      promotion.Code{RestrictedToEmail: "vip@example.com"},
			cartEmail: "vip@example.com",
			wantCents: 1000,
		},
		{
			name:      "email match is case and space insensitive",
			code:      promotion.Code{RestrictedToEmail: "VIP@Example.com "},
			cartEmail: "vip@example.com",
			wantCents: 1000,
		},
		{
			name:       "wrong email drops the discount",
			code:       promotion.Code{RestrictedToEmail: "vip@example.com"},
			cartEmail:  "someone.else@example.com",
			wantReason: "reserved for a different email address",
		},
		{
			name:       "unknown email fails a restricted code closed",
			code:       promotion.Code{RestrictedToEmail: "vip@example.com"},
			wantReason: "reserved for a different email address",
		},
		{
			name:       "the caller's email wins over the cart's",
			code:       promotion.Code{RestrictedToEmail: "vip@example.com"},
			cartEmail:  "stale@example.com",
			inputEmail: "vip@example.com",
			wantCents:  1000,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
			repo.rules = []promotion.Rule{{
				ID: ruleID, Slug: "vip50", Name: "VIP 50", RequiresCode: true, IsActive: true,
			}}
			repo.actions[ruleID] = []promotion.Action{{
				ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
				Params: json.RawMessage(`{"cents": 1000}`),
			}}
			code := tc.code
			code.ID, code.RuleID, code.Code, code.IsActive = codeID, ruleID, "VIP50", true
			repo.codes["VIP50"] = code
			repo.applied[cartID] = []promotion.AppliedCodeRecord{{
				CartID: cartID, RuleID: ruleID, CodeID: &codeID, CodeText: "VIP50",
			}}
			repo.cartEmails = map[uuid.UUID]string{cartID: tc.cartEmail}

			svc := promotion.NewService(repo)
			res, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
				CartID:        cartID,
				SubtotalCents: 5000,
				CustomerEmail: tc.inputEmail,
				Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
			})
			if err != nil {
				t.Fatalf("evaluate cart: %v", err)
			}
			if got := res.Result.Ledger.SubtotalDiscountCents(); got != tc.wantCents {
				t.Errorf("discount = %d, want %d", got, tc.wantCents)
			}
			if len(res.ManuallyApplied) != 1 {
				t.Fatalf("manually applied = %+v, want 1 row", res.ManuallyApplied)
			}
			if got := res.ManuallyApplied[0].InvalidatedReason; got != tc.wantReason {
				t.Errorf("reason = %q, want %q", got, tc.wantReason)
			}
		})
	}
}

// An expired code is terminal, so the reason is persisted and the code stops being re-evaluated;
// an email mismatch is not - the shopper can still type the right address at checkout
func TestEvaluateCart_OnlyTerminalGatesArePersisted(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	tests := []struct {
		name        string
		code        promotion.Code
		wantWritten bool
	}{
		{name: "expired persists", code: promotion.Code{ValidTo: &past}, wantWritten: true},
		{name: "email mismatch stays in-memory", code: promotion.Code{RestrictedToEmail: "vip@example.com"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
			repo.rules = []promotion.Rule{{ID: ruleID, Slug: "vip50", RequiresCode: true, IsActive: true}}
			repo.actions[ruleID] = []promotion.Action{{
				ID: uuid.New(), RuleID: ruleID, Type: "core.fixed_off_subtotal",
				Params: json.RawMessage(`{"cents": 1000}`),
			}}
			code := tc.code
			code.ID, code.RuleID, code.Code, code.IsActive = codeID, ruleID, "VIP50", true
			repo.codes["VIP50"] = code
			repo.applied[cartID] = []promotion.AppliedCodeRecord{{
				CartID: cartID, RuleID: ruleID, CodeID: &codeID, CodeText: "VIP50",
			}}

			svc := promotion.NewService(repo)
			if _, err := svc.EvaluateCart(context.Background(), promotion.CartEvalInput{
				CartID:        cartID,
				SubtotalCents: 5000,
				Lines:         []promotion.CartLineView{{LineID: uuid.New(), UnitPriceCents: 5000, Quantity: 1}},
			}); err != nil {
				t.Fatalf("evaluate cart: %v", err)
			}
			_, written := repo.invalidated[ruleID]
			if written != tc.wantWritten {
				t.Errorf("persisted = %v, want %v (invalidated: %+v)", written, tc.wantWritten, repo.invalidated)
			}
		})
	}
}

// ApplyCode is the shopper-facing gate - it has to name the reason rather than silently accepting a code the engine will drop
func TestApplyCode_RestrictedToEmail(t *testing.T) {
	tests := []struct {
		name      string
		cartEmail string
		wantErr   error
	}{
		{name: "matching email applies", cartEmail: "vip@example.com"},
		{name: "wrong email is rejected", cartEmail: "other@example.com", wantErr: promotion.ErrCodeEmailRestricted},
		{name: "guest with no email is rejected", wantErr: promotion.ErrCodeEmailRestricted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
			repo.rules = []promotion.Rule{{ID: ruleID, Slug: "vip50", RequiresCode: true, IsActive: true}}
			repo.codes["VIP50"] = promotion.Code{
				ID: codeID, RuleID: ruleID, Code: "VIP50", IsActive: true,
				RestrictedToEmail: "vip@example.com",
			}
			repo.cartEmails = map[uuid.UUID]string{cartID: tc.cartEmail}

			svc := promotion.NewService(repo)
			_, err := svc.ApplyCode(context.Background(), cartID, "vip50", nil)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// Redemption is the last place the code is consumed - a code that expired between apply and checkout must not increment counters or write an audit row
func TestRedeem_RechecksValidityWindow(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)
	tests := []struct {
		name        string
		ruleValidTo *time.Time
		codeValidTo *time.Time
		autoApply   bool // no code id on the applied row
		wantErr     error
	}{
		{name: "open window redeems"},
		{name: "future window redeems", codeValidTo: &future},
		{name: "code expired", codeValidTo: &past, wantErr: promotion.ErrCodeExpired},
		{name: "rule expired", ruleValidTo: &past, wantErr: promotion.ErrCodeExpired},
		// Code-level wins, so a live code overrides an expired rule window
		{name: "live code overrides expired rule", ruleValidTo: &past, codeValidTo: &future},
		// An auto-apply rule was live when the engine ran, so lapsing mid-checkout must not fail the order
		{name: "expired auto-apply rule still redeems", ruleValidTo: &past, autoApply: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			ruleID, codeID := uuid.New(), uuid.New()
			repo.rules = []promotion.Rule{{ID: ruleID, Slug: "vip50", RequiresCode: !tc.autoApply, IsActive: true, ValidTo: tc.ruleValidTo}}
			repo.codes["VIP50"] = promotion.Code{
				ID: codeID, RuleID: ruleID, Code: "VIP50", IsActive: true, ValidTo: tc.codeValidTo,
			}
			applied := promotion.AppliedRule{RuleID: ruleID, CodeID: &codeID, DiscountCents: 1000}
			if tc.autoApply {
				applied.CodeID = nil
			}

			svc := promotion.NewService(repo)
			err := svc.Redeem(context.Background(), nil, promotion.BuildEnvironment(promotion.EnvironmentInput{}),
				applied, nil, uuid.New(), nil)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// The window has to be checked at apply time too, so the shopper gets "expired" instead of a code that silently does nothing
func TestApplyCode_CodeWindow(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)
	tests := []struct {
		name    string
		from    *time.Time
		to      *time.Time
		wantErr error
	}{
		{name: "open window applies"},
		{name: "within window applies", from: &past, to: &future},
		{name: "expired", to: &past, wantErr: promotion.ErrCodeExpired},
		{name: "not yet valid", from: &future, wantErr: promotion.ErrCodeNotYetValid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			ruleID, codeID, cartID := uuid.New(), uuid.New(), uuid.New()
			repo.rules = []promotion.Rule{{ID: ruleID, Slug: "vip50", RequiresCode: true, IsActive: true}}
			repo.codes["VIP50"] = promotion.Code{
				ID: codeID, RuleID: ruleID, Code: "VIP50", IsActive: true,
				ValidFrom: tc.from, ValidTo: tc.to,
			}

			svc := promotion.NewService(repo)
			_, err := svc.ApplyCode(context.Background(), cartID, "VIP50", nil)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

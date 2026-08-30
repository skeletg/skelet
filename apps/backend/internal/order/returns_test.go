package order

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// memoRepo implements only the handful of Repository methods creditMemoForReturn
// touches - the embedded interface keeps the rest out of the way
type memoRepo struct {
	Repository
	existing *CreditMemo
	created  bool
	linked   bool
	lines    []CreditMemoLine
}

func (m *memoRepo) GetCreditMemoByReturn(context.Context, uuid.UUID) (CreditMemo, bool, error) {
	if m.existing != nil {
		return *m.existing, true, nil
	}
	return CreditMemo{}, false, nil
}

func (m *memoRepo) CreateCreditMemo(_ context.Context, memo CreditMemo) (CreditMemo, error) {
	m.created = true
	memo.ID = uuid.New()
	memo.Number = "CM-001999"
	return memo, nil
}

func (m *memoRepo) AddCreditMemoLine(_ context.Context, _ uuid.UUID, line CreditMemoLine) (CreditMemoLine, error) {
	m.lines = append(m.lines, line)
	return line, nil
}

func (m *memoRepo) LinkPaymentTransactionToMemo(context.Context, uuid.UUID, uuid.UUID) error {
	m.linked = true
	return nil
}

func TestValidReturnTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{ReturnRequested, ReturnApproved, true},
		{ReturnRequested, ReturnRejected, true},
		{ReturnRequested, ReturnReceived, false}, // must be approved first
		{ReturnRequested, ReturnRefunded, false},
		{ReturnApproved, ReturnReceived, true},
		{ReturnApproved, ReturnRejected, true},
		{ReturnApproved, ReturnRefunded, false}, // must be received first
		{ReturnReceived, ReturnRefunded, true},
		{ReturnReceived, ReturnRejected, false}, // can't reject after receipt
		{ReturnReceived, ReturnReceived, false}, // no self-transition
		{ReturnRejected, ReturnApproved, false}, // terminal
		{ReturnRefunded, ReturnReceived, false}, // terminal
		{"bogus", ReturnApproved, false},        // unknown from-status
	}
	for _, tt := range tests {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			if got := validReturnTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("validReturnTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

// A return refund and a directly-issued credit memo must agree, so the memo
// written for an RMA mirrors the return's own line snapshots
func TestCreditMemoForReturn_MirrorsTheReturnLines(t *testing.T) {
	lineA, lineB := uuid.New(), uuid.New()
	ret := OrderReturn{
		ID:           uuid.New(),
		RMANumber:    "RMA-001000",
		RefundTarget: RefundTargetOriginal,
		Lines: []OrderReturnLine{
			{OrderLineID: lineA, Quantity: 2, RefundUnitCents: 1_000},
			{OrderLineID: lineB, Quantity: 1, RefundUnitCents: 500},
		},
	}
	o := Order{ID: uuid.New()}
	repo := &memoRepo{}
	txn := PaymentTransaction{ID: uuid.New(), AmountCents: 2_500, ProviderSlug: "stripe"}

	memo, err := creditMemoForReturn(t.Context(), repo, o, ret, txn)
	if err != nil {
		t.Fatalf("creditMemoForReturn: %v", err)
	}
	if memo.LinesTotalCents != 2_500 {
		t.Errorf("lines total = %d, want 2500", memo.LinesTotalCents)
	}
	if memo.TotalCents != 2_500 {
		t.Errorf("total = %d, want the refunded amount", memo.TotalCents)
	}
	if memo.AdjustmentFeeCents != 0 {
		t.Errorf("adjustment fee = %d, want 0 when nothing was clamped", memo.AdjustmentFeeCents)
	}
	if len(repo.lines) != 2 {
		t.Fatalf("wrote %d memo lines, want 2", len(repo.lines))
	}
	if memo.ReturnID == nil || *memo.ReturnID != ret.ID {
		t.Error("memo should point back at the return it settles")
	}
	if !repo.linked {
		t.Error("the ledger row should be linked to the memo that explains it")
	}
}

// A refund clamped by the refundable remainder still has to produce a document
// whose arithmetic adds up - the shortfall is booked as a fee
func TestCreditMemoForReturn_ClampedRefundBooksTheShortfallAsAFee(t *testing.T) {
	ret := OrderReturn{
		ID:           uuid.New(),
		RMANumber:    "RMA-001001",
		RefundTarget: RefundTargetOriginal,
		Lines:        []OrderReturnLine{{OrderLineID: uuid.New(), Quantity: 2, RefundUnitCents: 1_000}},
	}
	// Only 1200 of the 2000 gross was still refundable
	txn := PaymentTransaction{ID: uuid.New(), AmountCents: 1_200, ProviderSlug: OfflineProviderSlug}

	memo, err := creditMemoForReturn(t.Context(), &memoRepo{}, Order{ID: uuid.New()}, ret, txn)
	if err != nil {
		t.Fatalf("creditMemoForReturn: %v", err)
	}
	if memo.AdjustmentFeeCents != 800 {
		t.Errorf("adjustment fee = %d, want 800 so lines - fee equals the refund", memo.AdjustmentFeeCents)
	}
	if got := memo.LinesTotalCents - memo.AdjustmentFeeCents; got != memo.TotalCents {
		t.Errorf("items %d - fee %d = %d, want the memo total %d",
			memo.LinesTotalCents, memo.AdjustmentFeeCents, got, memo.TotalCents)
	}
	if !memo.Offline {
		t.Error("a refund recorded against the offline provider is an offline memo")
	}
}

// A replayed finalize must reuse the memo, not raise a second one for the same RMA
func TestCreditMemoForReturn_IsIdempotent(t *testing.T) {
	existing := CreditMemo{ID: uuid.New(), Number: "CM-001000"}
	repo := &memoRepo{existing: &existing}
	ret := OrderReturn{ID: uuid.New(), RMANumber: "RMA-001002", RefundTarget: RefundTargetOriginal}

	memo, err := creditMemoForReturn(t.Context(), repo, Order{ID: uuid.New()}, ret, PaymentTransaction{AmountCents: 100})
	if err != nil {
		t.Fatalf("creditMemoForReturn: %v", err)
	}
	if memo.Number != existing.Number {
		t.Errorf("memo = %q, want the existing %q", memo.Number, existing.Number)
	}
	if repo.created {
		t.Error("a replay must not write a second memo for the same return")
	}
}

func TestReturnStatusConstants(t *testing.T) {
	// Stored string values - changing one is a data migration, so pin them
	for _, tt := range []struct{ got, want string }{
		{ReturnRequested, "requested"},
		{ReturnApproved, "approved"},
		{ReturnRejected, "rejected"},
		{ReturnReceived, "received"},
		{ReturnRefunded, "refunded"},
		{RefundTargetOriginal, "original"},
		{RefundTargetGiftCard, "gift_card"},
	} {
		if tt.got != tt.want {
			t.Errorf("constant = %q, want %q", tt.got, tt.want)
		}
	}
}

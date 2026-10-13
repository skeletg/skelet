package giftcard

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func activeCard(balance int64) GiftCard {
	return GiftCard{ID: uuid.New(), Code: "GC-TEST", CurrencyCode: "USD", Balance: balance, InitialBalance: balance, Status: StatusActive}
}

// fakeRepo implements Repository for testing issueTx/redeemUpToTx without a DB
type fakeRepo struct {
	byCode    map[string]GiftCard
	byRef     map[string]GiftCard // what a keyed (idempotent) issue already minted
	created   *GiftCard
	balanceTo *int64
	statusTo  *string
	txns      []Transaction
}

func (f *fakeRepo) WithTx(_ *sql.Tx) Repository { return f }
func (f *fakeRepo) CreateGiftCard(_ context.Context, code string, in IssueInput) (GiftCard, error) {
	c := GiftCard{ID: uuid.New(), Code: code, CurrencyCode: in.CurrencyCode, InitialBalance: in.Amount, Balance: in.Amount, Status: StatusActive}
	f.created = &c
	if f.byCode == nil {
		f.byCode = map[string]GiftCard{}
	}
	f.byCode[code] = c
	if in.IssueRef != "" { // mirrors the unique index on issue_ref
		if f.byRef == nil {
			f.byRef = map[string]GiftCard{}
		}
		if _, dup := f.byRef[in.IssueRef]; dup {
			return GiftCard{}, ErrCodeTaken
		}
		f.byRef[in.IssueRef] = c
	}
	return c, nil
}
func (f *fakeRepo) GetByIssueRef(_ context.Context, ref string) (GiftCard, error) {
	if c, ok := f.byRef[ref]; ok {
		return c, nil
	}
	return GiftCard{}, ErrNotFound
}
func (f *fakeRepo) GetByID(_ context.Context, _ uuid.UUID) (GiftCard, error) {
	return GiftCard{}, ErrNotFound
}
func (f *fakeRepo) GetByCode(_ context.Context, code string) (GiftCard, error) {
	if c, ok := f.byCode[code]; ok {
		return c, nil
	}
	return GiftCard{}, ErrNotFound
}
func (f *fakeRepo) GetByCodeForUpdate(_ context.Context, code string) (GiftCard, error) {
	return f.GetByCode(context.Background(), code)
}
func (f *fakeRepo) GetByIDForUpdate(ctx context.Context, id uuid.UUID) (GiftCard, error) {
	return f.GetByID(ctx, id)
}
func (f *fakeRepo) UpdateBalance(_ context.Context, id uuid.UUID, balance int64, status string) (GiftCard, error) {
	f.balanceTo, f.statusTo = &balance, &status
	return GiftCard{ID: id, Balance: balance, Status: status}, nil
}
func (f *fakeRepo) List(_ context.Context, _ []uuid.UUID, _, _ int32) ([]GiftCard, error) {
	return nil, nil
}
func (f *fakeRepo) ListByIssuedOrder(_ context.Context, _ uuid.UUID) ([]GiftCard, error) {
	return nil, nil
}
func (f *fakeRepo) InsertTransaction(_ context.Context, t Transaction) (Transaction, error) {
	f.txns = append(f.txns, t)
	return t, nil
}
func (f *fakeRepo) ListTransactions(_ context.Context, _ uuid.UUID) ([]Transaction, error) {
	return f.txns, nil
}

func (f *fakeRepo) ListTransactionsByOrder(_ context.Context, orderID uuid.UUID) ([]Transaction, error) {
	var out []Transaction
	for _, t := range f.txns {
		if t.OrderID != nil && *t.OrderID == orderID {
			out = append(out, t)
		}
	}
	return out, nil
}

func TestIssueTx_CreatesCardAndLedger(t *testing.T) {
	r := &fakeRepo{}
	card, err := issueTx(context.Background(), r, IssueInput{CurrencyCode: "USD", Amount: 5000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if card.Balance != 5000 || card.Status != StatusActive {
		t.Errorf("issued card balance=%d status=%s, want 5000/active", card.Balance, card.Status)
	}
	if len(r.txns) != 1 || r.txns[0].Reason != ReasonIssue || r.txns[0].Amount != 5000 {
		t.Errorf("expected one +5000 issue ledger row, got %+v", r.txns)
	}
}

func activeCardCode(code string, balance int64) GiftCard {
	c := activeCard(balance)
	c.Code = code
	return c
}

func TestTenderAmount_CoversFullDueWhenBalanceAmple(t *testing.T) {
	applied, bal, status, err := tenderAmount(activeCard(5000), 2000, "USD", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != 2000 || bal != 3000 || status != StatusActive {
		t.Errorf("got applied=%d bal=%d status=%s, want 2000/3000/active", applied, bal, status)
	}
}

func TestTenderAmount_PartialWhenBalanceShort(t *testing.T) {
	// $30 card against a $50 due: applies all $30, card depletes, $20 still owed
	applied, bal, status, err := tenderAmount(activeCard(3000), 5000, "USD", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != 3000 || bal != 0 || status != StatusDepleted {
		t.Errorf("got applied=%d bal=%d status=%s, want 3000/0/depleted", applied, bal, status)
	}
}

func TestTenderAmount_NothingDueAppliesZero(t *testing.T) {
	applied, _, _, err := tenderAmount(activeCard(5000), 0, "USD", time.Now())
	if err != nil || applied != 0 {
		t.Fatalf("nothing due should apply 0, got applied=%d err=%v", applied, err)
	}
}

func TestTenderAmount_InactiveAndExpiredAndCurrency(t *testing.T) {
	disabled := activeCard(5000)
	disabled.Status = StatusDisabled
	if _, _, _, err := tenderAmount(disabled, 1000, "USD", time.Now()); !errors.Is(err, ErrInactive) {
		t.Errorf("disabled: got %v, want ErrInactive", err)
	}
	expired := activeCard(5000)
	past := time.Now().Add(-time.Hour)
	expired.ExpiresAt = &past
	if _, _, _, err := tenderAmount(expired, 1000, "USD", time.Now()); !errors.Is(err, ErrExpired) {
		t.Errorf("expired: got %v, want ErrExpired", err)
	}
	if _, _, _, err := tenderAmount(activeCard(5000), 1000, "EUR", time.Now()); !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("currency: got %v, want ErrCurrencyMismatch", err)
	}
}

func TestRedeemUpToTx_PartialWritesAppliedAmount(t *testing.T) {
	r := &fakeRepo{byCode: map[string]GiftCard{"GC-X": activeCardCode("GC-X", 3000)}}
	oid := uuid.New()
	applied, err := redeemUpToTx(context.Background(), r, "GC-X", uuid.Nil, 5000, "USD", time.Now(), &oid)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if applied != 3000 {
		t.Errorf("applied=%d, want 3000", applied)
	}
	if r.balanceTo == nil || *r.balanceTo != 0 || r.statusTo == nil || *r.statusTo != StatusDepleted {
		t.Errorf("expected balance→0/depleted, got balance=%v status=%v", r.balanceTo, r.statusTo)
	}
	if len(r.txns) != 1 || r.txns[0].Amount != -3000 {
		t.Errorf("expected one -3000 redeem row, got %+v", r.txns)
	}
}

func TestRedeemUpToTx_EmptyCardIsNoOp(t *testing.T) {
	r := &fakeRepo{byCode: map[string]GiftCard{"GC-X": activeCardCode("GC-X", 0)}}
	applied, err := redeemUpToTx(context.Background(), r, "GC-X", uuid.Nil, 5000, "USD", time.Now(), nil)
	if err != nil || applied != 0 {
		t.Fatalf("empty card should apply 0, got applied=%d err=%v", applied, err)
	}
	if r.balanceTo != nil || len(r.txns) != 0 {
		t.Errorf("empty-card tender must not write balance/ledger")
	}
}

// A refund mints store value between its pending ledger row and the finalize, so an unguarded
// replay in that window hands the customer a SECOND card - the issue ref makes the second call a lookup
func TestIssueTx_KeyedIssueIsIdempotent(t *testing.T) {
	repo := &fakeRepo{}
	in := IssueInput{CurrencyCode: "USD", Amount: 2500, IssueRef: "return:" + uuid.NewString()}

	first, err := issueTx(context.Background(), repo, in)
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	second, err := issueTx(context.Background(), repo, in)
	if err != nil {
		t.Fatalf("replayed issue: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("replay minted a new card: got %s, want %s", second.ID, first.ID)
	}
	if got := len(repo.txns); got != 1 {
		t.Errorf("ledger rows: got %d, want 1 (the replay must not re-credit)", got)
	}
}

// Without a ref there's no key to dedupe on, so each call is a genuinely distinct card
func TestIssueTx_UnkeyedIssuesAreDistinct(t *testing.T) {
	repo := &fakeRepo{}
	in := IssueInput{CurrencyCode: "USD", Amount: 2500}

	first, err := issueTx(context.Background(), repo, in)
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	second, err := issueTx(context.Background(), repo, in)
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if second.ID == first.ID {
		t.Error("unkeyed issues returned the same card")
	}
}

func TestNormalizeCode(t *testing.T) {
	for in, want := range map[string]string{
		"GC-A7K2-9MTX-PQ4R": "GC-A7K2-9MTX-PQ4R",
		"gc-a7k2-9mtx-pq4r": "GC-A7K2-9MTX-PQ4R",
		" GCA7K29MTXPQ4R ":  "GC-A7K2-9MTX-PQ4R",
		"gc a7k2 9mtx pq4r": "GC-A7K2-9MTX-PQ4R",
		"legacy-code":       "LEGACY-CODE",
		"GC-A7K2":           "GC-A7K2",
	} {
		if got := NormalizeCode(in); got != want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", in, got, want)
		}
	}
}

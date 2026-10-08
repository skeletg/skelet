package cart

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepo implements Repository for authorization tests - Only the two lookups exercised by authorizeCart /
// verifyLineInCart are backed by func fields; the rest are inert stubs so the fake satisfies the interface
type fakeRepo struct {
	getCart  func(id uuid.UUID) (Cart, error)
	getLine  func(id uuid.UUID) (CartLine, error)
	setEmail func(email string)
}

func (f fakeRepo) GetCartByID(_ context.Context, id uuid.UUID) (Cart, error) {
	return f.getCart(id)
}
func (f fakeRepo) GetCartLineByID(_ context.Context, id uuid.UUID) (CartLine, error) {
	return f.getLine(id)
}

func (f fakeRepo) GetCartByCustomerAndChannel(context.Context, uuid.UUID, uuid.UUID) (Cart, error) {
	return Cart{}, nil
}
func (f fakeRepo) CreateCart(context.Context, *uuid.UUID, uuid.UUID) (Cart, error) {
	return Cart{}, nil
}
func (f fakeRepo) ReassignCart(context.Context, uuid.UUID, uuid.UUID) (Cart, error) {
	return Cart{}, nil
}
func (f fakeRepo) ListCartLinesByCart(context.Context, uuid.UUID) ([]CartLine, error) {
	return nil, nil
}
func (f fakeRepo) UpsertCartLineBySku(context.Context, uuid.UUID, AddCartLineInput) (CartLine, error) {
	return CartLine{}, nil
}
func (f fakeRepo) UpsertBundleCartLine(context.Context, uuid.UUID, AddBundleLineInput) (CartLine, error) {
	return CartLine{}, nil
}
func (f fakeRepo) UpsertConfiguredCartLine(context.Context, uuid.UUID, AddConfiguredLineInput) (CartLine, error) {
	return CartLine{}, nil
}
func (f fakeRepo) UpdateCartLineQuantityPrice(context.Context, uuid.UUID, float64, int64) (CartLine, error) {
	return CartLine{}, nil
}
func (f fakeRepo) UpdateCartLineQuantity(context.Context, uuid.UUID, float64) (CartLine, error) {
	return CartLine{}, nil
}
func (f fakeRepo) RemoveCartLine(context.Context, uuid.UUID) error { return nil }
func (f fakeRepo) ClearCartLines(context.Context, uuid.UUID) error { return nil }
func (f fakeRepo) ConvertCart(context.Context, uuid.UUID) error    { return nil }
func (f fakeRepo) SetCartMerged(context.Context, uuid.UUID) error  { return nil }
func (f fakeRepo) ListCarts(context.Context, CartFilter) ([]CartListItem, error) {
	return nil, nil
}
func (f fakeRepo) CountCarts(context.Context, CartFilter) (int32, error) { return 0, nil }
func (f fakeRepo) ListAbandoned(context.Context, uuid.UUID, time.Time, time.Time, int32) ([]AbandonedCart, error) {
	return nil, nil
}
func (f fakeRepo) MarkAbandonedNotified(context.Context, []uuid.UUID) error { return nil }
func (f fakeRepo) SetEmail(_ context.Context, _ uuid.UUID, email string) error {
	if f.setEmail != nil {
		f.setEmail(email)
	}
	return nil
}
func (f fakeRepo) SetNote(context.Context, uuid.UUID, string) error     { return nil }
func (f fakeRepo) PruneStale(context.Context, time.Time) (int64, error) { return 0, nil }
func (f fakeRepo) SaveItem(context.Context, uuid.UUID, string, string, float64) (SavedItem, error) {
	return SavedItem{}, nil
}
func (f fakeRepo) GetSavedItem(context.Context, uuid.UUID) (SavedItem, error) {
	return SavedItem{}, nil
}
func (f fakeRepo) DeleteSavedItem(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}
func (f fakeRepo) SetBundleLineQuantity(context.Context, uuid.UUID, float64) error { return nil }
func (f fakeRepo) ListSavedItems(context.Context, uuid.UUID) ([]SavedItem, error)  { return nil, nil }
func (f fakeRepo) WithTx(*sql.Tx) Repository                                       { return f }

func ptr(u uuid.UUID) *uuid.UUID { return &u }

func TestAuthorizeCart(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()
	cartID := uuid.New()

	dbDown := errors.New("db down")
	tests := []struct {
		name    string
		cart    Cart
		cartErr error
		caller  *uuid.UUID
		wantErr error // errors.Is target; nil means success
	}{
		{
			name:    "a missing cart is forbidden, not not-found", // telling them apart would let anyone enumerate cart UUIDs
			cartErr: sql.ErrNoRows,
			caller:  ptr(owner),
			wantErr: ErrCartForbidden,
		},
		{
			name:    "a repo failure propagates rather than reading as forbidden", // masking it told a shopper their own cart was gone whenever the DB blipped
			cartErr: dbDown,
			caller:  ptr(owner),
			wantErr: dbDown,
		},
		{
			name:    "non-active cart rejected",
			cart:    Cart{ID: cartID, Status: "converted", CustomerID: ptr(owner)},
			caller:  ptr(owner),
			wantErr: ErrCartNotActive,
		},
		{
			name:   "guest cart allows any caller (nil)",
			cart:   Cart{ID: cartID, Status: "active", CustomerID: nil},
			caller: nil,
		},
		{
			name:   "guest cart allows any caller (some customer)",
			cart:   Cart{ID: cartID, Status: "active", CustomerID: nil},
			caller: ptr(other),
		},
		{
			name:    "customer cart rejects anonymous caller",
			cart:    Cart{ID: cartID, Status: "active", CustomerID: ptr(owner)},
			caller:  nil,
			wantErr: ErrCartForbidden,
		},
		{
			name:    "customer cart rejects a different customer",
			cart:    Cart{ID: cartID, Status: "active", CustomerID: ptr(owner)},
			caller:  ptr(other),
			wantErr: ErrCartForbidden,
		},
		{
			name:   "customer cart allows its owner",
			cart:   Cart{ID: cartID, Status: "active", CustomerID: ptr(owner)},
			caller: ptr(owner),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := fakeRepo{getCart: func(uuid.UUID) (Cart, error) { return tc.cart, tc.cartErr }}
			got, err := authorizeCart(context.Background(), repo, cartID, tc.caller)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.ID != tc.cart.ID {
					t.Errorf("returned cart ID: got %s, want %s", got.ID, tc.cart.ID)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("got %v, want errors.Is %v", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyLineInCart(t *testing.T) {
	cartID := uuid.New()
	otherCart := uuid.New()
	lineID := uuid.New()

	tests := []struct {
		name    string
		line    CartLine
		lineErr error
		wantErr error
	}{
		{
			name:    "line not found",
			lineErr: sql.ErrNoRows,
			wantErr: ErrCartLineNotFound,
		},
		{
			name:    "line belongs to another cart",
			line:    CartLine{ID: lineID, CartID: otherCart},
			wantErr: ErrCartForbidden,
		},
		{
			name: "line belongs to this cart",
			line: CartLine{ID: lineID, CartID: cartID},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := fakeRepo{getLine: func(uuid.UUID) (CartLine, error) { return tc.line, tc.lineErr }}
			err := verifyLineInCart(context.Background(), repo, lineID, cartID)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want errors.Is %v", err, tc.wantErr)
			}
		})
	}
}

package cart

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type upsertCounter struct {
	fakeRepo
	upserts int
}

func (r *upsertCounter) UpsertCartLineBySku(context.Context, uuid.UUID, AddCartLineInput) (CartLine, error) {
	r.upserts++
	return CartLine{}, nil
}

// Without a catalog to price from there is no price but a caller's, so both doors refuse rather than store one
func TestAddingALineWithoutACatalogRefuses(t *testing.T) {
	cartID := uuid.New()
	repo := &upsertCounter{getCart: func(uuid.UUID) (Cart, error) {
		return Cart{ID: cartID, ChannelID: uuid.New(), Status: "active"}, nil
	}}
	svc := NewService(repo, nil, nil)

	if _, err := svc.AddToCart(context.Background(), cartID, nil, "TEE-1", 1); err == nil {
		t.Error("AddToCart added a line with no catalog to price it")
	}
	if _, err := svc.AdminAddToCart(context.Background(), cartID, "TEE-1", 1); err == nil {
		t.Error("AdminAddToCart added a line with no catalog to price it")
	}
	if repo.upserts != 0 {
		t.Errorf("%d lines written, want none", repo.upserts)
	}
}

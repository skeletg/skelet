package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/order"
)

type fakeOrders struct {
	o   order.Order
	err error
}

func (f fakeOrders) GetByID(context.Context, uuid.UUID) (order.Order, error) { return f.o, f.err }

// The guest path: a placed order with no customer id enrolls against the order
// email, so uuid.Nil is a valid value downstream rather than a bug
func TestCustomerIDOrNil(t *testing.T) {
	if got := customerIDOrNil(nil); got != uuid.Nil {
		t.Errorf("guest should resolve to uuid.Nil, got %v", got)
	}
	id := uuid.New()
	if got := customerIDOrNil(&id); got != id {
		t.Errorf("known customer should pass through, got %v want %v", got, id)
	}
}

func TestEnrollOrderSkipsOnLookupFailure(t *testing.T) {
	called := false
	enrollOrder(context.Background(), fakeOrders{err: errors.New("gone")}, uuid.New(),
		func(uuid.UUID, order.Order) { called = true })
	if called {
		t.Fatal("a failed order lookup must skip enrollment, not enroll against a half-known order")
	}
}

// Same resolution on the load path - a guest order loaded for a shipment trigger
// still enrols, keyed on its email
func TestEnrollOrderResolvesGuest(t *testing.T) {
	var gotID uuid.UUID
	var gotOrder order.Order
	enrollOrder(context.Background(), fakeOrders{o: order.Order{Email: "guest@example.com"}}, uuid.New(),
		func(id uuid.UUID, o order.Order) { gotID, gotOrder = id, o })
	if gotID != uuid.Nil {
		t.Errorf("guest order should enrol with uuid.Nil, got %v", gotID)
	}
	if gotOrder.Email != "guest@example.com" {
		t.Errorf("the order must reach the enroller so it can key on the email, got %q", gotOrder.Email)
	}

	cust := uuid.New()
	enrollOrder(context.Background(), fakeOrders{o: order.Order{CustomerID: &cust}}, uuid.New(),
		func(id uuid.UUID, _ order.Order) { gotID = id })
	if gotID != cust {
		t.Errorf("known customer should pass through, got %v want %v", gotID, cust)
	}
}

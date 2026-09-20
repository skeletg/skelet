package inventory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
)

// fakeKitLookup stubs ProductLookup: a SKU→id map plus a kit→components
// map - A product absent from `kits` is treated as a non-kit (no components)
type fakeKitLookup struct {
	idBySku map[string]uuid.UUID
	kits    map[uuid.UUID][]KitComponentRef
	kitErr  error // set to simulate a transient component-lookup failure
}

func (f fakeKitLookup) GetProductIDBySKU(_ context.Context, sku string) (uuid.UUID, error) {
	id, ok := f.idBySku[sku]
	if !ok {
		return uuid.Nil, errors.New("unknown sku")
	}
	return id, nil
}

func (f fakeKitLookup) KitComponents(_ context.Context, productID uuid.UUID) ([]KitComponentRef, error) {
	if f.kitErr != nil {
		return nil, f.kitErr
	}
	return f.kits[productID], nil
}

// A component lookup that FAILS must not fall through to reserving the kit wrapper: the wrapper owns no stock, so the
// reservation holds nothing while placement still reports stock-reserved, and the kit oversells on every later order
func TestLinesFromCart_KitLookupFailureDoesNotReserveWrapper(t *testing.T) {
	kitID := uuid.New()
	s := &Service{products: fakeKitLookup{
		idBySku: map[string]uuid.UUID{"starter-kit": kitID},
		kitErr:  errors.New("connection reset"),
	}}
	c := cart.Cart{Lines: []cart.CartLine{
		{ID: uuid.New(), ProductSku: "starter-kit", Quantity: 3},
	}}

	lines, err := s.linesFromCart(context.Background(), c)
	if err == nil {
		t.Fatalf("component lookup failed but linesFromCart returned lines %+v with no error", lines)
	}
	for _, l := range lines {
		if l.ProductID == kitID {
			t.Errorf("reserved the kit wrapper, which owns no stock: %+v", l)
		}
	}
}

func TestLinesFromCart_KitExpandsToComponents(t *testing.T) {
	kitID, compA, compB := uuid.New(), uuid.New(), uuid.New()
	s := &Service{products: fakeKitLookup{
		idBySku: map[string]uuid.UUID{"starter-kit": kitID},
		kits: map[uuid.UUID][]KitComponentRef{
			kitID: {{ProductID: compA, Quantity: 1}, {ProductID: compB, Quantity: 2}},
		},
	}}
	lineID := uuid.New()
	c := cart.Cart{Lines: []cart.CartLine{
		{ID: lineID, ProductSku: "starter-kit", Quantity: 3},
	}}

	lines, err := s.linesFromCart(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two component reservations, each quantity = perKit × lineQty, all
	// linked to the kit's order line
	if len(lines) != 2 {
		t.Fatalf("got %d reserve lines, want 2: %+v", len(lines), lines)
	}
	got := map[uuid.UUID]float64{}
	for _, l := range lines {
		if l.OrderLineID != lineID {
			t.Errorf("component reserve line not linked to kit line: %+v", l)
		}
		got[l.ProductID] = l.Quantity
	}
	if got[compA] != 3 { // 1 × 3
		t.Errorf("compA qty = %v, want 3", got[compA])
	}
	if got[compB] != 6 { // 2 × 3
		t.Errorf("compB qty = %v, want 6", got[compB])
	}
}

func TestLinesFromCart_SimpleStaysSingleLine(t *testing.T) {
	simpleID := uuid.New()
	s := &Service{products: fakeKitLookup{
		idBySku: map[string]uuid.UUID{"tee": simpleID},
		kits:    map[uuid.UUID][]KitComponentRef{}, // not a kit
	}}
	lineID := uuid.New()
	c := cart.Cart{Lines: []cart.CartLine{
		{ID: lineID, ProductSku: "tee", Quantity: 2},
	}}

	lines, err := s.linesFromCart(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 1 || lines[0].ProductID != simpleID || lines[0].Quantity != 2 {
		t.Fatalf("non-kit should reserve as one line {%s, 2}, got %+v", simpleID, lines)
	}
}

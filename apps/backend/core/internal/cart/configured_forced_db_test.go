package cart

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestAForcedQuantityIsHeldToTheLineCap(t *testing.T) {
	huge := 2 * maxLineQuantity
	v := ProductOptionValueConfig{ID: uuid.New(), Title: "Pallet", PriceType: "fixed", Quantity: 1, ForcedLineQty: &huge}
	opt := ProductOptionConfig{ID: uuid.New(), InputType: "radio", Title: "Pack", Values: []ProductOptionValueConfig{v}}
	r := newDBRig(t, catalogStub{
		prices:  map[string]SimplePricing{"bulk": {UnitPrice: 100}},
		options: map[string]ProductOptionsResolution{"bulk": {ProductID: uuid.New(), UnitPrice: 100, BasePrice: 100, Options: []ProductOptionConfig{opt}}},
	})
	c := r.guestCart(t)
	_, err := r.svc.AddConfiguredToCart(context.Background(), c.ID, nil, AddConfiguredToCartInput{
		ProductSku: "bulk", Quantity: 1,
		Options: []CartLineOptionInput{{OptionID: opt.ID, ValueIDs: []uuid.UUID{v.ID}}},
	})
	if !errors.Is(err, ErrQuantityInvalid) {
		t.Fatalf("forced quantity %v = %v, want ErrQuantityInvalid", huge, err)
	}
}

package front

import (
	"context"
	"maps"
	"testing"

	"github.com/google/uuid"

	frontmodel "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func TestCheckoutFieldsAreAskedAnsweredAndOwed(t *testing.T) {
	ctx, ch := context.Background(), uuid.New()
	asked := []plugin.CheckoutField{
		{Plugin: "acme", Key: "gift_message", Label: "gift message", Type: sdk.CheckoutFieldTextarea, MaxLength: 20},
		{Plugin: "acme", Key: "terms", Label: "the gift terms", Type: sdk.CheckoutFieldCheckbox, Required: true},
	}
	r := &Resolver{CheckoutFields: func(_ context.Context, got uuid.UUID) []plugin.CheckoutField {
		if got != ch {
			return nil
		}
		return asked
	}}

	got, err := r.answerFields(ctx, ch, []*frontmodel.CheckoutFieldValueInput{
		{Key: "gift_message", Value: " Happy birthday "}, {Key: "terms", Value: "on"}, {Key: "smuggled", Value: "x"},
	})
	if want := map[string]string{"gift_message": "Happy birthday", "terms": "true"}; err != nil || !maps.Equal(got, want) {
		t.Errorf("answers = %v, %v - want %v, the key nothing asks dropped", got, err, want)
	}
	if _, err := r.answerFields(ctx, ch, []*frontmodel.CheckoutFieldValueInput{{Key: "gift_message", Value: "hi"}}); apperr.CodeOf(err) != "CHECKOUT_FIELD_REQUIRED" {
		t.Errorf("the terms left unticked: %v", err)
	}

	c := cart.Cart{ChannelID: ch, Checkout: cart.Checkout{Fields: map[string]string{"gift_message": "hi"}}}
	if apperr.CodeOf(r.missingField(ctx, c)) != "CHECKOUT_FIELD_REQUIRED" {
		t.Error("a cart without the required answer reads as complete")
	}
	c.Checkout.Fields["terms"] = "true"
	if err := r.missingField(ctx, c); err != nil {
		t.Errorf("a cart with every answer still owes %v", err)
	}

	models := r.fieldModels(ctx, c)
	if len(models) != 2 || models[0].Type != frontmodel.CheckoutFieldTypeTextarea || models[0].MaxLength != 20 || models[0].Value != "hi" ||
		models[1].Options == nil || models[1].Value != "true" {
		t.Errorf("fields on the checkout = %+v", models)
	}
	if none := (&Resolver{}).fieldModels(ctx, c); none == nil || len(none) != 0 {
		t.Errorf("a store with no plugin fields lists %v, want an empty list", none)
	}
}

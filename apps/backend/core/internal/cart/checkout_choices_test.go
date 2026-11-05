package cart

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

func TestACheckoutChoiceIsReportedOnlyWhenItChanges(t *testing.T) {
	bus := events.NewBus()
	got := make(chan *CheckoutChosen, 4)
	bus.Subscribe(EventCheckoutChosen, func(_ context.Context, e events.Event) error {
		got <- e.(*CheckoutChosen)
		return nil
	})
	s := &Service{bus: bus}
	pickup := uuid.New()
	before := choicesOf(Checkout{PaymentProviderSlug: "cod"})
	after := choicesOf(Checkout{PaymentProviderSlug: "cod", Deliveries: map[string]DeliverySelection{
		"b": {OptionID: pickup}, "a": {OptionID: uuid.New(), CarrierSlug: "dhl", MethodCode: "express"},
	}})
	// in group order, a then b
	if want := "dhl:express," + pickup.String(); after.shipping != want {
		t.Fatalf("shipping %q, want %q", after.shipping, want)
	}
	s.publishChoices(context.Background(), Cart{ID: uuid.New()}, before, after)
	s.publishChoices(context.Background(), Cart{ID: uuid.New()}, after, after) // the same step saved again

	select {
	case e := <-got:
		if e.Step != StepShipping || e.Choice != after.shipping {
			t.Errorf("got %+v, want the new shipping choice", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no shipping choice reported")
	}
	select {
	case e := <-got:
		t.Errorf("reported %+v - the payment didn't change and a re-save is nothing", e)
	case <-time.After(100 * time.Millisecond):
	}
}

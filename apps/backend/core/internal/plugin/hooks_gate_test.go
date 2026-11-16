package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A numberer the gate refuses still answers for the channel, so numbering falls back to the store's format
func TestANumbererOverItsAllowanceLeavesTheOrderToTheStore(t *testing.T) {
	t.Cleanup(sdk.ResetForTest)
	origEnabled, origLimit := isEnabledFromContext, checkRateLimitFn
	t.Cleanup(func() { isEnabledFromContext, checkRateLimitFn = origEnabled, origLimit })
	isEnabledFromContext = func(context.Context, string, uuid.UUID) bool { return true }
	checkRateLimitFn = func(context.Context, string) error { return ErrPluginRateLimited }
	called := false
	sdk.NewPluginForTest("numbers-gated").RegisterOrderNumberer(func(context.Context, sdk.OrderNumberRequest) (string, error) {
		called = true
		return "PLUGIN-1", nil
	})
	num, ok, err := OrderNumber(context.Background(), order.NumberRequest{ChannelID: uuid.New()})
	if called || !ok || !errors.Is(err, ErrPluginRateLimited) || num != "" {
		t.Errorf("numbered %q, ok %v, %v, called %v - want the refusal, so the store's format names it", num, ok, err, called)
	}
}

// A check that fails with an ordinary error is a fault, kept out of the shopper's sight
func TestACheckoutCheckFailingOrdinarilyIsAFault(t *testing.T) {
	f := CheckoutField{Plugin: "acme", Key: "acme:msg", CheckoutField: sdk.CheckoutField{Key: "msg", Label: "message",
		Validate: func(context.Context, sdk.CheckoutFieldCheck) error { return errors.New("rules service down") }}}
	_, err := f.Answer(context.Background(), uuid.New(), "hi")
	var perr *PluginError
	if !errors.As(err, &perr) || perr.Slug != "acme" || apperr.ClassOf(err) == apperr.ClassValidation {
		t.Errorf("a failing check read as %v", err)
	}
}

type recordingStager struct{ key, plugin string }

func (r *recordingStager) SetStage(_ context.Context, _ uuid.UUID, key, plugin string) (order.Order, error) {
	r.key, r.plugin = key, plugin
	return order.Order{}, nil
}

func TestTheHostKeysAStageUnderItsPlugin(t *testing.T) {
	rec := &recordingStager{}
	var s OrderStager = rec
	prev := orderStager.Swap(&s)
	t.Cleanup(func() { orderStager.Store(prev) })
	h := &Host{}
	if err := h.SetOrderStage(context.Background(), "acme", uuid.New(), "awaiting_stock"); err != nil || rec.key != "acme:awaiting_stock" || rec.plugin != "acme" {
		t.Errorf("moved to %q for %q, %v", rec.key, rec.plugin, err)
	}
	if err := h.SetOrderStage(context.Background(), "acme", uuid.New(), ""); err != nil || rec.key != "" {
		t.Errorf("leaving a stage asked for %q, %v", rec.key, err)
	}
}

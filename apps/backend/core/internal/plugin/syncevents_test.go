package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// A plugin subscribes by the SDK's names, so each must be the string core publishes under
func TestSDKEventNamesAreCores(t *testing.T) {
	for sdkName, coreName := range map[string]string{
		sdk.EventCartLineAdding:      cart.EventLineAdding,
		sdk.EventCartLineUpdating:    cart.EventLineUpdating,
		sdk.EventCheckoutValidating:  order.EventCheckoutValidating,
		sdk.EventOrderPlacing:        order.EventPlacing,
		sdk.EventCustomerRegistering: customer.EventRegistering,
		sdk.EventOrderTendering:      order.EventTendering,
		sdk.EventCartBundleAdding:    cart.EventBundleAdding,
		sdk.EventCartLineRemoving:    cart.EventLineRemoving,
		sdk.EventOrderCancelling:     order.EventCancelling,
		sdk.EventOrderFulfilling:     order.EventFulfilling,
		sdk.EventInventoryAllocating: inventory.EventAllocating,
		sdk.EventReviewSubmitting:    review.EventSubmitting,
		sdk.EventCustomerDeleting:    customer.EventDeleting,
	} {
		if sdkName != coreName {
			t.Errorf("sdk says %q, core publishes %q - a plugin subscribing by the sdk name would never fire", sdkName, coreName)
		}
	}
}

// One event must never satisfy another's interface, or a handler asserting the wrong one would act on an event it didn't mean
// The one deliberate overlap: a bundle add is a CartLineAdding, so a single handler serves both
func TestEachEventMatchesOnlyItsOwnInterface(t *testing.T) {
	evs := map[string]any{
		"cart.LineAdding":              &cart.LineAdding{},
		"cart.BundleAdding":            &cart.BundleAdding{},
		"cart.LineUpdating":            &cart.LineUpdating{},
		"cart.LineRemoving":            &cart.LineRemoving{},
		"order.CheckoutValidating":     &order.CheckoutValidating{},
		"order.OrderPlacing":           &order.OrderPlacing{},
		"order.OrderTendering":         &order.OrderTendering{},
		"order.OrderCancelling":        &order.OrderCancelling{},
		"order.OrderFulfilling":        &order.OrderFulfilling{},
		"inventory.Allocating":         &inventory.Allocating{},
		"review.Submitting":            &review.Submitting{},
		"customer.CustomerRegistering": &customer.CustomerRegistering{},
		"customer.CustomerDeleting":    &customer.CustomerDeleting{},
	}
	matches := map[string]func(any) bool{
		"CartLineAdding":      func(e any) bool { _, ok := e.(sdk.CartLineAdding); return ok },
		"CartLineUpdating":    func(e any) bool { _, ok := e.(sdk.CartLineUpdating); return ok },
		"CartLineRemoving":    func(e any) bool { _, ok := e.(sdk.CartLineRemoving); return ok },
		"CheckoutValidating":  func(e any) bool { _, ok := e.(sdk.CheckoutValidating); return ok },
		"OrderPlacing":        func(e any) bool { _, ok := e.(sdk.OrderPlacing); return ok },
		"Tendering":           func(e any) bool { _, ok := e.(sdk.Tendering); return ok },
		"OrderCancelling":     func(e any) bool { _, ok := e.(sdk.OrderCancelling); return ok },
		"OrderFulfilling":     func(e any) bool { _, ok := e.(sdk.OrderFulfilling); return ok },
		"InventoryAllocating": func(e any) bool { _, ok := e.(sdk.InventoryAllocating); return ok },
		"ReviewSubmitting":    func(e any) bool { _, ok := e.(sdk.ReviewSubmitting); return ok },
		"CustomerRegistering": func(e any) bool { _, ok := e.(sdk.CustomerRegistering); return ok },
		"CustomerDeleting":    func(e any) bool { _, ok := e.(sdk.CustomerDeleting); return ok },
	}
	want := map[string]string{
		"cart.LineAdding": "CartLineAdding", "cart.BundleAdding": "CartLineAdding",
		"cart.LineUpdating": "CartLineUpdating", "cart.LineRemoving": "CartLineRemoving",
		"order.CheckoutValidating": "CheckoutValidating", "order.OrderPlacing": "OrderPlacing",
		"order.OrderTendering": "Tendering", "order.OrderCancelling": "OrderCancelling",
		"order.OrderFulfilling": "OrderFulfilling", "inventory.Allocating": "InventoryAllocating",
		"review.Submitting": "ReviewSubmitting", "customer.CustomerRegistering": "CustomerRegistering",
		"customer.CustomerDeleting": "CustomerDeleting",
	}
	for evName, ev := range evs {
		for iface, match := range matches {
			if got := match(ev); got != (want[evName] == iface) {
				t.Errorf("%s satisfies sdk.%s = %v", evName, iface, got)
			}
		}
	}
}

// The whole path a plugin takes: subscribe by the sdk name, assert the sdk interface, change the event, and core's own fields carry it
func TestAPluginChangesEachEventThroughTheRealBus(t *testing.T) {
	bus := freshHost(t)
	p := sdk.NewPluginForTest("seams")
	p.SubscribeSync(sdk.EventCartLineAdding, 100, func(_ context.Context, e any) error {
		line := e.(sdk.CartLineAdding)
		if line.LineSKU() != "STAFF-1" || line.LineUnitPriceCents() != 1000 {
			return errors.New("read the wrong line")
		}
		return errors.Join(line.SetLineUnitPrice(800), line.SetLineQuantity(3))
	})
	p.SubscribeSync(sdk.EventCartLineUpdating, 100, func(_ context.Context, e any) error {
		return e.(sdk.CartLineUpdating).SetLineQuantity(5)
	})
	review := func(_ context.Context, e any) error {
		r := e.(sdk.CheckoutReview)
		r.AddCheckoutDiscount(50)
		r.RaiseCheckoutRiskScore(40)
		r.RaiseCheckoutRiskScore(10) // a lower score never lowers it
		r.AddCheckoutTags("screened")
		return nil
	}
	p.SubscribeSync(sdk.EventCheckoutValidating, 100, review)
	p.SubscribeSync(sdk.EventOrderPlacing, 100, review)
	p.SubscribeSync(sdk.EventOrderPlacing, 200, func(_ context.Context, e any) error {
		o := e.(sdk.OrderPlacing)
		return errors.Join(o.SetPlacingTaxCents(120), o.SetPlacingShippingCents(0))
	})
	b2bGroup := uuid.New()
	p.SubscribeSync(sdk.EventCustomerRegistering, 100, func(_ context.Context, e any) error {
		reg := e.(sdk.CustomerRegistering)
		reg.AddRegisteringTags("corporate")
		reg.SetRegisteringGroup(b2bGroup)
		reg.SetRegisteringMarketingConsent(false)
		return nil
	})
	ctx := context.Background()

	price, qty := int64(1000), 1.0
	add := &cart.LineAdding{ProductSku: "STAFF-1", UnitPrice: &price, Quantity: &qty}
	if err := bus.PublishSync(ctx, add); err != nil {
		t.Fatal(err)
	}
	if *add.UnitPrice != 800 || *add.Quantity != 3 {
		t.Errorf("cart add: price %d qty %v, want 800 and 3", *add.UnitPrice, *add.Quantity)
	}
	upQty := 1.0
	up := &cart.LineUpdating{Quantity: &upQty}
	if err := bus.PublishSync(ctx, up); err != nil || *up.Quantity != 5 {
		t.Errorf("cart update: qty %v err %v, want 5", *up.Quantity, err)
	}
	val := &order.CheckoutValidating{DiscountCents: 10, RiskScore: 20}
	if err := bus.PublishSync(ctx, val); err != nil || val.DiscountCents != 60 || val.RiskScore != 40 || len(val.Tags) != 1 {
		t.Errorf("checkout validating: %+v err %v, want discount 60, risk 40, one tag", val, err)
	}
	placing := &order.OrderPlacing{}
	if err := bus.PublishSync(ctx, placing); err != nil {
		t.Fatal(err)
	}
	if placing.TaxCents == nil || *placing.TaxCents != 120 || placing.ShippingCents == nil || *placing.ShippingCents != 0 || placing.RiskScore != 40 {
		t.Errorf("order placing: tax %v shipping %v risk %d, want 120, 0 (set, not defaulted) and 40", placing.TaxCents, placing.ShippingCents, placing.RiskScore)
	}
	reg := &customer.CustomerRegistering{}
	if err := bus.PublishSync(ctx, reg); err != nil || len(reg.Tags) != 1 || reg.Tags[0] != "corporate" {
		t.Errorf("customer registering: tags %v err %v", reg.Tags, err)
	}
	if reg.GroupID == nil || *reg.GroupID != b2bGroup || reg.MarketingConsent == nil || *reg.MarketingConsent {
		t.Errorf("customer registering: group %v consent %v, want the plugin's group and an explicit no", reg.GroupID, reg.MarketingConsent)
	}
}

// The values a sync webhook's response is refused for are refused here too
func TestSettersRefuseWhatAWebhookWouldBeRefused(t *testing.T) {
	var add sdk.CartLineAdding = &cart.LineAdding{}
	var up sdk.CartLineUpdating = &cart.LineUpdating{}
	var placing sdk.OrderPlacing = &order.OrderPlacing{}
	for name, err := range map[string]error{
		"negative price":    add.SetLineUnitPrice(-1),
		"zero quantity":     add.SetLineQuantity(0),
		"negative quantity": up.SetLineQuantity(-2),
		"negative tax":      placing.SetPlacingTaxCents(-5),
		"negative shipping": placing.SetPlacingShippingCents(-5),
	} {
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A handler reading tags gets a copy - appending to it must not reach the event behind core's back
func TestTagReadsAreCopies(t *testing.T) {
	ev := &order.OrderPlacing{Tags: make([]string, 1, 4)}
	tags := sdk.OrderPlacing(ev).CheckoutTags()
	_ = append(tags, "sneaky")
	tags[0] = "changed"
	if ev.Tags[0] != "" || len(ev.Tags) != 1 {
		t.Errorf("reading tags let a handler change them: %v", ev.Tags)
	}
}

// The seven added after the first five, each driven the way a plugin would: subscribe by the sdk name, assert, change or refuse
func TestAPluginDrivesTheLaterEventsThroughTheRealBus(t *testing.T) {
	bus := freshHost(t)
	p := sdk.NewPluginForTest("seams-2")
	locked := errors.New("free gift stays while the cart qualifies")
	sole := errors.New("the company's only admin")
	// one handler for both adds - the bundle event arrives as a CartLineAdding
	capQty := func(_ context.Context, e any) error {
		line := e.(sdk.CartLineAdding)
		if line.LineQuantity() > 10 {
			return line.SetLineQuantity(10)
		}
		return nil
	}
	p.SubscribeSync(sdk.EventCartLineAdding, 100, capQty)
	p.SubscribeSync(sdk.EventCartBundleAdding, 100, capQty)
	gift := uuid.New()
	p.SubscribeSync(sdk.EventCartLineRemoving, 100, func(_ context.Context, e any) error {
		if e.(sdk.CartLineRemoving).RemovingLineID() == gift {
			return locked
		}
		return nil
	})
	p.SubscribeSync(sdk.EventOrderCancelling, 100, func(_ context.Context, e any) error {
		e.(sdk.OrderCancelling).AddCancellingNote("ERP notified")
		return nil
	})
	p.SubscribeSync(sdk.EventOrderFulfilling, 100, func(_ context.Context, e any) error {
		f := e.(sdk.OrderFulfilling)
		return errors.Join(f.SetFulfillingCarrier("dhl"), f.SetFulfillingTrackingNumber("JD0001"))
	})
	warehouse := uuid.New()
	p.SubscribeSync(sdk.EventInventoryAllocating, 100, func(_ context.Context, e any) error {
		a := e.(sdk.InventoryAllocating)
		for _, id := range a.AllocatingLineIDs() {
			if err := a.RouteAllocatingLine(id, warehouse); err != nil {
				return err
			}
		}
		return nil
	})
	p.SubscribeSync(sdk.EventReviewSubmitting, 100, func(_ context.Context, e any) error {
		r := e.(sdk.ReviewSubmitting)
		r.AddReviewTags("spam")
		return r.SetReviewStatus(sdk.ReviewStatusRejected)
	})
	p.SubscribeSync(sdk.EventCustomerDeleting, 100, func(_ context.Context, e any) error {
		if e.(sdk.CustomerDeleting).DeletingEmail() == "admin@company.test" {
			return sole
		}
		return nil
	})
	ctx := context.Background()

	for name, ev := range map[string]interface {
		events.Event
		sdk.CartLineAdding
	}{"line": &cart.LineAdding{Quantity: ptrTo(25.0)}, "bundle": &cart.BundleAdding{Quantity: ptrTo(25.0)}} {
		if err := bus.PublishSync(ctx, ev); err != nil || ev.LineQuantity() != 10 {
			t.Errorf("%s add: qty %v err %v, want capped to 10", name, ev.LineQuantity(), err)
		}
	}
	if err := bus.PublishSync(ctx, &cart.LineRemoving{LineID: gift}); !errors.Is(err, locked) {
		t.Errorf("removing the gift: %v, want it refused", err)
	}
	if err := bus.PublishSync(ctx, &cart.LineRemoving{LineID: uuid.New()}); err != nil {
		t.Errorf("removing another line: %v", err)
	}
	cancel := &order.OrderCancelling{Note: "customer asked"}
	if err := bus.PublishSync(ctx, cancel); err != nil || cancel.Note != "customer asked ERP notified" {
		t.Errorf("cancelling: note %q err %v", cancel.Note, err)
	}
	fulfil := &order.OrderFulfilling{}
	if err := bus.PublishSync(ctx, fulfil); err != nil || fulfil.Carrier != "dhl" || fulfil.TrackingNumber != "JD0001" {
		t.Errorf("fulfilling: %+v err %v", fulfil, err)
	}
	line := uuid.New()
	alloc := &inventory.Allocating{Lines: []inventory.ReserveLine{{OrderLineID: line, Quantity: 2}}}
	if err := bus.PublishSync(ctx, alloc); err != nil || alloc.Routing == nil || (*alloc.Routing)[line] != warehouse {
		t.Errorf("allocating: routing %v err %v, want the line on the plugin's warehouse", alloc.Routing, err)
	}
	status, tags := review.StatusPending, []string{}
	sub := &review.Submitting{Status: &status, Tags: &tags}
	if err := bus.PublishSync(ctx, sub); err != nil || status != review.StatusRejected || len(tags) != 1 {
		t.Errorf("review: status %q tags %v err %v", status, tags, err)
	}
	if err := bus.PublishSync(ctx, &customer.CustomerDeleting{Email: "admin@company.test"}); !errors.Is(err, sole) {
		t.Errorf("deleting the sole admin: %v, want it refused", err)
	}
}

func TestTheLaterSettersRefuseWhatAWebhookWouldBeRefused(t *testing.T) {
	var fulfil sdk.OrderFulfilling = &order.OrderFulfilling{}
	var alloc sdk.InventoryAllocating = &inventory.Allocating{}
	status := review.StatusPending
	var sub sdk.ReviewSubmitting = &review.Submitting{Status: &status}
	for name, err := range map[string]error{
		"empty carrier":           fulfil.SetFulfillingCarrier(" "),
		"empty tracking number":   fulfil.SetFulfillingTrackingNumber(""),
		"a line not allocated":    alloc.RouteAllocatingLine(uuid.New(), uuid.New()),
		"an unknown review state": sub.SetReviewStatus("published"),
	} {
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if status != review.StatusPending {
		t.Errorf("a refused status still changed the review to %q", status)
	}
}

func ptrTo[T any](v T) *T { return &v }

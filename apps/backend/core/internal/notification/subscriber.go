package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/webform"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

// Notifier turns selected domain events into inbox notifications - It subscribes async (post-commit) on the event bus,
// mirroring ordermail/activity - Creates are deduped by key, so a failed one returns its error and the redelivery that follows is harmless
type Notifier struct{ svc *Service }

func NewNotifier(svc *Service) *Notifier { return &Notifier{svc: svc} }

// Register wires the event-driven inbox producers onto the bus
func (n *Notifier) Register(bus *events.Bus) {
	if bus == nil {
		return
	}
	bus.Subscribe(order.EventReturnRequested, n.onReturnRequested)
	bus.Subscribe(review.EventSubmitted, n.onReviewSubmitted)
	bus.Subscribe(webform.EventFormSubmitted, n.onWebFormSubmitted)
	bus.Subscribe(inventory.EventStockLow, n.onStockLow)
	bus.Subscribe(inventory.EventStockOut, n.onStockOut)
	bus.Subscribe(ratelimit.EventThrottleTripped, n.onThrottleTripped)
}

// onThrottleTripped surfaces an abusive client: an IP that kept hammering a throttled surface through the slow-down zone until it hit refusal - The bus event already fires at most once per (IP, window); the dedupe key adds an hourly bucket so a sustained attack lands ONE inbox item per IP per hour instead of one per minute, while a fresh attack the next hour still notifies
func (n *Notifier) onThrottleTripped(ctx context.Context, e events.Event) error {
	ev, ok := e.(*ratelimit.ThrottleTripped)
	if !ok {
		return nil
	}
	input := Input{
		Category: CategorySystem,
		Severity: SeverityWarning,
		Title:    "Abusive traffic throttled",
		Body: fmt.Sprintf("IP %s pushed %d %s queries inside one minute (budget %d/min) and is now being refused for the rest of the window.",
			ev.IP, ev.Count, ev.Surface, ev.Budget),
		RequiredPermission: PermSystem,
		EntityType:         "ip",
		EntityID:           ev.IP,
		DedupeKey: fmt.Sprintf("throttle:%s:%s:%d", ev.Surface, ev.IP,
			time.Now().Truncate(time.Hour).Unix()),
	}
	if ev.ChannelID != uuid.Nil {
		c := ev.ChannelID
		input.ChannelID = &c
	}
	return n.svc.Create(ctx, input)
}

func (n *Notifier) onReturnRequested(ctx context.Context, e events.Event) error {
	ev, ok := e.(*order.OrderReturnRequested)
	if !ok {
		return nil
	}
	ch := ev.ChannelID
	return n.svc.Create(ctx, Input{
		Category:           CategoryReturn,
		Severity:           SeverityWarning,
		Title:              "Return requested",
		Body:               "A customer requested a return - it's awaiting your review.",
		RequiredPermission: PermOrders,
		ChannelID:          &ch,
		EntityType:         "order",
		EntityID:           ev.OrderID.String(),
		ActionURL:          "/orders/" + ev.OrderID.String(),
		DedupeKey:          "return_requested:" + ev.ReturnID.String(),
	})
}

func (n *Notifier) onReviewSubmitted(ctx context.Context, e events.Event) error {
	ev, ok := e.(*review.Submitted)
	if !ok || ev.Status != review.StatusPending {
		return nil // only pending reviews need moderation
	}
	ch := ev.ChannelID
	return n.svc.Create(ctx, Input{
		Category:           CategoryReview,
		Severity:           SeverityWarning,
		Title:              "Review awaiting moderation",
		Body:               "A new product review needs approval before it goes live.",
		RequiredPermission: PermReviews,
		ChannelID:          &ch,
		EntityType:         "review",
		EntityID:           ev.ReviewID.String(),
		ActionURL:          "/reviews/" + ev.ReviewID.String(),
		DedupeKey:          "review_pending:" + ev.ReviewID.String(),
	})
}

func (n *Notifier) onWebFormSubmitted(ctx context.Context, e events.Event) error {
	ev, ok := e.(*webform.FormSubmitted)
	if !ok {
		return nil
	}
	ch := ev.ChannelID
	return n.svc.Create(ctx, Input{
		Category:           CategoryForm,
		Severity:           SeverityInfo,
		Title:              "New form submission",
		Body:               fmt.Sprintf("A new submission arrived for %q.", ev.FormName),
		RequiredPermission: PermForms,
		ChannelID:          &ch,
		EntityType:         "webform",
		EntityID:           ev.FormID.String(),
		ActionURL:          "/cms/forms/" + ev.FormID.String() + "/submissions",
		DedupeKey:          "webform_submission:" + ev.SubmissionID.String(),
	})
}

func (n *Notifier) onStockLow(ctx context.Context, e events.Event) error {
	ev, ok := e.(*inventory.StockLow)
	if !ok {
		return nil
	}
	// Inventory is cross-channel (location-based) → global notification
	return n.svc.Create(ctx, Input{
		Category:           CategoryInventory,
		Severity:           SeverityWarning,
		Title:              "Low stock",
		Body:               fmt.Sprintf("A product dropped to %g (threshold %g) - consider restocking.", ev.Quantity, ev.Threshold),
		RequiredPermission: PermInventory,
		EntityType:         "product",
		EntityID:           ev.ProductID.String(),
		ActionURL:          "/products/" + ev.ProductID.String(),
		DedupeKey:          fmt.Sprintf("stock_low:%s:%s:%s", ev.InventoryItemID, ev.LocationID, n.svc.day()),
	})
}

func (n *Notifier) onStockOut(ctx context.Context, e events.Event) error {
	ev, ok := e.(*inventory.StockOut)
	if !ok {
		return nil
	}
	return n.svc.Create(ctx, Input{
		Category:           CategoryInventory,
		Severity:           SeverityCritical,
		Title:              "Out of stock",
		Body:               "A product is out of stock and can no longer be sold.",
		RequiredPermission: PermInventory,
		EntityType:         "product",
		EntityID:           ev.ProductID.String(),
		ActionURL:          "/products/" + ev.ProductID.String(),
		DedupeKey:          fmt.Sprintf("stock_out:%s:%s:%s", ev.InventoryItemID, ev.LocationID, n.svc.day()),
	})
}

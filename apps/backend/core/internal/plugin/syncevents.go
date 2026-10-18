package plugin

import (
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Every before-event the SDK exposes, pinned where core's events and the SDK interfaces meet - a plugin reaches the event
// through `any`, so a renamed accessor would otherwise fail its assertion silently and switch the hook off. syncevents_test.go
// pins the event names the same way
var (
	_ sdk.Tendering           = (*order.OrderTendering)(nil)
	_ sdk.CartLineAdding      = (*cart.LineAdding)(nil)
	_ sdk.CartLineUpdating    = (*cart.LineUpdating)(nil)
	_ sdk.CheckoutValidating  = (*order.CheckoutValidating)(nil)
	_ sdk.OrderPlacing        = (*order.OrderPlacing)(nil)
	_ sdk.CustomerRegistering = (*customer.CustomerRegistering)(nil)
	_ sdk.CartLineAdding      = (*cart.BundleAdding)(nil)
	_ sdk.CartLineRemoving    = (*cart.LineRemoving)(nil)
	_ sdk.OrderCancelling     = (*order.OrderCancelling)(nil)
	_ sdk.OrderFulfilling     = (*order.OrderFulfilling)(nil)
	_ sdk.InventoryAllocating = (*inventory.Allocating)(nil)
	_ sdk.ReviewSubmitting    = (*review.Submitting)(nil)
	_ sdk.CustomerDeleting    = (*customer.CustomerDeleting)(nil)
)

package plugin

import (
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// The tender seam's contract, pinned where core's event and the SDK interface meet - a plugin reaches the event
// through `any`, so a renamed accessor would otherwise fail its assertion silently and let the balance go unreserved
var _ sdk.Tendering = (*order.OrderTendering)(nil)

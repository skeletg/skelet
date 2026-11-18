package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/refs"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The most ids productsByIds and customersByIds answer in a call, and how many of the one-at-a-time records go by
// between deadline checks
const byIDsMax = 100

// ExposeRefs hands graph/admin/refs this graph's own by-id queries, with their permission checks, for plugins' types
// that point back at core's records
func ExposeRefs(r *Resolver) {
	q := &queryResolver{r}
	refs.Install(refs.Loaders{
		Products:    refs.Loader[model.Product]{Load: q.ProductsByIds, Batch: byIDsMax},
		Customers:   refs.Loader[model.Customer]{Load: q.CustomersByIds, Batch: byIDsMax},
		Orders:      refs.Loader[model.Order]{Load: r.ordersByIDs, Batch: byIDsMax},
		Categories:  refs.Loader[model.Category]{Load: refs.Each(q.Category), Batch: byIDsMax},
		Collections: refs.Loader[model.Collection]{Load: refs.Each(q.Collection), Batch: byIDsMax},
	})
}

// The order query's own gates, asked once for the batch rather than once an order: the permission, then each order's
// channel against the channels it's held on. An order that's gone or on a channel the admin can't see is left out
func (r *Resolver) ordersByIDs(ctx context.Context, ids []string) ([]*model.Order, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "orders.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "orders.view")
	if err != nil {
		return nil, err
	}
	found := make([]order.Order, 0, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		o, err := r.OrderSvc.GetByID(ctx, id)
		if errors.Is(err, order.ErrOrderNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		found = append(found, o)
	}
	visible := auth.FilterByChannel(found, perm, func(o order.Order) uuid.UUID { return o.ChannelID })
	out := make([]*model.Order, 0, len(visible))
	for _, o := range visible {
		out = append(out, toOrderModel(o))
	}
	return out, nil
}

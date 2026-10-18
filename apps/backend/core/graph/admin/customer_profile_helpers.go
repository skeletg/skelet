package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/lib/consentkind"
)

func buildCustomerCommerce(ctx context.Context, r *Resolver, cid uuid.UUID, cust customer.Customer) (*model.CustomerCommerce, error) {
	out := &model.CustomerCommerce{Spend: []*model.AttributedRevenue{}, MemberSince: cust.CreatedAt}
	if cust.Group != nil {
		out.Group = cust.Group.Name
	}

	orders, err := r.OrderSvc.ListOrdersByCustomer(ctx, cid)
	if err != nil {
		return nil, err
	}
	type acc struct {
		cents int64
		n     int32
	}
	byCur := map[string]*acc{}
	for _, o := range orders {
		if o.Status == order.OrderStatusCancelled || o.Status == order.OrderStatusDraft {
			continue
		}
		out.Orders++
		a := byCur[o.CurrencyCode]
		if a == nil {
			a = &acc{}
			byCur[o.CurrencyCode] = a
		}
		a.cents += o.TotalAmount
		a.n++
		t := o.CreatedAt
		if out.FirstOrderAt == nil || t.Before(*out.FirstOrderAt) {
			ft := t
			out.FirstOrderAt = &ft
		}
		if out.LastOrderAt == nil || t.After(*out.LastOrderAt) {
			lt := t
			out.LastOrderAt = &lt
		}
		if rets, e := r.OrderSvc.ListReturns(ctx, o.ID); e == nil {
			out.Returns += int32(len(rets))
		}
	}
	for cur, a := range byCur {
		out.Spend = append(out.Spend, &model.AttributedRevenue{Currency: cur, Cents: int(a.cents), Orders: a.n})
	}

	if subs, e := r.SubscriptionSvc.ListForCustomer(ctx, cid); e == nil {
		for _, s := range subs {
			if s.Status == subscription.StatusActive {
				out.ActiveSubscriptions++
			}
		}
	}
	out.MarketingOptedIn, _ = r.ConsentSvc.IsOptedIn(ctx, cid, consentkind.MarketingEmail)
	return out, nil
}

// A spine timestamp as the contract carries it - absent, or unparseable, reads as never
func parseSignalsTime(v *string) *time.Time {
	if v == nil || *v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return nil
	}
	return &t
}

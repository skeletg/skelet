package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Backs sdk.CreateDraftOrder. Declared here rather than taking *order.Service directly so the host
// keeps one narrow contract per capability, and so tests can substitute one
type DraftOrderCreator interface {
	CreateDraftOrder(ctx context.Context, in order.CreateDraftOrderInput) (order.Order, error)
}

// The customer half - a plugin passes a customer id and core resolves the addresses, because address
// selection and validation are core's concern and a plugin has no business reimplementing them
type DraftOrderCustomers interface {
	GetByIDWithAddresses(ctx context.Context, id uuid.UUID) (customer.Customer, error)
}

type draftOrderDeps struct {
	orders    DraftOrderCreator
	customers DraftOrderCustomers
}

var draftOrders atomic.Pointer[draftOrderDeps]

// Wires draft-order creation, once, after both services exist - lazy like SetConfigService, since
// plugin init() runs long before either does
func SetDraftOrderCreator(orders DraftOrderCreator, customers DraftOrderCustomers) {
	if installedHost.Load() == nil {
		panic("plugin.SetDraftOrderCreator called before plugin.Install")
	}
	if orders == nil || customers == nil {
		panic("plugin.SetDraftOrderCreator called with a nil dependency")
	}
	if !draftOrders.CompareAndSwap(nil, &draftOrderDeps{orders: orders, customers: customers}) {
		panic("plugin.SetDraftOrderCreator called twice")
	}
}

func (h *Host) CreateDraftOrder(ctx context.Context, req sdk.DraftOrderRequest) (sdk.DraftOrderResult, error) {
	deps := draftOrders.Load()
	if deps == nil {
		return sdk.DraftOrderResult{}, sdk.ErrDraftOrdersUnavailable
	}
	if req.CustomerID == uuid.Nil {
		return sdk.DraftOrderResult{}, sdk.Invalid("a draft order needs a customer")
	}
	if len(req.Lines) == 0 {
		return sdk.DraftOrderResult{}, sdk.Invalid("a draft order needs at least one line")
	}

	cust, err := deps.customers.GetByIDWithAddresses(ctx, req.CustomerID)
	if err != nil {
		return sdk.DraftOrderResult{}, fmt.Errorf("load customer: %w", err)
	}
	billing, shipping, err := defaultAddresses(cust)
	if err != nil {
		return sdk.DraftOrderResult{}, err
	}

	lines := make([]order.CreateDraftLine, 0, len(req.Lines))
	for _, l := range req.Lines {
		lines = append(lines, order.CreateDraftLine{
			ProductSku:     l.SKU,
			Quantity:       l.Quantity,
			UnitPriceCents: l.UnitPriceCents,
		})
	}

	draft, err := deps.orders.CreateDraftOrder(ctx, order.CreateDraftOrderInput{
		ChannelID:               req.ChannelID,
		CustomerID:              &req.CustomerID,
		Email:                   cust.Email,
		BillingAddress:          toOrderAddress(billing),
		ShippingAddress:         toOrderAddress(shipping),
		Lines:                   lines,
		CustomerNote:            req.Note,
		SkipRequiredOptionCheck: req.SkipRequiredOptionCheck,
	})
	if err != nil {
		return sdk.DraftOrderResult{}, err
	}
	return sdk.DraftOrderResult{
		OrderID:      draft.ID,
		Number:       draft.Number,
		Status:       draft.Status,
		TotalCents:   draft.TotalAmount,
		CurrencyCode: draft.CurrencyCode,
	}, nil
}

// Shipping falls back to the first saved address and billing falls back to shipping, so a customer
// with one address on file converts cleanly - no address at all is an error the operator fixes, not
// something to paper over with a blank
func defaultAddresses(c customer.Customer) (billing, shipping customer.Address, err error) {
	find := func(id *uuid.UUID) *customer.Address {
		if id == nil {
			return nil
		}
		for i := range c.Addresses {
			if c.Addresses[i].ID == *id {
				return &c.Addresses[i]
			}
		}
		return nil
	}
	s := find(c.DefaultShippingAddressID)
	if s == nil && len(c.Addresses) > 0 {
		s = &c.Addresses[0]
	}
	b := find(c.DefaultBillingAddressID)
	if b == nil {
		b = s
	}
	if b == nil || s == nil {
		return customer.Address{}, customer.Address{}, errors.New("the customer has no saved addresses - add one before converting")
	}
	return *b, *s, nil
}

func toOrderAddress(a customer.Address) order.OrderAddress {
	return order.OrderAddress{
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Company:     a.Company,
		Street1:     a.Street1,
		Street2:     a.Street2,
		City:        a.City,
		State:       a.State,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}

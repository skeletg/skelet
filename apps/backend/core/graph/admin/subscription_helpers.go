package admin

import (
	"github.com/google/uuid"

	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
)

// toSubscriptionTermsModel maps a product's recurring terms to GraphQL.
func toSubscriptionTermsModel(t catalog.SubscriptionTerms) *model.SubscriptionTerms {
	return &model.SubscriptionTerms{
		IntervalUnit:  model.SubscriptionInterval(strings.ToUpper(t.IntervalUnit)),
		IntervalCount: t.IntervalCount,
		TrialDays:     t.TrialDays,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

// toCustomerSubscriptionModel maps a subscription instance to GraphQL.
// invoices is optional (nil for list views, populated for the detail view).
func toCustomerSubscriptionModel(s subscription.Subscription, invoices []subscription.Invoice) *model.CustomerSubscription {
	m := &model.CustomerSubscription{
		PaymentMethodID:    uuidPtrToString(s.PaymentMethodID),
		ID:                 s.ID.String(),
		ProductID:          s.ProductID.String(),
		Status:             model.SubscriptionStatus(strings.ToUpper(s.Status)),
		IntervalUnit:       model.SubscriptionInterval(strings.ToUpper(s.IntervalUnit)),
		IntervalCount:      s.IntervalCount,
		Price:              int32(s.Price),
		CurrencyCode:       s.CurrencyCode,
		Quantity:           s.Quantity,
		Sku:                s.SKU,
		Title:              s.Title,
		CurrentPeriodStart: s.CurrentPeriodStart,
		CurrentPeriodEnd:   s.CurrentPeriodEnd,
		NextBillingAt:      s.NextBillingAt,
		CanceledAt:         s.CanceledAt,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
		Invoices:           make([]*model.SubscriptionInvoice, 0, len(invoices)),
	}
	if s.CustomerID != nil {
		v := s.CustomerID.String()
		m.CustomerID = &v
	}
	if s.OrderID != nil {
		v := s.OrderID.String()
		m.OrderID = &v
	}
	for _, inv := range invoices {
		m.Invoices = append(m.Invoices, toSubscriptionInvoiceModel(inv))
	}
	return m
}

func toSubscriptionInvoiceModel(inv subscription.Invoice) *model.SubscriptionInvoice {
	return &model.SubscriptionInvoice{
		ID:           inv.ID.String(),
		PeriodStart:  inv.PeriodStart,
		PeriodEnd:    inv.PeriodEnd,
		Amount:       int32(inv.Amount),
		CurrencyCode: inv.CurrencyCode,
		Status:       model.SubscriptionInvoiceStatus(strings.ToUpper(inv.Status)),
		Sequence:     inv.Sequence,
		CreatedAt:    inv.CreatedAt,
	}
}

func uuidPtrToString(p *uuid.UUID) *string {
	if p == nil {
		return nil
	}
	s := p.String()
	return &s
}

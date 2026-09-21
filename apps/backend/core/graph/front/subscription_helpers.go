package front

import (
	"github.com/google/uuid"

	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
)

// The front schema exposes a customer-facing subset of the shared CustomerSubscription type - no customer/order IDs or invoices
func toFrontSubscriptionModel(s subscription.Subscription) *model.CustomerSubscription {
	return &model.CustomerSubscription{
		PaymentMethodID:  uuidPtrToStrPtr(s.PaymentMethodID),
		ID:               s.ID.String(),
		ProductID:        s.ProductID.String(),
		Title:            s.Title,
		Status:           model.SubscriptionStatus(strings.ToUpper(s.Status)),
		IntervalUnit:     model.SubscriptionInterval(strings.ToUpper(s.IntervalUnit)),
		IntervalCount:    s.IntervalCount,
		Price:            int32(s.Price),
		CurrencyCode:     s.CurrencyCode,
		CurrentPeriodEnd: s.CurrentPeriodEnd,
		NextBillingAt:    s.NextBillingAt,
		CanceledAt:       s.CanceledAt,
		CreatedAt:        s.CreatedAt,
	}
}

// Used by the PDP to render the recurring price + trial
func toFrontSubscriptionTermsModel(t catalog.SubscriptionTerms) *model.SubscriptionTerms {
	return &model.SubscriptionTerms{
		IntervalUnit:  model.SubscriptionInterval(strings.ToUpper(t.IntervalUnit)),
		IntervalCount: t.IntervalCount,
		TrialDays:     t.TrialDays,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

func uuidPtrToStrPtr(p *uuid.UUID) *string {
	if p == nil {
		return nil
	}
	s := p.String()
	return &s
}

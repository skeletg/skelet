package catalog

import (
	"context"

	"github.com/google/uuid"
)

func (s *subscriptionTermsService) GetSubscriptionTerms(ctx context.Context, productID uuid.UUID) (SubscriptionTerms, error) {
	return s.repo.GetSubscriptionTerms(ctx, productID)
}

// Defaults a zero interval count to 1 and validates the interval unit
func (s *subscriptionTermsService) SetSubscriptionTerms(ctx context.Context, input SetSubscriptionTermsInput) (SubscriptionTerms, error) {
	if input.IntervalCount <= 0 {
		input.IntervalCount = 1
	}
	if input.TrialDays < 0 {
		input.TrialDays = 0
	}
	if !IsValidIntervalUnit(input.IntervalUnit) {
		return SubscriptionTerms{}, ErrSubscriptionTermsInvalid
	}
	return s.repo.UpsertSubscriptionTerms(ctx, input)
}

func (s *subscriptionTermsService) ClearSubscriptionTerms(ctx context.Context, productID uuid.UUID) error {
	return s.repo.DeleteSubscriptionTerms(ctx, productID)
}

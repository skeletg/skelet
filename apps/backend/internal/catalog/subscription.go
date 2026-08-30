package catalog

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Recurring billing config attached 1:1 to a kind=subscription product - purchase starts a subscription re-billing every IntervalCount x IntervalUnit
// TrialDays delays the first renewal charge, 0 bills one interval after purchase - Catalog-side config only - the billing engine is internal/subscription
type SubscriptionTerms struct {
	ProductID     uuid.UUID
	IntervalUnit  string // day | week | month | year
	IntervalCount int32
	TrialDays     int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SetSubscriptionTermsInput struct {
	ProductID     uuid.UUID
	IntervalUnit  string
	IntervalCount int32
	TrialDays     int32
}

var (
	ErrSubscriptionTermsNotFound = errors.New("subscription terms not found")
	ErrSubscriptionTermsInvalid  = errors.New("subscription terms require a valid interval unit and a positive interval count")
)

// Mirrors the CHECK constraint on catalog_subscription_terms
var validIntervalUnits = map[string]bool{
	"day": true, "week": true, "month": true, "year": true,
}

func IsValidIntervalUnit(u string) bool { return validIntervalUnits[u] }

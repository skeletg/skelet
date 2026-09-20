package review

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// RegisterSubscribers reacts to the review lifecycle. All async - a classification
// or a stale-marking failure must not fail the submission that triggered it.
func RegisterSubscribers(bus *events.Bus, svc *Service, aiEnabled bool) {
	if aiEnabled { // AI moderation: classify each submission async (off the submit hot path) - never changes status - just stores a hint for the human moderator
		bus.Subscribe(EventSubmitted, func(ctx context.Context, e events.Event) error {
			if ev, ok := e.(*Submitted); ok {
				_, _ = svc.ClassifyReview(ctx, ev.ReviewID)
			}
			return nil
		})
	}
	bus.Subscribe(EventApproved, func(ctx context.Context, e events.Event) error { // Approve/reject changes the live review set → mark the product's summary stale; the review-summary-refresh job regenerates it
		if ev, ok := e.(*Approved); ok {
			_ = svc.MarkSummaryStale(ctx, ev.ProductID)
		}
		return nil
	})
	bus.Subscribe(EventRejected, func(ctx context.Context, e events.Event) error {
		if ev, ok := e.(*Rejected); ok {
			_ = svc.MarkSummaryStale(ctx, ev.ProductID)
		}
		return nil
	})
}

package segment

import (
	"context"

	"github.com/google/uuid"
)

// SegmentRef is one audience a customer belongs to, by id and by the slug an operator typed
type SegmentRef struct {
	ID   uuid.UUID
	Slug string
}

// SegmentRefsForCustomer is the customer's native audiences - an external CDP's are unioned in by
// core, which is where a merchant's provider rows live
func (s *Service) SegmentRefsForCustomer(ctx context.Context, customerID uuid.UUID) ([]SegmentRef, error) {
	rows, err := s.repo.SegmentsForCustomer(ctx, customerID)
	if err != nil {
		// The segments live in the signals database, so an unreachable one reads the same as the suite being off
		return nil, nil
	}
	seen := make(map[string]struct{}, len(rows))
	out := make([]SegmentRef, 0, len(rows))
	for _, r := range rows {
		if _, dup := seen[r.Slug]; dup {
			continue
		}
		seen[r.Slug] = struct{}{}
		out = append(out, SegmentRef{ID: r.ID, Slug: r.Slug})
	}
	return out, nil
}

// SlugsForCustomer is the same list, for the callers that gate on the slug an operator typed
func (s *Service) SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) {
	refs, err := s.SegmentRefsForCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Slug)
	}
	return out, nil
}

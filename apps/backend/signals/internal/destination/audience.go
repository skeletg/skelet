package destination

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Audience sync posts a segment's membership to a destination as SHA-256-hashed emails
// The hashing happens where the addresses are, so this service holds no email of its own

// SyncAudience posts the hashed identifiers and returns how many were sent - webhooks only, since GA4 and Meta read neither this shape nor these endpoints
func (s *Service) SyncAudience(ctx context.Context, segmentID, destinationID uuid.UUID, identifiers []string) (int, error) {
	seg, err := s.queries.GetCustomerSegment(ctx, segmentID)
	if err != nil {
		return 0, fmt.Errorf("audience sync: segment: %w", err)
	}
	row, err := s.get(ctx, destinationID)
	if err != nil {
		return 0, err
	}
	if row.Kind != KindWebhook {
		return 0, ErrAudienceNeedsWebhook
	}
	secret, err := s.open(row.Secret)
	if err != nil {
		return 0, err
	}
	body, hashes := BuildAudiencePayload(seg.Name, identifiers)
	if err := s.post(ctx, toDestination(row), secret, body); err != nil {
		return 0, fmt.Errorf("audience sync: %w", err)
	}
	return len(hashes), nil
}

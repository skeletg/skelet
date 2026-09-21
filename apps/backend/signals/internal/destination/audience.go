package destination

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// Audience sync posts a segment's membership to a destination as SHA-256-hashed emails
// The hashing happens where the addresses are, so this service holds no email of its own

// SyncAudience posts the given hashed identifiers and returns how many were sent
func (s *Service) SyncAudience(ctx context.Context, segmentID, destinationID uuid.UUID, identifiers []string) (int, error) {
	seg, err := s.queries.GetCustomerSegment(ctx, segmentID)
	if err != nil {
		return 0, fmt.Errorf("audience sync: segment: %w", err)
	}
	dest, err := s.Get(ctx, destinationID)
	if err != nil {
		return 0, fmt.Errorf("audience sync: destination: %w", err)
	}
	body, hashes := BuildAudiencePayload(seg.Name, identifiers)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest.TargetURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("audience sync: post: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return 0, fmt.Errorf("audience sync: HTTP %d from %s", resp.StatusCode, dest.Name)
	}
	return len(hashes), nil
}

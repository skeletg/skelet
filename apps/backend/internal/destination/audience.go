package destination

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
)

// Audience sync pushes a segment's membership to a destination as SHA-256-hashed
// emails, or exports the same hashes as a CSV - Raw emails never leave the store

// SyncAudience posts the segment's hashed membership and returns how many identifiers were sent
func (s *Service) SyncAudience(ctx context.Context, segmentID, destinationID uuid.UUID) (int, error) {
	seg, err := s.queries.GetCustomerSegment(ctx, segmentID)
	if err != nil {
		return 0, fmt.Errorf("audience sync: segment: %w", err)
	}
	dest, err := s.Get(ctx, destinationID)
	if err != nil {
		return 0, fmt.Errorf("audience sync: destination: %w", err)
	}
	emails, err := s.queries.SegmentMemberEmails(ctx, segmentID)
	if err != nil {
		return 0, fmt.Errorf("audience sync: members: %w", err)
	}
	body, hashes := BuildAudiencePayload(seg.Name, emails)
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

// ExportAudience returns the membership as a hashed-email CSV for manual uploads
func (s *Service) ExportAudience(ctx context.Context, segmentID uuid.UUID) (string, int, error) {
	emails, err := s.queries.SegmentMemberEmails(ctx, segmentID)
	if err != nil {
		return "", 0, fmt.Errorf("audience export: members: %w", err)
	}
	_, hashes := BuildAudiencePayload("", emails)
	return AudienceCSV(hashes), len(hashes), nil
}

// Package suppression is the email suppression list - addresses that must not receive mail regardless of consent, sitting in front of every send as a deliverability guard
// Distinct from consent, which is a per-customer opt-in: suppression is address-level and protects sender reputation
package suppression

import (
	"context"
	"strings"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

const (
	ReasonHardBounce  = "hard_bounce"
	ReasonSoftBounce  = "soft_bounce"
	ReasonComplaint   = "complaint"
	ReasonManual      = "manual"
	ReasonUnsubscribe = "unsubscribe"
)

type Service struct {
	q *db.Queries
}

func New(q *db.Queries) *Service { return &Service{q: q} }

// Lowercases and trims an address so lookups and stores agree - exported so callers can key their own dedupe on the same form
func Normalize(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Fails SAFE: a DB error returns false and allows the send, since a missed suppression is recoverable but a stalled send queue is not
func (s *Service) IsSuppressed(ctx context.Context, email string) bool {
	e := Normalize(email)
	if e == "" {
		return false
	}
	ok, err := s.q.IsEmailSuppressed(ctx, e)
	if err != nil {
		return false
	}
	return ok
}

// The result is keyed by the caller's ORIGINAL input strings, with normalization internal, so callers just index by email with no Normalize dance - Fail-open like IsSuppressed - an error yields an empty set rather than blocking sends
func (s *Service) SuppressedSet(ctx context.Context, emails []string) map[string]bool {
	out := make(map[string]bool, len(emails))
	// Map the normalized address back to the caller's original input(s), so results key to what they passed in
	byNorm := make(map[string][]string, len(emails))
	norm := make([]string, 0, len(emails))
	for _, e := range emails {
		n := Normalize(e)
		if n == "" {
			continue
		}
		if _, seen := byNorm[n]; !seen {
			norm = append(norm, n)
		}
		byNorm[n] = append(byNorm[n], e)
	}
	if len(norm) == 0 {
		return out
	}
	rows, err := s.q.ListActiveSuppressedEmails(ctx, norm)
	if err != nil {
		return out // fail-open, matches IsSuppressed
	}
	for _, sn := range rows {
		for _, orig := range byNorm[sn] {
			out[orig] = true
		}
	}
	return out
}

// Adds or refreshes an immediate suppression - hard bounce, complaint, manual, unsubscribe - Idempotent
func (s *Service) Suppress(ctx context.Context, email, reason, detail string) error {
	e := Normalize(email)
	if e == "" {
		return nil
	}
	return s.q.SuppressEmail(ctx, db.SuppressEmailParams{Email: e, Reason: reason, Detail: detail})
}

// Accumulates a soft bounce and suppresses once the tally reaches threshold - nowSuppressed is true only on the bounce that TRIPS suppression, or any bounce while already suppressed, so the caller notifies exactly once
func (s *Service) RecordSoftBounce(ctx context.Context, email, detail string, threshold int) (bool, int, error) {
	e := Normalize(email)
	if e == "" {
		return false, 0, nil
	}
	if threshold < 1 {
		threshold = 1
	}
	row, err := s.q.RecordSoftBounce(ctx, db.RecordSoftBounceParams{Email: e, Detail: detail, Threshold: int32(threshold)})
	if err != nil {
		return false, 0, err
	}
	return row.Active, int(row.SoftBounces), nil
}

func (s *Service) Unsuppress(ctx context.Context, email string) error {
	return s.q.UnsuppressEmail(ctx, Normalize(email))
}

type Entry struct {
	ID          uuid.UUID
	Email       string
	Reason      string
	Detail      string
	SoftBounces int
	CreatedAt   int64 // unix; resolver maps to Time
}

func (s *Service) List(ctx context.Context, reason string, limit, offset int32) ([]db.ListSuppressionsRow, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.q.ListSuppressions(ctx, db.ListSuppressionsParams{Reason: reason, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountSuppressions(ctx, reason)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

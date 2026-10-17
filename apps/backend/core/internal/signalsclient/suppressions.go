package signalsclient

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The suppression list over the contract, shaped as the send loops read it
// A read fails open and a write is loud: a missed suppression is recoverable, a dropped one keeps mailing an address that complained

const (
	SuppressionHardBounce  = "hard_bounce"
	SuppressionSoftBounce  = "soft_bounce"
	SuppressionComplaint   = "complaint"
	SuppressionManual      = "manual"
	SuppressionUnsubscribe = "unsubscribe"
)

// Suppression is one listed address, for the deliverability screen
type Suppression struct {
	ID          uuid.UUID
	Email       string
	Reason      string
	Detail      string
	SoftBounces int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func (c *Client) IsSuppressed(ctx context.Context, email string) bool {
	if c == nil || normalizeEmail(email) == "" {
		return false
	}
	sys, err := c.System(ctx)
	if err != nil {
		return false
	}
	res, err := c.Suppressions.IsSuppressed(sys, &signalsv1.IsSuppressedRequest{Email: normalizeEmail(email)})
	if err != nil {
		return false
	}
	return res.GetSuppressed()
}

// SuppressedSet is keyed by the caller's ORIGINAL strings, with normalization internal, so a send
// loop indexes by the address it holds - one call for a whole recipient page
func (c *Client) SuppressedSet(ctx context.Context, emails []string) map[string]bool {
	out := make(map[string]bool, len(emails))
	byNorm := make(map[string][]string, len(emails))
	norm := make([]string, 0, len(emails))
	for _, e := range emails {
		n := normalizeEmail(e)
		if n == "" {
			continue
		}
		if _, seen := byNorm[n]; !seen {
			norm = append(norm, n)
		}
		byNorm[n] = append(byNorm[n], e)
	}
	if c == nil || len(norm) == 0 {
		return out
	}
	sys, err := c.System(ctx)
	if err != nil {
		return out
	}
	res, err := c.Suppressions.ListSuppressed(sys, &signalsv1.ListSuppressedRequest{Emails: norm})
	if err != nil {
		return out
	}
	for _, n := range res.GetEmails() {
		for _, orig := range byNorm[n] {
			out[orig] = true
		}
	}
	return out
}

func (c *Client) Suppress(ctx context.Context, email, reason, detail string) error {
	if normalizeEmail(email) == "" {
		return nil // an empty recipient is a caller bug, not a mailbox to suppress
	}
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Suppressions.Suppress(sys, &signalsv1.SuppressRequest{Email: normalizeEmail(email), Reason: reason, Detail: detail})
	return err
}

// RecordSoftBounce answers true only on the bounce that TRIPS suppression, or any bounce while already suppressed
func (c *Client) RecordSoftBounce(ctx context.Context, email, detail string, threshold int) (bool, int, error) {
	if normalizeEmail(email) == "" {
		return false, 0, nil
	}
	sys, err := c.System(ctx)
	if err != nil {
		return false, 0, err
	}
	res, err := c.Suppressions.RecordSoftBounce(sys, &signalsv1.RecordSoftBounceRequest{Email: normalizeEmail(email), Detail: detail, Threshold: int32(threshold)})
	if err != nil {
		return false, 0, err
	}
	return res.GetSuppressed(), int(res.GetSoftBounces()), nil
}

func (c *Client) Unsuppress(ctx context.Context, email string) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Suppressions.Unsuppress(sys, &signalsv1.UnsuppressRequest{Email: normalizeEmail(email)})
	return err
}

func (c *Client) ListSuppressions(ctx context.Context, reason string, limit, offset int32) ([]Suppression, int, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, 0, err
	}
	res, err := c.Suppressions.ListSuppressions(sys, &signalsv1.ListSuppressionsRequest{Reason: reason, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Suppression, 0, len(res.GetSuppressions()))
	for _, row := range res.GetSuppressions() {
		out = append(out, Suppression{
			ID: parseOrNil(row.GetId()), Email: row.GetEmail(), Reason: row.GetReason(), Detail: row.GetDetail(),
			SoftBounces: int(row.GetSoftBounces()),
			CreatedAt:   row.GetCreatedAt().AsTime(), UpdatedAt: row.GetUpdatedAt().AsTime(),
		})
	}
	return out, int(res.GetTotal()), nil
}

// Package audienceio turns a segment's membership into hashed email identifiers for an ad platform - exported for a manual upload, or pushed by the live audience sync.
// The addresses are core's, so the hashing happens here. A hash can't be reversed, but it still identifies a person to the platform it's matched against.
package audienceio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/dataio"
)

const (
	optSegmentID = "segment_id"
	// Resolved by a provider registered at wiring time, same as the promotion-rule picker
	SourceSegments = "customer_segments"
	// Hashed emails are still personal data leaving the system, so both the export and the live sync take the grant the customer export does
	Permission = "customers.manage"

	emailBatch = 5000 // addresses are fetched this many ids at a time, so a large segment isn't one enormous query
)

// Members answers a segment's customer ids - satisfied by the Signals client, since membership is the
// suite's. actorID is who asked: an export runs detached, so it acts as the admin who queued it
type Members func(ctx context.Context, actorID, segmentID uuid.UUID) ([]uuid.UUID, error)

// Emails answers those customers' addresses, in core's own records
type Emails func(ctx context.Context, ids []uuid.UUID) ([]string, error)

// HashIdentifier is the ad-platform normalization - trim, lowercase, SHA-256 hex, the recipe Meta
// and Google Customer Match specify for emails
func HashIdentifier(v string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(v))))
	return hex.EncodeToString(sum[:])
}

// Hashes resolves a segment to the hashed identifiers an ad platform takes, once each, skipping anyone with no address - one person can hold several customer records
func Hashes(ctx context.Context, members Members, emails Emails, actorID, segmentID uuid.UUID) ([]string, error) {
	ids, err := members(ctx, actorID, segmentID)
	if err != nil {
		return nil, fmt.Errorf("audience: members: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for start := 0; start < len(ids); start += emailBatch {
		addrs, err := emails(ctx, ids[start:min(start+emailBatch, len(ids))])
		if err != nil {
			return nil, fmt.Errorf("audience: addresses: %w", err)
		}
		for _, a := range addrs {
			if strings.TrimSpace(a) == "" {
				continue
			}
			h := HashIdentifier(a)
			if _, dup := seen[h]; dup {
				continue
			}
			seen[h] = struct{}{}
			out = append(out, h)
		}
	}
	return out, nil
}

func Resource(members Members, emails Emails) dataio.Resource {
	return dataio.Resource{
		Slug:           "audience",
		Label:          "Audience (hashed)",
		Group:          "Marketing",
		Description:    "One segment's members as hashed email identifiers, the format ad platforms accept for custom audiences.",
		ViewPermission: Permission,
		Fields: []dataio.Field{
			{Name: "hashed_email", Label: "Hashed email", Type: dataio.FieldText, Identifier: true, ReadOnly: true, Description: "Lowercased, trimmed, then SHA-256"},
		},
		Options: []dataio.OptionDef{
			{Name: optSegmentID, Label: "Segment", Type: dataio.OptSelect, Required: true, Source: SourceSegments, Applies: dataio.DirectionExport},
		},
		NewExporter: func(_ context.Context, o dataio.Opts) (dataio.Exporter, error) {
			raw := o.Str(optSegmentID)
			if raw == "" {
				return nil, errors.New("pick the segment to export")
			}
			id, err := uuid.Parse(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid segment id %q", raw)
			}
			return &exporter{members: members, emails: emails, segmentID: id, actorID: o.ActorID}, nil
		},
	}
}

type exporter struct {
	members   Members
	emails    Emails
	segmentID uuid.UUID
	actorID   uuid.UUID
	hashes    []string
}

func (e *exporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	h, err := Hashes(ctx, e.members, e.emails, e.actorID, e.segmentID)
	if err != nil {
		return nil, err
	}
	e.hashes = h
	return nil, nil
}

func (e *exporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	for _, h := range e.hashes {
		r := dataio.NewRow()
		r.Set("hashed_email", h)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

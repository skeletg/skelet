package destination

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/dataio"
)

const (
	optSegmentID = "segment_id"
	// Resolved by a provider registered at wiring time, same as the promotion-rule picker
	SourceSegments = "customer_segments"
)

// AudienceResource is a segment's membership as SHA-256-hashed emails, for a manual upload into an ad platform
// Raw addresses never leave the store, which is why there is no import side - a hash can't be turned back into a customer
func AudienceResource(svc *Service) dataio.Resource {
	return dataio.Resource{
		Slug:           "audience",
		Label:          "Audience (hashed)",
		Group:          "Marketing",
		Description:    "One segment's members as hashed email identifiers, the format ad platforms accept for custom audiences.",
		ViewPermission: "promotions.manage",
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
			return &audienceExporter{svc: svc, segmentID: id}, nil
		},
	}
}

type audienceExporter struct {
	svc       *Service
	segmentID uuid.UUID
	hashes    []string
}

func (e *audienceExporter) Columns(ctx context.Context) ([]dataio.Field, error) {
	emails, err := e.svc.queries.SegmentMemberEmails(ctx, e.segmentID)
	if err != nil {
		return nil, fmt.Errorf("audience export: members: %w", err)
	}
	_, e.hashes = BuildAudiencePayload("", emails)
	return nil, nil
}

func (e *audienceExporter) Stream(_ context.Context, emit func(*dataio.Row) error) error {
	for _, h := range e.hashes {
		r := dataio.NewRow()
		r.Set("hashed_email", h)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

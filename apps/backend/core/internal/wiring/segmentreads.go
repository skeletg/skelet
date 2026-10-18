package wiring

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
)

// Core's own segment reads - a cart being priced, a flow condition, a campaign audience - all over the contract
// Every one fails soft: with the suite unreachable nothing matches

// segmentReader is the slice of the contract client these reads use, named so a test can stand in for it
type segmentReader interface {
	CustomerSegments(ctx context.Context, customerID uuid.UUID) ([]signalsclient.SegmentRef, error)
	SegmentMemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error)
	AssignSegmentMembers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error
}

// SegmentLookup fills promotion's and the front API's audience seams from the shopper's own authority
// Segments are multi-source, so an external CDP's audiences union with the suite's native ones here
type SegmentLookup struct {
	client segmentReader
	remote RemoteSegments // nil leaves membership on the suite's native audiences alone
}

func NewSegmentLookup(c segmentReader, remote RemoteSegments) *SegmentLookup {
	return &SegmentLookup{client: c, remote: remote}
}

func newSegmentLookup(c *signalsclient.Client, prov *remoteprovider.Service, inv *remoteprovider.Invoker) *SegmentLookup {
	return NewSegmentLookup(c, NewRemoteSegmentSource(prov, inv))
}

// SlugsForCustomer is the union, for the callers that gate on the slug an operator typed - a down CDP
// falls back to native-only, and an unreachable suite to whatever the CDP still answers
func (l *SegmentLookup) SlugsForCustomer(ctx context.Context, customerID uuid.UUID) ([]string, error) {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(slug string) {
		if slug == "" {
			return
		}
		if _, dup := seen[slug]; dup {
			return
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	if refs, err := l.client.CustomerSegments(ctx, customerID); err == nil {
		for _, r := range refs {
			add(r.Slug)
		}
	}
	if l.remote != nil {
		if slugs, ok := l.remote.Slugs(ctx, customerID); ok {
			for _, s := range slugs {
				add(s)
			}
		}
	}
	return out, nil
}

// InSegment answers a saved flow's condition, which stored the segment by id - a CDP audience has no
// id of ours, so only native membership can answer it
func (l *SegmentLookup) InSegment(ctx context.Context, customerID, segmentID uuid.UUID) bool {
	refs, err := l.client.CustomerSegments(ctx, customerID)
	if err != nil {
		return false
	}
	for _, r := range refs {
		if r.ID == segmentID {
			return true
		}
	}
	return false
}

// SegmentMemberIDs is the audience a scheduled campaign send mails
func (l *SegmentLookup) SegmentMemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error) {
	return l.client.SegmentMemberIDs(ctx, segmentID)
}

// AssignCustomers is a flow's add_segment node, mid-run
func (l *SegmentLookup) AssignCustomers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error {
	return l.client.AssignSegmentMembers(ctx, segmentID, customerIDs)
}

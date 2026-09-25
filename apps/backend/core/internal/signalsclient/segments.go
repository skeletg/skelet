package signalsclient

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/circuit"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The segment reads core makes for its own work - a cart being priced, a flow condition, a campaign audience
// Every one fails soft: with the suite unreachable nothing matches rather than everything

// SegmentRef is one audience a shopper is in - an external CDP's carries a slug and no id of ours
type SegmentRef struct {
	ID   uuid.UUID
	Slug string
}

// A priced cart waits on this read, so it gets a short deadline and a failing suite is skipped for a cooldown
const (
	segmentReadTimeout   = time.Second
	segmentTripThreshold = 3
	segmentCooldown      = 15 * time.Second
)

type segmentGuard struct {
	breaker *circuit.Breaker
	timeout time.Duration
}

func newSegmentGuard() segmentGuard {
	return segmentGuard{breaker: circuit.New(segmentTripThreshold, segmentCooldown), timeout: segmentReadTimeout}
}

func (c *Client) SetSegmentReadTimeout(d time.Duration) {
	if c != nil && d > 0 {
		c.segments.timeout = d
	}
}

// A caller that gave up says nothing about the suite, so only a read that ran its own course is recorded
func (g segmentGuard) record(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	failed := Unreachable(err)
	before := g.breaker.Info(uuid.Nil).State
	g.breaker.Record(uuid.Nil, !failed)
	after := g.breaker.Info(uuid.Nil).State
	switch {
	case before == circuit.Closed && after != circuit.Closed:
		slog.Warn("signals segment reads failing, skipping them for a cooldown - segment-gated promotions and blocks drop out meanwhile", "cooldown", segmentCooldown, "err", err)
	case before != circuit.Closed && after == circuit.Closed:
		slog.Info("signals segment reads recovered")
	}
}

// CustomerSegments is the audiences one shopper is in, asked for with that shopper's own authority
func (c *Client) CustomerSegments(ctx context.Context, customerID uuid.UUID) ([]SegmentRef, error) {
	cctx, err := c.AsCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	if !c.segments.breaker.Allow(uuid.Nil) {
		return nil, ErrUnavailable
	}
	cctx, cancel := context.WithTimeout(cctx, c.segments.timeout)
	defer cancel()
	res, err := c.Segments.ListCustomerSegments(cctx, &signalsv1.ListCustomerSegmentsRequest{CustomerId: customerID.String()})
	c.segments.record(ctx, err)
	if err != nil {
		return nil, err
	}
	out := make([]SegmentRef, 0, len(res.GetSegments()))
	for _, sg := range res.GetSegments() {
		ref := SegmentRef{Slug: sg.GetSlug()}
		if id, perr := uuid.Parse(sg.GetId()); perr == nil {
			ref.ID = id
		}
		out = append(out, ref)
	}
	return out, nil
}

// SegmentMemberIDs is a segment's whole membership, for the campaign send that mails it - a scheduled
// send has no admin of its own, so core asks as itself
func (c *Client) SegmentMemberIDs(ctx context.Context, segmentID uuid.UUID) ([]uuid.UUID, error) {
	actx, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0)
	token := ""
	for {
		res, err := c.Segments.ListMembers(actx, &signalsv1.ListMembersRequest{
			SegmentId: segmentID.String(), PageSize: 500, PageToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, m := range res.GetMembers() {
			id, perr := uuid.Parse(m.GetCustomerId())
			if perr != nil {
				continue
			}
			out = append(out, id)
		}
		token = res.GetNextPageToken()
		if token == "" {
			return out, nil
		}
	}
}

// AssignSegmentMembers adds customers to an audience, which a flow's add_segment node does mid-run
func (c *Client) AssignSegmentMembers(ctx context.Context, segmentID uuid.UUID, customerIDs []uuid.UUID) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(customerIDs))
	for _, id := range customerIDs {
		ids = append(ids, id.String())
	}
	_, err = c.Segments.AssignCustomers(sys, &signalsv1.AssignCustomersRequest{SegmentId: segmentID.String(), CustomerIds: ids})
	return err
}

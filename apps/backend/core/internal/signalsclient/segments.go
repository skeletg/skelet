package signalsclient

import (
	"context"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The segment reads core makes for its own work - a cart being priced, a flow condition, a campaign audience
// Every one fails soft: with the suite unreachable nothing matches rather than everything

// SegmentRef is one audience a shopper is in - an external CDP's carries a slug and no id of ours
type SegmentRef struct {
	ID   uuid.UUID
	Slug string
}

// CustomerSegments is the audiences one shopper is in, asked for with that shopper's own authority
func (c *Client) CustomerSegments(ctx context.Context, customerID uuid.UUID) ([]SegmentRef, error) {
	cctx, err := c.AsCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	res, err := c.Segments.ListCustomerSegments(cctx, &signalsv1.ListCustomerSegmentsRequest{CustomerId: customerID.String()})
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

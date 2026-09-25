package signalsgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

func seedSegment(t *testing.T, qs *signalsdb.Queries, definition string) uuid.UUID {
	t.Helper()
	row, err := qs.CreateCustomerSegment(context.Background(), signalsdb.CreateCustomerSegmentParams{
		Slug: "seg-" + uuid.NewString()[:8], Name: "Seg", Definition: definition, IsActive: true,
	})
	if err != nil {
		t.Fatalf("seed segment: %v", err)
	}
	return row.ID
}

func TestSegmentsAnswerToTheChannelThatOwnsThem(t *testing.T) {
	qs, deps := channelScopeServer(t)
	deps.Segment = segment.NewService(segment.NewRepository(qs), nil, nil, nil)
	c := signalsv1.NewSegmentServiceClient(dial(t, deps))
	mine, theirs := uuid.New(), uuid.New()
	global := seedSegment(t, qs, "")
	foreignOwned := seedSegment(t, qs, `{"channelId":"`+theirs.String()+`","conditions":[{"event":"page_view"}]}`)
	own := seedRosterCustomer(t, qs, mine, "own@example.test", "Owen")
	foreign := seedRosterCustomer(t, qs, theirs, "far@example.test", "Fay")
	if err := qs.BulkAddCustomersToSegment(context.Background(), signalsdb.BulkAddCustomersToSegmentParams{SegmentID: global, CustomerIds: []uuid.UUID{own, foreign}}); err != nil {
		t.Fatal(err)
	}
	ctx := tokenCtx(t, grantOn("promotions.manage", mine))
	denied := func(name string, err error) {
		t.Helper()
		if status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s answered %v, want PermissionDenied", name, err)
		}
	}

	list, err := c.ListSegments(ctx, &signalsv1.ListSegmentsRequest{PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, sg := range list.GetSegments() {
		seen[sg.GetId()] = true
	}
	if !seen[global.String()] || seen[foreignOwned.String()] {
		t.Errorf("list saw global=%v foreign-owned=%v, want the global one only", seen[global.String()], seen[foreignOwned.String()])
	}
	_, err = c.GetSegment(ctx, &signalsv1.GetSegmentRequest{Id: foreignOwned.String()})
	denied("get another channel's segment", err)
	got, err := c.GetSegment(ctx, &signalsv1.GetSegmentRequest{Id: global.String()})
	if err != nil || got.GetSegment().GetMemberCount() != 1 {
		t.Errorf("global segment count = %d, %v, want only the caller's channel's member", got.GetSegment().GetMemberCount(), err)
	}

	_, err = c.DeleteSegment(ctx, &signalsv1.DeleteSegmentRequest{Id: global.String()})
	denied("delete a global segment with one channel's grant", err)
	_, err = c.ClearMembership(ctx, &signalsv1.ClearMembershipRequest{SegmentId: global.String()})
	denied("clear a global segment with one channel's grant", err)
	name := "Renamed"
	_, err = c.UpdateSegment(ctx, &signalsv1.UpdateSegmentRequest{Id: global.String(), Name: &name})
	denied("rename a global segment with one channel's grant", err)
	_, err = c.CreateSegment(ctx, &signalsv1.CreateSegmentRequest{Slug: "new-" + uuid.NewString()[:8], Name: "New"})
	denied("create a global segment with one channel's grant", err)

	_, err = c.AssignCustomers(ctx, &signalsv1.AssignCustomersRequest{SegmentId: global.String(), CustomerIds: []string{foreign.String()}})
	denied("assign another channel's customer", err)
	_, err = c.RemoveCustomer(ctx, &signalsv1.RemoveCustomerRequest{SegmentId: global.String(), CustomerId: foreign.String()})
	denied("remove another channel's customer", err)
	if _, err := c.AssignCustomers(ctx, &signalsv1.AssignCustomersRequest{SegmentId: global.String(), CustomerIds: []string{own.String()}}); err != nil {
		t.Errorf("assigning the caller's own customer was refused: %v", err)
	}

	if _, err := c.DeleteSegment(tokenCtx(t, auth.Grant{Slug: "promotions.manage"}), &signalsv1.DeleteSegmentRequest{Id: global.String()}); err != nil {
		t.Errorf("a global grant could not delete a global segment: %v", err)
	}
}

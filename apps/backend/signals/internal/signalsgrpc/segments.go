package signalsgrpc

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
)

type segmentServer struct {
	signalsv1.UnimplementedSegmentServiceServer
	svc *segment.Service
}

func (s *segmentServer) ListSegments(ctx context.Context, req *signalsv1.ListSegmentsRequest) (*signalsv1.ListSegmentsResponse, error) {
	if err := requirePermission(ctx, "promotions.view"); err != nil {
		return nil, err
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 25)
	if err != nil {
		return nil, err
	}
	segs, total, err := s.svc.List(ctx, size, offset)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.CustomerSegment, 0, len(segs))
	for _, sg := range segs {
		out = append(out, segmentProto(sg, 0))
	}
	return &signalsv1.ListSegmentsResponse{
		Segments: out, TotalCount: total,
		NextPageToken: nextToken(offset, size, int32(len(segs)), total),
	}, nil
}

func (s *segmentServer) GetSegment(ctx context.Context, req *signalsv1.GetSegmentRequest) (*signalsv1.GetSegmentResponse, error) {
	if err := requirePermission(ctx, "promotions.view"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	sg, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "segment not found")
	}
	count, _ := s.svc.MemberCount(ctx, id)
	return &signalsv1.GetSegmentResponse{Segment: segmentProto(sg, count)}, nil
}

// Members are customers, so this needs the customers permission on top of the segment one - a
// match-everything rule would otherwise enumerate the whole book through a marketing grant
func (s *segmentServer) ListMembers(ctx context.Context, req *signalsv1.ListMembersRequest) (*signalsv1.ListMembersResponse, error) {
	if err := requirePermissionOrSystem(ctx, "promotions.view", "customers.view"); err != nil {
		return nil, err
	}
	id, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 50)
	if err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, auth.GrantCheckerFrom(ctx), "customers.view")
	if err != nil {
		return nil, err
	}
	var channels []uuid.UUID
	if !perm.Bypass() {
		if channels = perm.IDs(); len(channels) == 0 {
			return &signalsv1.ListMembersResponse{}, nil
		}
	}
	rows, err := s.svc.ListMembers(ctx, id, size, offset, channels)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.SegmentMember, 0, len(rows))
	for _, m := range rows {
		out = append(out, &signalsv1.SegmentMember{CustomerId: m.ID.String(), JoinedAt: ts(m.JoinedAt)})
	}
	return &signalsv1.ListMembersResponse{Members: out, NextPageToken: nextToken(offset, size, int32(len(rows)), 0)}, nil
}

func (s *segmentServer) ListSegmentOptions(ctx context.Context, req *signalsv1.ListSegmentOptionsRequest) (*signalsv1.ListSegmentOptionsResponse, error) {
	if err := requirePermission(ctx, "experiments.view"); err != nil {
		return nil, err
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 200)
	if err != nil {
		return nil, err
	}
	segs, _, err := s.svc.List(ctx, size, offset)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.SegmentOption, 0, len(segs))
	for _, sg := range segs {
		if sg.IsActive { // an inactive segment is hidden from every picker
			out = append(out, &signalsv1.SegmentOption{Slug: sg.Slug, Name: sg.Name})
		}
	}
	return &signalsv1.ListSegmentOptionsResponse{Options: out, NextPageToken: nextToken(offset, size, int32(len(segs)), 0)}, nil
}

func (s *segmentServer) CreateSegment(ctx context.Context, req *signalsv1.CreateSegmentRequest) (*signalsv1.CreateSegmentResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	sg, err := s.svc.Create(ctx, segment.CreateInput{
		Slug: req.GetSlug(), Name: req.GetName(), Description: req.GetDescription(),
		IsActive: req.IsActive == nil || req.GetIsActive(), // a segment created without saying so is active
	})
	if err != nil {
		return nil, err
	}
	return &signalsv1.CreateSegmentResponse{Segment: segmentProto(sg, 0)}, nil
}

// Absent fields are left as they stand, which is why each one is read through its presence check
func (s *segmentServer) UpdateSegment(ctx context.Context, req *signalsv1.UpdateSegmentRequest) (*signalsv1.UpdateSegmentResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	existing, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "segment not found")
	}
	in := segment.UpdateInput{
		ID: id, Slug: existing.Slug, Name: existing.Name,
		Description: existing.Description, RefreshSQL: existing.RefreshSQL, IsActive: existing.IsActive,
	}
	if req.Slug != nil {
		in.Slug = req.GetSlug()
	}
	if req.Name != nil {
		in.Name = req.GetName()
	}
	if req.Description != nil {
		in.Description = req.GetDescription()
	}
	if req.IsActive != nil {
		in.IsActive = req.GetIsActive()
	}
	sg, err := s.svc.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	count, _ := s.svc.MemberCount(ctx, id)
	return &signalsv1.UpdateSegmentResponse{Segment: segmentProto(sg, count)}, nil
}

func (s *segmentServer) DeleteSegment(ctx context.Context, req *signalsv1.DeleteSegmentRequest) (*signalsv1.DeleteSegmentResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, id); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteSegmentResponse{Deleted: true}, nil
}

func (s *segmentServer) AssignCustomers(ctx context.Context, req *signalsv1.AssignCustomersRequest) (*signalsv1.AssignCustomersResponse, error) {
	if err := requirePermissionOrSystem(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(req.GetCustomerIds()))
	for _, raw := range req.GetCustomerIds() {
		cid, err := parseID("customer_ids", raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, cid)
	}
	if err := s.svc.AssignCustomers(ctx, id, ids); err != nil {
		return nil, err
	}
	return &signalsv1.AssignCustomersResponse{Assigned: true}, nil
}

func (s *segmentServer) RemoveCustomer(ctx context.Context, req *signalsv1.RemoveCustomerRequest) (*signalsv1.RemoveCustomerResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	segID, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	custID, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.RemoveCustomer(ctx, custID, segID); err != nil {
		return nil, err
	}
	return &signalsv1.RemoveCustomerResponse{Removed: true}, nil
}

func (s *segmentServer) ClearMembership(ctx context.Context, req *signalsv1.ClearMembershipRequest) (*signalsv1.ClearMembershipResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.ClearMembership(ctx, id); err != nil {
		return nil, err
	}
	return &signalsv1.ClearMembershipResponse{Cleared: true}, nil
}

// An empty rules_json clears the rules, turning the segment back to manual membership
func (s *segmentServer) SetRules(ctx context.Context, req *signalsv1.SetRulesRequest) (*signalsv1.SetRulesResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if body := req.GetRulesJson(); body != "" {
		if !json.Valid([]byte(body)) {
			return nil, invalidArgument("rules_json is not valid JSON")
		}
		raw = json.RawMessage(body)
	}
	sg, err := s.svc.SetRules(ctx, id, raw)
	if err != nil {
		return nil, err
	}
	count, _ := s.svc.MemberCount(ctx, id)
	return &signalsv1.SetRulesResponse{Segment: segmentProto(sg, count)}, nil
}

func (s *segmentServer) RefreshSegment(ctx context.Context, req *signalsv1.RefreshSegmentRequest) (*signalsv1.RefreshSegmentResponse, error) {
	if err := requirePermission(ctx, "promotions.manage"); err != nil {
		return nil, err
	}
	id, err := parseID("segment_id", req.GetSegmentId())
	if err != nil {
		return nil, err
	}
	n, err := s.svc.RefreshDynamic(ctx, id)
	if err != nil {
		return nil, err
	}
	return &signalsv1.RefreshSegmentResponse{MemberCount: int32(n)}, nil
}

func segmentProto(sg segment.Segment, memberCount int32) *signalsv1.CustomerSegment {
	return &signalsv1.CustomerSegment{
		Id: sg.ID.String(), Slug: sg.Slug, Name: sg.Name, Description: sg.Description,
		IsActive: sg.IsActive, MemberCount: memberCount, RulesJson: string(sg.Rules),
		CreatedAt: ts(sg.CreatedAt), UpdatedAt: ts(sg.UpdatedAt),
	}
}

// The page token is the offset, which is all the underlying repositories take. It stays opaque to the
// caller so a cursor could replace it later without the contract moving
func page(size int32, token string, fallback int32) (limit, offset int32, err error) {
	limit = fallback
	if size > 0 && size <= 500 {
		limit = size
	}
	if token == "" {
		return limit, 0, nil
	}
	n, convErr := strconv.Atoi(token)
	if convErr != nil || n < 0 {
		return 0, 0, invalidArgument("page_token is not one this server issued")
	}
	return limit, int32(n), nil
}

// Empty on the last page, which is what tells a caller to stop
func nextToken(offset, limit, returned, total int32) string {
	if returned < limit {
		return ""
	}
	next := offset + limit
	if total > 0 && next >= total {
		return ""
	}
	return strconv.Itoa(int(next))
}

// ListCustomerSegments answers about the one shopper the caller's token names and nobody else - a cart
// being priced or a gated block being rendered has no admin behind it, and this is the whole authority it needs
func (s *segmentServer) ListCustomerSegments(ctx context.Context, req *signalsv1.ListCustomerSegmentsRequest) (*signalsv1.ListCustomerSegmentsResponse, error) {
	asked, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	if caller := auth.CustomerCallerFrom(ctx); caller == uuid.Nil || caller != asked {
		return nil, status.Error(codes.PermissionDenied, "this method takes a token naming the customer it asks about")
	}
	refs, err := s.svc.SegmentRefsForCustomer(ctx, asked)
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.CustomerSegmentRef, 0, len(refs))
	for _, r := range refs {
		out = append(out, &signalsv1.CustomerSegmentRef{Id: idOrEmpty(r.ID), Slug: r.Slug})
	}
	return &signalsv1.ListCustomerSegmentsResponse{Segments: out}, nil
}

func idOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

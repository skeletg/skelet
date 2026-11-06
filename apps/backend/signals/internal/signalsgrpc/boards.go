package signalsgrpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/board"
)

type boardServer struct {
	signalsv1.UnimplementedBoardServiceServer
	svc *board.Service
}

// boardActor checks perm on the board's shop and says who's asking: their roles come with core's token, and
// boards.manage there decides what they may do with a shop board
func boardActor(ctx context.Context, perm, channelID string) (uuid.UUID, board.Actor, error) {
	cid, err := channelScope(ctx, perm, channelID)
	if err != nil {
		return uuid.Nil, board.Actor{}, err
	}
	c, ok := auth.AdminFromContext(ctx)
	if !ok {
		return uuid.Nil, board.Actor{}, status.Error(codes.Unauthenticated, "boards are an admin's")
	}
	return cid, board.Actor{
		AdminID: c.AdminID, RoleIDs: c.RoleIDs, Superuser: c.IsSuperuser,
		CanManage: requireChannel(ctx, "boards.manage", cid) == nil,
	}, nil
}

// boardRef is the shop, the actor and the board id every call on one board takes
func boardRef(ctx context.Context, perm, channelID, id string) (uuid.UUID, board.Actor, uuid.UUID, error) {
	cid, a, err := boardActor(ctx, perm, channelID)
	if err != nil {
		return uuid.Nil, a, uuid.Nil, err
	}
	bid, err := parseID("id", id)
	return cid, a, bid, err
}

func (s *boardServer) ListBoards(ctx context.Context, req *signalsv1.ListBoardsRequest) (*signalsv1.ListBoardsResponse, error) {
	cid, a, err := boardActor(ctx, "boards.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	bs, err := s.svc.List(ctx, cid, a, req.GetAll())
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.ListBoardsResponse{Boards: summariesProto(bs)}, nil
}

func (s *boardServer) GetBoard(ctx context.Context, req *signalsv1.GetBoardRequest) (*signalsv1.GetBoardResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	b, err := s.svc.Get(ctx, cid, a, id)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.GetBoardResponse{Board: boardProto(b)}, nil
}

func (s *boardServer) CreateBoard(ctx context.Context, req *signalsv1.CreateBoardRequest) (*signalsv1.CreateBoardResponse, error) {
	cid, a, err := boardActor(ctx, "boards.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	reports := reportsFrom(req.GetReports())
	b, err := s.svc.Create(ctx, cid, a, req.GetName(), reports)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.CreateBoardResponse{Board: boardProto(b)}, nil
}

// Saving takes boards.view on the shop and edit access to the board - an edit share is what lets someone save
func (s *boardServer) SaveBoard(ctx context.Context, req *signalsv1.SaveBoardRequest) (*signalsv1.SaveBoardResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	if req.GetReadAt() == nil {
		return nil, invalidArgument("read_at is required - the board's updated_at as loaded")
	}
	reports := reportsFrom(req.GetReports())
	b, err := s.svc.Save(ctx, cid, a, id, req.GetName(), reports, req.GetReadAt().AsTime())
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.SaveBoardResponse{Board: boardProto(b)}, nil
}

func (s *boardServer) DeleteBoard(ctx context.Context, req *signalsv1.DeleteBoardRequest) (*signalsv1.DeleteBoardResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, cid, a, id); err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.DeleteBoardResponse{}, nil
}

func (s *boardServer) CreateStarterBoards(ctx context.Context, req *signalsv1.CreateStarterBoardsRequest) (*signalsv1.CreateStarterBoardsResponse, error) {
	cid, a, err := boardActor(ctx, "boards.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	bs, err := s.svc.CreateStarters(ctx, cid, a)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.CreateStarterBoardsResponse{Boards: summariesProto(bs)}, nil
}

func (s *boardServer) SetBoardShares(ctx context.Context, req *signalsv1.SetBoardSharesRequest) (*signalsv1.SetBoardSharesResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	shares := make([]board.Share, 0, len(req.GetShares()))
	for _, sh := range req.GetShares() {
		shares = append(shares, board.Share{AdminID: uuidOrNil(sh.GetAdminId()), RoleID: uuidOrNil(sh.GetRoleId()), Level: sh.GetLevel()})
	}
	b, err := s.svc.SetShares(ctx, cid, a, id, shares)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.SetBoardSharesResponse{Board: boardProto(b)}, nil
}

func (s *boardServer) TransferBoard(ctx context.Context, req *signalsv1.TransferBoardRequest) (*signalsv1.TransferBoardResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId()) // own access - the service checks
	if err != nil {
		return nil, err
	}
	to := uuid.Nil
	if req.GetOwnerAdminId() != "" {
		if to, err = parseID("owner_admin_id", req.GetOwnerAdminId()); err != nil {
			return nil, err
		}
	}
	if err := s.svc.Transfer(ctx, cid, a, id, to); err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.TransferBoardResponse{}, nil
}

func (s *boardServer) SetBoardFavourite(ctx context.Context, req *signalsv1.SetBoardFavouriteRequest) (*signalsv1.SetBoardFavouriteResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.SetFavourite(ctx, cid, a, id, req.GetFavourite()); err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.SetBoardFavouriteResponse{}, nil
}

func (s *boardServer) SaveBoardDraft(ctx context.Context, req *signalsv1.SaveBoardDraftRequest) (*signalsv1.SaveBoardDraftResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	if req.GetBaseUpdatedAt() == nil {
		return nil, invalidArgument("base_updated_at is required - the board's updated_at when the edits began")
	}
	at, err := s.svc.SaveDraft(ctx, cid, a, id, []byte(req.GetPayloadJson()), req.GetBaseUpdatedAt().AsTime())
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.SaveBoardDraftResponse{UpdatedAt: timestamppb.New(at)}, nil
}

func (s *boardServer) DiscardBoardDraft(ctx context.Context, req *signalsv1.DiscardBoardDraftRequest) (*signalsv1.DiscardBoardDraftResponse, error) {
	cid, a, id, err := boardRef(ctx, "boards.view", req.GetChannelId(), req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.DiscardDraft(ctx, cid, a, id); err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.DiscardBoardDraftResponse{}, nil
}

func reportsFrom(in []*signalsv1.BoardReport) []board.Report {
	out := make([]board.Report, 0, len(in))
	for _, r := range in {
		d := r.GetDefinition()
		filters := make([]board.Filter, 0, len(d.GetFilters()))
		for _, f := range d.GetFilters() {
			filters = append(filters, board.Filter{Step: int(f.GetStep()), Key: f.GetKey(), Value: f.GetValue()})
		}
		out = append(out, board.Report{Title: r.GetTitle(), Size: r.GetSize(), Definition: board.Definition{
			Kind: d.GetKind(), SinceDays: int(d.GetSinceDays()), Event: d.GetEvent(), Measure: d.GetMeasure(), Unit: d.GetUnit(),
			Breakdown: d.GetBreakdown(), Steps: d.GetSteps(), Filters: filters, WindowHours: int(d.GetWindowHours()),
			StartEvent: d.GetStartEvent(), ReturnEvent: d.GetReturnEvent(), Periods: int(d.GetPeriods()), Depth: int(d.GetDepth()),
			WindowDays: int(d.GetWindowDays()),
		}})
	}
	return out
}

func summariesProto(bs []board.Summary) []*signalsv1.BoardSummary {
	out := make([]*signalsv1.BoardSummary, 0, len(bs))
	for _, b := range bs {
		p := &signalsv1.BoardSummary{
			Id: b.ID.String(), Name: b.Name, UpdatedAt: timestamppb.New(b.UpdatedAt), ReportCount: count32(b.ReportCount),
			Access: b.Access.String(), Favourite: b.Favourite,
		}
		p.OwnerAdminId, p.DraftUpdatedAt = ownerProto(b.Ownership), draftAtProto(b.Ownership)
		out = append(out, p)
	}
	return out
}

func ownerProto(o board.Ownership) string {
	if o.OwnerID == uuid.Nil {
		return ""
	}
	return o.OwnerID.String()
}

func draftAtProto(o board.Ownership) *timestamppb.Timestamp {
	if o.DraftAt.IsZero() {
		return nil
	}
	return timestamppb.New(o.DraftAt)
}

func boardProto(b board.Board) *signalsv1.Board {
	out := &signalsv1.Board{
		Id: b.ID.String(), ChannelId: b.ChannelID.String(), Name: b.Name, UpdatedAt: timestamppb.New(b.UpdatedAt),
		OwnerAdminId: ownerProto(b.Ownership), Access: b.Access.String(), Favourite: b.Favourite,
		DraftUpdatedAt: draftAtProto(b.Ownership),
	}
	for _, sh := range b.Shares {
		p := &signalsv1.BoardShare{Level: sh.Level}
		if sh.AdminID != uuid.Nil {
			p.AdminId = sh.AdminID.String()
		} else {
			p.RoleId = sh.RoleID.String()
		}
		out.Shares = append(out.Shares, p)
	}
	if d := b.Draft; d != nil {
		out.Draft = &signalsv1.BoardDraft{PayloadJson: string(d.Payload), BaseUpdatedAt: timestamppb.New(d.BaseUpdatedAt), UpdatedAt: timestamppb.New(d.UpdatedAt)}
	}
	for _, r := range b.Reports {
		d := r.Definition
		filters := make([]*signalsv1.FunnelFilter, 0, len(d.Filters))
		for _, f := range d.Filters {
			filters = append(filters, &signalsv1.FunnelFilter{Step: count32(f.Step), Key: f.Key, Value: f.Value})
		}
		out.Reports = append(out.Reports, &signalsv1.BoardReport{Title: r.Title, Size: r.Size, Definition: &signalsv1.BoardReportDefinition{
			Kind: d.Kind, SinceDays: count32(d.SinceDays), Event: d.Event, Measure: d.Measure, Unit: d.Unit,
			Breakdown: d.Breakdown, Steps: d.Steps, Filters: filters, WindowHours: count32(d.WindowHours),
			StartEvent: d.StartEvent, ReturnEvent: d.ReturnEvent, Periods: count32(d.Periods), Depth: count32(d.Depth),
			WindowDays: count32(d.WindowDays),
		}})
	}
	return out
}

func boardErr(err error) error {
	switch {
	case errors.Is(err, board.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, board.ErrInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, board.ErrStale):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, board.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	}
	return err
}

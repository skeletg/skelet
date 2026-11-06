package signalsgrpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/board"
)

type boardServer struct {
	signalsv1.UnimplementedBoardServiceServer
	svc *board.Service
}

func (s *boardServer) ListBoards(ctx context.Context, req *signalsv1.ListBoardsRequest) (*signalsv1.ListBoardsResponse, error) {
	cid, err := channelScope(ctx, "boards.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	bs, err := s.svc.List(ctx, cid)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.ListBoardsResponse{Boards: summariesProto(bs)}, nil
}

func (s *boardServer) GetBoard(ctx context.Context, req *signalsv1.GetBoardRequest) (*signalsv1.GetBoardResponse, error) {
	cid, err := channelScope(ctx, "boards.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	b, err := s.svc.Get(ctx, cid, id)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.GetBoardResponse{Board: boardProto(b)}, nil
}

func (s *boardServer) CreateBoard(ctx context.Context, req *signalsv1.CreateBoardRequest) (*signalsv1.CreateBoardResponse, error) {
	cid, err := channelScope(ctx, "boards.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	b, err := s.svc.Create(ctx, cid, req.GetName(), reportsFrom(req.GetReports()))
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.CreateBoardResponse{Board: boardProto(b)}, nil
}

func (s *boardServer) SaveBoard(ctx context.Context, req *signalsv1.SaveBoardRequest) (*signalsv1.SaveBoardResponse, error) {
	cid, err := channelScope(ctx, "boards.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	if req.GetReadAt() == nil {
		return nil, invalidArgument("read_at is required - the board's updated_at as loaded")
	}
	b, err := s.svc.Save(ctx, cid, id, req.GetName(), reportsFrom(req.GetReports()), req.GetReadAt().AsTime())
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.SaveBoardResponse{Board: boardProto(b)}, nil
}

func (s *boardServer) DeleteBoard(ctx context.Context, req *signalsv1.DeleteBoardRequest) (*signalsv1.DeleteBoardResponse, error) {
	cid, err := channelScope(ctx, "boards.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, cid, id); err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.DeleteBoardResponse{}, nil
}

func (s *boardServer) CreateStarterBoards(ctx context.Context, req *signalsv1.CreateStarterBoardsRequest) (*signalsv1.CreateStarterBoardsResponse, error) {
	cid, err := channelScope(ctx, "boards.manage", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	bs, err := s.svc.CreateStarters(ctx, cid)
	if err != nil {
		return nil, boardErr(err)
	}
	return &signalsv1.CreateStarterBoardsResponse{Boards: summariesProto(bs)}, nil
}

func reportsFrom(in []*signalsv1.BoardReport) []board.Report {
	out := make([]board.Report, 0, len(in))
	for _, r := range in {
		d := r.GetDefinition()
		filters := make([]board.Filter, 0, len(d.GetFilters()))
		for _, f := range d.GetFilters() {
			filters = append(filters, board.Filter{Step: int(f.GetStep()), Key: f.GetKey(), Value: f.GetValue()})
		}
		out = append(out, board.Report{Title: r.GetTitle(), Definition: board.Definition{
			Kind: d.GetKind(), SinceDays: int(d.GetSinceDays()), Event: d.GetEvent(), Measure: d.GetMeasure(), Unit: d.GetUnit(),
			Breakdown: d.GetBreakdown(), Steps: d.GetSteps(), Filters: filters, WindowHours: int(d.GetWindowHours()),
			StartEvent: d.GetStartEvent(), ReturnEvent: d.GetReturnEvent(), Periods: int(d.GetPeriods()), Depth: int(d.GetDepth()),
			WindowDays: int(d.GetWindowDays()), Size: d.GetSize(),
		}})
	}
	return out
}

func summariesProto(bs []board.Summary) []*signalsv1.BoardSummary {
	out := make([]*signalsv1.BoardSummary, 0, len(bs))
	for _, b := range bs {
		out = append(out, &signalsv1.BoardSummary{
			Id: b.ID.String(), Name: b.Name, UpdatedAt: timestamppb.New(b.UpdatedAt), ReportCount: count32(b.ReportCount),
		})
	}
	return out
}

func boardProto(b board.Board) *signalsv1.Board {
	out := &signalsv1.Board{Id: b.ID.String(), ChannelId: b.ChannelID.String(), Name: b.Name, UpdatedAt: timestamppb.New(b.UpdatedAt)}
	for _, r := range b.Reports {
		d := r.Definition
		filters := make([]*signalsv1.FunnelFilter, 0, len(d.Filters))
		for _, f := range d.Filters {
			filters = append(filters, &signalsv1.FunnelFilter{Step: count32(f.Step), Key: f.Key, Value: f.Value})
		}
		out.Reports = append(out.Reports, &signalsv1.BoardReport{Title: r.Title, Definition: &signalsv1.BoardReportDefinition{
			Kind: d.Kind, SinceDays: count32(d.SinceDays), Event: d.Event, Measure: d.Measure, Unit: d.Unit,
			Breakdown: d.Breakdown, Steps: d.Steps, Filters: filters, WindowHours: count32(d.WindowHours),
			StartEvent: d.StartEvent, ReturnEvent: d.ReturnEvent, Periods: count32(d.Periods), Depth: count32(d.Depth),
			WindowDays: count32(d.WindowDays), Size: d.Size,
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
	}
	return err
}

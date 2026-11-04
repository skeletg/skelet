package admin

import (
	"context"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r *mutationResolver) CreateBoard(ctx context.Context, channelID string, name string, reports []*model.BoardReportInput) (*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.manage", channelID)
	if err != nil {
		return nil, err
	}
	reps, err := boardReportsIn(reports)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.CreateBoard(authed, &signalsv1.CreateBoardRequest{ChannelId: channelID, Name: name, Reports: reps})
	if err != nil {
		return nil, signalsErr(err)
	}
	return boardOut(res.GetBoard())
}

func (r *mutationResolver) SaveBoard(ctx context.Context, channelID string, id string, name string, reports []*model.BoardReportInput, readAt time.Time) (*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.manage", channelID)
	if err != nil {
		return nil, err
	}
	reps, err := boardReportsIn(reports)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.SaveBoard(authed, &signalsv1.SaveBoardRequest{
		ChannelId: channelID, Id: id, Name: name, Reports: reps, ReadAt: timestamppb.New(readAt),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return boardOut(res.GetBoard())
}

func (r *mutationResolver) DeleteBoard(ctx context.Context, channelID string, id string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.manage", channelID)
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Boards.DeleteBoard(authed, &signalsv1.DeleteBoardRequest{ChannelId: channelID, Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) CreateStarterBoards(ctx context.Context, channelID string) ([]*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.manage", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.CreateStarterBoards(authed, &signalsv1.CreateStarterBoardsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return boardsOut(res.GetBoards())
}

func (r *queryResolver) Boards(ctx context.Context, channelID string) ([]*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.ListBoards(authed, &signalsv1.ListBoardsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return boardsOut(res.GetBoards())
}

func (r *queryResolver) Board(ctx context.Context, channelID string, id string) (*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.GetBoard(authed, &signalsv1.GetBoardRequest{ChannelId: channelID, Id: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	return boardOut(res.GetBoard())
}

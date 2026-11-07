package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r *mutationResolver) CreateBoard(ctx context.Context, channelID string, name string, reports []*model.BoardReportInput) (*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.manage", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.CreateBoard(authed, &signalsv1.CreateBoardRequest{ChannelId: channelID, Name: name, Reports: boardReportsIn(reports)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return r.oneBoard(ctx, res.GetBoard())
}

func (r *mutationResolver) SaveBoard(ctx context.Context, channelID string, id string, name string, reports []*model.BoardReportInput, readAt time.Time) (*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID) // an edit share is what lets someone save - Signals checks it
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.SaveBoard(authed, &signalsv1.SaveBoardRequest{
		ChannelId: channelID, Id: id, Name: name, Reports: boardReportsIn(reports), ReadAt: timestamppb.New(readAt),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return r.oneBoard(ctx, res.GetBoard())
}

func (r *mutationResolver) DeleteBoard(ctx context.Context, channelID string, id string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID) // its owner's - Signals checks
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Boards.DeleteBoard(authed, &signalsv1.DeleteBoardRequest{ChannelId: channelID, Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) CreateStarterBoards(ctx context.Context, channelID string) ([]*model.BoardSummary, error) {
	authed, err := r.signalsChannel(ctx, "boards.manage", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.CreateStarterBoards(authed, &signalsv1.CreateStarterBoardsRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	return r.manyBoards(ctx, res.GetBoards())
}

func (r *mutationResolver) SetBoardShares(ctx context.Context, channelID string, id string, shares []*model.BoardShareInput) (*model.Board, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID) // the owner's to change - Signals checks
	if err != nil {
		return nil, err
	}
	in := make([]*signalsv1.BoardShare, 0, len(shares))
	for _, sh := range shares {
		in = append(in, &signalsv1.BoardShare{AdminId: deref(sh.AdminID), RoleId: deref(sh.RoleID), Level: sh.Level})
	}
	res, err := r.SignalsAPI.Boards.SetBoardShares(authed, &signalsv1.SetBoardSharesRequest{ChannelId: channelID, Id: id, Shares: in})
	if err != nil {
		return nil, signalsErr(err)
	}
	return r.oneBoard(ctx, res.GetBoard())
}

func (r *mutationResolver) TransferBoard(ctx context.Context, channelID string, id string, ownerID *string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID) // own access - Signals checks
	if err != nil {
		return false, err
	}
	to := deref(ownerID)
	if to != "" {
		if err := r.checkBoardOwner(ctx, channelID, to); err != nil {
			return false, err
		}
	}
	if _, err := r.SignalsAPI.Boards.TransferBoard(authed, &signalsv1.TransferBoardRequest{ChannelId: channelID, Id: id, OwnerAdminId: to}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) SetBoardFavourite(ctx context.Context, channelID string, id string, favourite bool) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID)
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Boards.SetBoardFavourite(authed, &signalsv1.SetBoardFavouriteRequest{ChannelId: channelID, Id: id, Favourite: favourite}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) SaveBoardDraft(ctx context.Context, channelID string, id string, payload interface{}, baseUpdatedAt time.Time) (*time.Time, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID) // edit access - Signals checks
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("board draft: %w", err)
	}
	res, err := r.SignalsAPI.Boards.SaveBoardDraft(authed, &signalsv1.SaveBoardDraftRequest{
		ChannelId: channelID, Id: id, PayloadJson: string(raw), BaseUpdatedAt: timestamppb.New(baseUpdatedAt),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	at := res.GetUpdatedAt().AsTime()
	return &at, nil
}

func (r *mutationResolver) DiscardBoardDraft(ctx context.Context, channelID string, id string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID)
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Boards.DiscardBoardDraft(authed, &signalsv1.DiscardBoardDraftRequest{ChannelId: channelID, Id: id}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) RequestBoardAccess(ctx context.Context, channelID string, id string, level string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID)
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Boards.RequestBoardAccess(authed, &signalsv1.RequestBoardAccessRequest{ChannelId: channelID, Id: id, Level: level}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *mutationResolver) DismissBoardAccessRequest(ctx context.Context, channelID string, id string, adminID string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID) // the owner's to do - Signals checks
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Boards.DismissBoardAccessRequest(authed, &signalsv1.DismissBoardAccessRequestRequest{ChannelId: channelID, Id: id, AdminId: adminID}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *queryResolver) Boards(ctx context.Context, channelID string, all *bool) ([]*model.BoardSummary, error) {
	authed, err := r.signalsChannel(ctx, "boards.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Boards.ListBoards(authed, &signalsv1.ListBoardsRequest{ChannelId: channelID, All: all != nil && *all})
	if err != nil {
		return nil, signalsErr(err)
	}
	return r.manyBoards(ctx, res.GetBoards())
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
	return r.oneBoard(ctx, res.GetBoard())
}

func (r *queryResolver) BoardShareTargets(ctx context.Context, channelID string) ([]*model.BoardShareTarget, error) {
	if _, err := r.signalsChannel(ctx, "boards.manage", channelID); err != nil {
		return nil, err
	}
	return r.shareTargets(ctx, uuid.MustParse(channelID)) // signalsChannel parsed it
}

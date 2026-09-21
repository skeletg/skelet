package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) AnswerProductQuestion(ctx context.Context, id string, answer string) (*model.AdminProductQuestion, error) {
	q, err := r.qnaModerationTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	updated, err := r.QnaSvc.Answer(ctx, q.ID, claims.AdminID, answer)
	if err != nil {
		return nil, err
	}
	return toAdminProductQuestion(updated), nil
}

func (r *mutationResolver) RejectProductQuestion(ctx context.Context, id string) (*model.AdminProductQuestion, error) {
	q, err := r.qnaModerationTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	updated, err := r.QnaSvc.Reject(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	return toAdminProductQuestion(updated), nil
}

func (r *mutationResolver) DeleteProductQuestion(ctx context.Context, id string) (bool, error) {
	q, err := r.qnaModerationTarget(ctx, id)
	if err != nil {
		return false, err
	}
	if err := r.QnaSvc.Delete(ctx, q.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) ProductQuestions(ctx context.Context, status *string, limit *int32, offset *int32) ([]*model.AdminProductQuestion, error) {
	// LoadPermittedChannels is not a gate on its own - it hands a caller with no claims Bypass, which reads as every channel
	if err := auth.RequirePermission(ctx, r.AdminSvc, "qna.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "qna.view")
	if err != nil {
		return nil, err
	}
	var channelIDs []uuid.UUID
	if !perm.Bypass() {
		channelIDs = perm.IDs()
		if len(channelIDs) == 0 {
			return []*model.AdminProductQuestion{}, nil
		}
	}
	lim, off := int32(50), int32(0)
	if limit != nil {
		lim = *limit
	}
	if offset != nil {
		off = *offset
	}
	st := ""
	if status != nil {
		st = *status
	}
	questions, err := r.QnaSvc.ListForModeration(ctx, channelIDs, st, lim, off)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AdminProductQuestion, 0, len(questions))
	for _, q := range questions {
		out = append(out, toAdminProductQuestion(q))
	}
	return out, nil
}

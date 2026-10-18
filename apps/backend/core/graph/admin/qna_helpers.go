package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/qna"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *Resolver) qnaModerationTarget(ctx context.Context, id string) (qna.Question, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "qna.moderate"); err != nil {
		return qna.Question{}, err
	}
	qid, err := uuid.Parse(id)
	if err != nil {
		return qna.Question{}, qna.ErrNotFound
	}
	q, err := r.QnaSvc.Get(ctx, qid)
	if err != nil {
		return qna.Question{}, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "qna.moderate", q.ChannelID, nil); err != nil {
		return qna.Question{}, err
	}
	return q, nil
}

func toAdminProductQuestion(q qna.Question) *model.AdminProductQuestion {
	return &model.AdminProductQuestion{
		ID:          q.ID.String(),
		ProductID:   q.ProductID.String(),
		ChannelID:   q.ChannelID.String(),
		AuthorName:  q.AuthorName,
		AuthorEmail: q.AuthorEmail,
		Question:    q.Question,
		Answer:      q.Answer,
		Status:      q.Status,
		CreatedAt:   q.CreatedAt,
		AnsweredAt:  q.AnsweredAt,
	}
}

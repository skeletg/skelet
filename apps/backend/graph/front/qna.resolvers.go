package front

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/front/model"
	"github.com/skeletg/skelet/internal/apperr"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/ptr"
)

// SubmitProductQuestion records a pending question for staff to answer. Rate-limited; the product must be visible on
// the active store. Guests must supply a display name
func (r *mutationResolver) SubmitProductQuestion(ctx context.Context, productID string, question string, authorName *string, authorEmail *string) (bool, error) {
	if err := r.checkRate(ctx, "qna_submit"); err != nil {
		return false, err
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return false, storeRequired()
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return false, invalidID("product")
	}
	visible, err := r.CatalogSvc.FilterVisibleProductIDsForChannel(ctx, []uuid.UUID{pid}, *storeID)
	if err != nil {
		return false, err
	}
	if len(visible) == 0 {
		return false, apperr.New("QNA_PRODUCT_NOT_FOUND", apperr.ClassNotFound, "product not found")
	}

	name := strings.TrimSpace(ptr.Deref(authorName))
	email := strings.TrimSpace(ptr.Deref(authorEmail))
	customerID := auth.OptionalCustomerID(ctx)
	if customerID != nil {
		c, err := r.CustomerSvc.GetByID(ctx, *customerID)
		if err == nil {
			if name == "" {
				name = strings.TrimSpace(c.FirstName + " " + c.LastName)
			}
			email = c.Email
		}
	} else if name == "" {
		return false, apperr.New("QNA_NAME_REQUIRED", apperr.ClassValidation, "a display name is required")
	}

	if _, err := r.QnaSvc.Submit(ctx, pid, *storeID, customerID, name, email, question); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) ProductQuestions(ctx context.Context, productID string, limit *int32) ([]*model.ProductQuestion, error) {
	pid, err := uuid.Parse(productID)
	if err != nil {
		return nil, invalidID("product")
	}
	lim := int32(20)
	if limit != nil {
		lim = *limit
	}
	questions, err := r.QnaSvc.Published(ctx, pid, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ProductQuestion, 0, len(questions))
	for _, q := range questions {
		out = append(out, &model.ProductQuestion{
			ID:         q.ID.String(),
			AuthorName: q.AuthorName,
			Question:   q.Question,
			Answer:     q.Answer,
			AnsweredAt: q.AnsweredAt,
			CreatedAt:  q.CreatedAt,
		})
	}
	return out, nil
}

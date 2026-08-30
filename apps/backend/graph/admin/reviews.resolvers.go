package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/review"
)

// Two service calls and the second is best-effort - a failed content insert leaves the question label-less, which the admin fixes by re-saving on the detail page
func (r *mutationResolver) CreateReviewQuestion(ctx context.Context, input model.CreateReviewQuestionInput) (*model.ReviewQuestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	q, err := r.ReviewSvc.CreateQuestion(ctx, review.CreateQuestionInput{
		Slug:         input.Slug,
		InputType:    modelToReviewInputType(input.InputType),
		IsRequired:   input.IsRequired,
		AllowComment: input.AllowComment,
		SortOrder:    input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	help := ""
	if input.HelpText != nil {
		help = *input.HelpText
	}
	if _, err := r.ReviewSvc.UpsertQuestionContent(ctx, review.QuestionContentInput{
		QuestionID: q.ID,
		Locale:     input.Locale,
		Label:      input.Label,
		HelpText:   help,
	}); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetQuestion(ctx, q.ID)
	if err != nil {
		return nil, err
	}
	return reviewQuestionToModel(full, r.activeLocale(ctx)), nil
}

func (r *mutationResolver) UpdateReviewQuestion(ctx context.Context, input model.UpdateReviewQuestionInput) (*model.ReviewQuestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "question")
	if err != nil {
		return nil, err
	}
	if _, err := r.ReviewSvc.UpdateQuestion(ctx, review.UpdateQuestionInput{
		ID:           id,
		IsRequired:   input.IsRequired,
		AllowComment: input.AllowComment,
		SortOrder:    input.SortOrder,
	}); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetQuestion(ctx, id)
	if err != nil {
		return nil, err
	}
	return reviewQuestionToModel(full, r.activeLocale(ctx)), nil
}

func (r *mutationResolver) DeleteReviewQuestion(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(id, "question")
	if err != nil {
		return false, err
	}
	if err := r.ReviewSvc.DeleteQuestion(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UpsertReviewQuestionContent(ctx context.Context, input model.UpsertReviewQuestionContentInput) (*model.ReviewQuestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	qid, err := parseID(input.QuestionID, "question")
	if err != nil {
		return nil, err
	}
	help := ""
	if input.HelpText != nil {
		help = *input.HelpText
	}
	if _, err := r.ReviewSvc.UpsertQuestionContent(ctx, review.QuestionContentInput{
		QuestionID: qid,
		Locale:     input.Locale,
		Label:      input.Label,
		HelpText:   help,
	}); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetQuestion(ctx, qid)
	if err != nil {
		return nil, err
	}
	return reviewQuestionToModel(full, r.activeLocale(ctx)), nil
}

func (r *mutationResolver) AssignReviewQuestion(ctx context.Context, input model.AssignReviewQuestionInput) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return false, err
	}
	productTypeID, err := parseID(input.ProductTypeID, "product type")
	if err != nil {
		return false, err
	}
	questionID, err := parseID(input.QuestionID, "question")
	if err != nil {
		return false, err
	}
	if err := r.ReviewSvc.AssignQuestionToProductType(ctx, productTypeID, questionID, input.SortOrder); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) UnassignReviewQuestion(ctx context.Context, productTypeID string, questionID string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return false, err
	}
	ptID, err := parseID(productTypeID, "product type")
	if err != nil {
		return false, err
	}
	qID, err := parseID(questionID, "question")
	if err != nil {
		return false, err
	}
	if err := r.ReviewSvc.UnassignQuestionFromProductType(ctx, ptID, qID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ApproveReview(ctx context.Context, id string, note *string) (*model.Review, error) {
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return nil, err
	}
	return r.setReviewStatus(ctx, id, review.StatusApproved, note)
}

func (r *mutationResolver) RejectReview(ctx context.Context, id string, note *string) (*model.Review, error) {
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return nil, err
	}
	return r.setReviewStatus(ctx, id, review.StatusRejected, note)
}

func (r *mutationResolver) RecheckReviewModeration(ctx context.Context, id string) (*model.Review, error) {
	// stores a verdict but never changes status - it's a hint for the human moderator
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("review id", nil)
	}
	if _, err := r.ReviewSvc.ClassifyReview(ctx, uid); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, full)
}

func (r *mutationResolver) ReplyToReview(ctx context.Context, id string, body string) (*model.Review, error) {
	// an empty body clears the reply
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return nil, err
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("review id", nil)
	}
	var adminID uuid.UUID
	if claims, ok := auth.AdminFromContext(ctx); ok {
		adminID = claims.AdminID
	}
	if _, err := r.ReviewSvc.SetMerchantReply(ctx, uid, body, adminID); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, full)
}

func (r *mutationResolver) DraftReviewReply(ctx context.Context, id string) (string, error) {
	// suggestion only, nothing is persisted
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return "", err
	}
	if r.AISvc == nil {
		return "", errNotConfigured("ai")
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return "", errInvalidID("review id", nil)
	}
	rev, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return "", err
	}
	return r.AISvc.DraftReviewReply(ctx, "", rev.Title, rev.Body, avgReviewRating(rev))
}

func (r *mutationResolver) GenerateProductReviewSummary(ctx context.Context, productID string) (string, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.moderate"); err != nil {
		return "", err
	}
	pid, err := uuid.Parse(productID)
	if err != nil {
		return "", errInvalidID("product id", nil)
	}
	return r.ReviewSvc.GenerateProductSummary(ctx, pid, "")
}

func (r *mutationResolver) CreateReviewCategory(ctx context.Context, input model.CreateReviewCategoryInput) (*model.ReviewCategory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	desc := ""
	if input.Description != nil {
		desc = *input.Description
	}
	cat, err := r.ReviewSvc.CreateCategory(ctx, review.CreateCategoryInput{
		Slug:        input.Slug,
		Name:        input.Name,
		Description: desc,
		IsActive:    input.IsActive,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return reviewCategoryToModel(cat), nil
}

func (r *mutationResolver) UpdateReviewCategory(ctx context.Context, input model.UpdateReviewCategoryInput) (*model.ReviewCategory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	uid, err := parseID(input.ID, "category")
	if err != nil {
		return nil, err
	}
	desc := ""
	if input.Description != nil {
		desc = *input.Description
	}
	cat, err := r.ReviewSvc.UpdateCategory(ctx, review.UpdateCategoryInput{
		ID:          uid,
		Name:        input.Name,
		Description: desc,
		IsActive:    input.IsActive,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return reviewCategoryToModel(cat), nil
}

func (r *mutationResolver) DeleteReviewCategory(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(id, "category")
	if err != nil {
		return false, err
	}
	if err := r.ReviewSvc.DeleteCategory(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetReviewCategories(ctx context.Context, id string, categorySlugs []string) (*model.Review, error) {
	// moderator's manual override - an empty list clears all
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "review")
	if err != nil {
		return nil, err
	}
	if _, err := r.ReviewSvc.SetReviewCategories(ctx, uid, categorySlugs); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, full)
}

func (r *mutationResolver) CreateReviewModerationTag(ctx context.Context, input model.CreateReviewModerationTagInput) (*model.ReviewModerationTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	desc := ""
	if input.Description != nil {
		desc = *input.Description
	}
	tag, err := r.ReviewSvc.CreateModerationTag(ctx, review.CreateModerationTagInput{
		Slug:        input.Slug,
		Name:        input.Name,
		Description: desc,
		IsActive:    input.IsActive,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return reviewModerationTagToModel(tag), nil
}

func (r *mutationResolver) UpdateReviewModerationTag(ctx context.Context, input model.UpdateReviewModerationTagInput) (*model.ReviewModerationTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return nil, err
	}
	uid, err := parseID(input.ID, "moderation tag")
	if err != nil {
		return nil, err
	}
	desc := ""
	if input.Description != nil {
		desc = *input.Description
	}
	tag, err := r.ReviewSvc.UpdateModerationTag(ctx, review.UpdateModerationTagInput{
		ID:          uid,
		Name:        input.Name,
		Description: desc,
		IsActive:    input.IsActive,
		SortOrder:   input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return reviewModerationTagToModel(tag), nil
}

func (r *mutationResolver) DeleteReviewModerationTag(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.manage"); err != nil {
		return false, err
	}
	uid, err := parseID(id, "moderation tag")
	if err != nil {
		return false, err
	}
	if err := r.ReviewSvc.DeleteModerationTag(ctx, uid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) SetReviewModerationTags(ctx context.Context, id string, tagSlugs []string) (*model.Review, error) {
	// moderator's manual override - an empty list clears all
	if err := r.requireReviewModerate(ctx, id); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "review")
	if err != nil {
		return nil, err
	}
	if _, err := r.ReviewSvc.SetReviewModerationTags(ctx, uid, tagSlugs); err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, full)
}

func (r *productTypeResolver) ReviewQuestions(ctx context.Context, obj *model.ProductType) ([]*model.ReviewQuestion, error) {
	id, err := parseID(obj.ID, "product type")
	if err != nil {
		return nil, err
	}
	locale := r.activeLocale(ctx)
	questions, err := r.ReviewSvc.ListAssignedQuestionsLocalized(ctx, id, locale)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ReviewQuestion, len(questions))
	for i, q := range questions {
		if q.Content != nil { // ListAssignedQuestionsLocalized already hydrated q.Content - promoting it into AllContent gets the localized label with no extra DB hit
			q.AllContent = []review.QuestionContent{*q.Content}
		}
		out[i] = reviewQuestionToModel(q, locale)
	}
	return out, nil
}

func (r *queryResolver) ReviewQuestions(ctx context.Context) ([]*model.ReviewQuestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	questions, err := r.ReviewSvc.ListQuestions(ctx)
	if err != nil {
		return nil, err
	}
	// Every question's full content list, so the admin list can render per-locale completeness - these pages run under ~50 entries, so the N round-trips are affordable
	locale := r.activeLocale(ctx)
	out := make([]*model.ReviewQuestion, len(questions))
	for i, q := range questions {
		full, err := r.ReviewSvc.GetQuestion(ctx, q.ID)
		if err != nil {
			return nil, err
		}
		out[i] = reviewQuestionToModel(full, locale)
	}
	return out, nil
}

func (r *queryResolver) ReviewQuestion(ctx context.Context, id string) (*model.ReviewQuestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "question")
	if err != nil {
		return nil, err
	}
	q, err := r.ReviewSvc.GetQuestion(ctx, uid)
	if err != nil {
		return nil, err
	}
	return reviewQuestionToModel(q, r.activeLocale(ctx)), nil
}

// Applicable to a product means assigned to its product type - that's the questions table's per-product filter
func (r *queryResolver) ReviewQuestionsForProduct(ctx context.Context, productID string) ([]*model.ReviewQuestion, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	pid, err := parseID(productID, "product")
	if err != nil {
		return nil, err
	}
	prod, err := r.CatalogSvc.GetProductByID(ctx, pid)
	if err != nil {
		return nil, err
	}
	locale := r.activeLocale(ctx)
	questions, err := r.ReviewSvc.ListAssignedQuestionsLocalized(ctx, prod.ProductTypeID, locale)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ReviewQuestion, len(questions))
	for i, q := range questions {
		if q.Content != nil {
			q.AllContent = []review.QuestionContent{*q.Content}
		}
		out[i] = reviewQuestionToModel(q, locale)
	}
	return out, nil
}

// An admin with no reviews.view scope at all gets an empty page
func (r *queryResolver) Reviews(ctx context.Context, filter *model.ReviewFilter, limit *int32, offset *int32) (*model.ReviewListPage, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "reviews.view")
	if err != nil {
		return nil, err
	}
	lim := int32(50)
	if limit != nil {
		lim = *limit
	}
	off := int32(0)
	if offset != nil {
		off = *offset
	}
	repoFilter := review.ListReviewsFilter{}
	if filter != nil {
		if len(filter.Status) > 0 {
			repoFilter.Status = make([]review.Status, len(filter.Status))
			for i, s := range filter.Status {
				repoFilter.Status[i] = modelToReviewStatus(s)
			}
		}
		if filter.ProductID != nil {
			pid, err := uuid.Parse(*filter.ProductID)
			if err == nil {
				repoFilter.ProductID = &pid
			}
		}
		if len(filter.CategorySlugs) > 0 {
			repoFilter.CategorySlugs = filter.CategorySlugs
		}
	}
	// Scoped IN THE QUERY rather than post-filtered, so limit/offset paginate correctly and totalCount matches what the admin can act on with no per-page gaps
	repoFilter.ChannelIDs = perm.IDs() // nil for bypass admins means no channel filter, a non-nil but empty slice matches nothing
	rows, total, err := r.ReviewSvc.ListReviewsForModeration(ctx, repoFilter, lim, off)
	if err != nil {
		return nil, err
	}
	publicURL := r.MediaLibrarySvc.PublicURL
	// Each referenced question once - without it the answer's question stub misses the non-null content/createdAt/updatedAt the admin schema declares and the whole row fails to serialize
	locale := r.activeLocale(ctx)
	questionByID := map[uuid.UUID]*model.ReviewQuestion{}
	for _, row := range rows {
		for _, a := range row.Answers {
			if _, ok := questionByID[a.QuestionID]; ok {
				continue
			}
			q, err := r.ReviewSvc.GetQuestion(ctx, a.QuestionID)
			if err != nil {
				continue
			}
			questionByID[a.QuestionID] = reviewQuestionToModel(q, locale)
		}
	}
	models := make([]*model.Review, 0, len(rows))
	for _, row := range rows {
		models = append(models, reviewToModel(row, publicURL, questionByID))
	}
	return &model.ReviewListPage{Items: models, TotalCount: int32(total)}, nil
}

// A scoped admin asking for a review they can't see gets FORBIDDEN
func (r *queryResolver) Review(ctx context.Context, id string) (*model.Review, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "review")
	if err != nil {
		return nil, err
	}
	rev, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "reviews.view", rev.ChannelID, nil); err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, rev)
}

// Same channel scope as the reviews list, so the hub tiles match the table the admin can actually page through
func (r *queryResolver) ReviewCounts(ctx context.Context, productID *string) (*model.ReviewCounts, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "reviews.view")
	if err != nil {
		return nil, err
	}
	var pid *uuid.UUID
	if productID != nil && *productID != "" {
		parsed, err := uuid.Parse(*productID)
		if err != nil {
			return nil, errInvalidID("product id", nil)
		}
		pid = &parsed
	}
	counts, err := r.ReviewSvc.CountReviewsByStatus(ctx, perm.IDs(), pid)
	if err != nil {
		return nil, err
	}
	pending := int32(counts[review.StatusPending])
	approved := int32(counts[review.StatusApproved])
	rejected := int32(counts[review.StatusRejected])
	return &model.ReviewCounts{
		Pending:  pending,
		Approved: approved,
		Rejected: rejected,
		Total:    pending + approved + rejected,
	}, nil
}

func (r *queryResolver) ReviewCategories(ctx context.Context) ([]*model.ReviewCategory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	cats, err := r.ReviewSvc.ListCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ReviewCategory, len(cats))
	for i, c := range cats {
		out[i] = reviewCategoryToModel(c)
	}
	return out, nil
}

func (r *queryResolver) ReviewCategory(ctx context.Context, id string) (*model.ReviewCategory, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	uid, err := parseID(id, "category")
	if err != nil {
		return nil, err
	}
	cat, err := r.ReviewSvc.GetCategory(ctx, uid)
	if err != nil {
		return nil, err
	}
	return reviewCategoryToModel(cat), nil
}

func (r *queryResolver) ReviewModerationTags(ctx context.Context) ([]*model.ReviewModerationTag, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	tags, err := r.ReviewSvc.ListModerationTags(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ReviewModerationTag, len(tags))
	for i, t := range tags {
		out[i] = reviewModerationTagToModel(t)
	}
	return out, nil
}

func (r *queryResolver) ReviewInsights(ctx context.Context, channelID *string, startDate *time.Time, endDate *time.Time, bucket *string) (*model.ReviewInsights, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "reviews.view"); err != nil {
		return nil, err
	}
	perm, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "reviews.view")
	if err != nil {
		return nil, err
	}
	end := time.Now()
	if endDate != nil {
		end = *endDate
	}
	start := end.AddDate(0, 0, -30)
	if startDate != nil {
		start = *startDate
	}
	f := review.Filter{Start: start, End: end}
	switch {
	case channelID != nil && *channelID != "":
		cid, err := uuid.Parse(*channelID)
		if err != nil {
			return nil, errInvalidID("channel id", nil)
		}
		if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "reviews.view", cid, nil); err != nil {
			return nil, err
		}
		f.ChannelIDs = []uuid.UUID{cid}
	default:
		if ids := perm.IDs(); ids == nil { // nil means bypass admin, so no channel filter at all
			f.AllChannels = true
		} else {
			f.ChannelIDs = ids
		}
	}
	b := "day"
	if bucket != nil {
		switch *bucket {
		case "day", "week", "month":
			b = *bucket
		}
	}
	ins, err := r.ReviewSvc.Insights(ctx, f, b)
	if err != nil {
		return nil, err
	}
	return reviewInsightsToModel(ins), nil
}

package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/review"
)

// Returns the fully hydrated review so the dashboard can re-render the row without a follow-up query
func (r *mutationResolver) setReviewStatus(ctx context.Context, id string, status review.Status, note *string) (*model.Review, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("review id", nil)
	}
	claims, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}
	noteStr := ""
	if note != nil {
		noteStr = *note
	}
	switch status {
	case review.StatusApproved:
		if _, err := r.ReviewSvc.ApproveReview(ctx, uid, claims.AdminID, noteStr); err != nil {
			return nil, err
		}
	case review.StatusRejected:
		if _, err := r.ReviewSvc.RejectReview(ctx, uid, claims.AdminID, noteStr); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported moderation status %q", status)
	}
	full, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, full)
}

// Pulls each referenced question with its localized content attached, so answers carry the right label without a per-row N+1 out of the resolver
func (r *Resolver) hydrateReviewModel(ctx context.Context, rev review.Review) (*model.Review, error) {
	locale := r.activeLocale(ctx)
	byID := map[uuid.UUID]*model.ReviewQuestion{}
	for _, a := range rev.Answers {
		if _, ok := byID[a.QuestionID]; ok {
			continue
		}
		q, err := r.ReviewSvc.GetQuestion(ctx, a.QuestionID)
		if err != nil {
			continue
		}
		byID[a.QuestionID] = reviewQuestionToModel(q, locale)
	}
	return reviewToModel(rev, r.MediaLibrarySvc.PublicURL, byID), nil
}

func reviewCategoryToModel(c review.Category) *model.ReviewCategory {
	return &model.ReviewCategory{
		ID:          c.ID.String(),
		Slug:        c.Slug,
		Name:        c.Name,
		Description: c.Description,
		IsActive:    c.IsActive,
		SortOrder:   c.SortOrder,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func reviewModerationTagToModel(t review.ModerationTag) *model.ReviewModerationTag {
	return &model.ReviewModerationTag{
		ID:          t.ID.String(),
		Slug:        t.Slug,
		Name:        t.Name,
		Description: t.Description,
		IsActive:    t.IsActive,
		SortOrder:   t.SortOrder,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func reviewInsightsToModel(in review.Insights) *model.ReviewInsights {
	out := &model.ReviewInsights{
		Total:      int32(in.Total),
		PrevTotal:  int32(in.PrevTotal),
		ByCategory: make([]*model.ReviewCategoryStat, 0, len(in.ByCategory)),
		Timeseries: make([]*model.ReviewInsightPoint, 0, len(in.Timeseries)),
		Volume:     make([]*model.ReviewVolumePoint, 0, len(in.Volume)),
		ByStatus:   make([]*model.ReviewStatusStat, 0, len(in.ByStatus)),
		ByTag:      make([]*model.ReviewTagStat, 0, len(in.ByTag)),
	}
	for _, s := range in.ByStatus {
		out.ByStatus = append(out.ByStatus, &model.ReviewStatusStat{
			Status: reviewStatusToModel(review.Status(s.Status)),
			Count:  int32(s.Count),
		})
	}
	for _, t := range in.ByTag {
		out.ByTag = append(out.ByTag, &model.ReviewTagStat{
			Slug:      t.Slug,
			Name:      t.Name,
			Count:     int32(t.Count),
			PrevCount: int32(t.PrevCount),
		})
	}
	for _, s := range in.ByCategory {
		out.ByCategory = append(out.ByCategory, &model.ReviewCategoryStat{
			Slug:      s.Slug,
			Name:      s.Name,
			Count:     int32(s.Count),
			PrevCount: int32(s.PrevCount),
		})
	}
	for _, p := range in.Timeseries {
		out.Timeseries = append(out.Timeseries, &model.ReviewInsightPoint{Period: p.Period, Slug: p.Slug, Count: int32(p.Count)})
	}
	for _, v := range in.Volume {
		out.Volume = append(out.Volume, &model.ReviewVolumePoint{Period: v.Period, Count: int32(v.Count)})
	}
	return out
}

// AllContent is always taken from q.AllContent - preloading that slice is the resolver's job when it cares
func reviewQuestionToModel(q review.Question, locale string) *model.ReviewQuestion {
	out := &model.ReviewQuestion{
		ID:           q.ID.String(),
		Slug:         q.Slug,
		InputType:    reviewInputTypeToModel(q.InputType),
		IsRequired:   q.IsRequired,
		AllowComment: q.AllowComment,
		SortOrder:    q.SortOrder,
		CreatedAt:    q.CreatedAt,
		UpdatedAt:    q.UpdatedAt,
	}
	if len(q.AllContent) > 0 { // AllContent first - the per-locale list editor relies on it
		out.AllContent = make([]*model.ReviewQuestionContent, len(q.AllContent))
		for i, c := range q.AllContent {
			cc := c
			out.AllContent[i] = reviewQuestionContentToModel(cc)
		}
	}
	// Content wants an exact locale hit, then the first AllContent entry, then a slug-labeled stub so admin readers never meet a nil pointer
	for i := range q.AllContent {
		if q.AllContent[i].Locale == locale {
			out.Content = reviewQuestionContentToModel(q.AllContent[i])
			break
		}
	}
	if out.Content == nil && len(q.AllContent) > 0 {
		out.Content = reviewQuestionContentToModel(q.AllContent[0])
	}
	if out.Content == nil {
		out.Content = &model.ReviewQuestionContent{
			Locale: locale,
			Label:  q.Slug,
		}
	}
	return out
}

func reviewQuestionContentToModel(c review.QuestionContent) *model.ReviewQuestionContent {
	return &model.ReviewQuestionContent{
		Locale:    c.Locale,
		Label:     c.Label,
		HelpText:  c.HelpText,
		UpdatedAt: c.UpdatedAt,
	}
}

func reviewInputTypeToModel(t review.InputType) model.ReviewQuestionInputType {
	switch t {
	case review.InputTypeRating1To5:
		return model.ReviewQuestionInputTypeRating1_5
	case review.InputTypeYesNo:
		return model.ReviewQuestionInputTypeYesNo
	case review.InputTypeFreeText:
		return model.ReviewQuestionInputTypeFreeText
	}
	return model.ReviewQuestionInputTypeRating1_5
}

func modelToReviewInputType(t model.ReviewQuestionInputType) review.InputType {
	switch t {
	case model.ReviewQuestionInputTypeRating1_5:
		return review.InputTypeRating1To5
	case model.ReviewQuestionInputTypeYesNo:
		return review.InputTypeYesNo
	case model.ReviewQuestionInputTypeFreeText:
		return review.InputTypeFreeText
	}
	return review.InputTypeRating1To5
}

func reviewStatusToModel(s review.Status) model.ReviewStatus {
	switch s {
	case review.StatusPending:
		return model.ReviewStatusPending
	case review.StatusApproved:
		return model.ReviewStatusApproved
	case review.StatusRejected:
		return model.ReviewStatusRejected
	}
	return model.ReviewStatusPending
}

func modelToReviewStatus(s model.ReviewStatus) review.Status {
	switch s {
	case model.ReviewStatusPending:
		return review.StatusPending
	case model.ReviewStatusApproved:
		return review.StatusApproved
	case model.ReviewStatusRejected:
		return review.StatusRejected
	}
	return review.StatusPending
}

func reviewToModel(
	r review.Review,
	publicURL func(string) string, // pass medialibrary's PublicURL bound method - it turns an object key into a renderable URL
	questionByID map[uuid.UUID]*model.ReviewQuestion, // built by the caller, usually from one ListAssignedQuestions call, so answers carry the full localized question
) *model.Review {
	out := &model.Review{
		ID:                 r.ID.String(),
		ProductID:          r.ProductID.String(),
		GuestName:          r.GuestName,
		GuestEmail:         r.GuestEmail,
		ChannelID:          r.ChannelID.String(),
		Locale:             r.Locale,
		Title:              r.Title,
		Body:               r.Body,
		IsVerifiedPurchase: r.IsVerifiedPurchase,
		Status:             reviewStatusToModel(r.Status),
		SubmittedAt:        r.SubmittedAt,
		ModeratorNote:      r.ModeratorNote,
		ModerationLabel:    r.ModerationLabel,
		ModerationReason:   r.ModerationReason,
		MerchantReply:      r.MerchantReply,
	}
	if r.CustomerID != nil {
		s := r.CustomerID.String()
		out.CustomerID = &s
	}
	out.CategorySlugs = r.CategorySlugs // [String!]! is non-null, so these emit an empty array, never null
	if out.CategorySlugs == nil {
		out.CategorySlugs = []string{}
	}
	out.ModerationTags = r.ModerationTags
	if out.ModerationTags == nil {
		out.ModerationTags = []string{}
	}
	if r.MerchantReplyAt != nil {
		t := *r.MerchantReplyAt
		out.MerchantReplyAt = &t
	}
	if r.ModeratedAt != nil {
		t := *r.ModeratedAt
		out.ModeratedAt = &t
	}
	if r.ModeratedBy != nil {
		id := r.ModeratedBy.String()
		out.ModeratedBy = &id
	}
	out.Answers = make([]*model.ReviewAnswer, 0, len(r.Answers))
	for _, a := range r.Answers {
		ans := &model.ReviewAnswer{
			Text: a.TextValue,
		}
		if q := questionByID[a.QuestionID]; q != nil {
			ans.Question = q
		} else {
			ans.Question = &model.ReviewQuestion{ID: a.QuestionID.String()} // not in the hydration map, so a stub keeps the response's identity
		}
		if a.RatingValue != nil {
			r32 := int32(*a.RatingValue)
			ans.Rating = &r32
		}
		if a.BoolValue != nil {
			b := *a.BoolValue
			ans.BoolValue = &b
		}
		out.Answers = append(out.Answers, ans)
	}
	out.Media = make([]*model.ReviewMedia, 0, len(r.Media))
	for _, m := range r.Media {
		mm := &model.ReviewMedia{
			ID:        m.ID.String(),
			URL:       publicURL(m.ObjectKey),
			MimeType:  m.MimeType,
			SizeBytes: int32(m.SizeBytes),
		}
		if m.Width != nil {
			v := *m.Width
			mm.Width = &v
		}
		if m.Height != nil {
			v := *m.Height
			mm.Height = &v
		}
		out.Media = append(out.Media, mm)
	}
	return out
}

// Feeds the AI reply-drafter the review's sentiment - 0 when the review carries no rating answers
func avgReviewRating(rev review.Review) int {
	sum, n := 0, 0
	for _, a := range rev.Answers {
		if a.RatingValue != nil {
			sum += int(*a.RatingValue)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return (sum + n/2) / n
}

// The channel scope can't be inferred from a review id, so the lookup lives here rather than in every mutation
func (r *mutationResolver) requireReviewModerate(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return errInvalidID("review id", nil)
	}
	rev, err := r.ReviewSvc.GetReviewWithDetails(ctx, uid)
	if err != nil {
		return errNotFound("review")
	}
	return auth.RequireChannelPermission(ctx, r.AdminSvc, "reviews.moderate", rev.ChannelID, nil)
}

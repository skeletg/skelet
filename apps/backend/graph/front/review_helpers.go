package front

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	model1 "github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	"github.com/skeletg/skelet/apps/backend/internal/review"
)

func (r *Resolver) voterFromContext(ctx context.Context) (review.Voter, error) { // just gathers the two policy inputs - auth-context customer and X-Review-Voter-Token - and hands them to review.Service.ResolveVoter, the same implementation the dataloader path in cmd/server/main.go uses
	customerID := auth.OptionalCustomerID(ctx)
	var rawToken string
	if req, ok := auth.RequestFromContext(ctx); ok {
		rawToken = req.Header.Get("X-Review-Voter-Token")
	}
	return r.ReviewSvc.ResolveVoter(ctx, customerID, rawToken, r.VoterTokenSecret)
}

func (r *Resolver) voteCountsFor(ctx context.Context, obj *model1.Review) (*review.VoteCounts, error) { // hits the renderCache primed by Product.reviews, then falls through to a single-row lookup for paths that don't list reviews in batch (post-mutation responses)
	if obj == nil || obj.ID == "" {
		return nil, nil
	}
	rid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, nil
	}
	v, err := r.cachedVoteCounts(ctx, rid)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// With product context this keys the localized assigned-questions list (one batched, per-request-cached getAssignedQuestionsLocalized) - the per-distinct-question GetQuestion it replaced could fire 3-5 queries per page render even with that cache warm
// Reviews whose product type is unknown, like the post-vote path hydrating a single review, drop to the per-question lookup below
func (r *Resolver) buildQuestionLookup(ctx context.Context, prod *model1.Product, rows []review.Review) (map[uuid.UUID]*model1.ReviewQuestion, error) {
	out := map[uuid.UUID]*model1.ReviewQuestion{}
	if prod != nil && prod.ProductType != nil {
		if ptID, err := uuid.Parse(prod.ProductType.ID); err == nil {
			questions, err := r.getAssignedQuestionsLocalized(ctx, ptID, r.activeLocale(ctx))
			if err == nil {
				for _, q := range questions {
					out[q.ID] = reviewQuestionToModel(q)
				}
				return out, nil
			}
		}
	}
	loaders := dataloaders.For(ctx) // the Question DataLoader keeps several distinct answer-questions on one review down to a single query, at the cost of AllContent - so the post-mutation response shows non-localized labels
	for _, row := range rows {
		for _, a := range row.Answers {
			if _, ok := out[a.QuestionID]; ok {
				continue
			}
			if loaders != nil {
				q, err := loaders.ReviewQuestion.Load(ctx, a.QuestionID)()
				if err != nil {
					continue
				}
				out[a.QuestionID] = reviewQuestionToModel(q)
				continue
			}
			q, err := r.ReviewSvc.GetQuestion(ctx, a.QuestionID)
			if err != nil {
				continue
			}
			locale := r.activeLocale(ctx)
			for _, c := range q.AllContent {
				if c.Locale == locale {
					cc := c
					q.Content = &cc
					break
				}
			}
			if q.Content == nil && len(q.AllContent) > 0 {
				cc := q.AllContent[0]
				q.Content = &cc
			}
			out[a.QuestionID] = reviewQuestionToModel(q)
		}
	}
	return out, nil
}

// The post-submit / post-vote caller has no product model on hand (the row carries product_id only), so this takes the per-question fallback - one extra round-trip per distinct answered question, and only ever here
func (r *Resolver) hydrateReviewModel(ctx context.Context, rev review.Review) (*model1.Review, error) {
	questionByID, err := r.buildQuestionLookup(ctx, nil, []review.Review{rev})
	if err != nil {
		return nil, err
	}
	return reviewToModel(rev, r.MediaLibrarySvc.PublicURL, questionByID), nil
}

// Content must already be hydrated for the active locale; the front Label/HelpText resolvers read the Content slice, so both Content and AllContent get the same single row and either path works
func reviewQuestionToModel(q review.Question) *model1.ReviewQuestion {
	out := &model1.ReviewQuestion{
		ID:           q.ID.String(),
		Slug:         q.Slug,
		InputType:    inputTypeToModel(q.InputType),
		IsRequired:   q.IsRequired,
		AllowComment: q.AllowComment,
		SortOrder:    q.SortOrder,
		CreatedAt:    q.CreatedAt,
		UpdatedAt:    q.UpdatedAt,
	}
	if q.Content != nil {
		c := *q.Content
		out.Content = &model1.ReviewQuestionContent{
			Locale:    c.Locale,
			Label:     c.Label,
			HelpText:  c.HelpText,
			UpdatedAt: c.UpdatedAt,
		}
		out.AllContent = []*model1.ReviewQuestionContent{out.Content}
	}
	return out
}

func inputTypeToModel(t review.InputType) model1.ReviewQuestionInputType {
	switch t {
	case review.InputTypeRating1To5:
		return model1.ReviewQuestionInputTypeRating1_5
	case review.InputTypeYesNo:
		return model1.ReviewQuestionInputTypeYesNo
	case review.InputTypeFreeText:
		return model1.ReviewQuestionInputTypeFreeText
	}
	return model1.ReviewQuestionInputTypeRating1_5
}

// The PDP never shows the moderator note or id, but the autobound admin/model.Review carries them - setting them anyway is safe because gqlgen projects only what the schema declares
func reviewToModel(
	r review.Review,
	publicURL func(string) string,
	questionByID map[uuid.UUID]*model1.ReviewQuestion,
) *model1.Review {
	out := &model1.Review{
		ID:                 r.ID.String(),
		ProductID:          r.ProductID.String(),
		GuestName:          r.GuestName,
		GuestEmail:         r.GuestEmail,
		ChannelID:          r.ChannelID.String(),
		Locale:             r.Locale,
		Title:              r.Title,
		Body:               r.Body,
		IsVerifiedPurchase: r.IsVerifiedPurchase,
		SubmittedAt:        r.SubmittedAt,
		ModeratorNote:      r.ModeratorNote,
		MerchantReply:      r.MerchantReply,
	}
	if r.CustomerID != nil {
		s := r.CustomerID.String()
		out.CustomerID = &s
	}
	if r.MerchantReplyAt != nil {
		t := *r.MerchantReplyAt
		out.MerchantReplyAt = &t
	}
	switch r.Status { // status is admin-only in spirit, but the autobound struct demands a value - unset rows fall to PENDING
	case review.StatusApproved:
		out.Status = model1.ReviewStatusApproved
	case review.StatusRejected:
		out.Status = model1.ReviewStatusRejected
	default:
		out.Status = model1.ReviewStatusPending
	}
	out.Answers = make([]*model1.ReviewAnswer, 0, len(r.Answers))
	for _, a := range r.Answers {
		ans := &model1.ReviewAnswer{Text: a.TextValue}
		if q, ok := questionByID[a.QuestionID]; ok {
			ans.Question = q
		} else {
			ans.Question = &model1.ReviewQuestion{ID: a.QuestionID.String()}
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
	out.Media = make([]*model1.ReviewMedia, 0, len(r.Media))
	for _, m := range r.Media {
		mm := &model1.ReviewMedia{
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

func computeOverallRating(answers []*model1.ReviewAnswer) *float64 {
	count := 0
	sum := 0.0
	for _, a := range answers {
		if a == nil || a.Rating == nil {
			continue
		}
		sum += float64(*a.Rating)
		count++
	}
	if count == 0 {
		return nil
	}
	avg := sum / float64(count)
	return &avg
}

func sortReviews(rs []*model1.Review, sort model.ReviewSort) { // in place - NEWEST is already the resolver's natural order, everything else gets a stable sort
	swap := func(i, j int) { rs[i], rs[j] = rs[j], rs[i] } // hand-rolled instead of sort.Slice - typical batches are <=50 rows, so a one-pass bubble is fine and keeps the signature simple
	less := func(a, b *model1.Review) bool {
		switch sort {
		case model.ReviewSortOldest:
			return a.SubmittedAt.Before(b.SubmittedAt)
		case model.ReviewSortHighest:
			ar := computeOverallRating(a.Answers)
			br := computeOverallRating(b.Answers)
			if ar != nil && br != nil {
				return *ar > *br
			}
			return ar != nil
		case model.ReviewSortLowest:
			ar := computeOverallRating(a.Answers)
			br := computeOverallRating(b.Answers)
			if ar != nil && br != nil {
				return *ar < *br
			}
			return br != nil
		}
		return a.SubmittedAt.After(b.SubmittedAt) // NEWEST - the repo already hands these back DESC by SubmittedAt
	}
	for i := range rs {
		for j := i + 1; j < len(rs); j++ {
			if less(rs[j], rs[i]) {
				swap(i, j)
			}
		}
	}
}

func productIDFromObj(obj *model1.Product) (uuid.UUID, error) { // fails fast on a malformed UUID rather than carrying it down through the layers
	if obj == nil || obj.ID == "" {
		return uuid.Nil, fmt.Errorf("product context missing")
	}
	return uuid.Parse(obj.ID)
}

// The DataLoader batches N concurrent calls into one query when the middleware is mounted, and the single-row fallback keeps tests and CLI paths that skip the HTTP chain working
func (r *Resolver) loadCustomer(ctx context.Context, id uuid.UUID) (customer.Customer, error) {
	if loaders := dataloaders.For(ctx); loaders != nil {
		return loaders.Customer.Load(ctx, id)()
	}
	return r.CustomerSvc.GetByID(ctx, id)
}

package front

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/applog"
)

// An authenticated caller becomes the author with replace-on-resubmit semantics (UNIQUE partial index); a guest gets input.GuestName as the visible author, and only when reviews/policy/require_authentication is false - the service rejects the rest
// Per-question validation and answer/media bookkeeping are identical either way, with the branching living in the service layer
func (r *mutationResolver) SubmitProductReview(ctx context.Context, input model.SubmitReviewInput) (*model1.Review, error) {
	if err := r.checkRate(ctx, "review_submit"); err != nil {
		return nil, err
	}
	customerIDPtr := channel.OptionalCustomerID(ctx)
	productID, err := uuid.Parse(input.ProductID)
	if err != nil {
		return nil, invalidID("product")
	}
	storeID := channelIDFromContext(ctx) // answers validate against the product_type's curated question set, and going through the catalog service gets store visibility enforced on the way
	if storeID == nil {
		return nil, storeRequired()
	}
	prod, err := r.CatalogSvc.GetProductForChannel(ctx, productID, *storeID, r.activeLocale(ctx))
	if err != nil {
		return nil, err
	}
	answers := make([]review.AnswerInput, 0, len(input.Answers))
	for _, a := range input.Answers {
		if a == nil {
			continue
		}
		qid, err := uuid.Parse(a.QuestionID)
		if err != nil {
			return nil, apperr.New("INVALID_ID", apperr.ClassValidation, fmt.Sprintf("invalid question id %q", a.QuestionID))
		}
		ai := review.AnswerInput{
			QuestionID: qid,
		}
		if a.Rating != nil {
			v := int16(*a.Rating)
			ai.RatingValue = &v
		}
		if a.BoolValue != nil {
			v := *a.BoolValue
			ai.BoolValue = &v
		}
		if a.Text != nil {
			ai.TextValue = *a.Text
		}
		answers = append(answers, ai)
	}
	mediaIDs := make([]uuid.UUID, 0, len(input.MediaAssetIds))
	for _, idStr := range input.MediaAssetIds {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, apperr.New("INVALID_ID", apperr.ClassValidation, fmt.Sprintf("invalid media asset id %q", idStr))
		}
		mediaIDs = append(mediaIDs, id)
	}
	for _, id := range mediaIDs { // confirm what the customer just uploaded - skipping a missing asset is fine, the media insert below raises a clearer FK error on a bogus id
		if _, err := r.MediaLibrarySvc.ConfirmUpload(ctx, medialibrary.ConfirmUploadInput{AssetID: id}); err != nil {
			// Already-confirmed and never-pending are expected on resubmit and get swallowed so resubmits don't 500 - a DB/S3 failure is logged instead, since the review still submits best-effort but the failure has to stay visible
			if !errors.Is(err, medialibrary.ErrAssetNotPending) && !errors.Is(err, medialibrary.ErrAssetNotFound) {
				applog.From(ctx).Warn("review: confirm media upload failed", "asset_id", id, "err", err)
			}
		}
	}
	title := ""
	if input.Title != nil {
		title = *input.Title
	}
	body := ""
	if input.Body != nil {
		body = *input.Body
	}
	submitInput := review.SubmitReviewInput{
		ProductID:     productID,
		CustomerID:    customerIDPtr,
		ChannelID:     *storeID,
		Locale:        r.activeLocale(ctx),
		Title:         strings.TrimSpace(title),
		Body:          strings.TrimSpace(body),
		Answers:       answers,
		MediaAssetIDs: mediaIDs,
	}
	if customerIDPtr == nil {
		if input.GuestName != nil {
			submitInput.GuestName = strings.TrimSpace(*input.GuestName)
		}
		if input.GuestEmail != nil {
			submitInput.GuestEmail = strings.TrimSpace(*input.GuestEmail)
		}
	}
	rev, err := r.ReviewSvc.SubmitReview(ctx, prod.ProductTypeID, submitInput)
	if err != nil {
		return nil, err
	}
	full, err := r.ReviewSvc.GetReviewWithDetails(ctx, rev.ID)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, full)
}

// Presigns an S3 PUT the browser writes to directly with the declared Content-Type, and the returned assetId then rides submitProductReview's mediaAssetIds
// The auth gate mirrors the submission gate - customers always pass, guests only when the policy allows guest submissions, and everything else gets the standard "unauthenticated" error the storefront already handles
func (r *mutationResolver) RequestReviewMediaUpload(ctx context.Context, productID string, filename string, mimeType string, sizeBytes int32) (*model.ReviewMediaUpload, error) {
	if err := r.checkRate(ctx, "review_media_upload"); err != nil {
		return nil, err
	}
	if channel.OptionalCustomerID(ctx) == nil {
		policy := r.ReviewSvc.ReadPolicy(ctx)
		if policy.RequireAuthentication {
			if _, err := channel.RequireCustomerForCurrentStore(ctx); err != nil {
				return nil, err
			}
		}
	}
	// productID has to clear the catalog BEFORE presigning or a hostile caller poisons the media library with arbitrary subdirectories like reviews/foo/ or reviews/../../ - uuid.Parse kills non-UUIDs and GetProductByID kills unknown products
	pid, err := uuid.Parse(productID)
	if err != nil {
		return nil, invalidID("product")
	}
	if _, err := r.CatalogSvc.GetProductByID(ctx, pid); err != nil {
		return nil, err
	}
	// reviews/<productId>/<filename> keeps the tree one level deep, scales into thousands of products and lets admins bulk-clean a retired product's review media - scoping by reviewId is impossible because submitProductReview only creates the row after the upload
	res, err := r.MediaLibrarySvc.RequestUpload(ctx, medialibrary.RequestUploadInput{
		Path:      "reviews/" + pid.String(),
		Filename:  filename,
		MimeType:  mimeType,
		SizeBytes: int64(sizeBytes),
	})
	if err != nil {
		return nil, err
	}
	return &model.ReviewMediaUpload{
		AssetID:   res.Asset.ID.String(),
		UploadURL: res.UploadURL,
		ExpiresAt: res.ExpiresAt,
	}, nil
}

// Voter identity is the authenticated customer first, then the storefront's X-Review-Voter-Token cookie - the anon path only opens when reviews/votes/require_authentication is off, and same browser → same token → same row, so button-mashing on refresh still counts once
// value: 1 = helpful, -1 = not helpful, 0 = clear the caller's vote
func (r *mutationResolver) VoteReview(ctx context.Context, reviewID string, value int32) (*model1.Review, error) {
	if err := r.checkRate(ctx, "review_vote"); err != nil {
		return nil, err
	}
	rid, err := uuid.Parse(reviewID)
	if err != nil {
		return nil, invalidID("review")
	}
	voter, err := r.voterFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := r.ReviewSvc.VoteReview(ctx, voter, rid, int16(value)); err != nil {
		return nil, err
	}
	rev, err := r.ReviewSvc.GetReviewWithDetails(ctx, rid)
	if err != nil {
		return nil, err
	}
	return r.hydrateReviewModel(ctx, rev)
}

func (r *productResolver) ReviewQuestions(ctx context.Context, obj *model1.Product) ([]*model1.ReviewQuestion, error) {
	// each question comes back with its Content slot filled, so the Label/HelpText field resolvers never need a follow-up call
	ptID, err := r.productTypeIDOf(ctx, obj)
	if err != nil {
		return nil, err
	}
	locale := r.activeLocale(ctx)
	questions, err := r.getAssignedQuestionsLocalized(ctx, ptID, locale)
	if err != nil {
		return nil, err
	}
	out := make([]*model1.ReviewQuestion, len(questions))
	for i, q := range questions {
		out[i] = reviewQuestionToModel(q)
	}
	return out, nil
}

func (r *productResolver) Reviews(ctx context.Context, obj *model1.Product, limit *int32, offset *int32, sort *model.ReviewSort, withMediaOnly *bool) ([]*model1.Review, error) {
	// approved reviews only - the nullable args carry schema defaults and get normalized here
	productID, err := productIDFromObj(obj)
	if err != nil {
		return nil, err
	}
	lim := int32(10)
	if limit != nil {
		lim = *limit
	}
	off := int32(0)
	if offset != nil {
		off = *offset
	}
	var rows []review.Review // the loader batches across siblings like a configurable's children, the service call covers unmounted paths, and both hand back the same newest-first window
	if loaders := dataloaders.For(ctx); loaders != nil {
		rows, err = loaders.ProductReviews.Load(ctx, dataloaders.ReviewsKey{ProductID: productID, Limit: lim, Offset: off})()
	} else {
		rows, _, err = r.ReviewSvc.ListReviewsForProduct(ctx, productID, lim, off)
	}
	if err != nil {
		return nil, err
	}
	// Filtered here so the count stays the pre-filter total the UI wants - and it must NOT compact in place (rows[:0]), because under the ProductReviews DataLoader rows IS the cached backing slice and mutating it corrupts a later same-key Load
	if withMediaOnly != nil && *withMediaOnly {
		filtered := make([]review.Review, 0, len(rows))
		for _, r := range rows {
			if len(r.Media) > 0 {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	questionByID, err := r.buildQuestionLookup(ctx, obj, rows) // one lookup for the whole batch
	if err != nil {
		return nil, err
	}
	reviewIDs := make([]uuid.UUID, len(rows)) // priming tallies + my-votes in one round-trip keeps helpfulCount / notHelpfulCount / myVote from going N+1 across the page
	for i, row := range rows {
		reviewIDs[i] = row.ID
	}
	if err := r.primeReviewVotes(ctx, reviewIDs); err != nil {
		return nil, err
	}
	publicURL := r.MediaLibrarySvc.PublicURL
	out := make([]*model1.Review, len(rows))
	for i, row := range rows {
		out[i] = reviewToModel(row, publicURL, questionByID)
	}
	if sort != nil {
		sortReviews(out, *sort)
	}
	return out, nil
}

func (r *productResolver) ReviewAggregates(ctx context.Context, obj *model1.Product) (*model.ReviewAggregates, error) {
	productID, err := productIDFromObj(obj)
	if err != nil {
		return nil, err
	}
	agg, err := r.cachedAggregates(ctx, productID)
	if err != nil {
		return nil, err
	}
	policy := r.ReviewSvc.ReadPolicy(ctx)
	out := &model.ReviewAggregates{
		Count:        int32(agg.Count),
		Distribution: []int32{int32(agg.Distribution[0]), int32(agg.Distribution[1]), int32(agg.Distribution[2]), int32(agg.Distribution[3]), int32(agg.Distribution[4])},
		VotingPolicy: &model.ReviewVotingPolicy{
			RequireAuthentication: policy.RequireAuthenticationToVote,
		},
	}
	if agg.OverallAverage != nil {
		v := *agg.OverallAverage
		out.OverallAverage = &v
	}
	{ // per-question averages need question models attached for label rendering - the assigned-questions list hydrates all of them in one round-trip
		ptID, err := r.productTypeIDOf(ctx, obj)
		if err == nil {
			questions, qerr := r.getAssignedQuestionsLocalized(ctx, ptID, r.activeLocale(ctx))
			if qerr == nil {
				byID := make(map[uuid.UUID]review.Question, len(questions))
				for _, q := range questions {
					byID[q.ID] = q
				}
				out.PerQuestion = make([]*model.ReviewQuestionAverage, 0, len(agg.PerQuestion))
				for _, p := range agg.PerQuestion {
					q := byID[p.QuestionID]
					out.PerQuestion = append(out.PerQuestion, &model.ReviewQuestionAverage{
						Question: reviewQuestionToModel(q),
						Average:  p.Average,
						Count:    int32(p.Count),
					})
				}
			}
		}
	}
	if out.PerQuestion == nil {
		out.PerQuestion = []*model.ReviewQuestionAverage{}
	}
	return out, nil
}

// Reads the cached AI "what customers say" summary - no AI call at render time, and any error yields null rather than breaking the PDP
func (r *productResolver) ReviewSummary(ctx context.Context, obj *model1.Product) (*string, error) {
	pid, err := uuid.Parse(obj.ID)
	if err != nil {
		return nil, nil
	}
	summary, err := r.ReviewSvc.GetProductSummary(ctx, pid)
	if err != nil || summary == "" {
		return nil, nil
	}
	return &summary, nil
}

// One resolver, four outcomes - the PDP shows the Write-a-review CTA, the Sign-in prompt, the Already-reviewed link, or the Buy-it-first nudge off this
func (r *productResolver) CustomerCanReview(ctx context.Context, obj *model1.Product) (*model.ReviewEligibility, error) {
	productID, err := productIDFromObj(obj)
	if err != nil {
		return nil, err
	}
	customerIDPtr := channel.OptionalCustomerID(ctx)
	var customerID uuid.UUID
	if customerIDPtr != nil {
		customerID = *customerIDPtr
	}
	e, err := r.ReviewSvc.CheckEligibility(ctx, customerID, productID)
	if err != nil {
		return nil, err
	}
	out := &model.ReviewEligibility{CanReview: e.CanReview}
	if e.ExistingReviewID != nil {
		id := e.ExistingReviewID.String()
		out.ExistingReviewID = &id
	}
	switch e.Reason {
	case review.EligibilityNotAuthenticated:
		reason := model.ReviewIneligibilityReasonNotAuthenticated
		out.Reason = &reason
	case review.EligibilityNotPurchased:
		reason := model.ReviewIneligibilityReasonNotPurchased
		out.Reason = &reason
	case review.EligibilityAlreadyReviewed:
		reason := model.ReviewIneligibilityReasonAlreadyReviewed
		out.Reason = &reason
	}
	return out, nil
}

// Privacy line: a public review never reveals the reviewer's email or surname, so a customer renders as "First L." and a guest as their trimmed submission-time guest_name
func (r *reviewResolver) CustomerDisplayName(ctx context.Context, obj *model1.Review) (string, error) {
	if obj == nil {
		return "Customer", nil
	}
	if obj.CustomerID == nil || *obj.CustomerID == "" {
		name := strings.TrimSpace(obj.GuestName) // required at submission time, but hand-inserted rows still need the "Guest" fallback below
		if name == "" {
			return "Guest", nil
		}
		return name, nil
	}
	cid, err := uuid.Parse(*obj.CustomerID)
	if err != nil {
		return "Customer", nil
	}
	c, err := r.loadCustomer(ctx, cid) // every CustomerDisplayName in the request feeds the loader, which fires ONE batched SELECT for all distinct ids on the next tick
	if err != nil {
		return "Customer", nil // a missing customer row shouldn't take down the review list - a generic label keeps the PDP rendering
	}
	first := strings.TrimSpace(c.FirstName)
	last := strings.TrimSpace(c.LastName)
	if first == "" && last == "" {
		return "Customer", nil
	}
	if last != "" {
		return first + " " + string([]rune(last)[0]) + ".", nil
	}
	return first, nil
}

func (r *reviewResolver) OverallRating(ctx context.Context, obj *model1.Review) (*float64, error) {
	// null when the review carries only free-text / yes-no answers, which the schema allows though it's rare
	return computeOverallRating(obj.Answers), nil
}

func (r *reviewResolver) HelpfulCount(ctx context.Context, obj *model1.Review) (int32, error) {
	// goes through voteCountsFor, so hydrating a whole review list costs one round-trip across the batch rather than N
	counts, err := r.voteCountsFor(ctx, obj)
	if err != nil || counts == nil {
		return 0, err
	}
	return int32(counts.Helpful), nil
}

func (r *reviewResolver) NotHelpfulCount(ctx context.Context, obj *model1.Review) (int32, error) {
	counts, err := r.voteCountsFor(ctx, obj)
	if err != nil || counts == nil {
		return 0, err
	}
	return int32(counts.NotHelpful), nil
}

func (r *reviewResolver) MyVote(ctx context.Context, obj *model1.Review) (int32, error) {
	// 0 means the caller hasn't voted - reads the renderCache batch primed by Product.reviews, and single-review paths fall back to a direct lookup inside cachedMyVote
	if obj == nil || obj.ID == "" {
		return 0, nil
	}
	rid, err := uuid.Parse(obj.ID)
	if err != nil {
		return 0, nil
	}
	v, err := r.cachedMyVote(ctx, rid)
	if err != nil {
		return 0, err
	}
	return int32(v), nil
}

func (r *reviewQuestionResolver) Label(ctx context.Context, obj *model1.ReviewQuestion) (string, error) {
	// reads the hydrated Content slot, falling back to the slug so a prompt is never blank on the storefront
	if obj != nil && obj.Content != nil && obj.Content.Label != "" {
		return obj.Content.Label, nil
	}
	if obj != nil {
		return obj.Slug, nil
	}
	return "", nil
}

func (r *reviewQuestionResolver) HelpText(ctx context.Context, obj *model1.ReviewQuestion) (string, error) {
	if obj != nil && obj.Content != nil {
		return obj.Content.HelpText, nil
	}
	return "", nil
}

func (r *Resolver) Review() ReviewResolver { return &reviewResolver{r} }

func (r *Resolver) ReviewQuestion() ReviewQuestionResolver { return &reviewQuestionResolver{r} }

type (
	reviewResolver         struct{ *Resolver }
	reviewQuestionResolver struct{ *Resolver }
)

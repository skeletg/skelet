package review

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	pgdb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
}

func NewRepository(queries *db.Queries, txMgr *txrunner.Manager) *postgresRepository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) *postgresRepository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx}
}

// --- Questions ---

func (r *postgresRepository) CreateCategory(ctx context.Context, in CreateCategoryInput) (Category, error) {
	row, err := r.queries.CreateReviewCategory(ctx, db.CreateReviewCategoryParams{
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		IsActive:    in.IsActive,
		SortOrder:   in.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Category{}, ErrSlugTaken
		}
		return Category{}, fmt.Errorf("create review category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) GetCategory(ctx context.Context, id uuid.UUID) (Category, error) {
	row, err := r.queries.GetReviewCategoryByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("get review category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.queries.ListReviewCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list review categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) ListActiveCategories(ctx context.Context) ([]Category, error) {
	rows, err := r.queries.ListActiveReviewCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active review categories: %w", err)
	}
	out := make([]Category, len(rows))
	for i, row := range rows {
		out[i] = toCategory(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateCategory(ctx context.Context, in UpdateCategoryInput) (Category, error) {
	row, err := r.queries.UpdateReviewCategory(ctx, db.UpdateReviewCategoryParams{
		ID:          in.ID,
		Name:        in.Name,
		Description: in.Description,
		IsActive:    in.IsActive,
		SortOrder:   in.SortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Category{}, ErrCategoryNotFound
		}
		return Category{}, fmt.Errorf("update review category: %w", err)
	}
	return toCategory(row), nil
}

func (r *postgresRepository) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteReviewCategory(ctx, id); err != nil {
		return fmt.Errorf("delete review category: %w", err)
	}
	return nil
}

// --- Moderation tags (abstract moderation-concern taxonomy) ---

func (r *postgresRepository) CreateModerationTag(ctx context.Context, in CreateModerationTagInput) (ModerationTag, error) {
	row, err := r.queries.CreateReviewModerationTag(ctx, db.CreateReviewModerationTagParams{
		Slug:        in.Slug,
		Name:        in.Name,
		Description: in.Description,
		IsActive:    in.IsActive,
		SortOrder:   in.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ModerationTag{}, ErrSlugTaken
		}
		return ModerationTag{}, fmt.Errorf("create review moderation tag: %w", err)
	}
	return toModerationTag(row), nil
}

func (r *postgresRepository) GetModerationTag(ctx context.Context, id uuid.UUID) (ModerationTag, error) {
	row, err := r.queries.GetReviewModerationTagByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ModerationTag{}, ErrTagNotFound
		}
		return ModerationTag{}, fmt.Errorf("get review moderation tag: %w", err)
	}
	return toModerationTag(row), nil
}

func (r *postgresRepository) ListModerationTags(ctx context.Context) ([]ModerationTag, error) {
	rows, err := r.queries.ListReviewModerationTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list review moderation tags: %w", err)
	}
	out := make([]ModerationTag, len(rows))
	for i, row := range rows {
		out[i] = toModerationTag(row)
	}
	return out, nil
}

func (r *postgresRepository) ListActiveModerationTags(ctx context.Context) ([]ModerationTag, error) {
	rows, err := r.queries.ListActiveReviewModerationTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active review moderation tags: %w", err)
	}
	out := make([]ModerationTag, len(rows))
	for i, row := range rows {
		out[i] = toModerationTag(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateModerationTag(ctx context.Context, in UpdateModerationTagInput) (ModerationTag, error) {
	row, err := r.queries.UpdateReviewModerationTag(ctx, db.UpdateReviewModerationTagParams{
		ID:          in.ID,
		Name:        in.Name,
		Description: in.Description,
		IsActive:    in.IsActive,
		SortOrder:   in.SortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ModerationTag{}, ErrTagNotFound
		}
		return ModerationTag{}, fmt.Errorf("update review moderation tag: %w", err)
	}
	return toModerationTag(row), nil
}

func (r *postgresRepository) DeleteModerationTag(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteReviewModerationTag(ctx, id); err != nil {
		return fmt.Errorf("delete review moderation tag: %w", err)
	}
	return nil
}

func (r *postgresRepository) CreateQuestion(ctx context.Context, in CreateQuestionInput) (Question, error) {
	row, err := r.queries.CreateReviewQuestion(ctx, db.CreateReviewQuestionParams{
		Slug:         in.Slug,
		InputType:    string(in.InputType),
		IsRequired:   in.IsRequired,
		AllowComment: in.AllowComment,
		SortOrder:    in.SortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Question{}, ErrSlugTaken
		}
		return Question{}, fmt.Errorf("create review question: %w", err)
	}
	return toQuestion(row), nil
}

func (r *postgresRepository) GetQuestion(ctx context.Context, id uuid.UUID) (Question, error) {
	row, err := r.queries.GetReviewQuestionByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Question{}, ErrQuestionNotFound
		}
		return Question{}, fmt.Errorf("get review question: %w", err)
	}
	q := toQuestion(row)
	all, err := r.queries.ListReviewQuestionContent(ctx, id)
	if err != nil {
		return Question{}, fmt.Errorf("list question content: %w", err)
	}
	q.AllContent = make([]QuestionContent, len(all))
	for i, c := range all {
		q.AllContent[i] = toQuestionContent(c)
	}
	return q, nil
}

// GetQuestionsByIDs returns question rows by id - AllContent isn't hydrated - the
// loader path only needs identity fields; translations stay on single-id GetQuestion
func (r *postgresRepository) GetQuestionsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Question, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Question{}, nil
	}
	rows, err := r.queries.GetReviewQuestionsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get review questions by ids: %w", err)
	}
	out := make(map[uuid.UUID]Question, len(rows))
	for _, row := range rows {
		out[row.ID] = toQuestion(row)
	}
	return out, nil
}

func (r *postgresRepository) GetQuestionBySlug(ctx context.Context, slug string) (Question, error) {
	row, err := r.queries.GetReviewQuestionBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Question{}, ErrQuestionNotFound
		}
		return Question{}, fmt.Errorf("get review question by slug: %w", err)
	}
	return toQuestion(row), nil
}

func (r *postgresRepository) ListQuestions(ctx context.Context) ([]Question, error) {
	rows, err := r.queries.ListReviewQuestions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list review questions: %w", err)
	}
	out := make([]Question, len(rows))
	for i, row := range rows {
		out[i] = toQuestion(row)
	}
	return out, nil
}

func (r *postgresRepository) UpdateQuestion(ctx context.Context, in UpdateQuestionInput) (Question, error) {
	row, err := r.queries.UpdateReviewQuestion(ctx, db.UpdateReviewQuestionParams{
		ID:           in.ID,
		IsRequired:   in.IsRequired,
		AllowComment: in.AllowComment,
		SortOrder:    in.SortOrder,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Question{}, ErrQuestionNotFound
		}
		return Question{}, fmt.Errorf("update review question: %w", err)
	}
	return toQuestion(row), nil
}

func (r *postgresRepository) DeleteQuestion(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteReviewQuestion(ctx, id); err != nil {
		return fmt.Errorf("delete review question: %w", err)
	}
	return nil
}

func (r *postgresRepository) UpsertQuestionContent(ctx context.Context, in QuestionContentInput) (QuestionContent, error) {
	row, err := r.queries.UpsertReviewQuestionContent(ctx, db.UpsertReviewQuestionContentParams{
		QuestionID: in.QuestionID,
		Locale:     in.Locale,
		Label:      in.Label,
		HelpText:   in.HelpText,
	})
	if err != nil {
		return QuestionContent{}, fmt.Errorf("upsert question content: %w", err)
	}
	return toQuestionContent(row), nil
}

func (r *postgresRepository) ListQuestionContent(ctx context.Context, questionID uuid.UUID) ([]QuestionContent, error) {
	rows, err := r.queries.ListReviewQuestionContent(ctx, questionID)
	if err != nil {
		return nil, fmt.Errorf("list question content: %w", err)
	}
	out := make([]QuestionContent, len(rows))
	for i, c := range rows {
		out[i] = toQuestionContent(c)
	}
	return out, nil
}

func (r *postgresRepository) ListQuestionContentByIDs(ctx context.Context, ids []uuid.UUID, locale string) (map[uuid.UUID]QuestionContent, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]QuestionContent{}, nil
	}
	rows, err := r.queries.ListReviewQuestionContentByIDs(ctx, db.ListReviewQuestionContentByIDsParams{
		QuestionIds: ids,
		Locale:      locale,
	})
	if err != nil {
		return nil, fmt.Errorf("list question content by ids: %w", err)
	}
	out := make(map[uuid.UUID]QuestionContent, len(rows))
	for _, c := range rows {
		out[c.QuestionID] = toQuestionContent(c)
	}
	return out, nil
}

// --- Assignments ---

func (r *postgresRepository) AssignQuestion(ctx context.Context, productTypeID, questionID uuid.UUID, sortOrder int32) error {
	if err := r.queries.AssignReviewQuestion(ctx, db.AssignReviewQuestionParams{
		ProductTypeID: productTypeID,
		QuestionID:    questionID,
		SortOrder:     sortOrder,
	}); err != nil {
		return fmt.Errorf("assign review question: %w", err)
	}
	return nil
}

func (r *postgresRepository) UnassignQuestion(ctx context.Context, productTypeID, questionID uuid.UUID) (bool, error) {
	n, err := r.queries.UnassignReviewQuestion(ctx, db.UnassignReviewQuestionParams{
		ProductTypeID: productTypeID,
		QuestionID:    questionID,
	})
	if err != nil {
		return false, fmt.Errorf("unassign review question: %w", err)
	}
	return n > 0, nil
}

func (r *postgresRepository) ListAssignedQuestions(ctx context.Context, productTypeID uuid.UUID) ([]Question, error) {
	rows, err := r.queries.ListAssignedQuestions(ctx, productTypeID)
	if err != nil {
		return nil, fmt.Errorf("list assigned questions: %w", err)
	}
	out := make([]Question, len(rows))
	for i, row := range rows {
		// Discard the row's synthetic AssignmentSortOrder - the row order already reflects it
		out[i] = Question{
			ID:           row.ID,
			Slug:         row.Slug,
			InputType:    InputType(row.InputType),
			IsRequired:   row.IsRequired,
			AllowComment: row.AllowComment,
			SortOrder:    row.AssignmentSortOrder, // assignment order, not question's intrinsic order
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		}
	}
	return out, nil
}

// --- Reviews ---

func (r *postgresRepository) CreateReview(ctx context.Context, in Review) (Review, error) {
	customerID := uuid.NullUUID{}
	if in.CustomerID != nil {
		customerID = uuid.NullUUID{UUID: *in.CustomerID, Valid: true}
	}
	row, err := r.queries.CreateReview(ctx, db.CreateReviewParams{
		ProductID:          in.ProductID,
		CustomerID:         customerID,
		ChannelID:          in.ChannelID,
		Locale:             in.Locale,
		Title:              in.Title,
		Body:               in.Body,
		IsVerifiedPurchase: in.IsVerifiedPurchase,
		Status:             string(in.Status),
		GuestName:          in.GuestName,
		GuestEmail:         in.GuestEmail,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Review{}, ErrAlreadyReviewed
		}
		return Review{}, fmt.Errorf("create review: %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) UpdateReviewHeader(ctx context.Context, id uuid.UUID, title, body string) (Review, error) {
	row, err := r.queries.UpdateReview(ctx, db.UpdateReviewParams{
		ID:    id,
		Title: title,
		Body:  body,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("update review: %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) GetReview(ctx context.Context, id uuid.UUID) (Review, error) {
	row, err := r.queries.GetReviewByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("get review: %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) GetReviewByCustomerAndProduct(ctx context.Context, customerID, productID uuid.UUID) (Review, error) {
	row, err := r.queries.GetReviewByCustomerAndProduct(ctx, db.GetReviewByCustomerAndProductParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
		ProductID:  productID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("get review by (customer, product): %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) ListApprovedForProduct(ctx context.Context, productID uuid.UUID, limit, offset int32) ([]Review, int64, error) {
	rows, err := r.queries.ListApprovedReviewsForProduct(ctx, db.ListApprovedReviewsForProductParams{
		ProductID: productID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list approved reviews: %w", err)
	}
	count, err := r.queries.CountApprovedReviewsForProduct(ctx, productID)
	if err != nil {
		return nil, 0, fmt.Errorf("count approved reviews: %w", err)
	}
	out := make([]Review, len(rows))
	for i, row := range rows {
		out[i] = toReview(row)
	}
	return out, count, nil
}

// ListApprovedForProductIDs returns the top-N approved reviews for each of productIDs in a single windowed query, grouped by product id - The generated row carries the same columns as db.Review (the window's rn is filtered out in the SELECT), so it maps through the same toReview
func (r *postgresRepository) ListApprovedForProductIDs(ctx context.Context, productIDs []uuid.UUID, limit, offset int32) (map[uuid.UUID][]Review, error) {
	out := make(map[uuid.UUID][]Review, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := r.queries.ListApprovedReviewsForProductIDs(ctx, db.ListApprovedReviewsForProductIDsParams{
		ProductIds: productIDs,
		RowOffset:  offset,
		RowLimit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list approved reviews by product ids: %w", err)
	}
	for _, row := range rows {
		out[row.ProductID] = append(out[row.ProductID], toReview(db.Review{
			ID:                  row.ID,
			ProductID:           row.ProductID,
			CustomerID:          row.CustomerID,
			GuestName:           row.GuestName,
			GuestEmail:          row.GuestEmail,
			ChannelID:           row.ChannelID,
			Locale:              row.Locale,
			Title:               row.Title,
			Body:                row.Body,
			IsVerifiedPurchase:  row.IsVerifiedPurchase,
			Status:              row.Status,
			SubmittedAt:         row.SubmittedAt,
			ModeratedAt:         row.ModeratedAt,
			ModeratedBy:         row.ModeratedBy,
			ModeratorNote:       row.ModeratorNote,
			MerchantReply:       row.MerchantReply,
			MerchantReplyAt:     row.MerchantReplyAt,
			MerchantReplyBy:     row.MerchantReplyBy,
			ModerationLabel:     row.ModerationLabel,
			ModerationReason:    row.ModerationReason,
			ModerationCheckedAt: row.ModerationCheckedAt,
		}))
	}
	return out, nil
}

func (r *postgresRepository) ListForModeration(ctx context.Context, filter ListReviewsFilter, limit, offset int32) ([]Review, int64, error) {
	// sqlc.narg("status_filter")::text[] receives a Go []string - passing nil makes the IS NULL check true and the filter
	// no-ops, exactly the "no filter selected" semantics we want
	var statusSlice []string
	if len(filter.Status) > 0 {
		statusSlice = make([]string, len(filter.Status))
		for i, s := range filter.Status {
			statusSlice[i] = string(s)
		}
	}
	productID := uuid.NullUUID{}
	if filter.ProductID != nil {
		productID = uuid.NullUUID{UUID: *filter.ProductID, Valid: true}
	}
	rows, err := r.queries.ListReviewsForModeration(ctx, db.ListReviewsForModerationParams{
		StatusFilter:  statusSlice,
		ProductID:     productID,
		ChannelIds:    filter.ChannelIDs,
		CategorySlugs: filter.CategorySlugs,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list reviews for moderation: %w", err)
	}
	count, err := r.queries.CountReviewsForModeration(ctx, db.CountReviewsForModerationParams{
		StatusFilter:  statusSlice,
		ProductID:     productID,
		ChannelIds:    filter.ChannelIDs,
		CategorySlugs: filter.CategorySlugs,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count reviews for moderation: %w", err)
	}
	out := make([]Review, len(rows))
	for i, row := range rows {
		out[i] = toReview(row)
	}
	return out, count, nil
}

// CountByStatus returns per-status review tallies for the hub tiles, scoped to the same channel set as ListForModeration (nil channelIDs →
// all channels). productID is optional - Statuses with zero reviews are simply absent from the map
func (r *postgresRepository) CountByStatus(ctx context.Context, channelIDs []uuid.UUID, productID *uuid.UUID) (map[Status]int64, error) {
	pid := uuid.NullUUID{}
	if productID != nil {
		pid = uuid.NullUUID{UUID: *productID, Valid: true}
	}
	rows, err := r.queries.CountReviewsByStatus(ctx, db.CountReviewsByStatusParams{
		ProductID:  pid,
		ChannelIds: channelIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("count reviews by status: %w", err)
	}
	out := make(map[Status]int64, len(rows))
	for _, row := range rows {
		out[Status(row.Status)] = row.Count
	}
	return out, nil
}

func (r *postgresRepository) SetReviewStatus(ctx context.Context, id uuid.UUID, status Status, moderatorID uuid.UUID, note string) (Review, error) {
	row, err := r.queries.SetReviewStatus(ctx, db.SetReviewStatusParams{
		ID:            id,
		Status:        string(status),
		ModeratedBy:   uuid.NullUUID{UUID: moderatorID, Valid: true},
		ModeratorNote: note,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("set review status: %w", err)
	}
	return toReview(row), nil
}

// --- AI: moderation verdict, merchant reply, product summary ---

func (r *postgresRepository) SetReviewModeration(ctx context.Context, id uuid.UUID, label, reason string, categorySlugs, tags []string) (Review, error) {
	row, err := r.queries.SetReviewModeration(ctx, db.SetReviewModerationParams{
		ID: id, ModerationLabel: label, ModerationReason: reason,
		CategorySlugs: tagsJSON(categorySlugs), ModerationTags: tagsJSON(tags),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("set review moderation: %w", err)
	}
	return toReview(row), nil
}

// SetReviewModerationTags is a moderator's manual override of the tag set (a
// '[]' clears all) - Independent of the AI label/reason
func (r *postgresRepository) SetReviewModerationTags(ctx context.Context, id uuid.UUID, tags []string) (Review, error) {
	row, err := r.queries.SetReviewModerationTags(ctx, db.SetReviewModerationTagsParams{
		ID: id, ModerationTags: tagsJSON(tags),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("set review moderation tags: %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) SetReviewCategories(ctx context.Context, id uuid.UUID, slugs []string) (Review, error) {
	row, err := r.queries.SetReviewCategories(ctx, db.SetReviewCategoriesParams{ID: id, CategorySlugs: tagsJSON(slugs)})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("set review categories: %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) SetMerchantReply(ctx context.Context, id uuid.UUID, body string, adminID uuid.UUID) (Review, error) {
	var (
		row db.Review
		err error
	)
	if strings.TrimSpace(body) == "" {
		row, err = r.queries.ClearReviewMerchantReply(ctx, id)
	} else {
		row, err = r.queries.SetReviewMerchantReply(ctx, db.SetReviewMerchantReplyParams{
			ID: id, Reply: body, AdminID: uuid.NullUUID{UUID: adminID, Valid: true},
		})
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Review{}, ErrNotFound
		}
		return Review{}, fmt.Errorf("set merchant reply: %w", err)
	}
	return toReview(row), nil
}

func (r *postgresRepository) GetProductSummary(ctx context.Context, productID uuid.UUID) (ProductReviewSummary, bool, error) {
	row, err := r.queries.GetProductReviewSummary(ctx, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProductReviewSummary{}, false, nil
		}
		return ProductReviewSummary{}, false, fmt.Errorf("get product summary: %w", err)
	}
	return ProductReviewSummary{
		ProductID:   row.ProductID,
		Summary:     row.Summary,
		ReviewCount: int(row.ReviewCount),
		IsStale:     row.IsStale,
		GeneratedAt: nullTimePtr(row.GeneratedAt),
		UpdatedAt:   row.UpdatedAt,
	}, true, nil
}

func (r *postgresRepository) UpsertProductSummary(ctx context.Context, productID uuid.UUID, summary string, reviewCount int) error {
	if err := r.queries.UpsertProductReviewSummary(ctx, db.UpsertProductReviewSummaryParams{
		ProductID: productID, Summary: summary, ReviewCount: int32(reviewCount),
	}); err != nil {
		return fmt.Errorf("upsert product summary: %w", err)
	}
	return nil
}

func (r *postgresRepository) MarkSummaryStale(ctx context.Context, productID uuid.UUID) error {
	if err := r.queries.MarkProductReviewSummaryStale(ctx, productID); err != nil {
		return fmt.Errorf("mark summary stale: %w", err)
	}
	return nil
}

func (r *postgresRepository) MarkSummaryFresh(ctx context.Context, productID uuid.UUID) error {
	if err := r.queries.MarkProductReviewSummaryFresh(ctx, productID); err != nil {
		return fmt.Errorf("mark summary fresh: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListStaleSummaryIDs(ctx context.Context, limit int32) ([]uuid.UUID, error) {
	ids, err := r.queries.ListStaleProductReviewSummaryIDs(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale summaries: %w", err)
	}
	return ids, nil
}

// --- Answers ---

func (r *postgresRepository) UpsertAnswer(ctx context.Context, in Answer) error {
	if err := r.queries.UpsertReviewAnswer(ctx, db.UpsertReviewAnswerParams{
		ReviewID:    in.ReviewID,
		QuestionID:  in.QuestionID,
		RatingValue: nullInt16Ptr(in.RatingValue),
		BoolValue:   nullBoolPtr(in.BoolValue),
		TextValue:   in.TextValue,
	}); err != nil {
		return fmt.Errorf("upsert answer: %w", err)
	}
	return nil
}

func (r *postgresRepository) DeleteAnswers(ctx context.Context, reviewID uuid.UUID) error {
	if err := r.queries.DeleteReviewAnswersForReview(ctx, reviewID); err != nil {
		return fmt.Errorf("delete answers: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListAnswers(ctx context.Context, reviewID uuid.UUID) ([]Answer, error) {
	rows, err := r.queries.ListReviewAnswersForReview(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("list answers: %w", err)
	}
	out := make([]Answer, len(rows))
	for i, row := range rows {
		out[i] = toAnswer(row)
	}
	return out, nil
}

func (r *postgresRepository) ListAnswersByReviewIDs(ctx context.Context, reviewIDs []uuid.UUID) (map[uuid.UUID][]Answer, error) {
	if len(reviewIDs) == 0 {
		return map[uuid.UUID][]Answer{}, nil
	}
	rows, err := r.queries.ListReviewAnswersByReviewIDs(ctx, reviewIDs)
	if err != nil {
		return nil, fmt.Errorf("list answers by ids: %w", err)
	}
	out := make(map[uuid.UUID][]Answer, len(reviewIDs))
	for _, row := range rows {
		out[row.ReviewID] = append(out[row.ReviewID], toAnswer(row))
	}
	return out, nil
}

// --- Media ---

func (r *postgresRepository) InsertMedia(ctx context.Context, reviewID, assetID uuid.UUID, sortOrder int32) (Media, error) {
	row, err := r.queries.InsertReviewMedia(ctx, db.InsertReviewMediaParams{
		ReviewID:  reviewID,
		AssetID:   assetID,
		SortOrder: sortOrder,
	})
	if err != nil {
		return Media{}, fmt.Errorf("insert review media: %w", err)
	}
	return Media{
		ID:        row.ID,
		ReviewID:  row.ReviewID,
		AssetID:   row.AssetID,
		SortOrder: row.SortOrder,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (r *postgresRepository) DeleteMedia(ctx context.Context, reviewID uuid.UUID) error {
	if err := r.queries.DeleteReviewMediaForReview(ctx, reviewID); err != nil {
		return fmt.Errorf("delete review media: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListMedia(ctx context.Context, reviewID uuid.UUID) ([]Media, error) {
	rows, err := r.queries.ListReviewMediaForReview(ctx, reviewID)
	if err != nil {
		return nil, fmt.Errorf("list review media: %w", err)
	}
	out := make([]Media, len(rows))
	for i, row := range rows {
		out[i] = Media{
			ID:        row.ID,
			ReviewID:  row.ReviewID,
			AssetID:   row.AssetID,
			SortOrder: row.SortOrder,
			CreatedAt: row.CreatedAt,
			ObjectKey: row.ObjectKey,
			MimeType:  row.MimeType,
			Width:     nullInt32Ptr(row.Width),
			Height:    nullInt32Ptr(row.Height),
			SizeBytes: row.SizeBytes,
		}
	}
	return out, nil
}

func (r *postgresRepository) ListMediaByReviewIDs(ctx context.Context, reviewIDs []uuid.UUID) (map[uuid.UUID][]Media, error) {
	if len(reviewIDs) == 0 {
		return map[uuid.UUID][]Media{}, nil
	}
	rows, err := r.queries.ListReviewMediaByReviewIDs(ctx, reviewIDs)
	if err != nil {
		return nil, fmt.Errorf("list review media by ids: %w", err)
	}
	out := make(map[uuid.UUID][]Media, len(reviewIDs))
	for _, row := range rows {
		out[row.ReviewID] = append(out[row.ReviewID], Media{
			ID:        row.ID,
			ReviewID:  row.ReviewID,
			AssetID:   row.AssetID,
			SortOrder: row.SortOrder,
			CreatedAt: row.CreatedAt,
			ObjectKey: row.ObjectKey,
			MimeType:  row.MimeType,
			Width:     nullInt32Ptr(row.Width),
			Height:    nullInt32Ptr(row.Height),
			SizeBytes: row.SizeBytes,
		})
	}
	return out, nil
}

// --- Aggregates + purchase check ---

func (r *postgresRepository) GetAggregates(ctx context.Context, productID uuid.UUID) (Aggregates, error) {
	overall, err := r.queries.AggregateOverallRating(ctx, productID)
	if err != nil {
		return Aggregates{}, fmt.Errorf("aggregate overall: %w", err)
	}
	perQ, err := r.queries.AggregateReviewRatings(ctx, productID)
	if err != nil {
		return Aggregates{}, fmt.Errorf("aggregate per-question: %w", err)
	}
	agg := Aggregates{
		Count: int(overall.ReviewCount),
	}
	if overall.ReviewCount > 0 {
		v := overall.OverallAverage
		agg.OverallAverage = &v
	}
	agg.Distribution = [5]int{
		int(overall.Bucket1),
		int(overall.Bucket2),
		int(overall.Bucket3),
		int(overall.Bucket4),
		int(overall.Bucket5),
	}
	agg.PerQuestion = make([]QuestionAverage, len(perQ))
	for i, row := range perQ {
		agg.PerQuestion[i] = QuestionAverage{
			QuestionID: row.QuestionID,
			Average:    row.Average,
			Count:      int(row.SampleCount),
		}
	}
	return agg, nil
}

func (r *postgresRepository) GetAggregatesByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID]Aggregates, error) {
	if len(productIDs) == 0 {
		return map[uuid.UUID]Aggregates{}, nil
	}
	overall, err := r.queries.AggregateOverallRatingsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("aggregate overall by ids: %w", err)
	}
	perQ, err := r.queries.AggregateReviewRatingsByProductIDs(ctx, productIDs)
	if err != nil {
		return nil, fmt.Errorf("aggregate per-question by ids: %w", err)
	}
	out := make(map[uuid.UUID]Aggregates, len(overall))
	for _, row := range overall {
		agg := Aggregates{
			Count: int(row.ReviewCount),
		}
		if row.ReviewCount > 0 {
			v := row.OverallAverage
			agg.OverallAverage = &v
		}
		agg.Distribution = [5]int{
			int(row.Bucket1),
			int(row.Bucket2),
			int(row.Bucket3),
			int(row.Bucket4),
			int(row.Bucket5),
		}
		out[row.ProductID] = agg
	}
	for _, row := range perQ {
		agg := out[row.ProductID]
		agg.PerQuestion = append(agg.PerQuestion, QuestionAverage{
			QuestionID: row.QuestionID,
			Average:    row.Average,
			Count:      int(row.SampleCount),
		})
		out[row.ProductID] = agg
	}
	return out, nil
}

// --- Votes ---

func (r *postgresRepository) UpsertVote(ctx context.Context, reviewID uuid.UUID, voter Voter, value int16) error {
	if voter.IsCustomer() {
		if err := r.queries.UpsertReviewVoteByCustomer(ctx, db.UpsertReviewVoteByCustomerParams{
			ReviewID:   reviewID,
			CustomerID: uuid.NullUUID{UUID: *voter.CustomerID, Valid: true},
			Value:      value,
		}); err != nil {
			return fmt.Errorf("upsert review vote (customer): %w", err)
		}
		return nil
	}
	if voter.IsAnon() {
		if err := r.queries.UpsertReviewVoteByAnon(ctx, db.UpsertReviewVoteByAnonParams{
			ReviewID:   reviewID,
			VoterToken: uuid.NullUUID{UUID: *voter.AnonToken, Valid: true},
			Value:      value,
		}); err != nil {
			return fmt.Errorf("upsert review vote (anon): %w", err)
		}
		return nil
	}
	return ErrInvalidVoter
}

func (r *postgresRepository) DeleteVote(ctx context.Context, reviewID uuid.UUID, voter Voter) error {
	if voter.IsCustomer() {
		if _, err := r.queries.DeleteReviewVoteByCustomer(ctx, db.DeleteReviewVoteByCustomerParams{
			ReviewID:   reviewID,
			CustomerID: uuid.NullUUID{UUID: *voter.CustomerID, Valid: true},
		}); err != nil {
			return fmt.Errorf("delete review vote (customer): %w", err)
		}
		return nil
	}
	if voter.IsAnon() {
		if _, err := r.queries.DeleteReviewVoteByAnon(ctx, db.DeleteReviewVoteByAnonParams{
			ReviewID:   reviewID,
			VoterToken: uuid.NullUUID{UUID: *voter.AnonToken, Valid: true},
		}); err != nil {
			return fmt.Errorf("delete review vote (anon): %w", err)
		}
		return nil
	}
	return ErrInvalidVoter
}

func (r *postgresRepository) GetVoteCountsByReviewIDs(ctx context.Context, reviewIDs []uuid.UUID) (map[uuid.UUID]VoteCounts, error) {
	if len(reviewIDs) == 0 {
		return map[uuid.UUID]VoteCounts{}, nil
	}
	rows, err := r.queries.AggregateReviewVotesByReviewIDs(ctx, reviewIDs)
	if err != nil {
		return nil, fmt.Errorf("aggregate review votes: %w", err)
	}
	out := make(map[uuid.UUID]VoteCounts, len(rows))
	for _, row := range rows {
		out[row.ReviewID] = VoteCounts{
			Helpful:    int(row.HelpfulCount),
			NotHelpful: int(row.NotHelpfulCount),
		}
	}
	return out, nil
}

func (r *postgresRepository) GetMyVotesByReviewIDs(ctx context.Context, voter Voter, reviewIDs []uuid.UUID) (map[uuid.UUID]int16, error) {
	if len(reviewIDs) == 0 || !voter.IsValid() {
		return map[uuid.UUID]int16{}, nil
	}
	out := make(map[uuid.UUID]int16, len(reviewIDs))
	if voter.IsCustomer() {
		rows, err := r.queries.GetMyReviewVotesByCustomer(ctx, db.GetMyReviewVotesByCustomerParams{
			CustomerID: uuid.NullUUID{UUID: *voter.CustomerID, Valid: true},
			ReviewIds:  reviewIDs,
		})
		if err != nil {
			return nil, fmt.Errorf("get my review votes (customer): %w", err)
		}
		for _, row := range rows {
			out[row.ReviewID] = row.Value
		}
		return out, nil
	}
	rows, err := r.queries.GetMyReviewVotesByAnon(ctx, db.GetMyReviewVotesByAnonParams{
		VoterToken: uuid.NullUUID{UUID: *voter.AnonToken, Valid: true},
		ReviewIds:  reviewIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("get my review votes (anon): %w", err)
	}
	for _, row := range rows {
		out[row.ReviewID] = row.Value
	}
	return out, nil
}

func (r *postgresRepository) HasCustomerPurchased(ctx context.Context, customerID, productID uuid.UUID) (bool, error) {
	ok, err := r.queries.HasCustomerPurchasedProduct(ctx, db.HasCustomerPurchasedProductParams{
		CustomerID: uuid.NullUUID{UUID: customerID, Valid: true},
		ID:         productID,
	})
	if err != nil {
		return false, fmt.Errorf("has customer purchased: %w", err)
	}
	return ok, nil
}

// --- Mappers + helpers ---

func toCategory(row db.ReviewCategory) Category {
	return Category{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		IsActive:    row.IsActive,
		SortOrder:   row.SortOrder,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toQuestion(row db.ReviewQuestion) Question {
	return Question{
		ID:           row.ID,
		Slug:         row.Slug,
		InputType:    InputType(row.InputType),
		IsRequired:   row.IsRequired,
		AllowComment: row.AllowComment,
		SortOrder:    row.SortOrder,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func toQuestionContent(row db.ReviewQuestionContent) QuestionContent {
	return QuestionContent{
		QuestionID: row.QuestionID,
		Locale:     row.Locale,
		Label:      row.Label,
		HelpText:   row.HelpText,
		UpdatedAt:  row.UpdatedAt,
	}
}

func toReview(row db.Review) Review {
	var moderatedAt *time.Time
	if row.ModeratedAt.Valid {
		t := row.ModeratedAt.Time
		moderatedAt = &t
	}
	var moderatedBy *uuid.UUID
	if row.ModeratedBy.Valid {
		u := row.ModeratedBy.UUID
		moderatedBy = &u
	}
	var customerID *uuid.UUID
	if row.CustomerID.Valid {
		u := row.CustomerID.UUID
		customerID = &u
	}
	return Review{
		ID:                  row.ID,
		ProductID:           row.ProductID,
		CustomerID:          customerID,
		GuestName:           row.GuestName,
		GuestEmail:          row.GuestEmail,
		ChannelID:           row.ChannelID,
		Locale:              row.Locale,
		Title:               row.Title,
		Body:                row.Body,
		IsVerifiedPurchase:  row.IsVerifiedPurchase,
		Status:              Status(row.Status),
		SubmittedAt:         row.SubmittedAt,
		ModeratedAt:         moderatedAt,
		ModeratedBy:         moderatedBy,
		ModeratorNote:       row.ModeratorNote,
		ModerationLabel:     row.ModerationLabel,
		ModerationReason:    row.ModerationReason,
		ModerationCheckedAt: nullTimePtr(row.ModerationCheckedAt),
		CategorySlugs:       parseTagsJSON(row.CategorySlugs),
		ModerationTags:      parseTagsJSON(row.ModerationTags),
		MerchantReply:       row.MerchantReply,
		MerchantReplyAt:     nullTimePtr(row.MerchantReplyAt),
		MerchantReplyBy:     nullUUIDPtr(row.MerchantReplyBy),
	}
}

// tagsJSON marshals a tag-slug slice into the jsonb column value, normalizing
// nil to an empty array ('[]', not 'null') so the column invariant holds
func tagsJSON(tags []string) json.RawMessage {
	if tags == nil {
		tags = []string{}
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

// parseTagsJSON decodes the jsonb moderation_tags array into a slice - a missing
// or malformed value yields nil (treated as "no tags" everywhere)
func parseTagsJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil
	}
	return tags
}

func toModerationTag(row db.ReviewModerationTag) ModerationTag {
	return ModerationTag{
		ID:          row.ID,
		Slug:        row.Slug,
		Name:        row.Name,
		Description: row.Description,
		IsActive:    row.IsActive,
		SortOrder:   row.SortOrder,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func toAnswer(row db.ReviewAnswer) Answer {
	var ratingValue *int16
	if row.RatingValue.Valid {
		v := row.RatingValue.Int16
		ratingValue = &v
	}
	var boolValue *bool
	if row.BoolValue.Valid {
		v := row.BoolValue.Bool
		boolValue = &v
	}
	return Answer{
		ReviewID:    row.ReviewID,
		QuestionID:  row.QuestionID,
		RatingValue: ratingValue,
		BoolValue:   boolValue,
		TextValue:   row.TextValue,
	}
}

func nullInt16Ptr(v *int16) sql.NullInt16 {
	if v == nil {
		return sql.NullInt16{}
	}
	return sql.NullInt16{Int16: *v, Valid: true}
}

func nullBoolPtr(v *bool) sql.NullBool {
	if v == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *v, Valid: true}
}

func nullTimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}

func nullUUIDPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	u := n.UUID
	return &u
}

func nullInt32Ptr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}

func isUniqueViolation(err error) bool { return pgdb.IsUniqueViolation(err) } // shared SQLSTATE-based check - the per-package copies drifted into fragile message matching

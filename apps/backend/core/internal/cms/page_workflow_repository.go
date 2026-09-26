package cms

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

func (r *postgresRepository) SubmitPageForReview(ctx context.Context, id uuid.UUID, by *uuid.UUID) (Page, error) {
	row, err := r.queries.SubmitCMSPageForReview(ctx, db.SubmitCMSPageForReviewParams{ID: id, SubmittedBy: sqlnull.UUID(by)})
	if err != nil {
		return Page{}, fmt.Errorf("submit for review: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) ApprovePageWorkflow(ctx context.Context, id uuid.UUID, by *uuid.UUID) (Page, error) {
	row, err := r.queries.ApproveCMSPageWorkflow(ctx, db.ApproveCMSPageWorkflowParams{ID: id, ApprovedBy: sqlnull.UUID(by)})
	if err != nil {
		return Page{}, fmt.Errorf("approve page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) SetPageWorkflowState(ctx context.Context, id uuid.UUID, state WorkflowState) (Page, error) {
	row, err := r.queries.SetCMSPageWorkflowState(ctx, db.SetCMSPageWorkflowStateParams{ID: id, WorkflowState: string(state)})
	if err != nil {
		return Page{}, fmt.Errorf("set workflow state: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) AssignPage(ctx context.Context, id uuid.UUID, to *uuid.UUID) (Page, error) {
	row, err := r.queries.AssignCMSPage(ctx, db.AssignCMSPageParams{ID: id, AssignedTo: sqlnull.UUID(to)})
	if err != nil {
		return Page{}, fmt.Errorf("assign page: %w", err)
	}
	return toPage(row), nil
}

func (r *postgresRepository) CreateComment(ctx context.Context, in CommentInput, authorID uuid.UUID) (PageComment, error) {
	row, err := r.queries.CreateCMSPageComment(ctx, db.CreateCMSPageCommentParams{
		PageID:   in.PageID,
		Locale:   sqlnull.StringNonEmpty(in.Locale),
		BlockID:  sqlnull.StringNonEmpty(in.BlockID),
		AuthorID: authorID,
		Body:     in.Body,
	})
	if err != nil {
		return PageComment{}, fmt.Errorf("create comment: %w", err)
	}
	return toPageComment(row), nil
}

func (r *postgresRepository) ListComments(ctx context.Context, pageID uuid.UUID) ([]PageComment, error) {
	rows, err := r.queries.ListCMSPageComments(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	out := make([]PageComment, len(rows))
	for i, row := range rows {
		out[i] = toPageComment(row)
	}
	return out, nil
}

func (r *postgresRepository) ResolveComment(ctx context.Context, id uuid.UUID, resolved bool, by *uuid.UUID) (PageComment, error) {
	row, err := r.queries.ResolveCMSPageComment(ctx, db.ResolveCMSPageCommentParams{ID: id, Resolved: resolved, ResolvedBy: sqlnull.UUID(by)})
	if err != nil {
		return PageComment{}, fmt.Errorf("resolve comment: %w", err)
	}
	return toPageComment(row), nil
}

func (r *postgresRepository) GetComment(ctx context.Context, id uuid.UUID) (PageComment, error) {
	row, err := r.queries.GetCMSPageComment(ctx, id)
	if err != nil {
		return PageComment{}, fmt.Errorf("get comment: %w", err)
	}
	return toPageComment(row), nil
}

func (r *postgresRepository) DeleteComment(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteCMSPageComment(ctx, id); err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}

func toPageComment(row db.CmsPageComment) PageComment {
	return PageComment{
		ID:         row.ID,
		PageID:     row.PageID,
		Locale:     sqlnull.StringPtr(row.Locale),
		BlockID:    sqlnull.StringPtr(row.BlockID),
		AuthorID:   row.AuthorID,
		Body:       row.Body,
		Resolved:   row.Resolved,
		ResolvedBy: sqlnull.UUIDPtr(row.ResolvedBy),
		ResolvedAt: sqlnull.TimePtr(row.ResolvedAt),
		CreatedAt:  row.CreatedAt,
	}
}

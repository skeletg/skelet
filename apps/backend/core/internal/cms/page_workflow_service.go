package cms

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type pageWorkflowStore interface {
	ApprovePageWorkflow(ctx context.Context, id uuid.UUID, by *uuid.UUID) (Page, error)
	AssignPage(ctx context.Context, id uuid.UUID, to *uuid.UUID) (Page, error)
	CreateComment(ctx context.Context, in CommentInput, authorID uuid.UUID) (PageComment, error)
	DeleteComment(ctx context.Context, id uuid.UUID) error
	GetComment(ctx context.Context, id uuid.UUID) (PageComment, error)
	GetPageByID(ctx context.Context, id uuid.UUID) (Page, error)
	ListComments(ctx context.Context, pageID uuid.UUID) ([]PageComment, error)
	ResolveComment(ctx context.Context, id uuid.UUID, resolved bool, by *uuid.UUID) (PageComment, error)
	SetPageWorkflowState(ctx context.Context, id uuid.UUID, state WorkflowState) (Page, error)
	SubmitPageForReview(ctx context.Context, id uuid.UUID, by *uuid.UUID) (Page, error)
}

// Whether the publish gate is enabled - defaults to false, free publish, when no config reader is wired or the lookup fails
func (s *core) requireApproval(ctx context.Context) bool {
	if s.cfg == nil {
		return false
	}
	v, err := s.cfg.GetBool(ctx, cfgRequireApproval, config.DefaultScope)
	return err == nil && v
}

func (s *pageWorkflowService) SubmitPageForReview(ctx context.Context, id uuid.UUID) (Page, error) {
	if _, err := s.repo.SubmitPageForReview(ctx, id, adminPtr(ctx)); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

// Moves a page to approved, stamping the approver - the resolver gates this on cms.approve
func (s *pageWorkflowService) ApprovePage(ctx context.Context, id uuid.UUID) (Page, error) {
	if _, err := s.repo.ApprovePageWorkflow(ctx, id, adminPtr(ctx)); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

// Sends a page back to draft, recording the reason as a comment when one is given - the reason is saved first, so a rejection never lands without it
func (s *pageWorkflowService) RejectPage(ctx context.Context, id uuid.UUID, note string) (Page, error) {
	if n := strings.TrimSpace(note); n != "" {
		if by := adminPtr(ctx); by != nil {
			if _, err := s.repo.CreateComment(ctx, CommentInput{PageID: id, Body: "Rejected: " + n}, *by); err != nil {
				return Page{}, fmt.Errorf("save rejection reason: %w", err)
			}
		}
	}
	if _, err := s.repo.SetPageWorkflowState(ctx, id, WorkflowDraft); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

// Returns a page to draft from any state, e.g. to revise a published page through review again
func (s *pageWorkflowService) ReopenPage(ctx context.Context, id uuid.UUID) (Page, error) {
	if _, err := s.repo.SetPageWorkflowState(ctx, id, WorkflowDraft); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageWorkflowService) AssignPage(ctx context.Context, id uuid.UUID, to *uuid.UUID) (Page, error) {
	if _, err := s.repo.AssignPage(ctx, id, to); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, id)
}

func (s *pageWorkflowService) AddComment(ctx context.Context, in CommentInput) (PageComment, error) {
	if strings.TrimSpace(in.Body) == "" {
		return PageComment{}, fmt.Errorf("comment body is required")
	}
	by := adminPtr(ctx)
	if by == nil {
		return PageComment{}, fmt.Errorf("an authenticated admin is required to comment")
	}
	return s.repo.CreateComment(ctx, in, *by)
}

func (s *pageWorkflowService) ListComments(ctx context.Context, pageID uuid.UUID) ([]PageComment, error) {
	return s.repo.ListComments(ctx, pageID)
}

func (s *pageWorkflowService) ResolveComment(ctx context.Context, id uuid.UUID, resolved bool) (PageComment, error) {
	return s.repo.ResolveComment(ctx, id, resolved, adminPtr(ctx))
}

func (s *pageWorkflowService) GetComment(ctx context.Context, id uuid.UUID) (PageComment, error) {
	return s.repo.GetComment(ctx, id)
}

func (s *pageWorkflowService) DeleteComment(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteComment(ctx, id)
}

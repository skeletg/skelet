package cms

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

func (s *pageService) ListPageRevisions(ctx context.Context, pageID uuid.UUID, locale string) ([]PageRevision, error) {
	return s.repo.ListRevisions(ctx, pageID, locale, 50)
}

// Re-applies a revision's content, and since that itself snapshots, a restore is reversible
func (s *pageService) GetRevision(ctx context.Context, revisionID uuid.UUID) (PageRevision, error) {
	return s.repo.GetRevision(ctx, revisionID)
}

func (s *pageService) RestoreRevision(ctx context.Context, revisionID uuid.UUID) (Page, error) {
	rev, err := s.repo.GetRevision(ctx, revisionID)
	if err != nil {
		return Page{}, err
	}
	return s.UpsertPageContent(ctx, rev.PageID, PageContentInput{
		Locale:          rev.Locale,
		Title:           rev.Title,
		Body:            rev.Body,
		MetaTitle:       strOrEmpty(rev.MetaTitle),
		MetaDescription: strOrEmpty(rev.MetaDescription),
		MetaKeywords:    strOrEmpty(rev.MetaKeywords),
		Data:            rev.Data,
	})
}

// Sets, or clears when at is nil, a page's future go-live time - the cms-publish job flips is_published when it arrives
func (s *pageService) SchedulePage(ctx context.Context, pageID uuid.UUID, at *time.Time) (Page, error) {
	if _, err := s.repo.SetScheduledAt(ctx, pageID, at); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// Publishes every unpublished page whose scheduled_at has arrived, clearing the schedule - run by the cms-publish scheduler job
func (s *pageService) PromoteDuePages(ctx context.Context) (int, error) {
	due, err := s.repo.ListDueScheduledPages(ctx)
	if err != nil {
		return 0, err
	}
	gate := s.requireApproval(ctx)
	n := 0
	for _, p := range due {
		// Respect the editorial gate - a scheduled page that isn't approved stays scheduled rather than erroring, and the operator must approve it
		if gate && p.WorkflowState != WorkflowApproved && p.WorkflowState != WorkflowPublished {
			log.Printf("[cms-publish] skipping due page %s: workflow_state=%s (approval required)", p.ID, p.WorkflowState)
			continue
		}
		if _, err := s.repo.SetScheduledAt(ctx, p.ID, nil); err != nil {
			return n, err
		}
		now := time.Now()
		if _, err := s.repo.SetPagePublished(ctx, p.ID, true, &now); err != nil {
			return n, err
		}
		_, _ = s.repo.SetPageWorkflowState(ctx, p.ID, WorkflowPublished)
		if s.bus != nil {
			s.bus.Publish(ctx, &PagePublished{
				PageID:     p.ID,
				PageTypeID: p.PageTypeID,
				Slug:       p.Slug,
				URLPath:    p.URLPath,
			})
		}
		n++
	}
	return n, nil
}

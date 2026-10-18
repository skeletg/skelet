package cms

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
)

// How many revisions a (page, locale) keeps, and so how many the history lists
const revisionRetention = 50

func (s *pageService) ListPageRevisions(ctx context.Context, pageID uuid.UUID, locale string) ([]PageRevision, error) {
	return s.repo.ListRevisions(ctx, pageID, locale, revisionRetention)
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
		// Held back by the gate or a hook - it stays scheduled and gets asked again next run
		if err := s.admitPublish(ctx, p, gate); err != nil {
			log.Printf("[cms-publish] skipping due page %s (workflow_state=%s): %v", p.ID, p.WorkflowState, err)
			continue
		}
		if p.ScheduledAt == nil {
			continue
		}
		// A first go-live is dated when it was scheduled for, not when the job ran - and it only lands on the schedule read here, so an unpublish or a cleared schedule since then wins
		live, err := s.repo.GoLiveScheduled(ctx, p.ID, publishDate(p, *p.ScheduledAt), *p.ScheduledAt)
		if err != nil {
			return n, err
		}
		if !live {
			continue
		}
		s.announcePublished(ctx, p)
		n++
	}
	return n, nil
}

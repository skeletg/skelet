package cms

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// Creates or updates a per-store availability row for a page and returns it with all listings hydrated
// Mirrors blog.Service.SetPostChannelListing - admins use it to toggle a page per store without flipping the global flag
func (s *pageService) SetPageChannelListing(ctx context.Context, in SetPageChannelListingInput) (Page, error) {
	// Demote any existing homepage first when flagging this one, inside a single tx, so the partial unique index never sees two mid-flight
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		r := s.repo.WithTx(t)
		if in.IsHomepage {
			if err := r.ClearChannelHomepage(ctx, in.ChannelID, in.PageID); err != nil {
				return err
			}
		}
		_, err := r.UpsertPageChannelListing(ctx, in)
		return err
	})
	if err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, in.PageID)
}

// The channel's designated homepage - the page flagged is_homepage on its published listing - with content for the locale, or ErrPageNotFound when none is set
func (s *pageService) GetHomePageForChannel(ctx context.Context, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetHomePageForChannel(ctx, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) DeletePageChannelListing(ctx context.Context, pageID, channelID uuid.UUID) (Page, error) {
	if err := s.repo.DeletePageChannelListing(ctx, pageID, channelID); err != nil {
		return Page{}, err
	}
	return s.repo.GetPageByID(ctx, pageID)
}

// The per-channel listing rows for a page - exposed so resolvers can gate localized-content mutations against the page's current listing channel set
func (s *pageService) ListPageChannelListings(ctx context.Context, pageID uuid.UUID) ([]PageChannelListing, error) {
	return s.repo.ListPageChannelListings(ctx, pageID)
}

// Resolves a page by id with the per-store listing enforced, so globally unpublished or unlisted pages return ErrPageNotFound - locale applies to Content as in GetPage
func (s *pageService) GetPublishedPageForChannel(ctx context.Context, pageID, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPublishedPageForChannel(ctx, pageID, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) GetPublishedPageBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPublishedPageBySlugForChannel(ctx, slug, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

func (s *pageService) GetPublishedPageByURLPathForChannel(ctx context.Context, urlPath string, channelID uuid.UUID, locale string) (Page, error) {
	p, err := s.repo.GetPublishedPageByURLPathForChannel(ctx, urlPath, channelID)
	if err != nil {
		return Page{}, err
	}
	s.fillPageContent(ctx, &p, locale)
	return p, nil
}

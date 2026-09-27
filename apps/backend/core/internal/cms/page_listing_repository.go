package cms

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func (r *postgresRepository) UpsertPageChannelListing(ctx context.Context, in SetPageChannelListingInput) (PageChannelListing, error) {
	row, err := r.queries.UpsertCMSPageChannelListing(ctx, db.UpsertCMSPageChannelListingParams{
		PageID:      in.PageID,
		ChannelID:   in.ChannelID,
		IsPublished: in.IsPublished,
		SeoNoindex:  in.SEONoindex,
		IsHomepage:  in.IsHomepage,
	})
	if err != nil {
		return PageChannelListing{}, fmt.Errorf("upsert cms page store listing: %w", err)
	}
	return toPageChannelListing(row), nil
}

// ClearChannelHomepage unsets the homepage flag on every page of a channel except keepPageID - Run before flagging a
// new homepage so the per-channel partial unique index isn't violated mid-transaction
func (r *postgresRepository) ClearChannelHomepage(ctx context.Context, channelID, keepPageID uuid.UUID) error {
	if err := r.queries.ClearCMSChannelHomepage(ctx, db.ClearCMSChannelHomepageParams{
		ChannelID: channelID,
		PageID:    keepPageID,
	}); err != nil {
		return fmt.Errorf("clear cms channel homepage: %w", err)
	}
	return nil
}

// GetHomePageForChannel returns the channel's homepage (the flagged, published
// page) - Returns ErrPageNotFound when the channel has no homepage set
func (r *postgresRepository) GetHomePageForChannel(ctx context.Context, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetCMSHomePageForChannel(ctx, channelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get cms homepage for channel: %w", err)
	}
	// GetCMSHomePageForChannel returns the bare page row only - Reuse GetPageByID to hydrate AllContent (+ selections/relationships/listings) so the resolver's `content` field resolves - without this, fillPageContent finds no AllContent and the storefront falls back to its built-in homepage
	return r.GetPageByID(ctx, toPage(row).ID)
}

func (r *postgresRepository) DeletePageChannelListing(ctx context.Context, pageID, channelID uuid.UUID) error {
	_, err := r.queries.DeleteCMSPageChannelListing(ctx, db.DeleteCMSPageChannelListingParams{
		PageID:    pageID,
		ChannelID: channelID,
	})
	if err != nil {
		return fmt.Errorf("delete cms page store listing: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPageChannelListings(ctx context.Context, pageID uuid.UUID) ([]PageChannelListing, error) {
	rows, err := r.queries.ListCMSPageChannelListings(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("list cms page store listings: %w", err)
	}
	out := make([]PageChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toPageChannelListing(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPageChannelListingsByPageIDs(ctx context.Context, pageIDs []uuid.UUID) (map[uuid.UUID][]PageChannelListing, error) {
	if len(pageIDs) == 0 {
		return map[uuid.UUID][]PageChannelListing{}, nil
	}
	rows, err := r.queries.ListCMSPageChannelListingsByPageIDs(ctx, pageIDs)
	if err != nil {
		return nil, fmt.Errorf("list cms page store listings by ids: %w", err)
	}
	out := make(map[uuid.UUID][]PageChannelListing)
	for _, row := range rows {
		out[row.PageID] = append(out[row.PageID], toPageChannelListing(row))
	}
	return out, nil
}

func (r *postgresRepository) GetPublishedPageForChannel(ctx context.Context, pageID, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetPublishedCMSPageForChannel(ctx, db.GetPublishedCMSPageForChannelParams{
		ID:        pageID,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get published cms page for store: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) GetPublishedPageBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetPublishedCMSPageBySlugForChannel(ctx, db.GetPublishedCMSPageBySlugForChannelParams{
		Slug:      slug,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get published cms page by slug for store: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func (r *postgresRepository) GetPublishedPageByURLPathForChannel(ctx context.Context, urlPath string, channelID uuid.UUID) (Page, error) {
	row, err := r.queries.GetPublishedCMSPageByURLPathForChannel(ctx, db.GetPublishedCMSPageByURLPathForChannelParams{
		UrlPath:   urlPath,
		ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Page{}, ErrPageNotFound
		}
		return Page{}, fmt.Errorf("get published cms page by url for store: %w", err)
	}
	return r.GetPageByID(ctx, row.ID)
}

func toPageChannelListing(row db.CmsPageChannelListing) PageChannelListing {
	return PageChannelListing{
		ChannelID:   row.ChannelID,
		IsPublished: row.IsPublished,
		SEONoindex:  row.SeoNoindex,
		IsHomepage:  row.IsHomepage,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

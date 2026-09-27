package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func (r *postgresRepository) UpsertPostChannelListing(ctx context.Context, in SetPostChannelListingInput) (PostChannelListing, error) {
	row, err := r.queries.UpsertBlogPostChannelListing(ctx, db.UpsertBlogPostChannelListingParams{
		PostID:      in.PostID,
		ChannelID:   in.ChannelID,
		IsPublished: in.IsPublished,
		SeoNoindex:  in.SEONoindex,
	})
	if err != nil {
		return PostChannelListing{}, fmt.Errorf("upsert post store listing: %w", err)
	}
	return toPostChannelListing(row), nil
}

func (r *postgresRepository) DeletePostChannelListing(ctx context.Context, postID, channelID uuid.UUID) error {
	_, err := r.queries.DeleteBlogPostChannelListing(ctx, db.DeleteBlogPostChannelListingParams{
		PostID: postID, ChannelID: channelID,
	})
	if err != nil {
		return fmt.Errorf("delete post store listing: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListPostChannelListings(ctx context.Context, postID uuid.UUID) ([]PostChannelListing, error) {
	rows, err := r.queries.ListBlogPostChannelListings(ctx, postID)
	if err != nil {
		return nil, fmt.Errorf("list post store listings: %w", err)
	}
	out := make([]PostChannelListing, len(rows))
	for i, row := range rows {
		out[i] = toPostChannelListing(row)
	}
	return out, nil
}

func (r *postgresRepository) ListPostChannelListingsByPostIDs(ctx context.Context, postIDs []uuid.UUID) (map[uuid.UUID][]PostChannelListing, error) {
	if len(postIDs) == 0 {
		return map[uuid.UUID][]PostChannelListing{}, nil
	}
	rows, err := r.queries.ListBlogPostChannelListingsByPostIDs(ctx, postIDs)
	if err != nil {
		return nil, fmt.Errorf("list post store listings by post ids: %w", err)
	}
	out := make(map[uuid.UUID][]PostChannelListing)
	for _, row := range rows {
		out[row.PostID] = append(out[row.PostID], toPostChannelListing(row))
	}
	return out, nil
}

func (r *postgresRepository) GetPublishedPostForChannel(ctx context.Context, postID, channelID uuid.UUID) (Post, error) {
	row, err := r.queries.GetPublishedBlogPostForChannel(ctx, db.GetPublishedBlogPostForChannelParams{
		ID: postID, ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get published post for store: %w", err)
	}
	return r.GetPostByID(ctx, row.ID)
}

func (r *postgresRepository) GetPublishedPostBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID) (Post, error) {
	row, err := r.queries.GetPublishedBlogPostBySlugForChannel(ctx, db.GetPublishedBlogPostBySlugForChannelParams{
		Slug: slug, ChannelID: channelID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get published post by slug for store: %w", err)
	}
	return r.GetPostByID(ctx, row.ID)
}

func toPostChannelListing(row db.BlogPostChannelListing) PostChannelListing {
	return PostChannelListing{
		ChannelID:   row.ChannelID,
		IsPublished: row.IsPublished,
		SEONoindex:  row.SeoNoindex,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

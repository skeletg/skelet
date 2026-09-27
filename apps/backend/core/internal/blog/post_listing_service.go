package blog

import (
	"context"

	"github.com/google/uuid"
)

// SetPostChannelListing creates or updates a per-store availability row for a post - Combined with the post's global
// status, this gates whether storefront callers see the post on a given store
func (s *postService) SetPostChannelListing(ctx context.Context, in SetPostChannelListingInput) (Post, error) {
	if _, err := s.repo.GetPostByID(ctx, in.PostID); err != nil {
		return Post{}, err
	}
	listing, err := s.repo.UpsertPostChannelListing(ctx, in)
	if err != nil {
		return Post{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostListingChanged{
			PostID:      in.PostID,
			ChannelID:   in.ChannelID,
			IsPublished: listing.IsPublished,
			Cleared:     false,
		})
	}
	return s.GetPost(ctx, in.PostID, "")
}

func (s *postService) DeletePostChannelListing(ctx context.Context, postID, channelID uuid.UUID) (Post, error) {
	if err := s.repo.DeletePostChannelListing(ctx, postID, channelID); err != nil {
		return Post{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &PostListingChanged{
			PostID:      postID,
			ChannelID:   channelID,
			IsPublished: false,
			Cleared:     true,
		})
	}
	return s.GetPost(ctx, postID, "")
}

// ListPostChannelListings returns the per-channel listing rows for a post - Exposed so resolvers can gate
// localized-content mutations against the post's current listing channel set (auth RequireAllListedChannelsPermission)
func (s *postService) ListPostChannelListings(ctx context.Context, postID uuid.UUID) ([]PostChannelListing, error) {
	return s.repo.ListPostChannelListings(ctx, postID)
}

// GetPublishedPostBySlugForChannel returns a post only if it is globally published and listed on the given store with
// is_published=true - Used by storefront resolvers and feed/sitemap handlers
func (s *postService) GetPublishedPostBySlugForChannel(ctx context.Context, slug string, channelID uuid.UUID, locale string) (Post, error) {
	p, err := s.repo.GetPublishedPostBySlugForChannel(ctx, slug, channelID)
	if err != nil {
		return Post{}, err
	}
	s.fillPostContent(ctx, &p, locale)
	if p.AuthorID != nil {
		if a, err := s.repo.GetAuthorByID(ctx, *p.AuthorID); err == nil {
			s.fillAuthorContent(ctx, &a, locale)
			p.Author = &a
		}
	}
	return p, nil
}

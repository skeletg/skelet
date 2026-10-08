package cms

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type tagStore interface {
	DeleteTag(ctx context.Context, slug string) error
	ListTags(ctx context.Context) ([]Tag, error)
	RenameTag(ctx context.Context, slug, name string) (Tag, error)
	TagUsageCounts(ctx context.Context) (map[string]int, error)
	UpsertTag(ctx context.Context, slug, name string) (Tag, error)
}

var tagSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Derives the stable slug pages actually store from a display name
func slugifyTag(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = tagSlugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "-")
}

func (s *tagService) ListTags(ctx context.Context) ([]Tag, error) { return s.repo.ListTags(ctx) }

func (s *tagService) TagUsageCounts(ctx context.Context) (map[string]int, error) {
	return s.repo.TagUsageCounts(ctx)
}

// Adds a tag, or updates the display name on a slug collision
func (s *tagService) CreateTag(ctx context.Context, name string) (Tag, error) {
	slug := slugifyTag(name)
	if slug == "" {
		return Tag{}, fmt.Errorf("%w: a tag name needs a letter or number", ErrInvalidSlug)
	}
	return s.repo.UpsertTag(ctx, slug, strings.TrimSpace(name))
}

// Changes a tag's display name - the slug pages store stays stable
func (s *tagService) RenameTag(ctx context.Context, slug, name string) (Tag, error) {
	display := strings.TrimSpace(name)
	if display == "" {
		return Tag{}, fmt.Errorf("%w: tag name", ErrRequiredFieldMissing)
	}
	return s.repo.RenameTag(ctx, slugifyTag(slug), display)
}

// Removes a tag from the vocabulary and from every page using it
func (s *tagService) DeleteTag(ctx context.Context, slug string) error {
	return s.repo.DeleteTag(ctx, slugifyTag(slug))
}

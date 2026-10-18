package admin

// Helpers for blog.resolvers.go. Kept in a separate file because gqlgen rewrites *.resolvers.go on regeneration and
// moves any non-resolver functions into a /* */ comment block at the bottom

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
)

func parseUUIDs(in []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, len(in))
	for i, s := range in {
		u, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("invalid uuid %q: %w", s, err)
		}
		out[i] = u
	}
	return out, nil
}

// parseUUIDOpt parses an optional UUID string, surfacing errors. Distinct from
// graph.parseOptionalUUID which silently returns nil on parse failure.
func parseUUIDOpt(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return nil, errInvalidID("uuid", err)
	}
	return &u, nil
}

func boolDeref(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

func intDeref(n *int32, def int32) int32 {
	if n == nil {
		return def
	}
	return *n
}

// rawJSON converts a JSON-scalar input value (decoded as `any` by gqlgen) to json.RawMessage. Returns nil for nil
// inputs so the domain layer can treat "absent" the same as "no content"
func rawJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if string(raw) == "null" {
		return nil
	}
	return raw
}

func unmarshalJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func strLowerStatus(s model.BlogPostStatus) string {
	switch s {
	case model.BlogPostStatusDraft:
		return "draft"
	case model.BlogPostStatusScheduled:
		return "scheduled"
	case model.BlogPostStatusPublished:
		return "published"
	case model.BlogPostStatusArchived:
		return "archived"
	}
	return "draft"
}

func blogMetadataFromInput(in []*model.MetadataInput) blog.Metadata {
	if in == nil {
		return nil
	}
	out := make(blog.Metadata, len(in))
	for _, item := range in {
		if item == nil {
			continue
		}
		out[item.Key] = item.Value
	}
	return out
}

// ==================== Conversions: Author ====================

func fromCreateBlogAuthorInput(in model.CreateBlogAuthorInput) (blog.CreateAuthorInput, error) {
	out := blog.CreateAuthorInput{
		Slug:        in.Slug,
		DisplayName: in.DisplayName,
		Email:       in.Email,
		EmailPublic: boolDeref(in.EmailPublic, false),
		Metadata:    blogMetadataFromInput(in.Metadata),
		Locale:      ptr.Deref(in.Locale),
		Name:        ptr.Deref(in.Name),
		Bio:         rawJSON(in.Bio),
	}
	if v, err := parseUUIDOpt(in.AvatarAssetID); err != nil {
		return blog.CreateAuthorInput{}, err
	} else {
		out.AvatarAssetID = v
	}
	if v, err := parseUUIDOpt(in.LinkedAdminID); err != nil {
		return blog.CreateAuthorInput{}, err
	} else {
		out.LinkedAdminID = v
	}
	return out, nil
}

func fromUpdateBlogAuthorInput(in model.UpdateBlogAuthorInput) (blog.UpdateAuthorInput, error) {
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return blog.UpdateAuthorInput{}, errInvalidID("id", err)
	}
	out := blog.UpdateAuthorInput{
		ID:          id,
		Slug:        in.Slug,
		DisplayName: in.DisplayName,
		Email:       in.Email,
		ClearEmail:  boolDeref(in.ClearEmail, false),
		EmailPublic: in.EmailPublic,
		ClearAvatar: boolDeref(in.ClearAvatar, false),
		ClearAdmin:  boolDeref(in.ClearLinkedAdmin, false),
	}
	if v, err := parseUUIDOpt(in.AvatarAssetID); err != nil {
		return blog.UpdateAuthorInput{}, err
	} else {
		out.AvatarAssetID = v
	}
	if v, err := parseUUIDOpt(in.LinkedAdminID); err != nil {
		return blog.UpdateAuthorInput{}, err
	} else {
		out.LinkedAdminID = v
	}
	if in.Metadata != nil {
		md := blogMetadataFromInput(in.Metadata)
		out.Metadata = &md
	}
	return out, nil
}

// ==================== Conversions: Category ====================

func fromCreateBlogCategoryInput(in model.CreateBlogCategoryInput) (blog.CreateCategoryInput, error) {
	out := blog.CreateCategoryInput{
		Slug:            in.Slug,
		Position:        intDeref(in.Position, 0),
		IsPublished:     boolDeref(in.IsPublished, false),
		SEONoindex:      boolDeref(in.SeoNoindex, false),
		Locale:          in.Locale,
		Name:            in.Name,
		Description:     rawJSON(in.Description),
		MetaTitle:       ptr.Deref(in.MetaTitle),
		MetaDescription: ptr.Deref(in.MetaDescription),
		MetaKeywords:    ptr.Deref(in.MetaKeywords),
	}
	if v, err := parseUUIDOpt(in.ParentID); err != nil {
		return blog.CreateCategoryInput{}, err
	} else {
		out.ParentID = v
	}
	return out, nil
}

func fromUpdateBlogCategoryInput(in model.UpdateBlogCategoryInput) (blog.UpdateCategoryInput, error) {
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return blog.UpdateCategoryInput{}, errInvalidID("id", err)
	}
	out := blog.UpdateCategoryInput{
		ID:          id,
		Slug:        in.Slug,
		Position:    in.Position,
		IsPublished: in.IsPublished,
		SEONoindex:  in.SeoNoindex,
		ClearParent: boolDeref(in.ClearParent, false),
	}
	if v, err := parseUUIDOpt(in.ParentID); err != nil {
		return blog.UpdateCategoryInput{}, err
	} else {
		out.ParentID = v
	}
	return out, nil
}

// ==================== Conversions: Post ====================

func fromCreateBlogPostInput(in model.CreateBlogPostInput) (blog.CreatePostInput, error) {
	out := blog.CreatePostInput{
		Slug:            in.Slug,
		SEONoindex:      boolDeref(in.SeoNoindex, false),
		Metadata:        blogMetadataFromInput(in.Metadata),
		PrivateMetadata: blogMetadataFromInput(in.PrivateMetadata),
		Locale:          in.Locale,
		Title:           in.Title,
		Excerpt:         rawJSON(in.Excerpt),
		Body:            rawJSON(in.Body),
		MetaTitle:       ptr.Deref(in.MetaTitle),
		MetaDescription: ptr.Deref(in.MetaDescription),
		MetaKeywords:    ptr.Deref(in.MetaKeywords),
	}
	if in.Status != nil {
		out.Status = blog.PostStatus(strLowerStatus(*in.Status))
	}
	if in.PublishedAt != nil {
		t := *in.PublishedAt
		out.PublishedAt = &t
	}
	if in.ReadingTimeMinutes != nil {
		v := *in.ReadingTimeMinutes
		out.ReadingTimeMinutes = &v
	}
	if v, err := parseUUIDOpt(in.AuthorID); err != nil {
		return blog.CreatePostInput{}, err
	} else {
		out.AuthorID = v
	}
	if v, err := parseUUIDOpt(in.CoverAssetID); err != nil {
		return blog.CreatePostInput{}, err
	} else {
		out.CoverAssetID = v
	}
	if len(in.CategoryIds) > 0 {
		ids, err := parseUUIDs(in.CategoryIds)
		if err != nil {
			return blog.CreatePostInput{}, err
		}
		out.CategoryIDs = ids
	}
	if len(in.TagIds) > 0 {
		ids, err := parseUUIDs(in.TagIds)
		if err != nil {
			return blog.CreatePostInput{}, err
		}
		out.TagIDs = ids
	}
	return out, nil
}

func fromUpdateBlogPostInput(in model.UpdateBlogPostInput) (blog.UpdatePostInput, error) {
	id, err := uuid.Parse(in.ID)
	if err != nil {
		return blog.UpdatePostInput{}, errInvalidID("id", err)
	}
	out := blog.UpdatePostInput{
		ID:          id,
		Slug:        in.Slug,
		SEONoindex:  in.SeoNoindex,
		ClearAuthor: boolDeref(in.ClearAuthor, false),
		ClearCover:  boolDeref(in.ClearCover, false),
	}
	if in.ReadingTimeMinutes != nil {
		v := *in.ReadingTimeMinutes
		out.ReadingTimeMinutes = &v
	}
	if v, err := parseUUIDOpt(in.AuthorID); err != nil {
		return blog.UpdatePostInput{}, err
	} else {
		out.AuthorID = v
	}
	if v, err := parseUUIDOpt(in.CoverAssetID); err != nil {
		return blog.UpdatePostInput{}, err
	} else {
		out.CoverAssetID = v
	}
	if in.Metadata != nil {
		md := blogMetadataFromInput(in.Metadata)
		out.Metadata = &md
	}
	if in.PrivateMetadata != nil {
		md := blogMetadataFromInput(in.PrivateMetadata)
		out.PrivateMetadata = &md
	}
	return out, nil
}

// List reads carry only the resolved locale, and a list answering allContent with fewer rows than the record
// has replaced the ones the dashboard's cache held from the detail query - the next save wrote the blanks back.
// These fill every locale's rows in one query per list

func (r *Resolver) withBlogCategoryTranslations(ctx context.Context, cats []blog.Category) ([]blog.Category, error) {
	var ids []uuid.UUID
	var collect func([]blog.Category)
	collect = func(level []blog.Category) {
		for _, c := range level {
			ids = append(ids, c.ID)
			collect(c.Children)
		}
	}
	collect(cats)
	byID, err := r.BlogSvc.ListCategoryTranslations(ctx, ids)
	if err != nil {
		return nil, err
	}
	var fill func([]blog.Category)
	fill = func(level []blog.Category) {
		for i := range level {
			level[i].AllContent = byID[level[i].ID]
			fill(level[i].Children)
		}
	}
	fill(cats)
	return cats, nil
}

func (r *Resolver) withBlogTagTranslations(ctx context.Context, tags []blog.Tag) ([]blog.Tag, error) {
	ids := make([]uuid.UUID, len(tags))
	for i, t := range tags {
		ids[i] = t.ID
	}
	byID, err := r.BlogSvc.ListTagTranslations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range tags {
		tags[i].AllContent = byID[tags[i].ID]
	}
	return tags, nil
}

func (r *Resolver) withBlogAuthorTranslations(ctx context.Context, authors []blog.Author) ([]blog.Author, error) {
	ids := make([]uuid.UUID, len(authors))
	for i, a := range authors {
		ids[i] = a.ID
	}
	byID, err := r.BlogSvc.ListAuthorTranslations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range authors {
		authors[i].AllContent = byID[authors[i].ID]
	}
	return authors, nil
}

func (r *Resolver) withBlogPostTranslations(ctx context.Context, posts []blog.Post) ([]blog.Post, error) {
	ids := make([]uuid.UUID, len(posts))
	for i, p := range posts {
		ids[i] = p.ID
	}
	byID, err := r.BlogSvc.ListPostTranslations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range posts {
		posts[i].AllContent = byID[posts[i].ID]
	}
	return posts, nil
}

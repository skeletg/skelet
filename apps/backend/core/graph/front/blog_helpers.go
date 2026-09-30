package front

// Helpers live out of blog.resolvers.go because gqlgen rewrites *.resolvers.go on regeneration and buries any non-resolver function in a comment block at the bottom

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/blog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

func channelIDFromContext(ctx context.Context) *uuid.UUID { // nil when no store was resolved - admin GraphQL traffic, introspection and the like
	if sv, ok := channel.FromContext(ctx); ok && sv != nil {
		id := sv.ID
		return &id
	}
	return nil
}

func parseUUIDOpt(s *string) (*uuid.UUID, error) { // surfaces the error, unlike graph.parseOptionalUUID which silently returns nil on a bad parse
	if s == nil || *s == "" {
		return nil, nil
	}
	u, err := uuid.Parse(*s)
	if err != nil {
		return nil, invalidID("")
	}
	return &u, nil
}

func intDeref(n *int32, def int32) int32 {
	if n == nil {
		return def
	}
	return *n
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

// The storefront shares the admin's models and services, which return unpublished categories and tags too - these prune them wherever they reach a shopper, down to a post's own lists and a category's parent and children

func publicCategory(c *model.BlogCategory) (*model.BlogCategory, error) {
	if c == nil || !c.IsPublished {
		return nil, blog.ErrCategoryNotFound // an unpublished category reads exactly like a missing one
	}
	return prunedCategory(c), nil
}

func prunedCategory(c *model.BlogCategory) *model.BlogCategory {
	cp := *c
	if cp.Parent != nil && !cp.Parent.IsPublished {
		cp.Parent = nil
	}
	cp.Children = publicCategories(c.Children)
	return &cp
}

func publicCategories(in []*model.BlogCategory) []*model.BlogCategory {
	out := make([]*model.BlogCategory, 0, len(in))
	for _, c := range in {
		if c != nil && c.IsPublished {
			out = append(out, prunedCategory(c))
		}
	}
	return out
}

func publicTag(t *model.BlogTag) (*model.BlogTag, error) {
	if t == nil || !t.IsPublished {
		return nil, blog.ErrTagNotFound
	}
	return t, nil
}

func publicTags(in []*model.BlogTag) []*model.BlogTag {
	out := make([]*model.BlogTag, 0, len(in))
	for _, t := range in {
		if t != nil && t.IsPublished {
			out = append(out, t)
		}
	}
	return out
}

func publicPost(p *model.BlogPost) *model.BlogPost {
	if p == nil {
		return nil
	}
	cp := *p
	cp.Categories = publicCategories(p.Categories)
	cp.Tags = publicTags(p.Tags)
	return &cp
}

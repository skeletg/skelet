package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/front/model"
	"github.com/skeletg/skelet/apps/backend/internal/urlrewrite"
)

// No resolved channel = zeros, not an error - a misconfigured host still renders an empty (valid) sitemap
func (r *queryResolver) SitemapCounts(ctx context.Context) (*model.SitemapCounts, error) {
	ch := channelIDFromContext(ctx)
	if ch == nil {
		return &model.SitemapCounts{}, nil
	}
	products, categories, collections, err := r.URLRewriteSvc.SitemapCounts(ctx, *ch)
	if err != nil {
		return nil, err
	}
	return &model.SitemapCounts{
		Products:    int32(products),
		Categories:  int32(categories),
		Collections: int32(collections),
	}, nil
}

func (r *queryResolver) SitemapEntries(ctx context.Context, entityType model.SitemapEntityType, limit int32, offset int32) ([]*model.SitemapEntry, error) {
	ch := channelIDFromContext(ctx)
	if ch == nil {
		return []*model.SitemapEntry{}, nil
	}
	var et urlrewrite.EntityType
	switch entityType {
	case model.SitemapEntityTypeProduct:
		et = urlrewrite.EntityTypeProduct
	case model.SitemapEntityTypeCategory:
		et = urlrewrite.EntityTypeCategory
	case model.SitemapEntityTypeCollection:
		et = urlrewrite.EntityTypeCollection
	}
	rows, err := r.URLRewriteSvc.SitemapEntries(ctx, et, *ch, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]*model.SitemapEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, &model.SitemapEntry{Path: row.Path, UpdatedAt: row.UpdatedAt})
	}
	return out, nil
}

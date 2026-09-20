package search

import (
	"context"

	"github.com/google/uuid"
)

// Engine is the storefront's READ contract for product discovery. Everything a resolver needs, phrased in domain terms rather than query DSL - which is what lets a non-document store implement it.
type Engine interface {
	// Free-text product search. An empty query yields nothing - BrowseProducts is the "list everything" path
	SearchProducts(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int, boosts map[uuid.UUID]float64) ([]ProductHit, int64, error)
	// Faceted listing. Facets may come back empty on an engine that can't compute them; callers must treat that as "no filters offered", never as "no values match"
	BrowseProducts(ctx context.Context, channelID uuid.UUID, locale, query string, filter BrowseFilter, sortSlug string, limit, offset int, boosts map[uuid.UUID]float64) (BrowseResult, error)
	// The sort options to offer. Engines differ on what they can order by, so this is per-engine rather than global config
	ListSortOptions(ctx context.Context) []SortOption
	// Short name for logs and health output, e.g. "opensearch" or "postgres"
	Name() string
}

// Compile-time proof the OpenSearch indexer satisfies the contract
var _ Engine = (*ProductIndexer)(nil)

func (i *ProductIndexer) Name() string { return "opensearch" }

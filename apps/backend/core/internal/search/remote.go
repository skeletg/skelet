package search

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
)

// Merchant HTTP endpoint (remoteprovider kind=search) answering storefront product search in place of OpenSearch - the escape hatch for Algolia/Constructor-class engines without a Go plugin Covers the search box and faceted browse; for browse the provider returns ids + facet buckets in index-level shape and the front resolver enriches names/translations, so the provider needs no catalog awareness search request {query, channelId, locale, limit, offset}, response {hits: [{productId, sku, name, url}], total} browse request {channelId, locale, query, sort, limit, offset, filter: {categoryId?, collectionId?, priceMinCents?, priceMaxCents?, kinds?, attributeIn: [{slug, values}]}} browse response {productIds, total, facets: [{field, slug, buckets: [{value, slug, count, isIndexable}]}]} Channel-scoped, and fail_open falls back to OpenSearch - the local index is always the baseline, so a down search API degrades to native results rather than an empty page

type RemoteSearchSource interface {
	// ok=false falls back to OpenSearch
	Search(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int) ([]ProductHit, int64, bool)
	Browse(ctx context.Context, channelID uuid.UUID, locale, query string, filter BrowseFilter, sortSlug string, limit, offset int) (BrowseResult, bool)
}

// Optional - nil leaves product search on OpenSearch
func (i *ProductIndexer) SetRemoteSource(src RemoteSearchSource) { i.remoteSearch = src }

type remoteSearchRequest struct {
	Query     string `json:"query"`
	ChannelID string `json:"channelId"`
	Locale    string `json:"locale"`
	Limit     int    `json:"limit"`
	Offset    int    `json:"offset"`
}

type remoteSearchHit struct {
	ProductID string `json:"productId"`
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	URL       string `json:"url"`
}

type remoteSearchResponse struct {
	Hits  []remoteSearchHit `json:"hits"`
	Total int64             `json:"total"`
}

type providerLookup interface {
	ActiveFor(ctx context.Context, kind remoteprovider.Kind, channelID *uuid.UUID) (remoteprovider.Provider, bool, error)
}

type remoteSearchSource struct {
	svc     providerLookup
	invoker *remoteprovider.Invoker
}

func NewRemoteSearchSource(svc *remoteprovider.Service, invoker *remoteprovider.Invoker) RemoteSearchSource {
	return &remoteSearchSource{svc: svc, invoker: invoker}
}

// The channel's own provider wins over a global one - the first by slug let a global engine answer a store that brought its own
func (r *remoteSearchSource) provider(ctx context.Context, channelID uuid.UUID) (remoteprovider.Provider, bool) {
	p, ok, err := r.svc.ActiveFor(ctx, remoteprovider.KindSearch, &channelID)
	if err != nil {
		log.Printf("[search] resolve remote search provider: %v", err)
	}
	return p, ok && err == nil
}

func (r *remoteSearchSource) Search(ctx context.Context, query string, channelID uuid.UUID, locale string, limit, offset int) ([]ProductHit, int64, bool) {
	p, ok := r.provider(ctx, channelID)
	if !ok {
		return nil, 0, false
	}

	req := remoteSearchRequest{Query: query, ChannelID: channelID.String(), Locale: locale, Limit: limit, Offset: offset}
	var resp remoteSearchResponse
	if err := r.invoker.Invoke(ctx, p, req, &resp); err != nil {
		log.Printf("[search] remote search %q failed (OpenSearch fallback): %v", p.Slug, err)
		return nil, 0, false
	}
	hits := make([]ProductHit, 0, len(resp.Hits))
	for _, h := range resp.Hits {
		id, perr := uuid.Parse(h.ProductID)
		if perr != nil {
			continue
		}
		hits = append(hits, ProductHit{ProductID: id, URL: h.URL, SKU: h.SKU, Name: h.Name})
	}
	return hits, resp.Total, true
}

type remoteBrowseRequest struct {
	ChannelID string             `json:"channelId"`
	Locale    string             `json:"locale"`
	Query     string             `json:"query"`
	Sort      string             `json:"sort"`
	Limit     int                `json:"limit"`
	Offset    int                `json:"offset"`
	Filter    remoteBrowseFilter `json:"filter"`
}

type remoteBrowseFilter struct {
	CategoryID    string                  `json:"categoryId,omitempty"`
	CollectionID  string                  `json:"collectionId,omitempty"`
	PriceMinCents *int64                  `json:"priceMinCents,omitempty"`
	PriceMaxCents *int64                  `json:"priceMaxCents,omitempty"`
	Kinds         []string                `json:"kinds,omitempty"`
	AttributeIn   []remoteBrowseAttribute `json:"attributeIn,omitempty"`
}

type remoteBrowseAttribute struct {
	Slug   string   `json:"slug"`
	Values []string `json:"values"`
}

type remoteBrowseBucket struct {
	Value       string `json:"value"`
	Slug        string `json:"slug"`
	Count       int64  `json:"count"`
	IsIndexable bool   `json:"isIndexable"`
}

type remoteBrowseFacet struct {
	Field   string               `json:"field"`
	Slug    string               `json:"slug"`
	Buckets []remoteBrowseBucket `json:"buckets"`
}

type remoteBrowseResponse struct {
	ProductIDs []string            `json:"productIds"`
	Total      int64               `json:"total"`
	Facets     []remoteBrowseFacet `json:"facets"`
}

func (r *remoteSearchSource) Browse(ctx context.Context, channelID uuid.UUID, locale, query string, filter BrowseFilter, sortSlug string, limit, offset int) (BrowseResult, bool) {
	p, ok := r.provider(ctx, channelID)
	if !ok {
		return BrowseResult{}, false
	}

	req := remoteBrowseRequest{
		ChannelID: channelID.String(), Locale: locale, Query: query, Sort: sortSlug,
		Limit: limit, Offset: offset, Filter: browseFilterToWire(filter),
	}
	var resp remoteBrowseResponse
	if err := r.invoker.Invoke(ctx, p, req, &resp); err != nil {
		log.Printf("[search] remote browse %q failed (OpenSearch fallback): %v", p.Slug, err)
		return BrowseResult{}, false
	}
	out := BrowseResult{Total: resp.Total}
	for _, idStr := range resp.ProductIDs {
		if id, perr := uuid.Parse(idStr); perr == nil {
			out.IDs = append(out.IDs, id)
		}
	}
	for _, f := range resp.Facets {
		fg := FacetGroup{Field: f.Field, Slug: f.Slug}
		for _, b := range f.Buckets {
			fg.Buckets = append(fg.Buckets, FacetBucket{
				Value: b.Value, Slug: b.Slug, Count: b.Count, IsIndexable: b.IsIndexable,
			})
		}
		out.Facets = append(out.Facets, fg)
	}
	return out, true
}

func browseFilterToWire(f BrowseFilter) remoteBrowseFilter {
	out := remoteBrowseFilter{
		PriceMinCents: f.PriceMinCents, PriceMaxCents: f.PriceMaxCents, Kinds: f.Kinds,
	}
	if f.CategoryID != nil {
		out.CategoryID = f.CategoryID.String()
	}
	if f.CollectionID != nil {
		out.CollectionID = f.CollectionID.String()
	}
	for _, a := range f.AttributeIn {
		out.AttributeIn = append(out.AttributeIn, remoteBrowseAttribute(a)) // same fields, differ only in JSON tags
	}
	return out
}

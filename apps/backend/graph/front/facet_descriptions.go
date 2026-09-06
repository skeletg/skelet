package front

// Locale- and channel-aware dressing for browse facets, loaded Go-side after the OpenSearch query so the indexer never has to know about translations or channel URL vocabularies
// One ListAttributes call (render-cache memo, shared with the per-product localizer and the filter-slug translation below) plus three batch lookups - attribute content, value content, channel value-slug overrides - so a browse request costs O(1) extra round trips regardless of bucket count

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/search"
)

// Keyed by the visible strings the resolver already holds (attribute slug, bucket display value) so no further uuid plumbing is needed
type facetDescriptionMaps struct {
	attrDescBySlug            map[string]string            // [attrSlug] -> facet panel header description in the active locale
	attrNameBySlug            map[string]string            // [attrSlug] -> localized panel header name; missing or empty keeps the global Attribute.Name
	valueDescByAttrAndDisplay map[string]map[string]string // [attrSlug][displayValue] -> bucket description; scalar attrs contribute nothing, their buckets have no AttributeValue row to translate
	valueNameByAttrAndDisplay map[string]map[string]string // [attrSlug][displayValue] -> localized name for one selectable bucket
	valueSlugByAttrAndDisplay map[string]map[string]string // [attrSlug][displayValue] -> channel override of the bucket's filter-URL slug; missing keeps the global AttributeValue.Slug the search layer resolved
}

func (r *Resolver) loadFacetDescriptions(
	ctx context.Context,
	locale string,
	channelID *uuid.UUID,
	facets []search.FacetGroup,
) facetDescriptionMaps {
	out := facetDescriptionMaps{
		attrDescBySlug:            map[string]string{},
		attrNameBySlug:            map[string]string{},
		valueDescByAttrAndDisplay: map[string]map[string]string{},
		valueNameByAttrAndDisplay: map[string]map[string]string{},
		valueSlugByAttrAndDisplay: map[string]map[string]string{},
	}

	attrs, err := r.getAttributesCached(ctx)
	if err != nil {
		return out
	}

	// Narrowed to the attributes actually in the response, so the batch queries only fetch what gets rendered
	wantedAttrSlugs := map[string]struct{}{}
	for _, fg := range facets {
		wantedAttrSlugs[fg.Slug] = struct{}{}
	}

	var attrIDs []uuid.UUID
	idToSlug := map[uuid.UUID]string{}
	// Per in-response selectable attribute, the value ids matching a bucket's display string - scalars carry no AttributeValue and contribute none
	var valueIDs []uuid.UUID
	valueIDToAttrAndDisplay := map[uuid.UUID]struct {
		attrSlug string
		display  string
	}{}
	for _, a := range attrs {
		if _, ok := wantedAttrSlugs[a.Slug]; !ok {
			continue
		}
		attrIDs = append(attrIDs, a.ID)
		idToSlug[a.ID] = a.Slug
		if !a.InputType.AcceptsValues() {
			continue
		}
		nameToID := map[string]uuid.UUID{}
		for _, v := range a.Values {
			nameToID[v.Name] = v.ID
		}
		for _, fg := range facets {
			if fg.Slug != a.Slug {
				continue
			}
			for _, b := range fg.Buckets {
				vid, ok := nameToID[b.Value]
				if !ok {
					continue
				}
				valueIDs = append(valueIDs, vid)
				valueIDToAttrAndDisplay[vid] = struct {
					attrSlug string
					display  string
				}{attrSlug: a.Slug, display: b.Value}
			}
			break
		}
	}

	if len(attrIDs) > 0 {
		rows, err := r.CatalogSvc.ListAttributeContentByAttributeIDs(ctx, attrIDs)
		if err == nil {
			for _, r := range rows {
				if r.Locale != locale {
					continue
				}
				slug, ok := idToSlug[r.AttributeID]
				if !ok {
					continue
				}
				out.attrDescBySlug[slug] = r.Description
				if r.Name != "" {
					out.attrNameBySlug[slug] = r.Name
				}
			}
		}
	}
	if len(valueIDs) > 0 {
		rows, err := r.CatalogSvc.ListAttributeValueContentByValueIDs(ctx, valueIDs)
		if err == nil {
			for _, r := range rows {
				if r.Locale != locale {
					continue
				}
				ref, ok := valueIDToAttrAndDisplay[r.ValueID]
				if !ok {
					continue
				}
				if out.valueDescByAttrAndDisplay[ref.attrSlug] == nil {
					out.valueDescByAttrAndDisplay[ref.attrSlug] = map[string]string{}
				}
				out.valueDescByAttrAndDisplay[ref.attrSlug][ref.display] = r.Description
				if r.Name != "" {
					if out.valueNameByAttrAndDisplay[ref.attrSlug] == nil {
						out.valueNameByAttrAndDisplay[ref.attrSlug] = map[string]string{}
					}
					out.valueNameByAttrAndDisplay[ref.attrSlug][ref.display] = r.Name
				}
			}
		}
		// Bucket slugs give way where this channel pins its own for a value
		if channelID != nil {
			rows, err := r.CatalogSvc.ListAttributeValueChannelSlugsByValueIDs(ctx, valueIDs)
			if err == nil {
				for _, r := range rows {
					if r.ChannelID != *channelID {
						continue
					}
					ref, ok := valueIDToAttrAndDisplay[r.ValueID]
					if !ok {
						continue
					}
					if out.valueSlugByAttrAndDisplay[ref.attrSlug] == nil {
						out.valueSlugByAttrAndDisplay[ref.attrSlug] = map[string]string{}
					}
					out.valueSlugByAttrAndDisplay[ref.attrSlug][ref.display] = r.Slug
				}
			}
		}
	}
	return out
}

// Maps a channel's own URL vocabulary back to the global AttributeValue slugs the search layer understands
// Anything that isn't a channel override passes through untouched - it's already a global slug, a scalar attrslug_ key that the per-channel index docs carry channel-resolved, or a display name
func (r *Resolver) resolveChannelFilterSlugs(
	ctx context.Context,
	channelID uuid.UUID,
	filters []search.AttributeIn,
) []search.AttributeIn {
	overrides, err := r.CatalogSvc.ListAttributeValueChannelSlugsForChannel(ctx, channelID)
	if err != nil || len(overrides) == 0 {
		return filters
	}
	attrs, err := r.getAttributesCached(ctx)
	if err != nil {
		return filters
	}
	globalSlugByValueID := map[uuid.UUID]string{}
	attrSlugByAttrID := map[uuid.UUID]string{}
	for _, a := range attrs {
		attrSlugByAttrID[a.ID] = a.Slug
		for _, v := range a.Values {
			globalSlugByValueID[v.ID] = v.Slug
		}
	}
	channelToGlobal := map[string]map[string]string{} // [attrSlug][channelSlug] -> globalSlug
	for _, o := range overrides {
		attrSlug, ok := attrSlugByAttrID[o.AttributeID]
		if !ok {
			continue
		}
		global, ok := globalSlugByValueID[o.ValueID]
		if !ok {
			continue
		}
		if channelToGlobal[attrSlug] == nil {
			channelToGlobal[attrSlug] = map[string]string{}
		}
		channelToGlobal[attrSlug][o.Slug] = global
	}
	if len(channelToGlobal) == 0 {
		return filters
	}
	out := make([]search.AttributeIn, len(filters))
	for i, f := range filters {
		mapped := f
		if m, ok := channelToGlobal[f.Slug]; ok {
			vals := make([]string, len(f.Values))
			for j, v := range f.Values {
				if g, ok := m[v]; ok {
					vals[j] = g
				} else {
					vals[j] = v
				}
			}
			mapped.Values = vals
		}
		out[i] = mapped
	}
	return out
}

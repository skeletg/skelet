package search

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/config"
)

// Falls back on a nil config service or a read error - search should never wedge because a config row went missing
func cfgIntOr(ctx context.Context, cfg *config.Service, path string, fallback int64) int64 {
	if cfg == nil {
		return fallback
	}
	v, err := cfg.GetInt(ctx, path, config.DefaultScope)
	if err != nil {
		return fallback
	}
	return v
}

func cfgStringOr(ctx context.Context, cfg *config.Service, path string, fallback string) string {
	if cfg == nil {
		return fallback
	}
	v, err := cfg.GetString(ctx, path, config.DefaultScope)
	if err != nil {
		return fallback
	}
	return v
}

// Any error returns the fallback, which should be the registry default mirroring the historic hardcoded behavior
func loadAttributes(ctx context.Context, cfg *config.Service, path string, fallback []Attribute) []Attribute {
	if cfg == nil {
		return fallback
	}
	raw, err := cfg.GetString(ctx, path, config.DefaultScope)
	if err != nil || raw == "" {
		return fallback
	}
	var out []Attribute
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return fallback
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

// Assembles the must clause from the per-attribute config
// Homogeneous fast path: when every searchable attribute shares matchType and fuzziness, emit one weighted multi_match
// Mixed path: one should clause per attribute (SKU exact, name fuzzy multi_match, description bool_prefix) with minimum_should_match requiring a hit
// searchable=false attributes drop out entirely - if nothing remains the clause matches everything, so "every attribute disabled" degrades instead of hard-failing the API
func buildSearchQuery(q string, attrs []Attribute) map[string]any {
	active := make([]Attribute, 0, len(attrs))
	for _, a := range attrs {
		if a.Searchable && a.Name != "" {
			active = append(active, a)
		}
	}
	if len(active) == 0 {
		return map[string]any{"match_all": map[string]any{}}
	}

	if isHomogeneous(active) {
		fields := make([]string, len(active))
		for i, a := range active {
			fields[i] = fmt.Sprintf("%s^%d", a.Name, normalizeWeight(a.Weight))
		}
		base := active[0]
		switch base.MatchType {
		case matchBoolPrefix:
			return map[string]any{
				"multi_match": map[string]any{
					"query":  q,
					"fields": fields,
					"type":   "bool_prefix",
				},
			}
		case matchPhrasePrefix:
			// multi_match takes type=phrase_prefix, and OpenSearch ignores fuzziness here
			return map[string]any{
				"multi_match": map[string]any{
					"query":  q,
					"fields": fields,
					"type":   "phrase_prefix",
				},
			}
		case matchExact:
			// All-exact with a shared fuzziness isn't a typical homogeneous case (exact ignores it anyway) - model it as one multi_match without fuzziness to keep the wire shape clean
			return map[string]any{
				"multi_match": map[string]any{
					"query":  q,
					"fields": fields,
				},
			}
		default:
			inner := map[string]any{"query": q, "fields": fields}
			if base.Fuzziness != "" {
				inner["fuzziness"] = base.Fuzziness
			}
			return map[string]any{"multi_match": inner}
		}
	}

	// Boost lives on the individual clause - weight maps onto OpenSearch's boost for match / match_bool_prefix / match_phrase_prefix
	should := make([]any, 0, len(active))
	for _, a := range active {
		should = append(should, buildAttributeClause(q, a))
	}
	return map[string]any{
		"bool": map[string]any{
			"should":               should,
			"minimum_should_match": 1,
		},
	}
}

// Each branch maps a MatchType onto its OpenSearch query type - multi_match to match, bool_prefix to match_bool_prefix, match_phrase_prefix straight through, exact to term
func buildAttributeClause(q string, a Attribute) map[string]any {
	boost := float64(normalizeWeight(a.Weight))
	switch a.MatchType {
	case matchBoolPrefix:
		return map[string]any{
			"match_bool_prefix": map[string]any{
				a.Name: map[string]any{
					"query": q,
					"boost": boost,
				},
			},
		}
	case matchPhrasePrefix:
		return map[string]any{
			"match_phrase_prefix": map[string]any{
				a.Name: map[string]any{
					"query": q,
					"boost": boost,
				},
			},
		}
	case matchExact:
		// One term clause per whitespace token, OR'd - a single term over the whole
		// string meant ANY multi-word query missed every exact field, and synonym
		// expansion appends words to the query, so a synonym hit silently turned SKU
		// search off. case_insensitive spares a reindex; ID fields keep their casing
		// and someone typing "tee-cls-001" should still find "TEE-CLS-001"
		terms := strings.Fields(q)
		if len(terms) <= 1 {
			return map[string]any{
				"term": map[string]any{
					a.Name: map[string]any{
						"value":            q,
						"boost":            boost,
						"case_insensitive": true,
					},
				},
			}
		}
		should := make([]any, 0, len(terms))
		for _, t := range terms {
			should = append(should, map[string]any{
				"term": map[string]any{
					a.Name: map[string]any{"value": t, "case_insensitive": true},
				},
			})
		}
		return map[string]any{
			"bool": map[string]any{
				"should":               should,
				"minimum_should_match": 1,
				"boost":                boost,
			},
		}
	default:
		inner := map[string]any{
			"query": q,
			"boost": boost,
		}
		if a.Fuzziness != "" {
			inner["fuzziness"] = a.Fuzziness
		}
		return map[string]any{
			"match": map[string]any{
				a.Name: inner,
			},
		}
	}
}

// The marker for the fast multi_match path - every attribute sharing a matchType and fuzziness
func isHomogeneous(attrs []Attribute) bool {
	if len(attrs) <= 1 {
		return true
	}
	base := attrs[0]
	for _, a := range attrs[1:] {
		if a.MatchType != base.MatchType {
			return false
		}
		if a.Fuzziness != base.Fuzziness {
			return false
		}
	}
	return true
}

// Guarantees a positive value - weight 0 boosts the field to nothing, which is almost never what an operator meant to type
func normalizeWeight(w int) int {
	if w < 1 {
		return 1
	}
	if w > 100 {
		return 100
	}
	return w
}

// Catalog attributes flagged Searchable get sensible defaults (analyzed match, AUTO fuzziness, weight 1) so they join search the moment they're flagged, before anyone visits the settings page
// Existing rows are preserved verbatim - the JSON config wins for any attribute the operator already tuned
func mergeWithCatalogAttributes(existing []Attribute, catalogAttrs []catalog.Attribute) []Attribute {
	if len(catalogAttrs) == 0 {
		return existing
	}
	have := make(map[string]struct{}, len(existing))
	for _, a := range existing {
		have[a.Name] = struct{}{}
	}
	out := make([]Attribute, len(existing), len(existing)+len(catalogAttrs))
	copy(out, existing)
	for _, ca := range catalogAttrs {
		field := "attr_" + ca.Slug
		if _, ok := have[field]; ok {
			continue
		}
		out = append(out, Attribute{
			Name:       field,
			Searchable: true,
			Weight:     1,
			MatchType:  matchMultiMatch,
			Fuzziness:  "AUTO",
		})
	}
	return out
}

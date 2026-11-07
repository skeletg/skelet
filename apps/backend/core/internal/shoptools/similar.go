package shoptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

const (
	maxSimilarSeeds   = 5
	similarPageSize   = 8
	maxSimilarPage    = 12
	similarOffMeaning = "Finding similar products isn't on for this store - search for what the product is instead, like its type and what it's for."
)

func similarTool() tool {
	return tool{
		def: ai.Tool{
			Name: "find_similar_products",
			Description: "Products like one or more the shopper points to - the store's products closest to them in what they are, keeping to " +
				"what the store says must match, like the same gender or brand. Use it for \"more like this\", \"something similar\" or " +
				"\"like these two but...\". Pass the products' ids, SKUs, links or exact names - no need to search for them first.",
			Schema: schema(map[string]any{
				"ids": map[string]any{"type": "array", "minItems": 1, "maxItems": maxSimilarSeeds, "items": map[string]any{"type": "string"},
					"description": "The products to find more like - ids, SKUs, links or exact names."},
				"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": maxSimilarPage, "description": fmt.Sprintf("How many to find (default %d).", similarPageSize)},
			}, "ids"),
			Effect: ai.Reads, Idempotent: true,
		},
		run: runSimilar,
	}
}

func runSimilar(ctx context.Context, c call) (string, error) {
	if len(c.a.IDs) == 0 || len(c.a.IDs) > maxSimilarSeeds {
		return "", refusal(fmt.Sprintf("Pass 1 to %d products to find more like.", maxSimilarSeeds))
	}
	// products sit far closer to each other than to a question, so search's reach would cut what's plainly alike - the
	// matching attributes and the limit keep it on target
	if _, on := c.reach(ctx); !on {
		return similarOffMeaning, nil
	}
	var seeds []uuid.UUID
	var names []string
	for _, ref := range c.a.IDs {
		hit, err := similarSeed(ctx, c, strings.TrimSpace(ref))
		if err != nil {
			return "", err
		}
		if id, err := uuid.Parse(hit.ID); err == nil {
			seeds, names = append(seeds, id), append(names, hit.Name)
		}
	}
	limit := clamp(c.a.Limit, similarPageSize, maxSimilarPage)
	found, err := c.svc.deps.Meaning.Similar(ctx, c.ch.ID, searchLocales(ctx, c.ch), seeds, limit)
	if err != nil {
		return "", err
	}
	if found.Unembedded {
		return fmt.Sprintf("%s %s too new to the store's search to compare yet - suggest searching for what the shopper likes about it instead.", joinAnd(names), isAre(len(names))), nil
	}
	like := "like " + joinAnd(names)
	if len(found.Matched) > 0 {
		like += fmt.Sprintf(", each with the same %s", strings.ToLower(joinAnd(found.Matched)))
	}
	var ids []string
	for _, h := range found.Hits {
		ids = append(ids, h.ProductID.String())
	}
	if len(ids) == 0 {
		return fmt.Sprintf("Nothing else in the store is close enough to be %s.", like), nil
	}
	var out struct {
		ProductsByIds []productHit `json:"productsByIds"`
	}
	if err := c.gql().Do(ctx, hitsDoc, map[string]any{"ids": ids}, &out); err != nil {
		return "", err
	}
	byID := make(map[string]productHit, len(out.ProductsByIds))
	for _, p := range out.ProductsByIds {
		byID[p.ID] = p
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Products %s, closest first:\n", like)
	listed := 0
	for _, id := range ids {
		if p, ok := byID[id]; ok { // one the storefront doesn't show this shopper stays out
			b.WriteString("- ")
			writeHit(&b, c, p)
			listed++
		}
	}
	if listed == 0 {
		return fmt.Sprintf("Nothing else in the store is close enough to be %s.", like), nil
	}
	return b.String(), nil
}

// similarSeed takes a product's name too, so the model needn't search for it first - a search whose other products it
// then showed alongside, ignoring what the store says must match
func similarSeed(ctx context.Context, c call, ref string) (productHit, error) {
	p, err := findProduct(ctx, c, ref)
	if err == nil || !strings.ContainsAny(ref, " \t") {
		return p.productHit, err
	}
	var out struct {
		BrowseProducts struct {
			Items []productHit `json:"items"`
		} `json:"browseProducts"`
	}
	if serr := c.gql().Do(ctx, searchDoc, map[string]any{"query": ref, "filter": map[string]any{}, "limit": 5, "offset": 0}, &out); serr != nil {
		return productHit{}, serr
	}
	for _, h := range out.BrowseProducts.Items {
		if strings.EqualFold(strings.TrimSpace(h.Name), ref) {
			return h, nil
		}
	}
	return productHit{}, refusal("No product goes by that name - pass the id or SKU search_products gave.")
}

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

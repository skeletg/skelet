package shoptools

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

const (
	maxLinks      = 3
	maxLabelChars = 60
)

// Link is a place on the store a reply sends the shopper to: a narrowed product listing, or a page. The storefront
// builds the address from it with its own filter grammar, so the backend never spells out a storefront URL
type Link struct {
	Label   string
	Path    string // "/search", a category's or collection's own path, or a page's
	Query   string
	Filters map[string][]string
	Sort    string
	Count   *int // products the listing shows; nil for a page
}

var (
	countDoc = document("count", `query AgentCount($query: String, $filter: BrowseProductsFilter, $sort: String) {
  browseProducts(query: $query, filter: $filter, sort: $sort, limit: 1) { totalCount }
}`)
	categoryPathDoc   = document("categoryPath", `query AgentCategoryPath($id: ID!) { category(id: $id) { name urlPath } }`)
	collectionPathDoc = document("collectionPath", `query AgentCollectionPath($id: ID!) { collection(id: $id) { name slug } }`)
)

// countListing is how many products a listing narrowed by k's filters shows
func countListing(ctx context.Context, k call, query, sort string) (int, error) {
	filter, err := searchFilter(k)
	if err != nil {
		return 0, err
	}
	var counted struct {
		BrowseProducts struct {
			TotalCount int `json:"totalCount"`
		} `json:"browseProducts"`
	}
	if err := k.gql().Do(ctx, countDoc, map[string]any{"query": query, "filter": filter, "sort": sort}, &counted); err != nil {
		return 0, err
	}
	return counted.BrowseProducts.TotalCount, nil
}

func linkTools() []ai.Tool {
	search := searchTool().def.Schema["properties"].(map[string]any)
	label := str("A few words for the button, like \"Waterproof jackets under 2kg\".")
	return []ai.Tool{{
		Name: "show_results",
		Description: "Put a button under your message to a list of products narrowed to what the shopper wants: a search for words, or a category or collection, " +
			"with the facet filters and sort search_products offered for them. It says how many products the list shows, and refuses one that shows none.",
		Schema: schema(map[string]any{
			"label": label, "query": search["query"], "category": search["category"], "collection": search["collection"],
			"filters": search["filters"], "sort": search["sort"],
		}, "label"),
		Effect: ai.Reads, Idempotent: true,
	}, {
		Name:        "show_page",
		Description: "Put a button under your message to a page on the store: a category, a collection, or an information page. Pass the link list_categories, search_store_pages or another tool gave.",
		Schema:      schema(map[string]any{"label": label, "path": str("The page's link or path on this store.")}, "label", "path"),
		Effect:      ai.Reads, Idempotent: true,
	}}
}

func (c *conversation) showResults(ctx context.Context, k call) (string, error) {
	a := k.a
	label, query := clip(a.Label, maxLabelChars), strings.TrimSpace(a.Query)
	switch {
	case len(c.links) >= maxLinks:
		return "", refusal(fmt.Sprintf("A reply carries at most %d buttons.", maxLinks))
	case label == "":
		return "", refusal("Give the button a few words of label.")
	case a.PriceMin != nil || a.PriceMax != nil:
		return "", refusal("A listing link can't carry a price range - narrow it with filters or the sort instead.")
	case query == "" && a.Category == "" && a.Collection == "":
		return "", refusal("Pass words to search for, or a category or a collection.")
	case a.Category != "" && a.Collection != "":
		return "", refusal("Pick a category or a collection, not both.")
	case query != "" && (a.Category != "" || a.Collection != ""):
		return "", refusal("A search can't be narrowed to a category or collection - send the words, or the category or collection with filters.")
	}
	if err := checkSort(ctx, k, strings.TrimSpace(a.Sort)); err != nil {
		return "", err
	}
	total, err := countListing(ctx, k, query, a.Sort)
	if err != nil {
		return "", err
	}
	if total == 0 {
		return "", refusal("Nothing on the store matches that - loosen it before sending the shopper there.")
	}
	path := "/search"
	switch {
	case a.Category != "":
		var out struct {
			Category struct {
				URLPath string `json:"urlPath"`
			} `json:"category"`
		}
		if err := k.gql().Do(ctx, categoryPathDoc, map[string]any{"id": a.Category}, &out); err != nil {
			return "", err
		}
		path = "/" + strings.Trim(out.Category.URLPath, "/")
	case a.Collection != "":
		var out struct {
			Collection struct {
				Slug string `json:"slug"`
			} `json:"collection"`
		}
		if err := k.gql().Do(ctx, collectionPathDoc, map[string]any{"id": a.Collection}, &out); err != nil {
			return "", err
		}
		path = "/" + strings.Trim(out.Collection.Slug, "/")
	}
	c.links = append(c.links, Link{Label: label, Path: path, Query: query, Filters: maps.Clone(a.Filters), Sort: strings.TrimSpace(a.Sort), Count: &total})
	return fmt.Sprintf("The shopper sees a button to %d products under your message.", total), nil
}

func (c *conversation) showPage(ctx context.Context, k call) (string, error) {
	label := clip(k.a.Label, maxLabelChars)
	ref := strings.TrimSpace(k.a.Path)
	path, ok := productPath(ref)
	if !ok {
		path = strings.Trim(ref, "/")
	}
	switch {
	case len(c.links) >= maxLinks:
		return "", refusal(fmt.Sprintf("A reply carries at most %d buttons.", maxLinks))
	case label == "":
		return "", refusal("Give the button a few words of label.")
	case path == "":
		return "", refusal("Pass the page's link or path.")
	}
	r, err := resolveRoute(ctx, k, path)
	switch {
	case err != nil:
		return "", err
	case r == nil:
		return "", refusal("That isn't a page on this store - use a link list_categories or search_store_pages gave.")
	case r.Kind == "PRODUCT":
		return "", refusal("That's a product - show it with show_product_details instead.")
	}
	c.links = append(c.links, Link{Label: label, Path: "/" + r.Path})
	return "The shopper sees a button to that page under your message.", nil
}

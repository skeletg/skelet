package shoptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

const (
	searchPageSize   = 10
	maxSearchPage    = 24
	maxSearchOffset  = 1000
	facetsShown      = 8
	bucketsShown     = 8
	variantsShown    = 50
	summaryChars     = 400
	descriptionChars = 1500
)

type productHit struct {
	ID                  string `json:"id"`
	SKU                 string `json:"sku"`
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	URL                 string `json:"url"`
	Price               int64  `json:"price"`
	CompareAtPrice      *int64 `json:"compareAtPrice"`
	DefaultCurrencyCode string `json:"defaultCurrencyCode"`
	InStock             bool   `json:"inStock"`
}

var searchDoc = document("search", `query AgentSearch($query: String, $filter: BrowseProductsFilter, $sort: String, $limit: Int, $offset: Int) {
  browseProducts(query: $query, filter: $filter, sort: $sort, limit: $limit, offset: $offset) {
    totalCount
    items { id sku name kind url price compareAtPrice defaultCurrencyCode inStock }
    facets { slug name buckets { value slug count } }
  }
}`)

func searchTool() tool {
	return tool{
		def: ai.Tool{
			Name: "search_products",
			Description: "Search the store's catalog. Every argument is optional - with none it lists the default ordering. Results carry each product's id and SKU " +
				"for get_product, and facets you can pass back as filters to narrow the search.",
			Schema: schema(map[string]any{
				"query":      str("What the shopper wants, in their words, like \"waterproof hiking jacket\" or \"something warm for winter hikes\" - it finds products close in meaning, not only ones using the words."),
				"category":   str("A category id from list_categories."),
				"collection": str("A collection id."),
				"price_min":  map[string]any{"type": "number", "minimum": 0, "maximum": maxPrice, "description": "Lowest price, in the store's currency (major units, like 49.99)."},
				"price_max":  map[string]any{"type": "number", "minimum": 0, "maximum": maxPrice, "description": "Highest price, in the store's currency."},
				"filters": map[string]any{
					"type":                 "object",
					"description":          "Facet slug to the value slugs to keep, from a previous search's facets - like {\"color\": [\"blue\", \"green\"]}.",
					"maxProperties":        maxFilterFacets,
					"additionalProperties": map[string]any{"type": "array", "maxItems": maxFilterValues, "items": map[string]any{"type": "string"}},
				},
				"sort":   str("A sort slug from get_store_info. Omit for the store's default."),
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": maxSearchPage, "description": fmt.Sprintf("Products per page (default %d).", searchPageSize)},
				"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": maxSearchOffset, "description": "Products to skip, for the next page."},
			}),
			Effect: ai.Reads, Idempotent: true,
		},
		run: runSearch,
	}
}

func searchFilter(c call) (map[string]any, error) {
	a := c.a
	filter := map[string]any{}
	if a.Category != "" {
		if _, err := uuid.Parse(a.Category); err != nil {
			return nil, refusal("category takes a category id from list_categories.")
		}
		filter["categoryId"] = a.Category
	}
	if a.Collection != "" {
		if _, err := uuid.Parse(a.Collection); err != nil {
			return nil, refusal("collection takes a collection id.")
		}
		filter["collectionId"] = a.Collection
	}
	if a.PriceMin != nil {
		filter["priceMinCents"] = toMinor(*a.PriceMin, c.ch.CurrencyCode)
	}
	if a.PriceMax != nil {
		filter["priceMaxCents"] = toMinor(*a.PriceMax, c.ch.CurrencyCode)
	}
	var attrs []map[string]any
	for slug, values := range a.Filters {
		if len(values) > 0 {
			attrs = append(attrs, map[string]any{"slug": slug, "values": values})
		}
	}
	if len(attrs) > 0 {
		filter["attributes"] = attrs
	}
	return filter, nil
}

func runSearch(ctx context.Context, c call) (string, error) {
	filter, err := searchFilter(c)
	if err != nil {
		return "", err
	}
	limit := clamp(c.a.Limit, searchPageSize, maxSearchPage)
	offset := min(max(c.a.Offset, 0), maxSearchOffset)
	var out struct {
		BrowseProducts struct {
			TotalCount int          `json:"totalCount"`
			Items      []productHit `json:"items"`
			Facets     []struct {
				Slug    string `json:"slug"`
				Name    string `json:"name"`
				Buckets []struct {
					Value string `json:"value"`
					Slug  string `json:"slug"`
					Count int    `json:"count"`
				} `json:"buckets"`
			} `json:"facets"`
		} `json:"browseProducts"`
	}
	vars := map[string]any{"query": strings.TrimSpace(c.a.Query), "filter": filter, "sort": c.a.Sort, "limit": limit, "offset": offset}
	if err := c.gql().Do(ctx, searchDoc, vars, &out); err != nil {
		return "", err
	}
	res := out.BrowseProducts
	items, fused := res.Items, false
	// Meaning joins a first page of plain words in the store's own order - with filters or a sort the model asked for exactly that list
	if query := strings.TrimSpace(c.a.Query); query != "" && offset == 0 && c.a.Sort == "" && len(filter) == 0 {
		if near := c.nearProducts(ctx, query, limit); len(near) > 0 {
			if items, err = fuseHits(ctx, c, res.Items, near, limit); err != nil {
				return "", err
			}
			fused = true
		}
	}
	if offset == 0 { // what the words matched - meaning finds something close for nearly anything, so it can't show a gap
		logSearch(ctx, c.a.Query, res.TotalCount)
	}
	if len(items) == 0 {
		if offset > 0 {
			return fmt.Sprintf("No more products - all %d were on earlier pages.", res.TotalCount), nil
		}
		return noProductsFound, nil
	}
	var b strings.Builder
	switch {
	case fused && len(res.Items) == 0:
		b.WriteString("No product uses those words, but these are close in what they mean:\n")
	case fused:
		fmt.Fprintf(&b, "%d products match the words. The best of them and of the products closest in meaning:\n", res.TotalCount)
	default:
		fmt.Fprintf(&b, "%d products match (showing %d-%d):\n", res.TotalCount, offset+1, offset+len(res.Items))
	}
	for _, p := range items {
		b.WriteString("- ")
		writeHit(&b, c, p)
	}
	if offset+len(res.Items) < res.TotalCount {
		fmt.Fprintf(&b, "More with offset %d.\n", offset+len(res.Items))
	}
	header := "Facets to narrow with filters (slug: value slugs with counts):\n"
	shown := 0
	for _, f := range res.Facets {
		if len(f.Buckets) == 0 || shown == facetsShown {
			continue
		}
		vals := make([]string, 0, bucketsShown)
		for _, bk := range f.Buckets[:min(len(f.Buckets), bucketsShown)] {
			vals = append(vals, fmt.Sprintf("%s %q (%d)", bk.Slug, bk.Value, bk.Count))
		}
		b.WriteString(header)
		header = ""
		fmt.Fprintf(&b, "- %s %q: %s\n", f.Slug, f.Name, strings.Join(vals, ", "))
		shown++
	}
	return b.String(), nil
}

// What a search that found nothing answers - the assistant's transcript counts on these to tell an empty search from one
// that found something
const (
	noProductsFound = "No products match that. Try fewer words or filters."
	noPagesFound    = "No store page mentions that. Omit query to see every page."
	noPagesAtAll    = "The store has no pages published."
)

var hitsDoc = document("hits", `query AgentHits($ids: [ID!]!) {
  productsByIds(ids: $ids) { id sku name kind url price compareAtPrice defaultCurrencyCode inStock }
}`)

// fuseHits ranks the words' first page and the nearest products together, reading the near ones the storefront way - one
// it doesn't show this shopper drops out
func fuseHits(ctx context.Context, c call, words []productHit, near []string, limit int) ([]productHit, error) {
	byID := make(map[string]productHit, len(words)+len(near))
	ids := make([]string, len(words))
	for i, p := range words {
		byID[p.ID], ids[i] = p, p.ID
	}
	order := fuse(ids, near, limit)
	var missing []string
	for _, id := range order {
		if _, ok := byID[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		var out struct {
			ProductsByIds []productHit `json:"productsByIds"`
		}
		if err := c.gql().Do(ctx, hitsDoc, map[string]any{"ids": missing}, &out); err != nil {
			return nil, err
		}
		for _, p := range out.ProductsByIds {
			byID[p.ID] = p
		}
	}
	items := make([]productHit, 0, len(order))
	for _, id := range order {
		if p, ok := byID[id]; ok {
			items = append(items, p)
		}
	}
	return items, nil
}

func writeHit(b *strings.Builder, c call, p productHit) {
	cur := currency(p.DefaultCurrencyCode, c)
	fmt.Fprintf(b, "%s [sku %s, id %s] %s", p.Name, p.SKU, p.ID, priceOf(p, cur))
	if p.CompareAtPrice != nil && *p.CompareAtPrice > p.Price {
		fmt.Fprintf(b, " (was %s)", price(*p.CompareAtPrice, cur))
	}
	if !p.InStock {
		b.WriteString(", out of stock")
	}
	fmt.Fprintf(b, " - %s\n", link(c.ch, p.URL))
}

// A bundle or a grouped product is priced by what's picked, so its own price says nothing
func priceOf(p productHit, cur string) string {
	if p.Price == 0 && (p.Kind == "BUNDLE" || p.Kind == "GROUPED") {
		return "priced by what's chosen"
	}
	return price(p.Price, cur)
}

func currency(code string, c call) string {
	if code != "" {
		return code
	}
	return c.ch.CurrencyCode
}

const productFragment = `fragment AgentProduct on Product {
  id sku name kind url price compareAtPrice defaultCurrencyCode inStock backorderable minOrderQty maxOrderQty quantityStep
  shortDescription description
  attributeValues { attribute { slug name unit } selectedValues { name } scalarValue }
  configurableOptions { slug }
  children { sku price inStock attributeValues { attribute { slug name unit } selectedValues { name } scalarValue } }
  groupedMembers { defaultQuantity member { sku name price inStock } }
  options { title isRequired }
  categories { name }
  promotionalDisplays { publicLabel }
  reviewAggregates { count overallAverage }
  reviewSummary
  reviews(limit: 3) { overallRating title body isVerifiedPurchase }
}`

type attrValue struct {
	Attribute struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
		Unit string `json:"unit"`
	} `json:"attribute"`
	SelectedValues []struct {
		Name string `json:"name"`
	} `json:"selectedValues"`
	ScalarValue any `json:"scalarValue"`
}

type product struct {
	productHit
	Backorderable    bool            `json:"backorderable"`
	MinOrderQty      *float64        `json:"minOrderQty"`
	MaxOrderQty      *float64        `json:"maxOrderQty"`
	QuantityStep     float64         `json:"quantityStep"`
	ShortDescription json.RawMessage `json:"shortDescription"`
	Description      json.RawMessage `json:"description"`
	AttributeValues  []attrValue     `json:"attributeValues"`
	ConfigurableOpts []struct {
		Slug string `json:"slug"`
	} `json:"configurableOptions"`
	Children []struct {
		SKU             string      `json:"sku"`
		Price           int64       `json:"price"`
		InStock         bool        `json:"inStock"`
		AttributeValues []attrValue `json:"attributeValues"`
	} `json:"children"`
	GroupedMembers []struct {
		DefaultQuantity float64    `json:"defaultQuantity"`
		Member          productHit `json:"member"`
	} `json:"groupedMembers"`
	Options []struct {
		Title      string `json:"title"`
		IsRequired bool   `json:"isRequired"`
	} `json:"options"`
	Categories []struct {
		Name string `json:"name"`
	} `json:"categories"`
	Promotions []struct {
		PublicLabel string `json:"publicLabel"`
	} `json:"promotionalDisplays"`
	ReviewAggregates struct {
		Count          int      `json:"count"`
		OverallAverage *float64 `json:"overallAverage"`
	} `json:"reviewAggregates"`
	ReviewSummary *string `json:"reviewSummary"`
	Reviews       []struct {
		OverallRating      *float64 `json:"overallRating"`
		Title              string   `json:"title"`
		Body               string   `json:"body"`
		IsVerifiedPurchase bool     `json:"isVerifiedPurchase"`
	} `json:"reviews"`
}

func productTool() tool {
	return tool{
		def: ai.Tool{
			Name: "get_product",
			Description: "Everything about one product: price, stock, description, attributes, its variants or parts, and what reviewers say. " +
				"It also says which SKU goes into the cart with update_cart.",
			Schema: schema(map[string]any{
				"product": str("A product id or SKU from search_products, or a link to the product on this store."),
			}, "product"),
			Effect: ai.Reads, Idempotent: true,
		},
		run: runProduct,
	}
}

func runProduct(ctx context.Context, c call) (string, error) {
	p, err := findProduct(ctx, c, strings.TrimSpace(c.a.Product))
	if err != nil {
		return "", err
	}
	return describeProduct(c, p), nil
}

func describeProduct(c call, p product) string {
	cur := currency(p.DefaultCurrencyCode, c)
	var b strings.Builder
	fmt.Fprintf(&b, "%s - %s\nid %s, sku %s\n", p.Name, link(c.ch, p.URL), p.ID, p.SKU)
	fmt.Fprintf(&b, "Price %s", priceOf(p.productHit, cur))
	if p.CompareAtPrice != nil && *p.CompareAtPrice > p.Price {
		fmt.Fprintf(&b, " (was %s)", price(*p.CompareAtPrice, cur))
	}
	switch {
	case p.InStock:
		b.WriteString(", in stock\n")
	case p.Backorderable:
		b.WriteString(", out of stock but can be backordered\n")
	default:
		b.WriteString(", out of stock\n")
	}
	writeQuantityRules(&b, p)
	var promos []string
	for _, pr := range p.Promotions {
		if label := strings.TrimSpace(pr.PublicLabel); label != "" {
			promos = append(promos, label)
		}
	}
	if len(promos) > 0 {
		fmt.Fprintf(&b, "Promotions on it: %s (store-wide conditions, like a minimum spend, are settled in the cart)\n", strings.Join(promos, "; "))
	}
	if len(p.Categories) > 0 {
		names := make([]string, len(p.Categories))
		for i, cat := range p.Categories {
			names[i] = cat.Name
		}
		fmt.Fprintf(&b, "In: %s\n", strings.Join(names, ", "))
	}
	if s := plainText(p.ShortDescription, summaryChars); s != "" {
		fmt.Fprintf(&b, "Summary: %s\n", s)
	}
	if s := plainText(p.Description, descriptionChars); s != "" {
		fmt.Fprintf(&b, "Description: %s\n", s)
	}
	if attrs := attributeLine(p.AttributeValues, nil); attrs != "" {
		fmt.Fprintf(&b, "Details: %s\n", attrs)
	}
	writeBuyable(&b, c, p, cur)
	writeReviews(&b, p)
	return b.String()
}

func writeQuantityRules(b *strings.Builder, p product) {
	stepped := p.QuantityStep > 0 && p.QuantityStep != 1
	if p.MinOrderQty == nil && p.MaxOrderQty == nil && !stepped {
		return
	}
	b.WriteString("Order quantity:")
	if p.MinOrderQty != nil {
		fmt.Fprintf(b, " at least %g", *p.MinOrderQty)
	}
	if p.MaxOrderQty != nil {
		fmt.Fprintf(b, " at most %g", *p.MaxOrderQty)
	}
	if stepped {
		fmt.Fprintf(b, " in steps of %g", p.QuantityStep)
	}
	b.WriteString("\n")
}

func writeBuyable(b *strings.Builder, c call, p product, cur string) {
	var required []string
	for _, o := range p.Options {
		if o.IsRequired {
			required = append(required, o.Title)
		}
	}
	page := link(c.ch, p.URL)
	switch {
	case p.Kind == "CONFIGURABLE" && len(p.Children) > 0:
		keep := map[string]bool{}
		for _, o := range p.ConfigurableOpts {
			keep[o.Slug] = true
		}
		b.WriteString("Variants - put one of these SKUs in the cart, not the parent's:\n")
		for _, ch := range p.Children[:min(len(p.Children), variantsShown)] {
			stock := "in stock"
			if !ch.InStock {
				stock = "out of stock"
			}
			fmt.Fprintf(b, "- %s: %s - %s, %s\n", ch.SKU, attributeLine(ch.AttributeValues, keep), price(ch.Price, cur), stock)
		}
		if more := len(p.Children) - variantsShown; more > 0 {
			fmt.Fprintf(b, "And %d more on the product page: %s\n", more, page)
		}
	case p.Kind == "GROUPED" && len(p.GroupedMembers) > 0:
		b.WriteString("Sold as separate items - put the SKUs the shopper wants in the cart:\n")
		for _, m := range p.GroupedMembers {
			fmt.Fprintf(b, "- %s %s - %s (suggested quantity %g)\n", m.Member.SKU, m.Member.Name, price(m.Member.Price, cur), m.DefaultQuantity)
		}
	case p.Kind == "BUNDLE", p.Kind == "GIFTCARD", p.Kind == "SUBSCRIPTION", p.Kind == "CONFIGURABLE":
		fmt.Fprintf(b, "Can't go in the cart from here - the shopper makes their choices on the product page: %s\n", page)
	case len(required) > 0:
		fmt.Fprintf(b, "Needs %s chosen on the product page before it can go in the cart: %s\n", strings.Join(required, ", "), page)
	default:
		fmt.Fprintf(b, "Put it in the cart with sku %s.\n", p.SKU)
	}
}

func writeReviews(b *strings.Builder, p product) {
	agg := p.ReviewAggregates
	if agg.Count == 0 {
		return
	}
	if agg.OverallAverage != nil {
		fmt.Fprintf(b, "Reviews: %.1f out of 5 from %d\n", *agg.OverallAverage, agg.Count)
	} else {
		fmt.Fprintf(b, "Reviews: %d\n", agg.Count)
	}
	if p.ReviewSummary != nil && *p.ReviewSummary != "" {
		fmt.Fprintf(b, "What reviewers say: %s\n", quote(*p.ReviewSummary))
	}
	for _, r := range p.Reviews {
		b.WriteString("- ")
		if r.OverallRating != nil {
			fmt.Fprintf(b, "%.0f/5 ", *r.OverallRating)
		}
		if r.IsVerifiedPurchase {
			b.WriteString("(verified purchase) ")
		}
		fmt.Fprintf(b, "%s %s\n", quote(r.Title), quote(r.Body))
	}
}

func attributeLine(values []attrValue, only map[string]bool) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if only != nil && !only[v.Attribute.Slug] {
			continue
		}
		val := attributeText(v)
		if val == "" {
			continue
		}
		if v.Attribute.Unit != "" {
			val += " " + v.Attribute.Unit
		}
		if v.Attribute.Name != "" {
			val = v.Attribute.Name + ": " + val
		}
		parts = append(parts, val)
	}
	return strings.Join(parts, "; ")
}

func attributeText(v attrValue) string {
	if len(v.SelectedValues) > 0 {
		names := make([]string, len(v.SelectedValues))
		for i, sv := range v.SelectedValues {
			names[i] = sv.Name
		}
		return strings.Join(names, ", ")
	}
	switch s := v.ScalarValue.(type) {
	case string:
		return s
	case float64:
		return fmt.Sprintf("%g", s)
	case bool:
		if s {
			return "yes"
		}
		return "no"
	}
	return ""
}

var (
	productByIDDoc  = document("productByID", productFragment+"\nquery AgentProduct($id: ID!) { product(id: $id) { ...AgentProduct } }")
	productBySKUDoc = document("productBySKU", productFragment+"\nquery AgentProductBySku($skus: [String!]!) { productsBySkus(skus: $skus) { ...AgentProduct } }")
	routeDoc        = document("route", `query AgentRoute($path: String!) { route(path: $path) { kind entityId redirectTo } }`)
)

var notFound = refusal("There's no product like that on this store - search_products gives ids and SKUs that work here.")

func findProduct(ctx context.Context, c call, ref string) (product, error) {
	if ref == "" {
		return product{}, refusal("Pass a product id, SKU or link.")
	}
	if _, err := uuid.Parse(ref); err == nil {
		return productByID(ctx, c, ref)
	}
	if path, ok := productPath(ref); ok {
		id, err := routeToProduct(ctx, c, path)
		if err != nil {
			return product{}, err
		}
		if id != "" {
			return productByID(ctx, c, id)
		}
	}
	var out struct {
		ProductsBySkus []product `json:"productsBySkus"`
	}
	if err := c.gql().Do(ctx, productBySKUDoc, map[string]any{"skus": []string{ref}}, &out); err != nil {
		return product{}, err
	}
	if len(out.ProductsBySkus) == 0 {
		// A name isn't a SKU, and saying the product doesn't exist sent the model off telling shoppers the store doesn't carry it
		if strings.ContainsAny(ref, " \t") {
			return product{}, refusal("That looks like a name - get_product takes an id, SKU or link. Find the product with search_products first.")
		}
		return product{}, notFound
	}
	return out.ProductsBySkus[0], nil
}

func productByID(ctx context.Context, c call, id string) (product, error) {
	var out struct {
		Product *product `json:"product"`
	}
	if err := c.gql().Do(ctx, productByIDDoc, map[string]any{"id": id}, &out); err != nil {
		if gql, ok := errors.AsType[*Error](err); ok && (gql.Code == "NOT_FOUND" || gql.Code == "PRODUCT_NOT_FOUND") {
			return product{}, notFound
		}
		return product{}, err
	}
	if out.Product == nil {
		return product{}, notFound
	}
	return *out.Product, nil
}

// A link or path on this store, as the route resolver takes it - a bare word is a SKU, not a path
func productPath(ref string) (string, bool) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		u, err := url.Parse(ref)
		if err != nil {
			return "", false
		}
		ref = u.Path
	} else if !strings.Contains(ref, "/") {
		return "", false
	}
	path := strings.Trim(ref, "/")
	return path, path != ""
}

// A BCP 47 tag as the storefront prefixes paths with - "de", "en-GB", "pt-BR"
var localeSegment = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)

type storeRoute struct {
	Kind     string
	EntityID string
	Path     string // where it was found, a redirect followed
}

// resolveRoute finds what a store path shows: the path as given, then without a leading locale segment, following one redirect - nil when it's nothing
func resolveRoute(ctx context.Context, c call, path string) (*storeRoute, error) {
	candidates := []string{path}
	if first, rest, ok := strings.Cut(path, "/"); ok && localeSegment.MatchString(first) {
		candidates = append(candidates, rest)
	}
	for _, p := range candidates {
		for range 2 {
			var out struct {
				Route *struct {
					Kind       string  `json:"kind"`
					EntityID   string  `json:"entityId"`
					RedirectTo *string `json:"redirectTo"`
				} `json:"route"`
			}
			if err := c.gql().Do(ctx, routeDoc, map[string]any{"path": p}, &out); err != nil {
				return nil, err
			}
			r := out.Route
			if r == nil {
				break
			}
			if r.RedirectTo != nil && *r.RedirectTo != "" {
				p = strings.Trim(*r.RedirectTo, "/")
				continue
			}
			return &storeRoute{Kind: r.Kind, EntityID: r.EntityID, Path: p}, nil
		}
	}
	return nil, nil
}

func routeToProduct(ctx context.Context, c call, path string) (string, error) {
	r, err := resolveRoute(ctx, c, path)
	if err != nil || r == nil {
		return "", err
	}
	if r.Kind != "PRODUCT" {
		return "", refusal(fmt.Sprintf("That link is a %s page, not a product.", strings.ToLower(strings.ReplaceAll(r.Kind, "_", " "))))
	}
	return r.EntityID, nil
}

type category struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URLPath  string `json:"urlPath"`
	Children []struct {
		ID string `json:"id"`
	} `json:"children"`
}

var (
	rootCategoriesDoc   = document("rootCategories", `query AgentRootCategories { rootCategories { id name urlPath children { id } } }`)
	categoryChildrenDoc = document("categoryChildren", `query AgentCategoryChildren($id: ID!) { categoryChildren(parentId: $id) { id name urlPath children { id } } }`)
)

func categoriesTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "list_categories",
			Description: "The store's categories, top level first. Pass parent to see one category's subcategories. Category ids narrow search_products.",
			Schema: schema(map[string]any{
				"parent": str("A category id whose subcategories to list. Omit for the top level."),
			}),
			Effect: ai.Reads, Idempotent: true,
		},
		run: runCategories,
	}
}

func runCategories(ctx context.Context, c call) (string, error) {
	var cats []category
	if parent := strings.TrimSpace(c.a.Parent); parent != "" {
		if _, err := uuid.Parse(parent); err != nil {
			return "", refusal("parent takes a category id from list_categories.")
		}
		var out struct {
			CategoryChildren []category `json:"categoryChildren"`
		}
		if err := c.gql().Do(ctx, categoryChildrenDoc, map[string]any{"id": parent}, &out); err != nil {
			return "", err
		}
		cats = out.CategoryChildren
	} else {
		var out struct {
			RootCategories []category `json:"rootCategories"`
		}
		if err := c.gql().Do(ctx, rootCategoriesDoc, nil, &out); err != nil {
			return "", err
		}
		cats = out.RootCategories
	}
	if len(cats) == 0 {
		return "No categories there.", nil
	}
	var b strings.Builder
	b.WriteString("Categories:\n")
	for _, cat := range cats {
		fmt.Fprintf(&b, "- %s [id %s] %s", cat.Name, cat.ID, link(c.ch, cat.URLPath))
		if n := len(cat.Children); n > 0 {
			fmt.Fprintf(&b, " (%d subcategories)", n)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

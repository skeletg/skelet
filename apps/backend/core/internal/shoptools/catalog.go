package shoptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
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

const searchDoc = `query AgentSearch($query: String, $filter: BrowseProductsFilter, $sort: String, $limit: Int, $offset: Int) {
  browseProducts(query: $query, filter: $filter, sort: $sort, limit: $limit, offset: $offset) {
    totalCount
    items { id sku name kind url price compareAtPrice defaultCurrencyCode inStock }
    facets { slug name buckets { value slug count } }
  }
}`

func searchTool() tool {
	return tool{
		def: ai.Tool{
			Name: "search_products",
			Description: "Search the store's catalog. Every argument is optional - with none it lists the default ordering. Results carry each product's id and SKU " +
				"for get_product, and facets you can pass back as filters to narrow the search.",
			Schema: schema(map[string]any{
				"query":      str("Words to search for, like \"waterproof hiking jacket\"."),
				"category":   str("A category id from list_categories."),
				"collection": str("A collection id."),
				"price_min":  map[string]any{"type": "number", "minimum": 0, "description": "Lowest price, in the store's currency (major units, like 49.99)."},
				"price_max":  map[string]any{"type": "number", "minimum": 0, "description": "Highest price, in the store's currency."},
				"filters": map[string]any{
					"type":                 "object",
					"description":          "Facet slug to the value slugs to keep, from a previous search's facets - like {\"color\": [\"blue\", \"green\"]}.",
					"additionalProperties": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"sort":   str("A sort slug from get_store_info. Omit for the store's default."),
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 24, "description": "Products per page (default 10)."},
				"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 1000, "description": "Products to skip, for the next page."},
			}),
			ReadOnly: true, Idempotent: true,
		},
		run: runSearch,
	}
}

func runSearch(ctx context.Context, c call) (string, error) {
	a := c.a
	filter := map[string]any{}
	if a.Category != "" {
		if _, err := uuid.Parse(a.Category); err != nil {
			return "", refusal("category takes a category id from list_categories.")
		}
		filter["categoryId"] = a.Category
	}
	if a.Collection != "" {
		if _, err := uuid.Parse(a.Collection); err != nil {
			return "", refusal("collection takes a collection id.")
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
	limit := clamp(a.Limit, 10, 24)
	offset := min(max(a.Offset, 0), 1000)
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
	vars := map[string]any{"query": strings.TrimSpace(a.Query), "filter": filter, "sort": a.Sort, "limit": limit, "offset": offset}
	if err := c.deps.GraphQL.Do(ctx, searchDoc, vars, &out); err != nil {
		return "", err
	}
	res := out.BrowseProducts
	if len(res.Items) == 0 {
		if offset > 0 {
			return fmt.Sprintf("No more products - all %d were on earlier pages.", res.TotalCount), nil
		}
		return "No products match that. Try fewer words or filters.", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d products match (showing %d-%d):\n", res.TotalCount, offset+1, offset+len(res.Items))
	for _, p := range res.Items {
		b.WriteString("- ")
		writeHit(&b, c, p)
	}
	if offset+len(res.Items) < res.TotalCount {
		fmt.Fprintf(&b, "More with offset %d.\n", offset+len(res.Items))
	}
	header := "Facets to narrow with filters (slug: value slugs with counts):\n"
	shown := 0
	for _, f := range res.Facets {
		if len(f.Buckets) == 0 || shown == 8 {
			continue
		}
		vals := make([]string, 0, 8)
		for _, bk := range f.Buckets[:min(len(f.Buckets), 8)] {
			vals = append(vals, fmt.Sprintf("%s %q (%d)", bk.Slug, bk.Value, bk.Count))
		}
		b.WriteString(header)
		header = ""
		fmt.Fprintf(&b, "- %s %q: %s\n", f.Slug, f.Name, strings.Join(vals, ", "))
		shown++
	}
	return b.String(), nil
}

func writeHit(b *strings.Builder, c call, p productHit) {
	fmt.Fprintf(b, "%s [sku %s, id %s] %s", p.Name, p.SKU, p.ID, priceOf(p, currency(p.DefaultCurrencyCode, c)))
	if p.CompareAtPrice != nil && *p.CompareAtPrice > p.Price {
		fmt.Fprintf(b, " (was %s)", price(*p.CompareAtPrice, currency(p.DefaultCurrencyCode, c)))
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
  configurableOptions { slug name }
  children { sku name price inStock attributeValues { attribute { slug } selectedValues { name } } }
  groupedMembers { defaultQuantity member { sku name price inStock } }
  options { title isRequired }
  categories { name }
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
	Backorderable    bool                          `json:"backorderable"`
	MinOrderQty      *float64                      `json:"minOrderQty"`
	MaxOrderQty      *float64                      `json:"maxOrderQty"`
	QuantityStep     float64                       `json:"quantityStep"`
	ShortDescription json.RawMessage               `json:"shortDescription"`
	Description      json.RawMessage               `json:"description"`
	AttributeValues  []attrValue                   `json:"attributeValues"`
	ConfigurableOpts []struct{ Slug, Name string } `json:"configurableOptions"`
	Children         []struct {
		SKU             string      `json:"sku"`
		Name            string      `json:"name"`
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
			ReadOnly: true, Idempotent: true,
		},
		run: runProduct,
	}
}

func runProduct(ctx context.Context, c call) (string, error) {
	p, err := findProduct(ctx, c, strings.TrimSpace(c.a.Product))
	if err != nil {
		return "", err
	}
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
	if p.MinOrderQty != nil || p.MaxOrderQty != nil || (p.QuantityStep > 0 && p.QuantityStep != 1) {
		b.WriteString("Order quantity:")
		if p.MinOrderQty != nil {
			fmt.Fprintf(&b, " at least %g", *p.MinOrderQty)
		}
		if p.MaxOrderQty != nil {
			fmt.Fprintf(&b, " at most %g", *p.MaxOrderQty)
		}
		if p.QuantityStep > 0 && p.QuantityStep != 1 {
			fmt.Fprintf(&b, " in steps of %g", p.QuantityStep)
		}
		b.WriteString("\n")
	}
	if len(p.Categories) > 0 {
		names := make([]string, len(p.Categories))
		for i, cat := range p.Categories {
			names[i] = cat.Name
		}
		fmt.Fprintf(&b, "In: %s\n", strings.Join(names, ", "))
	}
	if s := plainText(p.ShortDescription, 400); s != "" {
		fmt.Fprintf(&b, "Summary: %s\n", s)
	}
	if s := plainText(p.Description, 1500); s != "" {
		fmt.Fprintf(&b, "Description: %s\n", s)
	}
	if attrs := attributeLine(p.AttributeValues, nil); attrs != "" {
		fmt.Fprintf(&b, "Details: %s\n", attrs)
	}
	writeBuyable(&b, c, p, cur)
	writeReviews(&b, p)
	return b.String(), nil
}

func writeBuyable(b *strings.Builder, c call, p product, cur string) {
	var required []string
	for _, o := range p.Options {
		if o.IsRequired {
			required = append(required, o.Title)
		}
	}
	switch {
	case p.Kind == "CONFIGURABLE" && len(p.Children) > 0:
		keep := map[string]bool{}
		for _, o := range p.ConfigurableOpts {
			keep[o.Slug] = true
		}
		b.WriteString("Variants - put one of these SKUs in the cart, not the parent's:\n")
		for _, ch := range p.Children {
			stock := "in stock"
			if !ch.InStock {
				stock = "out of stock"
			}
			fmt.Fprintf(b, "- %s: %s - %s, %s\n", ch.SKU, attributeLine(ch.AttributeValues, keep), price(ch.Price, cur), stock)
		}
	case p.Kind == "GROUPED" && len(p.GroupedMembers) > 0:
		b.WriteString("Sold as separate items - put the SKUs the shopper wants in the cart:\n")
		for _, m := range p.GroupedMembers {
			fmt.Fprintf(b, "- %s %s - %s (suggested quantity %g)\n", m.Member.SKU, m.Member.Name, price(m.Member.Price, cur), m.DefaultQuantity)
		}
	case p.Kind == "BUNDLE", p.Kind == "GIFTCARD", p.Kind == "SUBSCRIPTION", p.Kind == "CONFIGURABLE":
		fmt.Fprintf(b, "Can't go in the cart from here - the shopper makes their choices on the product page: %s\n", link(c.ch, p.URL))
	case len(required) > 0:
		fmt.Fprintf(b, "Needs %s chosen on the product page before it can go in the cart: %s\n", strings.Join(required, ", "), link(c.ch, p.URL))
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
		var val string
		if len(v.SelectedValues) > 0 {
			names := make([]string, len(v.SelectedValues))
			for i, sv := range v.SelectedValues {
				names[i] = sv.Name
			}
			val = strings.Join(names, ", ")
		} else if v.ScalarValue != nil {
			switch s := v.ScalarValue.(type) {
			case string:
				val = s
			case bool:
				val = map[bool]string{true: "yes", false: "no"}[s]
			case float64:
				val = fmt.Sprintf("%g", s)
			}
		}
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

const productByIDDoc = productFragment + `
query AgentProduct($id: ID!) { product(id: $id) { ...AgentProduct } }`

const productBySKUDoc = productFragment + `
query AgentProductBySku($skus: [String!]!) { productsBySkus(skus: $skus) { ...AgentProduct } }`

const routeDoc = `query AgentRoute($path: String!) { route(path: $path) { kind entityId redirectTo } }`

var notFound = refusal("There's no product like that on this store - search_products gives ids and SKUs that work here.")

func findProduct(ctx context.Context, c call, ref string) (product, error) {
	if ref == "" {
		return product{}, refusal("Pass a product id, SKU or link.")
	}
	if _, err := uuid.Parse(ref); err == nil {
		return productByID(ctx, c, ref)
	}
	if path, ok := productPath(ref); ok {
		if id, err := routeToProduct(ctx, c, path); err != nil || id != "" {
			if err != nil {
				return product{}, err
			}
			return productByID(ctx, c, id)
		}
	}
	var out struct {
		ProductsBySkus []product `json:"productsBySkus"`
	}
	if err := c.deps.GraphQL.Do(ctx, productBySKUDoc, map[string]any{"skus": []string{ref}}, &out); err != nil {
		return product{}, err
	}
	if len(out.ProductsBySkus) == 0 {
		return product{}, notFound
	}
	return out.ProductsBySkus[0], nil
}

func productByID(ctx context.Context, c call, id string) (product, error) {
	var out struct {
		Product *product `json:"product"`
	}
	if err := c.deps.GraphQL.Do(ctx, productByIDDoc, map[string]any{"id": id}, &out); err != nil {
		var gql *Error
		if errors.As(err, &gql) && (gql.Code == "NOT_FOUND" || gql.Code == "PRODUCT_NOT_FOUND") {
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

// The path as given, then without a leading locale segment, following one redirect
func routeToProduct(ctx context.Context, c call, path string) (string, error) {
	candidates := []string{path}
	if first, rest, ok := strings.Cut(path, "/"); ok && len(first) <= 5 {
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
			if err := c.deps.GraphQL.Do(ctx, routeDoc, map[string]any{"path": p}, &out); err != nil {
				return "", err
			}
			r := out.Route
			if r == nil {
				break
			}
			if r.RedirectTo != nil && *r.RedirectTo != "" {
				p = strings.Trim(*r.RedirectTo, "/")
				continue
			}
			if r.Kind != "PRODUCT" {
				return "", refusal(fmt.Sprintf("That link is a %s page, not a product.", strings.ToLower(strings.ReplaceAll(r.Kind, "_", " "))))
			}
			return r.EntityID, nil
		}
	}
	return "", nil
}

type category struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URLPath  string `json:"urlPath"`
	Children []struct {
		ID string `json:"id"`
	} `json:"children"`
}

const rootCategoriesDoc = `query AgentRootCategories { rootCategories { id name urlPath children { id } } }`
const categoryChildrenDoc = `query AgentCategoryChildren($id: ID!) { categoryChildren(parentId: $id) { id name urlPath children { id } } }`

func categoriesTool() tool {
	return tool{
		def: ai.Tool{
			Name:        "list_categories",
			Description: "The store's categories, top level first. Pass parent to see one category's subcategories. Category ids narrow search_products.",
			Schema: schema(map[string]any{
				"parent": str("A category id whose subcategories to list. Omit for the top level."),
			}),
			ReadOnly: true, Idempotent: true,
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
		if err := c.deps.GraphQL.Do(ctx, categoryChildrenDoc, map[string]any{"id": parent}, &out); err != nil {
			return "", err
		}
		cats = out.CategoryChildren
	} else {
		var out struct {
			RootCategories []category `json:"rootCategories"`
		}
		if err := c.deps.GraphQL.Do(ctx, rootCategoriesDoc, nil, &out); err != nil {
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

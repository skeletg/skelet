package agenttools

// Agent tool definitions and handlers for browsing the product catalog. The
// shared registry, dispatch and schema helpers live in tools.go.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

// catalogTools returns the catalog browsing tools (catalog.view).
func catalogTools() []tool {
	return []tool{
		// ---- Catalog (catalog.view) - browse the product catalog ----
		{
			def: ai.Tool{
				Name:        "search_products",
				Description: "Search/list catalog products by name or SKU. Omit 'query' to list products; use 'limit' to cap results.",
				Schema: windowSchema(map[string]any{
					"query": map[string]any{"type": "string", "description": "Case-insensitive match on product name or SKU. Omit to list all."},
					"limit": limitProp("Max products to return (1-50). Default 20.", 50),
				}),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolSearchProducts(ctx, deps, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_categories",
				Description: "List catalog categories. Omit 'parent_id' for top-level categories, or pass one to list its children.",
				Schema: objSchema(map[string]any{
					"parent_id": map[string]any{"type": "string", "description": "Parent category UUID (omit for root categories)."},
				}),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolListCategories(ctx, deps, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_category",
				Description: "A category's detail by id OR slug: name, slug, product count, and child categories.",
				Schema: objSchema(map[string]any{
					"category": map[string]any{"type": "string", "description": "Category UUID or slug."},
				}, "category"),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolGetCategory(ctx, deps, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_collections",
				Description: "List catalog collections (id, name, slug).",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListCollections(ctx, deps)
			},
		},
		{
			def: ai.Tool{
				Name:        "get_collection",
				Description: "A collection's detail by id: name, slug, and the products in it (count + sample).",
				Schema: objSchema(map[string]any{
					"collection_id": map[string]any{"type": "string", "description": "Collection UUID."},
				}, "collection_id"),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, a toolArgs) string {
				return toolGetCollection(ctx, deps, a)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_product_types",
				Description: "List the catalog's product types (id, name, slug) - the schemas products are built from.",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListProductTypes(ctx, deps)
			},
		},
		{
			def: ai.Tool{
				Name:        "list_attributes",
				Description: "List the catalog's product attributes (slug, name, input type) - the fields products can carry.",
				Schema:      objSchema(map[string]any{}),
			},
			perm: func(p Perms) bool { return p.Catalog },
			run: func(ctx context.Context, deps *Deps, _ uuid.UUID, _ time.Time, _ int, _ toolArgs) string {
				return toolListAttributes(ctx, deps)
			},
		},
	}
}

// ---- catalog formatters ----

func catProductName(p catalog.Product) string {
	if p.Content != nil && strings.TrimSpace(p.Content.Name) != "" {
		return p.Content.Name
	}
	return p.SKU
}

func catCategoryName(c catalog.Category) string {
	if c.Content != nil && strings.TrimSpace(c.Content.Name) != "" {
		return c.Content.Name
	}
	return c.Slug
}

func catCollectionName(c catalog.Collection) string {
	if c.Content != nil && strings.TrimSpace(c.Content.Name) != "" {
		return c.Content.Name
	}
	return c.Slug
}

func toolSearchProducts(ctx context.Context, deps *Deps, a toolArgs) string {
	q := strings.TrimSpace(a.Query)
	limit := clampLimit(a.Limit, 20, 50)
	all, _, err := deps.Catalog.ListPageWithGraph(ctx, catalog.ProductPageFilter{Locale: deps.activeLocale(ctx), Search: q, SortKey: "createdAt", SortDesc: true, Limit: int32(limit)})
	if err != nil {
		return toolFailed(ctx, err, "Could not load products.")
	}
	if len(all) == 0 {
		if q != "" {
			return fmt.Sprintf("No products match %q.", a.Query)
		}
		return "No products in the catalog."
	}
	var b strings.Builder
	if q != "" {
		fmt.Fprintf(&b, "Products matching %q:\n", a.Query)
	} else {
		b.WriteString("Products:\n")
	}
	for _, p := range all {
		fmt.Fprintf(&b, "- %s (%s)", catProductName(p), p.SKU)
		if p.DefaultPrice > 0 {
			fmt.Fprintf(&b, " - %s", money.FormatCode(p.DefaultPrice, p.DefaultCurrencyCode))
		}
		fmt.Fprintf(&b, " [id %s]\n", p.ID)
	}
	return b.String()
}

func toolListCategories(ctx context.Context, deps *Deps, a toolArgs) string {
	locale := deps.activeLocale(ctx)
	var cats []catalog.Category
	var err error
	if pid := strings.TrimSpace(a.ParentID); pid != "" {
		id, perr := uuid.Parse(pid)
		if perr != nil {
			return "Provide a valid 'parent_id' (or omit for root categories)."
		}
		cats, err = deps.Catalog.ListCategoryChildren(ctx, id, locale)
	} else {
		cats, err = deps.Catalog.ListRootCategories(ctx, locale)
	}
	if err != nil {
		return toolFailed(ctx, err, "Could not load categories.")
	}
	if len(cats) == 0 {
		return "No categories found."
	}
	var b strings.Builder
	b.WriteString("Categories (id - name - slug):\n")
	for _, c := range cats {
		fmt.Fprintf(&b, "%s - %s - %s\n", c.ID, catCategoryName(c), c.Slug)
	}
	return b.String()
}

func toolGetCategory(ctx context.Context, deps *Deps, a toolArgs) string {
	key := strings.TrimSpace(a.Category)
	if key == "" {
		return "Provide a 'category' (id or slug)."
	}
	locale := deps.activeLocale(ctx)
	var cat catalog.Category
	var err error
	if id, perr := uuid.Parse(key); perr == nil {
		cat, err = deps.Catalog.GetCategory(ctx, id, locale)
	} else {
		cat, err = deps.Catalog.GetCategoryBySlug(ctx, key)
	}
	if err != nil {
		return toolFailed(ctx, err, "Could not find that category.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Category: %s (slug %s, id %s)\n", catCategoryName(cat), cat.Slug, cat.ID)
	if n, e := deps.Catalog.CountProductsInCategory(ctx, cat.ID); e == nil {
		fmt.Fprintf(&b, "%d products.\n", n)
	}
	if kids, e := deps.Catalog.ListCategoryChildren(ctx, cat.ID, locale); e == nil && len(kids) > 0 {
		parts := make([]string, 0, len(kids))
		for _, k := range kids {
			parts = append(parts, catCategoryName(k))
		}
		fmt.Fprintf(&b, "Subcategories: %s\n", strings.Join(parts, ", "))
	}
	return b.String()
}

func toolListCollections(ctx context.Context, deps *Deps) string {
	cols, err := deps.Catalog.ListCollections(ctx, deps.activeLocale(ctx))
	if err != nil {
		return toolFailed(ctx, err, "Could not load collections.")
	}
	if len(cols) == 0 {
		return "No collections defined."
	}
	var b strings.Builder
	b.WriteString("Collections (id - name - slug):\n")
	for _, c := range cols {
		fmt.Fprintf(&b, "%s - %s - %s\n", c.ID, catCollectionName(c), c.Slug)
	}
	return b.String()
}

func toolGetCollection(ctx context.Context, deps *Deps, a toolArgs) string {
	id, err := uuid.Parse(strings.TrimSpace(a.CollectionID))
	if err != nil {
		return "Provide a valid 'collection_id' (use list_collections)."
	}
	locale := deps.activeLocale(ctx)
	col, err := deps.Catalog.GetCollection(ctx, id, locale)
	if err != nil {
		return toolFailed(ctx, err, "Could not find that collection.")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Collection: %s (slug %s, id %s)\n", catCollectionName(col), col.Slug, col.ID)
	if prods, e := deps.Catalog.ListProductsByCollection(ctx, id, locale, catalog.MaxUnpagedRows); e == nil {
		if len(prods) >= int(catalog.MaxUnpagedRows) {
			fmt.Fprintf(&b, "%d+ products", len(prods))
		} else {
			fmt.Fprintf(&b, "%d products", len(prods))
		}
		if len(prods) > 0 {
			parts := make([]string, 0, 10)
			for i, p := range prods {
				if i >= 10 {
					break
				}
				parts = append(parts, catProductName(p))
			}
			fmt.Fprintf(&b, ": %s", strings.Join(parts, "; "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func toolListProductTypes(ctx context.Context, deps *Deps) string {
	types, err := deps.Catalog.ListProductTypes(ctx)
	if err != nil {
		return toolFailed(ctx, err, "Could not load product types.")
	}
	if len(types) == 0 {
		return "No product types defined."
	}
	var b strings.Builder
	b.WriteString("Product types (id - name - slug):\n")
	for _, t := range types {
		fmt.Fprintf(&b, "%s - %s - %s\n", t.ID, t.Name, t.Slug)
	}
	return b.String()
}

func toolListAttributes(ctx context.Context, deps *Deps) string {
	attrs, err := deps.Catalog.ListAttributes(ctx)
	if err != nil {
		return toolFailed(ctx, err, "Could not load attributes.")
	}
	if len(attrs) == 0 {
		return "No attributes defined."
	}
	var b strings.Builder
	b.WriteString("Attributes (slug - name - input type):\n")
	for _, at := range attrs {
		fmt.Fprintf(&b, "%s - %s - %s\n", at.Slug, at.Name, at.InputType)
	}
	return b.String()
}

package shoptools

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

// listing is a storefront address split the way the storefront writes it: the entity's path, then
// /f/<facet>/v/<value>.../s/<sort>/pg/<page> (filter-path.ts in storefront-core), and ?q= on the search page
type listing struct {
	Entity  string
	Filters map[string][]string
	Sort    string
	Query   string
}

var (
	chainMarkers = []string{"f", "s", "pg"}
	sortsDoc     = document("sorts", `query AgentSorts { productSortOptions { slug label } }`)
)

type sortOption struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

func sortOptions(ctx context.Context, k call) ([]sortOption, error) {
	var out struct {
		Options []sortOption `json:"productSortOptions"`
	}
	if err := k.gql().Do(ctx, sortsDoc, nil, &out); err != nil {
		return nil, err
	}
	return out.Options, nil
}

func describeSorts(opts []sortOption) string {
	names := make([]string, len(opts))
	for i, o := range opts {
		names[i] = fmt.Sprintf("%s (%s)", o.Slug, o.Label)
	}
	return strings.Join(names, ", ")
}

// checkSort refuses a slug the store doesn't sort by - the storefront would quietly fall back to its default instead
func checkSort(ctx context.Context, k call, slug string) error {
	if slug == "" {
		return nil
	}
	opts, err := sortOptions(ctx, k)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(opts, func(o sortOption) bool { return o.Slug == slug }) {
		return nil
	}
	return refusal("The store sorts by " + describeSorts(opts) + " - pass one of those slugs.")
}

func parseListing(raw string) listing {
	u, err := url.Parse(raw)
	if err != nil {
		return listing{}
	}
	var segs []string
	for s := range strings.SplitSeq(strings.Trim(u.Path, "/"), "/") {
		if d, err := url.PathUnescape(s); err == nil && d != "" {
			segs = append(segs, d)
		}
	}
	start := slices.IndexFunc(segs, func(s string) bool { return slices.Contains(chainMarkers, s) })
	if start < 0 {
		start = len(segs)
	}
	l := listing{Entity: strings.Join(segs[:start], "/"), Query: strings.TrimSpace(u.Query().Get("q"))}
	facet := ""
	for i := start; i+1 < len(segs); i += 2 {
		switch segs[i] {
		case "f":
			facet = segs[i+1]
		case "v":
			if facet != "" {
				if l.Filters == nil {
					l.Filters = map[string][]string{}
				}
				l.Filters[facet] = append(l.Filters[facet], segs[i+1])
			}
		case "s":
			l.Sort = segs[i+1]
		case "pg":
		default:
			return l // a malformed tail reads as no more filters, as the storefront drops it too
		}
	}
	return l
}

// isSearch reads the storefront's search page, with or without the language prefix it puts on every language but the default
func isSearch(entity string) bool {
	if first, rest, ok := strings.Cut(entity, "/"); ok && localeSegment.MatchString(first) {
		entity = rest
	}
	return entity == "search"
}

// shownList is the product list the shopper is on, as update_listing changes it
type shownList struct {
	Kind       string // search, category or collection
	Name       string // the words searched for, or the category's or collection's name
	Path       string
	Query      string
	Category   string
	Collection string
	Filters    map[string][]string
	Sort       string
}

// currentList reads the shopper's page as a product list - nil when it's anything else
func (c *conversation) currentList(ctx context.Context, k call) (*shownList, error) {
	l := parseListing(c.page)
	if isSearch(l.Entity) {
		return &shownList{Kind: "search", Name: l.Query, Path: "/search", Query: l.Query, Filters: l.Filters, Sort: l.Sort}, nil
	}
	if l.Entity == "" {
		return nil, nil
	}
	r, err := resolveRoute(ctx, k, l.Entity)
	if err != nil || r == nil {
		return nil, err
	}
	list := &shownList{Path: "/" + r.Path, Filters: l.Filters, Sort: l.Sort}
	switch r.Kind {
	case "CATEGORY":
		list.Category = r.EntityID
		var out struct {
			Category struct {
				Name string `json:"name"`
			} `json:"category"`
		}
		if err := k.gql().Do(ctx, categoryPathDoc, map[string]any{"id": r.EntityID}, &out); err != nil {
			return nil, err
		}
		list.Kind, list.Name = "category", out.Category.Name
		return list, nil
	case "COLLECTION":
		list.Collection = r.EntityID
		var out struct {
			Collection struct {
				Name string `json:"name"`
			} `json:"collection"`
		}
		if err := k.gql().Do(ctx, collectionPathDoc, map[string]any{"id": r.EntityID}, &out); err != nil {
			return nil, err
		}
		list.Kind, list.Name = "collection", out.Collection.Name
		return list, nil
	}
	return nil, nil
}

// place is what the shopper's page shows: a product list, the home page, or what the store's routes say - a nil route is one of the storefront's own pages
type place struct {
	list  *shownList
	home  bool
	route *storeRoute
}

// A short first segment is a page's own path when the store has one there - "/faq", "/men" - and the home page in another language only when nothing answers
func (c *conversation) place(ctx context.Context, k call) (place, error) {
	list, err := c.currentList(ctx, k)
	if err != nil || list != nil {
		return place{list: list}, err
	}
	entity := parseListing(c.page).Entity
	if entity == "" {
		return place{home: true}, nil
	}
	r, err := resolveRoute(ctx, k, entity)
	if err != nil {
		return place{}, err
	}
	if r == nil && localeSegment.MatchString(entity) {
		return place{home: true}, nil
	}
	return place{route: r}, nil
}

// cmsPage is the store's information page a route shows, from the page cache - nil when it isn't one
func cmsPage(ctx context.Context, k call, r *storeRoute) (*page, error) {
	pages, err := storePages(ctx, k)
	if err != nil {
		return nil, err
	}
	for i, p := range pages {
		if p.url == link(k.ch, "/"+r.Path) {
			return &pages[i], nil
		}
	}
	return nil, nil
}

func (list shownList) args(limit int) args {
	return args{Query: list.Query, Category: list.Category, Collection: list.Collection, Filters: list.Filters, Sort: list.Sort, Limit: limit}
}

func pageTools() []ai.Tool {
	search := searchTool().def.Schema["properties"].(map[string]any)
	return []ai.Tool{{
		Name: "get_current_page",
		Description: "What the shopper is looking at right now: a product's details, a product list with its filters, sort, products and the facets it can be " +
			"narrowed by, or an information page's text. Call it whenever they say \"this\" or \"here\" or ask about the page.",
		Schema: schema(map[string]any{}),
		Effect: ai.Reads, Idempotent: true,
	}, {
		Name: "update_listing",
		Description: "Change the product list the shopper is on - its filters and sort - and their page updates straight away. Pass the complete filters " +
			"it should have as facet slug to value slugs, from get_current_page's facets; {} clears them, and leaving filters out keeps them. " +
			"It refuses a change that would leave the list empty.",
		Schema: schema(map[string]any{"filters": search["filters"], "sort": str("A sort slug from get_store_info. Leave it out to keep the current sort.")}),
		Effect: ai.Writes, Idempotent: true,
	}}
}

func (c *conversation) currentPage(ctx context.Context, k call) (string, error) {
	if c.page == "" {
		return "The storefront didn't say which page the shopper is on.", nil
	}
	where := link(c.ch, c.page)
	at, err := c.place(ctx, k)
	switch {
	case err != nil:
		return "", err
	case at.list != nil:
		return c.describeList(ctx, k, *at.list, where)
	case at.home:
		return fmt.Sprintf("The shopper is on the home page - %s", where), nil
	case at.route == nil:
		return fmt.Sprintf("The shopper is on %s - one of the store's own pages, like the cart, checkout or their account, with nothing for you to read.", where), nil
	case at.route.Kind == "PRODUCT":
		p, err := productByID(ctx, k, at.route.EntityID)
		if err != nil {
			return "", err
		}
		// Remembered for later turns, but not the panel's backstop - being on a product's page isn't asking about it
		if id, err := uuid.Parse(p.ID); err == nil {
			c.remember(id, p.Name)
		}
		return "The shopper is on this product's page:\n" + describeProduct(k, p), nil
	case at.route.Kind == "CMS_PAGE":
		p, err := cmsPage(ctx, k, at.route)
		if err != nil {
			return "", err
		}
		if p != nil {
			return fmt.Sprintf("The shopper is reading the page %s - %s\n%s", quote(p.title), where, p.text), nil
		}
	}
	return fmt.Sprintf("The shopper is on a %s page - %s", strings.ToLower(strings.ReplaceAll(at.route.Kind, "_", " ")), where), nil
}

func (c *conversation) describeList(ctx context.Context, k call, list shownList, where string) (string, error) {
	if list.Path == "/search" && list.Query == "" {
		return "The shopper is on the search page with nothing searched yet.", nil
	}
	title := "the " + list.Kind + " " + quote(list.Name)
	if list.Kind == "search" {
		title = "the search for " + quote(list.Name)
	}
	k.a = list.args(6)
	out, err := runSearch(ctx, k)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The shopper is on %s - %s\n", title, where)
	for _, slug := range slices.Sorted(maps.Keys(list.Filters)) {
		fmt.Fprintf(&b, "Filtered by %s: %s\n", slug, strings.Join(list.Filters[slug], ", "))
	}
	if list.Sort != "" {
		fmt.Fprintf(&b, "Sorted by %s\n", list.Sort)
	}
	if opts, err := sortOptions(ctx, k); err == nil && len(opts) > 0 {
		fmt.Fprintf(&b, "It can be sorted by %s\n", describeSorts(opts))
	}
	b.WriteString(out)
	b.WriteString("update_listing changes its filters and sort.\n")
	return b.String(), nil
}

func (c *conversation) updateListing(ctx context.Context, k call) (string, error) {
	if k.a.PriceMin != nil || k.a.PriceMax != nil {
		return "", refusal("A list's address can't carry a price range - narrow it with filters or the sort instead.")
	}
	list, err := c.currentList(ctx, k)
	switch {
	case err != nil:
		return "", err
	case list == nil || (list.Path == "/search" && list.Query == ""):
		return "", refusal("The shopper isn't on a product list - send them to one with show_results.")
	}
	if k.a.Filters != nil {
		list.Filters = k.a.Filters
	}
	if s := strings.TrimSpace(k.a.Sort); s != "" {
		if err := checkSort(ctx, k, s); err != nil {
			return "", err
		}
		list.Sort = s
	}
	k.a = list.args(1)
	total, err := countListing(ctx, k, list.Query, list.Sort)
	if err != nil {
		return "", err
	}
	if total == 0 {
		return "", refusal("No products on this list match that - keep at least one product in it.")
	}
	c.navigate = &Link{Path: list.Path, Query: list.Query, Filters: list.Filters, Sort: list.Sort, Count: &total}
	return fmt.Sprintf("The shopper's list now shows %d products.", total), nil
}

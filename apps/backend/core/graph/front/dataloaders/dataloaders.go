// Package dataloaders is the storefront's request-scoped batching layer - a Loader collects keys across the resolver tick and fires one batched query per entity type, even when the field is reached from many parents at once; it exists alongside render_cache.go's prime functions because those need the LIST resolver to call the right primeX before its children run, and a loader inverts that so new field resolvers, plugin ones included, get N+1 prevention for free
// One instance per HTTP request, and the cache MUST NOT survive the request - a mutation halfway through the tree would otherwise hand back stale rows; with the middleware unmounted (tests, CLI) For(ctx) returns nil and callers fail open to the single-row service method, same posture as render_cache.getRenderCache
package dataloaders

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/graph-gophers/dataloader/v7"

	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/medialibrary"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// 250µs catches every concurrent sibling field resolver gqlgen dispatches for one operation - longer windows add latency without batching more
const loaderWait = 250 * time.Microsecond

// Injected as a function pointer so this package stays free of auth and review-policy imports - the resolver-side voterFromContext owns the policy and JWT/HMAC verification
type VoterResolver func(ctx context.Context) (review.Voter, error)

// All fields are required - a nil dep panics on first Load
type Deps struct {
	Customer     *customer.Service
	Review       *review.Service
	Channel      *channel.Service
	Catalog      *catalog.Service
	Promotion    *promotion.Service
	MediaLibrary *medialibrary.Service
	Voter        VoterResolver
	// Boot-time snapshot of every sdk.RegisterDataLoader call - New walks it once per request and asks each registration to Build a fresh request-scoped loader, and slug uniqueness is the host's job since internal/plugin.Host.RegisterDataLoader rejects duplicates at boot
	PluginLoaders []sdk.DataLoaderRegistration
}

// The review list is paginated, so two products asked for at the same (limit, offset) batch into one windowed query while another page of the same product is a distinct key - sort/withMediaOnly are deliberately NOT part of it, the resolver applies them in memory after the fetch just as the single-product path does
type ReviewsKey struct {
	ProductID uuid.UUID
	Limit     int32
	Offset    int32
}

type ProductOptionsKey struct {
	ProductID uuid.UUID
	Locale    string
}

type Loaders struct {
	Customer         *dataloader.Loader[uuid.UUID, customer.Customer]
	ProductReviews   *dataloader.Loader[ReviewsKey, []review.Review]
	ReviewAggregates *dataloader.Loader[uuid.UUID, review.Aggregates]
	ReviewVoteCounts *dataloader.Loader[uuid.UUID, review.VoteCounts]
	MyReviewVote     *dataloader.Loader[uuid.UUID, int16]
	Channel          *dataloader.Loader[uuid.UUID, channel.Channel]
	ProductListings  *dataloader.Loader[uuid.UUID, []catalog.ProductChannelListing]
	// Inverse membership lookups - ConfigurableParent maps a child id to its parent link (zero value = no parent), Bundle/Grouped map a product id to its links
	ConfigurableParent   *dataloader.Loader[uuid.UUID, catalog.ConfigurableParentLink]
	ProductOptions       *dataloader.Loader[ProductOptionsKey, []catalog.ProductOption]
	ChildIDs             *dataloader.Loader[uuid.UUID, []uuid.UUID] // every variant id, published or not
	BundleMemberships    *dataloader.Loader[uuid.UUID, []catalog.BundleSelectionLink]
	GroupedMemberships   *dataloader.Loader[uuid.UUID, []catalog.GroupedMemberLink]
	ReviewQuestion       *dataloader.Loader[uuid.UUID, review.Question]
	PromotionConditions  *dataloader.Loader[uuid.UUID, []promotion.Condition]
	PromotionRuleContent *dataloader.Loader[uuid.UUID, promotion.RuleContent]
	PromotionAsset       *dataloader.Loader[uuid.UUID, medialibrary.Asset]

	productFieldLoaders

	catalog    variantSource
	variantsMu sync.Mutex
	variants   map[uuid.UUID]*variantGroup

	plugin map[string]any // registered slug -> the opaque loader Build() returned; read through the generic Plugin[K, V] below, since direct map access bypasses type safety
}

// Each loader gets its own cache, so an entity fetched twice in one request hits the DB once and nothing is shared across requests
func New(d Deps) *Loaders {
	loaders := &Loaders{
		Customer: dataloader.NewBatchedLoader(
			customerBatchFn(d.Customer),
			dataloader.WithWait[uuid.UUID, customer.Customer](loaderWait),
			dataloader.WithCache[uuid.UUID, customer.Customer](dataloader.NewCache[uuid.UUID, customer.Customer]()),
		),
		ProductReviews: dataloader.NewBatchedLoader(
			productReviewsBatchFn(d.Review),
			dataloader.WithWait[ReviewsKey, []review.Review](loaderWait),
			dataloader.WithCache[ReviewsKey, []review.Review](dataloader.NewCache[ReviewsKey, []review.Review]()),
		),
		ReviewAggregates: dataloader.NewBatchedLoader(
			reviewAggregatesBatchFn(d.Review),
			dataloader.WithWait[uuid.UUID, review.Aggregates](loaderWait),
			dataloader.WithCache[uuid.UUID, review.Aggregates](dataloader.NewCache[uuid.UUID, review.Aggregates]()),
		),
		ReviewVoteCounts: dataloader.NewBatchedLoader(
			reviewVoteCountsBatchFn(d.Review),
			dataloader.WithWait[uuid.UUID, review.VoteCounts](loaderWait),
			dataloader.WithCache[uuid.UUID, review.VoteCounts](dataloader.NewCache[uuid.UUID, review.VoteCounts]()),
		),
		MyReviewVote: dataloader.NewBatchedLoader(
			myReviewVoteBatchFn(d.Review, d.Voter),
			dataloader.WithWait[uuid.UUID, int16](loaderWait),
			dataloader.WithCache[uuid.UUID, int16](dataloader.NewCache[uuid.UUID, int16]()),
		),
		Channel: dataloader.NewBatchedLoader(
			channelBatchFn(d.Channel),
			dataloader.WithWait[uuid.UUID, channel.Channel](loaderWait),
			dataloader.WithCache[uuid.UUID, channel.Channel](dataloader.NewCache[uuid.UUID, channel.Channel]()),
		),
		ProductListings: dataloader.NewBatchedLoader(
			productListingsBatchFn(d.Catalog),
			dataloader.WithWait[uuid.UUID, []catalog.ProductChannelListing](loaderWait),
			dataloader.WithCache[uuid.UUID, []catalog.ProductChannelListing](dataloader.NewCache[uuid.UUID, []catalog.ProductChannelListing]()),
		),
		ConfigurableParent: dataloader.NewBatchedLoader(
			configurableParentBatchFn(d.Catalog),
			dataloader.WithWait[uuid.UUID, catalog.ConfigurableParentLink](loaderWait),
			dataloader.WithCache[uuid.UUID, catalog.ConfigurableParentLink](dataloader.NewCache[uuid.UUID, catalog.ConfigurableParentLink]()),
		),
		ChildIDs: dataloader.NewBatchedLoader(
			childIDsBatchFn(d.Catalog),
			dataloader.WithWait[uuid.UUID, []uuid.UUID](loaderWait),
			dataloader.WithCache[uuid.UUID, []uuid.UUID](dataloader.NewCache[uuid.UUID, []uuid.UUID]()),
		),
		ProductOptions: dataloader.NewBatchedLoader(
			productOptionsBatchFn(d.Catalog),
			dataloader.WithWait[ProductOptionsKey, []catalog.ProductOption](loaderWait),
			dataloader.WithCache[ProductOptionsKey, []catalog.ProductOption](dataloader.NewCache[ProductOptionsKey, []catalog.ProductOption]()),
		),
		BundleMemberships: dataloader.NewBatchedLoader(
			bundleMembershipsBatchFn(d.Catalog),
			dataloader.WithWait[uuid.UUID, []catalog.BundleSelectionLink](loaderWait),
			dataloader.WithCache[uuid.UUID, []catalog.BundleSelectionLink](dataloader.NewCache[uuid.UUID, []catalog.BundleSelectionLink]()),
		),
		GroupedMemberships: dataloader.NewBatchedLoader(
			groupedMembershipsBatchFn(d.Catalog),
			dataloader.WithWait[uuid.UUID, []catalog.GroupedMemberLink](loaderWait),
			dataloader.WithCache[uuid.UUID, []catalog.GroupedMemberLink](dataloader.NewCache[uuid.UUID, []catalog.GroupedMemberLink]()),
		),
		ReviewQuestion: dataloader.NewBatchedLoader(
			reviewQuestionBatchFn(d.Review),
			dataloader.WithWait[uuid.UUID, review.Question](loaderWait),
			dataloader.WithCache[uuid.UUID, review.Question](dataloader.NewCache[uuid.UUID, review.Question]()),
		),
		PromotionConditions: dataloader.NewBatchedLoader(
			promotionConditionsBatchFn(d.Promotion),
			dataloader.WithWait[uuid.UUID, []promotion.Condition](loaderWait),
			dataloader.WithCache[uuid.UUID, []promotion.Condition](dataloader.NewCache[uuid.UUID, []promotion.Condition]()),
		),
		PromotionRuleContent: dataloader.NewBatchedLoader(
			promotionRuleContentBatchFn(d.Promotion),
			dataloader.WithWait[uuid.UUID, promotion.RuleContent](loaderWait),
			dataloader.WithCache[uuid.UUID, promotion.RuleContent](dataloader.NewCache[uuid.UUID, promotion.RuleContent]()),
		),
		PromotionAsset: dataloader.NewBatchedLoader(
			promotionAssetBatchFn(d.MediaLibrary),
			dataloader.WithWait[uuid.UUID, medialibrary.Asset](loaderWait),
			dataloader.WithCache[uuid.UUID, medialibrary.Asset](dataloader.NewCache[uuid.UUID, medialibrary.Asset]()),
		),
	}
	loaders.productFieldLoaders = newProductFieldLoaders(d.Catalog)
	if d.Catalog != nil {
		loaders.catalog = d.Catalog
	}
	if len(d.PluginLoaders) > 0 {
		loaders.plugin = make(map[string]any, len(d.PluginLoaders))
		for _, reg := range d.PluginLoaders {
			loaders.plugin[reg.Slug()] = reg.Build()
		}
	}
	return loaders
}

// (nil, false) when the slug isn't registered or the stored loader's type doesn't match the requested K/V - the second case means the plugin's registration and resolver calls disagree, a programming error worth surfacing, and callers fall back to a per-id service call just like the core loaders' nil-loaders path
func Plugin[K comparable, V any](loaders *Loaders, slug string) (*dataloader.Loader[K, V], bool) {
	if loaders == nil || len(loaders.plugin) == 0 {
		return nil, false
	}
	raw, ok := loaders.plugin[slug]
	if !ok {
		return nil, false
	}
	typed, ok := raw.(*dataloader.Loader[K, V])
	return typed, ok
}

// Every batch fn below has the same shape: call the *ByIDs service method with the collected keys, fan a whole-batch error out to all waiters, then walk keys and map results - what a missing key means is per-entity (error vs zero value)

func customerBatchFn(svc *customer.Service) dataloader.BatchFunc[uuid.UUID, customer.Customer] { // ErrCustomerNotFound for a missing id - the review display-name path reads that as "fall back to a generic label" rather than failing the resolver tree
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[customer.Customer] {
		out := make([]*dataloader.Result[customer.Customer], len(keys))
		byID, err := svc.GetByIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[customer.Customer]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			if c, ok := byID[key]; ok {
				out[i] = &dataloader.Result[customer.Customer]{Data: c}
				continue
			}
			out[i] = &dataloader.Result[customer.Customer]{Error: customer.ErrCustomerNotFound}
		}
		return out
	}
}

// Keys group by their (limit, offset) page and each distinct page becomes one windowed query over every product id asking for it - a configurable's children all requesting the same args is one query for the lot
func productReviewsBatchFn(svc *review.Service) dataloader.BatchFunc[ReviewsKey, []review.Review] {
	return func(ctx context.Context, keys []ReviewsKey) []*dataloader.Result[[]review.Review] {
		out := make([]*dataloader.Result[[]review.Review], len(keys))

		type page struct{ limit, offset int32 }
		groups := make(map[page][]uuid.UUID)
		for _, k := range keys {
			p := page{k.Limit, k.Offset}
			groups[p] = append(groups[p], k.ProductID)
		}

		// Each page group resolves once, into a per-(page,product) map
		results := make(map[page]map[uuid.UUID][]review.Review, len(groups))
		errs := make(map[page]error, len(groups))
		for p, ids := range groups {
			byProduct, err := svc.ListReviewsForProductIDs(ctx, ids, p.limit, p.offset)
			if err != nil {
				errs[p] = err
				continue
			}
			results[p] = byProduct
		}

		for i, k := range keys {
			p := page{k.Limit, k.Offset}
			if err := errs[p]; err != nil {
				out[i] = &dataloader.Result[[]review.Review]{Error: err}
				continue
			}
			out[i] = &dataloader.Result[[]review.Review]{Data: results[p][k.ProductID]}
		}
		return out
	}
}

// Zero-value Aggregates for products with no approved reviews - that's the storefront-visible truth, and matches how the resolver already treats absent map entries
func reviewAggregatesBatchFn(svc *review.Service) dataloader.BatchFunc[uuid.UUID, review.Aggregates] {
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[review.Aggregates] {
		out := make([]*dataloader.Result[review.Aggregates], len(keys))
		byID, err := svc.GetAggregatesByProductIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[review.Aggregates]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[review.Aggregates]{Data: byID[key]}
		}
		return out
	}
}

func reviewVoteCountsBatchFn(svc *review.Service) dataloader.BatchFunc[uuid.UUID, review.VoteCounts] { // zero counts, not an error, for reviews with no votes - the (review, voter) unique index means most start with no rows at all
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[review.VoteCounts] {
		out := make([]*dataloader.Result[review.VoteCounts], len(keys))
		byID, err := svc.GetVoteCountsByReviewIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[review.VoteCounts]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[review.VoteCounts]{Data: byID[key]}
		}
		return out
	}
}

// The voter resolves once per batch, then the service answers for the whole id set - a missing id is 0 (no vote cast), and an unresolvable voter zeroes every key so the field resolver renders an inactive button
func myReviewVoteBatchFn(svc *review.Service, voterFn VoterResolver) dataloader.BatchFunc[uuid.UUID, int16] {
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[int16] {
		out := make([]*dataloader.Result[int16], len(keys))
		voter, err := voterFn(ctx)
		if err != nil {
			// Anon-voting disabled or no signed token - every key resolves to "no vote" rather than fanning the auth error out to every waiter
			for i := range keys {
				out[i] = &dataloader.Result[int16]{Data: 0}
			}
			return out
		}
		byID, err := svc.GetMyVotesByReviewIDs(ctx, voter, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[int16]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[int16]{Data: byID[key]}
		}
		return out
	}
}

func channelBatchFn(svc *channel.Service) dataloader.BatchFunc[uuid.UUID, channel.Channel] { // errors on a missing id - a referenced channel should always exist, so a dangling FK is worth surfacing rather than rendering blank
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[channel.Channel] {
		out := make([]*dataloader.Result[channel.Channel], len(keys))
		byID, err := svc.ListByIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[channel.Channel]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			if sv, ok := byID[key]; ok {
				out[i] = &dataloader.Result[channel.Channel]{Data: sv}
				continue
			}
			out[i] = &dataloader.Result[channel.Channel]{Error: channel.ErrChannelNotFound}
		}
		return out
	}
}

func configurableParentBatchFn(svc *catalog.Service) dataloader.BatchFunc[uuid.UUID, catalog.ConfigurableParentLink] { // a product that isn't a variant gets the zero value - normal, not an error
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[catalog.ConfigurableParentLink] {
		out := make([]*dataloader.Result[catalog.ConfigurableParentLink], len(keys))
		byID, err := svc.ListConfigurableParentByChildIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[catalog.ConfigurableParentLink]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[catalog.ConfigurableParentLink]{Data: byID[key]}
		}
		return out
	}
}

func childIDsBatchFn(svc *catalog.Service) dataloader.BatchFunc[uuid.UUID, []uuid.UUID] {
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[[]uuid.UUID] {
		out := make([]*dataloader.Result[[]uuid.UUID], len(keys))
		byID, err := svc.ListProductChildIDsByParentIDs(ctx, keys)
		for i, key := range keys {
			if err != nil {
				out[i] = &dataloader.Result[[]uuid.UUID]{Error: err}
				continue
			}
			out[i] = &dataloader.Result[[]uuid.UUID]{Data: byID[key]}
		}
		return out
	}
}

func productOptionsBatchFn(svc *catalog.Service) dataloader.BatchFunc[ProductOptionsKey, []catalog.ProductOption] {
	return func(ctx context.Context, keys []ProductOptionsKey) []*dataloader.Result[[]catalog.ProductOption] {
		out := make([]*dataloader.Result[[]catalog.ProductOption], len(keys))
		idsByLocale := map[string][]uuid.UUID{}
		for _, k := range keys {
			idsByLocale[k.Locale] = append(idsByLocale[k.Locale], k.ProductID)
		}
		byLocale := make(map[string]map[uuid.UUID][]catalog.ProductOption, len(idsByLocale))
		errByLocale := map[string]error{}
		for loc, ids := range idsByLocale {
			byLocale[loc], errByLocale[loc] = svc.GetProductOptionsByProductIDs(ctx, ids, loc)
		}
		for i, k := range keys {
			if err := errByLocale[k.Locale]; err != nil {
				out[i] = &dataloader.Result[[]catalog.ProductOption]{Error: err}
				continue
			}
			opts := byLocale[k.Locale][k.ProductID]
			if opts == nil {
				opts = []catalog.ProductOption{}
			}
			out[i] = &dataloader.Result[[]catalog.ProductOption]{Data: opts}
		}
		return out
	}
}

func bundleMembershipsBatchFn(svc *catalog.Service) dataloader.BatchFunc[uuid.UUID, []catalog.BundleSelectionLink] { // no memberships means a nil slice, not an error
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[[]catalog.BundleSelectionLink] {
		out := make([]*dataloader.Result[[]catalog.BundleSelectionLink], len(keys))
		byID, err := svc.ListBundleSelectionLinksByProductIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[[]catalog.BundleSelectionLink]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[[]catalog.BundleSelectionLink]{Data: byID[key]}
		}
		return out
	}
}

func groupedMembershipsBatchFn(svc *catalog.Service) dataloader.BatchFunc[uuid.UUID, []catalog.GroupedMemberLink] { // no memberships means a nil slice, not an error
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[[]catalog.GroupedMemberLink] {
		out := make([]*dataloader.Result[[]catalog.GroupedMemberLink], len(keys))
		byID, err := svc.ListGroupedMemberLinksByMemberIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[[]catalog.GroupedMemberLink]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[[]catalog.GroupedMemberLink]{Data: byID[key]}
		}
		return out
	}
}

type ListingReader interface {
	ListProductChannelListingsByProductIDs(ctx context.Context, productIDs []uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error)
	ListProductChannelListingsByProductIDsForChannel(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error)
}

func StorefrontListings(ctx context.Context, svc ListingReader, productIDs []uuid.UUID) (map[uuid.UUID][]catalog.ProductChannelListing, error) {
	if ch, ok := channel.FromContext(ctx); ok {
		return svc.ListProductChannelListingsByProductIDsForChannel(ctx, productIDs, ch.ID)
	}
	return svc.ListProductChannelListingsByProductIDs(ctx, productIDs)
}

func productListingsBatchFn(svc ListingReader) dataloader.BatchFunc[uuid.UUID, []catalog.ProductChannelListing] { // no listings means a nil slice, not an error
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[[]catalog.ProductChannelListing] {
		out := make([]*dataloader.Result[[]catalog.ProductChannelListing], len(keys))
		byID, err := StorefrontListings(ctx, svc, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[[]catalog.ProductChannelListing]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[[]catalog.ProductChannelListing]{Data: byID[key]}
		}
		return out
	}
}

func reviewQuestionBatchFn(svc *review.Service) dataloader.BatchFunc[uuid.UUID, review.Question] { // errors on a missing id - answers reference questions through a NOT NULL FK, so a missing row is probably a deleted question still referenced by a stale answer
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[review.Question] {
		out := make([]*dataloader.Result[review.Question], len(keys))
		byID, err := svc.GetQuestionsByIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[review.Question]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			if q, ok := byID[key]; ok {
				out[i] = &dataloader.Result[review.Question]{Data: q}
				continue
			}
			out[i] = &dataloader.Result[review.Question]{Error: review.ErrQuestionNotFound}
		}
		return out
	}
}

func promotionConditionsBatchFn(svc *promotion.Service) dataloader.BatchFunc[uuid.UUID, []promotion.Condition] { // a rule with no conditions gets a nil slice, which the engine reads as "always matches" for product-level matching - see MatchesProduct
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[[]promotion.Condition] {
		out := make([]*dataloader.Result[[]promotion.Condition], len(keys))
		conds, err := svc.ListConditionsByRules(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[[]promotion.Condition]{Error: err}
			}
			return out
		}
		byRule := make(map[uuid.UUID][]promotion.Condition, len(keys))
		for _, c := range conds {
			byRule[c.RuleID] = append(byRule[c.RuleID], c)
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[[]promotion.Condition]{Data: byRule[key]}
		}
		return out
	}
}

// A rule with no row for the active locale gets a zero RuleContent, not an error, and the caller falls back to its own public_label - the active locale comes from request context, so every key in one batch shares it
func promotionRuleContentBatchFn(svc *promotion.Service) dataloader.BatchFunc[uuid.UUID, promotion.RuleContent] {
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[promotion.RuleContent] {
		out := make([]*dataloader.Result[promotion.RuleContent], len(keys))
		loc := locale.FromContext(ctx)
		if loc == "" {
			// No locale, so every key resolves to a zero RuleContent and the resolver renders the rule's own public_label
			for i := range keys {
				out[i] = &dataloader.Result[promotion.RuleContent]{Data: promotion.RuleContent{}}
			}
			return out
		}
		byID, err := svc.GetRuleContentsByIDs(ctx, keys, loc)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[promotion.RuleContent]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[promotion.RuleContent]{Data: byID[key]}
		}
		return out
	}
}

// A missing id resolves to a zero Asset, not an error - promotional displays render without their badge/banner when it's gone, matching loadPromotionMedia
func promotionAssetBatchFn(svc *medialibrary.Service) dataloader.BatchFunc[uuid.UUID, medialibrary.Asset] {
	return func(ctx context.Context, keys []uuid.UUID) []*dataloader.Result[medialibrary.Asset] {
		out := make([]*dataloader.Result[medialibrary.Asset], len(keys))
		byID, err := svc.GetAssetsByIDs(ctx, keys)
		if err != nil {
			for i := range keys {
				out[i] = &dataloader.Result[medialibrary.Asset]{Error: err}
			}
			return out
		}
		for i, key := range keys {
			out[i] = &dataloader.Result[medialibrary.Asset]{Data: byID[key]}
		}
		return out
	}
}

package front

import (
	"context"
	"maps"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/review"
)

// Request-scoped batch of pre-fetched rows - list resolvers fill it once up front so per-product field resolvers and renderProduct read maps instead of one round-trip per row; every helper is nil-safe and falls through to a single-row fetch on a miss
type renderCache struct {
	mu sync.RWMutex

	productTypes map[uuid.UUID]catalog.ProductType // by product_type_id, filled lazily by getProductType - a memoize cache, not a batch query, since catalog surfaces touch only a handful of types per request

	listings map[uuid.UUID][]catalog.ProductChannelListing // by product_id, from CatalogSvc.ListProductChannelListingsByProductIDs

	channels map[uuid.UUID]channel.Channel // by channel_id, from ChannelSvc.ListByIDs

	salable map[uuid.UUID]float64 // by product_id, always one channel per request (headers resolve exactly one) - value is the salable count, sentinel -1 = always in stock

	backorderable map[uuid.UUID]bool // by product_id; presence means primed, and since the bool zero value is a real answer callers MUST use comma-ok

	aggregates map[uuid.UUID]review.Aggregates // by product_id, missing = no approved reviews so callers fall back to the empty Aggregates

	assignedQuestions map[questionCacheKey][]review.Question // by (product_type_id, locale), reused across products of the same type within a request

	voteCounts map[uuid.UUID]review.VoteCounts // by review id, primed in one batch by Product.reviews so helpfulCount / notHelpfulCount don't each round-trip; missing = no votes yet

	votesPrimed map[uuid.UUID]struct{} // lets MyVote tell "no row for this review (didn't vote)" from "never primed" - a separate set rather than a map sentinel keeps the counts clean for signed-in and anon callers alike

	myVotes map[uuid.UUID]int16 // by review id, value is the caller's own -1 or +1; missing = hasn't voted

	currencies map[string]currency.Currency // by uppercase ISO 4217 code

	// One read for the whole request - the browse path needs attributes twice (facet dressing + channel filter-slug translation) and every rendered product derives localized names off them
	attributes       []catalog.Attribute
	attributesLoaded bool // distinguishes "no attributes" from "not fetched yet"

	attrNameMaps map[string]attrNameMaps // locale-resolved display names built once per (request, locale) from the full attribute content tables - spares the localizer a content query per product

	localeValues       map[uuid.UUID][]catalog.ProductAttributeLocaleValue // by product_id, plain_text translations across ALL locales - callers filter
	localeValuesPrimed map[uuid.UUID]struct{}                              // marks ids the batch loader has seen so "no translations" doesn't re-query per product

	promoAttrKVs map[uuid.UUID]map[string]string // by product_id, attribute KVs captured BEFORE the isPublic filter so promotion display rules keep matching private attributes
}

type attrNameMaps struct { // keyed by stringified attribute / value UUIDs, the shape the model walkers consume directly
	attrNames  map[string]string
	valueNames map[string]string
}

type questionCacheKey struct {
	productTypeID uuid.UUID
	locale        string
}

type renderCacheCtxKey struct{}

func withRenderCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, renderCacheCtxKey{}, newRenderCache())
}

// Without this middleware the primeX / cachedX helpers fall through to single-row fetches - safe, but the whole optimisation is gone
func RenderCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(withRenderCache(r.Context())))
	})
}

func getRenderCache(ctx context.Context) *renderCache { // nil when no middleware ran - callers must nil-check
	c, _ := ctx.Value(renderCacheCtxKey{}).(*renderCache)
	return c
}

func newRenderCache() *renderCache {
	return &renderCache{
		productTypes:       map[uuid.UUID]catalog.ProductType{},
		listings:           map[uuid.UUID][]catalog.ProductChannelListing{},
		channels:           map[uuid.UUID]channel.Channel{},
		salable:            map[uuid.UUID]float64{},
		backorderable:      map[uuid.UUID]bool{},
		aggregates:         map[uuid.UUID]review.Aggregates{},
		assignedQuestions:  map[questionCacheKey][]review.Question{},
		voteCounts:         map[uuid.UUID]review.VoteCounts{},
		votesPrimed:        map[uuid.UUID]struct{}{},
		myVotes:            map[uuid.UUID]int16{},
		currencies:         map[string]currency.Currency{},
		attrNameMaps:       map[string]attrNameMaps{},
		localeValues:       map[uuid.UUID][]catalog.ProductAttributeLocaleValue{},
		localeValuesPrimed: map[uuid.UUID]struct{}{},
		promoAttrKVs:       map[uuid.UUID]map[string]string{},
	}
}

func (r *Resolver) stashPromotionAttrKVs(ctx context.Context, productID uuid.UUID, values []*model.ProductAttributeValue) {
	c := getRenderCache(ctx)
	if c == nil {
		return
	}
	kvs := attributeKVsFromModelValues(values)
	c.mu.Lock()
	c.promoAttrKVs[productID] = kvs
	c.mu.Unlock()
}

func (r *Resolver) cachedPromotionAttrKVs(ctx context.Context, productID uuid.UUID) (map[string]string, bool) {
	c := getRenderCache(ctx)
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	kvs, ok := c.promoAttrKVs[productID]
	return kvs, ok
}

func (r *Resolver) getAttributesCached(ctx context.Context) ([]catalog.Attribute, error) { // attribute metadata is small and read-only within a request, and browse + render both consult it repeatedly
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		loaded, attrs := c.attributesLoaded, c.attributes
		c.mu.RUnlock()
		if loaded {
			return attrs, nil
		}
	}
	attrs, err := r.CatalogSvc.ListAttributes(ctx)
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.mu.Lock()
		c.attributes = attrs
		c.attributesLoaded = true
		c.mu.Unlock()
	}
	return attrs, nil
}

// One query per request for the whole product list, plus one per configurable parent for its children, instead of one per rendered product
func (r *Resolver) primeProductAttributeLocaleValues(ctx context.Context, productIDs []uuid.UUID) {
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 {
		return
	}
	c.mu.RLock()
	missing := make([]uuid.UUID, 0, len(productIDs))
	for _, id := range productIDs {
		if _, ok := c.localeValuesPrimed[id]; !ok {
			missing = append(missing, id)
		}
	}
	c.mu.RUnlock()
	if len(missing) == 0 {
		return
	}
	rows, err := r.CatalogSvc.ListProductAttributeLocaleValuesByProductIDs(ctx, missing)
	if err != nil {
		return // localization is dressing - a failed batch degrades to canonical values rather than failing the render
	}
	c.mu.Lock()
	for _, row := range rows {
		c.localeValues[row.ProductID] = append(c.localeValues[row.ProductID], row)
	}
	for _, id := range missing {
		c.localeValuesPrimed[id] = struct{}{}
	}
	c.mu.Unlock()
}

func (r *Resolver) cachedProductLocaleValues(ctx context.Context, productID uuid.UUID) []catalog.ProductAttributeLocaleValue { // all locales, callers filter - only meaningful after primeProductAttributeLocaleValues
	c := getRenderCache(ctx)
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.localeValues[productID]
}

// Hands back the type WITH assigned attributes hydrated - buildProductAttributeValues projects attributeValues by iterating pt.Attributes, so a bare row silently empties them for the product and every child
// The ONLY product-type getter - a bare or DataLoader variant would hand back attribute-less rows, so there deliberately isn't one
func (r *Resolver) getProductType(ctx context.Context, id uuid.UUID) (catalog.ProductType, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		pt, ok := c.productTypes[id]
		c.mu.RUnlock()
		if ok {
			return pt, nil
		}
	}
	pt, err := r.CatalogSvc.GetProductType(ctx, id)
	if err != nil {
		return catalog.ProductType{}, err
	}
	if c != nil {
		c.mu.Lock()
		c.productTypes[id] = pt
		c.mu.Unlock()
	}
	return pt, nil
}

// Listings only read CurrencyCode / slug, so the bare row without locale hydration is enough - render cache first, then DataLoader for unprimed paths (Channel.ListByIDs, same bare shape), then the service
func (r *Resolver) getChannel(ctx context.Context, id uuid.UUID) (channel.Channel, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		sv, ok := c.channels[id]
		c.mu.RUnlock()
		if ok {
			return sv, nil
		}
	}
	if loaders := dataloaders.For(ctx); loaders != nil {
		return loaders.Channel.Load(ctx, id)()
	}
	sv, err := r.ChannelSvc.GetByID(ctx, id)
	if err != nil {
		return channel.Channel{}, err
	}
	if c != nil {
		c.mu.Lock()
		c.channels[id] = sv
		c.mu.Unlock()
	}
	return sv, nil
}

// primeListingsAndChannels is the hot-path source on list resolvers, and the DataLoader behind it covers per-field-resolver call sites where no primer ran
func (r *Resolver) getProductChannelListings(ctx context.Context, productID uuid.UUID) ([]catalog.ProductChannelListing, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		rows, ok := c.listings[productID]
		c.mu.RUnlock()
		if ok {
			return rows, nil
		}
	}
	if loaders := dataloaders.For(ctx); loaders != nil {
		return loaders.ProductListings.Load(ctx, productID)()
	}
	rows, err := r.CatalogSvc.ListProductChannelListings(ctx, productID)
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.mu.Lock()
		c.listings[productID] = rows
		c.mu.Unlock()
	}
	return rows, nil
}

func (r *Resolver) primeListingsAndChannels(ctx context.Context, productIDs []uuid.UUID) error { // one batch for the listings, one for the channels they reference - safe with an empty slice
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 {
		return nil
	}
	byProduct, err := r.CatalogSvc.ListProductChannelListingsByProductIDs(ctx, productIDs)
	if err != nil {
		return err
	}
	svIDSet := map[uuid.UUID]struct{}{}
	for _, rows := range byProduct {
		for _, row := range rows {
			svIDSet[row.ChannelID] = struct{}{}
		}
	}
	c.mu.Lock()
	maps.Copy(c.listings, byProduct)
	for _, pid := range productIDs { // every requested product needs an entry or a missing-key read falls through to a redundant single-row fetch
		if _, ok := c.listings[pid]; !ok {
			c.listings[pid] = nil
		}
	}
	c.mu.Unlock()

	if len(svIDSet) > 0 {
		ids := make([]uuid.UUID, 0, len(svIDSet))
		for id := range svIDSet {
			ids = append(ids, id)
		}
		byID, err := r.ChannelSvc.ListByIDs(ctx, ids)
		if err != nil {
			return err
		}
		c.mu.Lock()
		maps.Copy(c.channels, byID)
		c.mu.Unlock()
	}
	return nil
}

func (r *Resolver) primeSalable(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) error { // one pass so later InStock / SalableQuantity field resolvers just read the map
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 || channelID == uuid.Nil || r.InventorySvc == nil {
		return nil
	}
	bySalable, err := r.InventorySvc.SalableByProductIDs(ctx, productIDs, channelID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	maps.Copy(c.salable, bySalable)
	c.mu.Unlock()
	return nil
}

func (r *Resolver) cachedSalable(ctx context.Context, productID uuid.UUID) (float64, bool) { // false means primeSalable never covered this pair - the field resolver falls back to a single-product Salable call
	c := getRenderCache(ctx)
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.salable[productID]
	return v, ok
}

// One query so Product.backorderable and the InStock zero-stock fallback don't each fire an inventory lookup per card - products with no tracked inventory item map to false
func (r *Resolver) primeBackorderable(ctx context.Context, productIDs []uuid.UUID) error {
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 || r.InventorySvc == nil {
		return nil
	}
	byID, err := r.InventorySvc.BackorderableByProductIDs(ctx, productIDs)
	if err != nil {
		return err
	}
	c.mu.Lock()
	for _, pid := range productIDs {
		c.backorderable[pid] = byID[pid] // missing key = false (untracked)
	}
	c.mu.Unlock()
	return nil
}

func (r *Resolver) cachedBackorderable(ctx context.Context, productID uuid.UUID) (bool, bool) { // (false, false) means primeBackorderable never ran, so the field resolver falls back to a single-item lookup
	c := getRenderCache(ctx)
	if c == nil {
		return false, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.backorderable[productID]
	return v, ok
}

// Batches every per-card association so the Product field resolvers on a rendered list read the cache instead of one query per card - best-effort, a failed batch degrades to per-card fetches rather than failing the render
func (r *Resolver) primeProductCards(ctx context.Context, productIDs []uuid.UUID, channelID uuid.UUID) {
	if len(productIDs) == 0 {
		return
	}
	_ = r.primeListingsAndChannels(ctx, productIDs)
	_ = r.primeReviewAggregates(ctx, productIDs)
	r.primeProductAttributeLocaleValues(ctx, productIDs)
	_ = r.primeBackorderable(ctx, productIDs)
	if channelID != uuid.Nil {
		_ = r.primeSalable(ctx, productIDs, channelID)
	}
}

func (r *Resolver) primeReviewAggregates(ctx context.Context, productIDs []uuid.UUID) error {
	c := getRenderCache(ctx)
	if c == nil || len(productIDs) == 0 || r.ReviewSvc == nil {
		return nil
	}
	byID, err := r.ReviewSvc.GetAggregatesByProductIDs(ctx, productIDs)
	if err != nil {
		return err
	}
	c.mu.Lock()
	maps.Copy(c.aggregates, byID)
	for _, pid := range productIDs { // mark the review-less products too, or every miss re-queries
		if _, ok := c.aggregates[pid]; !ok {
			c.aggregates[pid] = review.Aggregates{Distribution: [5]int{}}
		}
	}
	c.mu.Unlock()
	return nil
}

// Two round-trips total (counts, then my-votes) for the whole page - without it Review.helpfulCount / notHelpfulCount / myVote each fire their own SQL per review, an N+1 on the PDP reviews list
// Safe to call repeatedly: ids already in votesPrimed are skipped, so a re-render after a vote mutation doesn't redo the batch
func (r *Resolver) primeReviewVotes(ctx context.Context, reviewIDs []uuid.UUID) error {
	c := getRenderCache(ctx)
	if c == nil || len(reviewIDs) == 0 || r.ReviewSvc == nil {
		return nil
	}
	c.mu.RLock()
	pending := make([]uuid.UUID, 0, len(reviewIDs)) // only the ids not yet primed
	for _, id := range reviewIDs {
		if _, ok := c.votesPrimed[id]; !ok {
			pending = append(pending, id)
		}
	}
	c.mu.RUnlock()
	if len(pending) == 0 {
		return nil
	}
	counts, err := r.ReviewSvc.GetVoteCountsByReviewIDs(ctx, pending)
	if err != nil {
		return err
	}
	voter, voterErr := r.voterFromContext(ctx) // my-votes is caller-keyed, so it rides the primer instead of a cached-by-reviewID field to keep the one-round-trip win - empty map when the caller has no usable voter identity
	var mine map[uuid.UUID]int16
	if voterErr == nil {
		mine, err = r.ReviewSvc.GetMyVotesByReviewIDs(ctx, voter, pending)
		if err != nil {
			return err
		}
	}
	c.mu.Lock()
	for _, id := range pending {
		c.votesPrimed[id] = struct{}{}
		if vc, ok := counts[id]; ok {
			c.voteCounts[id] = vc
		}
		if v, ok := mine[id]; ok {
			c.myVotes[id] = v
		}
	}
	c.mu.Unlock()
	return nil
}

// Resolution order is render-cache primed map (filled by primeReviewVotes on the paths that prime it), then the DataLoader that auto-batches across the resolver tree, then a single-row service call
func (r *Resolver) cachedVoteCounts(ctx context.Context, reviewID uuid.UUID) (review.VoteCounts, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		_, primed := c.votesPrimed[reviewID]
		v := c.voteCounts[reviewID]
		c.mu.RUnlock()
		if primed {
			return v, nil
		}
	}
	if loaders := dataloaders.For(ctx); loaders != nil {
		return loaders.ReviewVoteCounts.Load(ctx, reviewID)()
	}
	counts, err := r.ReviewSvc.GetVoteCountsByReviewIDs(ctx, []uuid.UUID{reviewID})
	if err != nil {
		return review.VoteCounts{}, err
	}
	return counts[reviewID], nil
}

// Same resolution order as cachedVoteCounts - the loader's batch fn resolves the voter from context itself, which keeps this helper signature-compatible
func (r *Resolver) cachedMyVote(ctx context.Context, reviewID uuid.UUID) (int16, error) {
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		_, primed := c.votesPrimed[reviewID]
		v := c.myVotes[reviewID]
		c.mu.RUnlock()
		if primed {
			return v, nil
		}
	}
	if loaders := dataloaders.For(ctx); loaders != nil {
		return loaders.MyReviewVote.Load(ctx, reviewID)()
	}
	voter, err := r.voterFromContext(ctx)
	if err != nil {
		return 0, nil
	}
	mine, err := r.ReviewSvc.GetMyVotesByReviewIDs(ctx, voter, []uuid.UUID{reviewID})
	if err != nil {
		return 0, err
	}
	return mine[reviewID], nil
}

func (r *Resolver) cachedAggregates(ctx context.Context, productID uuid.UUID) (review.Aggregates, error) { // same render-cache then loader then service chain as cachedVoteCounts
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		agg, ok := c.aggregates[productID]
		c.mu.RUnlock()
		if ok {
			return agg, nil
		}
	}
	if loaders := dataloaders.For(ctx); loaders != nil {
		return loaders.ReviewAggregates.Load(ctx, productID)()
	}
	return r.ReviewSvc.GetAggregates(ctx, productID)
}

func (r *Resolver) getAssignedQuestionsLocalized(ctx context.Context, productTypeID uuid.UUID, locale string) ([]review.Question, error) { // memoized per (product type, locale) so a PLP of 24 same-type cards costs exactly one round-trip
	key := questionCacheKey{productTypeID: productTypeID, locale: locale}
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		qs, ok := c.assignedQuestions[key]
		c.mu.RUnlock()
		if ok {
			return qs, nil
		}
	}
	qs, err := r.ReviewSvc.ListAssignedQuestionsLocalized(ctx, productTypeID, locale)
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.mu.Lock()
		c.assignedQuestions[key] = qs
		c.mu.Unlock()
	}
	return qs, nil
}

func (r *Resolver) getCurrency(ctx context.Context, code string) (currency.Currency, error) { // memoized so N channels asking for `currency` cost at most one round-trip per unique code
	c := getRenderCache(ctx)
	if c != nil {
		c.mu.RLock()
		cur, ok := c.currencies[code]
		c.mu.RUnlock()
		if ok {
			return cur, nil
		}
	}
	cur, err := r.CurrencySvc.GetByCode(ctx, code)
	if err != nil {
		return currency.Currency{}, err
	}
	if c != nil {
		c.mu.Lock()
		c.currencies[code] = cur
		c.mu.Unlock()
	}
	return cur, nil
}

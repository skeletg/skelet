package embedding

import (
	"container/list"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

const (
	// A shopper's question asked again - a follow-up, a retried turn, a popular one - reuses its vector for this long
	queryTTL     = 10 * time.Minute
	queryEntries = 2000
	maxQueryRune = 500
	// after a failed or timed-out call every search answers on its words for this long, rather than each one waiting on the provider
	providerRest = 30 * time.Second
	// a question is asked once for everyone waiting on it, under its own deadline rather than the first asker's
	questionTimeout = 10 * time.Second
)

var errProviderResting = fmt.Errorf("embedding: the provider failed a moment ago, resting it: %w", ratelimit.ErrLimited)

// Searcher finds what's close in meaning to a shopper's words. With embeddings off it finds nothing, and search stays on words
type Searcher struct {
	q       *db.Queries
	tx      *txrunner.Manager
	emb     Embedder
	cache   *queryCache
	flight  singleflight.Group
	resting atomic.Int64
	pace    Pace
	rate    func(ctx context.Context, channelID uuid.UUID) int
	store   func(ctx context.Context, channelID uuid.UUID) int
}

type Pace interface {
	AllowN(ctx context.Context, key string, rate int) bool
}

// SetPace limits the new questions each shopper's searches turn into vectors, at the rate the store set - one already
// asked is free, and past the limit the search goes on with its words alone
func (s *Searcher) SetPace(p Pace, perMinute, storePerMinute func(ctx context.Context, channelID uuid.UUID) int) {
	s.pace, s.rate, s.store = p, perMinute, storePerMinute
}

func NewSearcher(q *db.Queries, tx *txrunner.Manager, emb Embedder) *Searcher {
	return &Searcher{q: q, tx: tx, emb: emb, cache: newQueryCache(queryEntries, queryTTL)}
}

// Hit is a product close in meaning, nearest first
type Hit struct {
	ProductID uuid.UUID
	Distance  float64
}

// Passage is a piece of a page close in meaning, with the words it holds
type Passage struct {
	PageID   uuid.UUID
	Title    string
	Path     string
	Text     string
	Distance float64
}

func (s *Searcher) vector(ctx context.Context, channelID uuid.UUID, model, query string) (string, error) {
	query = tidy(query)
	if r := []rune(query); len(r) > maxQueryRune {
		query = string(r[:maxQueryRune])
	}
	key := questionKey(model, query)
	if v, ok := s.cache.get(key); ok {
		return v, nil
	}
	if time.Now().UnixNano() < s.resting.Load() {
		return "", errProviderResting
	}
	if who := shopperKey(ctx); s.pace != nil && who != "" && !ratelimit.IsCounted(ctx) && !s.pace.AllowN(ctx, "meaning:"+channelID.String()+":"+who, s.rate(ctx, channelID)) {
		return "", fmt.Errorf("embedding: too many new questions from %s: %w", who, ratelimit.ErrLimited)
	}
	if s.pace != nil && s.store != nil && !s.pace.AllowN(ctx, "meaning-store:"+channelID.String(), s.store(ctx, channelID)) {
		return "", fmt.Errorf("embedding: the store's searches asked too many new questions: %w", ratelimit.ErrLimited)
	}
	done := s.flight.DoChan(string(key[:]), func() (any, error) {
		qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), questionTimeout)
		defer cancel()
		vecs, _, err := s.emb.Embed(qctx, ai.ForQuery, []string{query})
		if err != nil {
			s.rest(ctx, err)
			return "", err
		}
		v := vectorLiteral(vecs[0])
		s.cache.put(key, v)
		return v, nil
	})
	select {
	case <-ctx.Done(): // this asker gave up - the question still finishes for the rest, and the cache
		return "", ctx.Err()
	case r := <-done:
		if r.Err != nil {
			return "", r.Err
		}
		return r.Val.(string), nil
	}
}

func questionKey(model, query string) [32]byte { return sha256.Sum256([]byte(model + "\x00" + query)) }

func (s *Searcher) rest(ctx context.Context, err error) {
	until := time.Now().Add(providerRest).UnixNano()
	if prev := s.resting.Swap(until); prev < time.Now().UnixNano() {
		slog.WarnContext(ctx, "embedding: the provider failed, so searches answer on their words for a while", "rest", providerRest, "err", err)
	}
}

// Products is the store's listed products nearest the words in the given languages, the first a shopper's and the rest fallbacks.
// One in several of those languages counts once, at its nearest
func (s *Searcher) Products(ctx context.Context, channelID uuid.UUID, locales []string, query string, limit int) ([]Hit, error) {
	model := s.emb.Model(ctx)
	if model == "" || strings.TrimSpace(query) == "" || limit <= 0 {
		return nil, nil
	}
	vec, err := s.vector(ctx, channelID, model, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: the question: %w", err)
	}
	return s.nearest(ctx, channelID, model, vec, locales, limit, nil)
}

// nearest is the store's listed products closest to a vector, one in several languages counted once at its nearest
func (s *Searcher) nearest(ctx context.Context, channelID uuid.UUID, model, vec string, locales []string, limit int, skip map[uuid.UUID]bool) ([]Hit, error) {
	var rows []db.NearestProductsRow
	err := s.tx.InTransaction(ctx, func(tx *sql.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.AllowIterativeVectorScan(ctx); err != nil {
			return err
		}
		var err error
		rows, err = q.NearestProducts(ctx, db.NearestProductsParams{Query: vec, Model: model, Locales: locales, ChannelID: channelID, Lim: int32((limit + len(skip)) * len(locales))})
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("embedding: nearest products: %w", err)
	}
	seen := map[uuid.UUID]bool{}
	out := make([]Hit, 0, limit)
	for _, r := range rows {
		if !seen[r.SourceID] && !skip[r.SourceID] && len(out) < limit {
			seen[r.SourceID] = true
			out = append(out, Hit{ProductID: r.SourceID, Distance: r.Distance})
		}
	}
	return out, nil
}

// Similar is what's found near products, and the shopper-visible attributes it was held to
type Similar struct {
	Hits    []Hit
	Matched []string
	// none of the given products has a vector yet, so there's nothing to compare against - not the same as nothing close
	Unembedded bool
}

const (
	similarDraw    = 8 // drawn per product wanted, so the ones that don't match leave enough
	maxSimilarDraw = 200
)

// Similar is the store's listed products nearest in meaning to the given ones together, each sharing a value with them on
// every attribute marked to match that they have - a value any of them has counts, and a variant stands for its product.
// The given products are left out
func (s *Searcher) Similar(ctx context.Context, channelID uuid.UUID, locales []string, products []uuid.UUID, limit int) (Similar, error) {
	model := s.emb.Model(ctx)
	if model == "" || len(products) == 0 || limit <= 0 {
		return Similar{}, nil
	}
	parents, err := s.q.ProductParents(ctx, products)
	if err != nil {
		return Similar{}, fmt.Errorf("embedding: similar products' parents: %w", err)
	}
	given := map[uuid.UUID]bool{}
	for _, id := range products {
		given[id] = true
	}
	for _, p := range parents {
		delete(given, p.ChildID)
		given[p.ParentID] = true
	}
	seeds := slices.Collect(maps.Keys(given))
	vec, err := s.q.AverageProductVector(ctx, db.AverageProductVectorParams{Model: model, Ids: seeds, Locales: locales})
	if err != nil || vec == "" {
		return Similar{Unembedded: err == nil}, err
	}
	want, names, err := s.matchValues(ctx, seeds, true)
	if err != nil {
		return Similar{}, err
	}
	draw := limit
	if len(want) > 0 {
		draw = min(limit*similarDraw, maxSimilarDraw)
	}
	near, err := s.nearest(ctx, channelID, model, vec, locales, draw, given)
	if err != nil || len(want) == 0 {
		return Similar{Hits: near[:min(len(near), limit)]}, err
	}
	found, _, err := s.matchValues(ctx, hitIDs(near), false)
	if err != nil {
		return Similar{}, err
	}
	out := Similar{Matched: slices.Sorted(maps.Values(names))}
	for _, h := range near {
		if len(out.Hits) < limit && matches(found[h.ProductID], want[uuid.Nil]) {
			out.Hits = append(out.Hits, h)
		}
	}
	return out, nil
}

// matchValues is each product's values by attribute - or, pooled, every product's under uuid.Nil
func (s *Searcher) matchValues(ctx context.Context, ids []uuid.UUID, pooled bool) (map[uuid.UUID]map[uuid.UUID]map[string]bool, map[uuid.UUID]string, error) {
	rows, err := s.q.SimilarMatchValues(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("embedding: values to match: %w", err)
	}
	out := map[uuid.UUID]map[uuid.UUID]map[string]bool{}
	names := map[uuid.UUID]string{}
	for _, r := range rows {
		owner := r.ProductID
		if pooled {
			owner = uuid.Nil
		}
		if out[owner] == nil {
			out[owner] = map[uuid.UUID]map[string]bool{}
		}
		if out[owner][r.AttributeID] == nil {
			out[owner][r.AttributeID] = map[string]bool{}
		}
		out[owner][r.AttributeID][r.Value] = true
		if r.MatchesAny {
			out[owner][r.AttributeID][anyValue] = true
		}
		if r.IsPublic {
			names[r.AttributeID] = r.AttributeName
		}
	}
	return out, names, nil
}

// anyValue marks a value that stands for all the others, so either side holding one matches whatever the other has
const anyValue = "*"

// One that lacks an attribute the given products have doesn't match on it
func matches(have, want map[uuid.UUID]map[string]bool) bool {
	for attr, values := range want {
		got := have[attr]
		if len(got) == 0 {
			return false
		}
		if values[anyValue] || got[anyValue] {
			continue
		}
		if !sharesOne(got, values) {
			return false
		}
	}
	return true
}

func sharesOne(got, values map[string]bool) bool {
	for v := range got {
		if values[v] {
			return true
		}
	}
	return false
}

func hitIDs(hits []Hit) []uuid.UUID {
	out := make([]uuid.UUID, len(hits))
	for i, h := range hits {
		out[i] = h.ProductID
	}
	return out
}

// ProductTexts is each product's embedded words in the first language that has them - none for one never embedded
func (s *Searcher) ProductTexts(ctx context.Context, ids []uuid.UUID, locales []string) (map[uuid.UUID]string, error) {
	model := s.emb.Model(ctx)
	if model == "" || len(ids) == 0 {
		return map[uuid.UUID]string{}, nil
	}
	rows, err := s.q.EmbeddedProductTexts(ctx, db.EmbeddedProductTextsParams{Model: model, Ids: ids, Locales: locales})
	if err != nil {
		return nil, fmt.Errorf("embedding: product texts: %w", err)
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		out[r.SourceID] = r.Content
	}
	return out, nil
}

// Passages is the store's published page passages nearest the words, a passage in a fallback language only where the
// shopper's own has none of that page
func (s *Searcher) Passages(ctx context.Context, channelID uuid.UUID, locales []string, query string, limit int) ([]Passage, error) {
	model := s.emb.Model(ctx)
	if model == "" || strings.TrimSpace(query) == "" || limit <= 0 {
		return nil, nil
	}
	vec, err := s.vector(ctx, channelID, model, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: the question: %w", err)
	}
	rows, err := s.q.NearestPassages(ctx, db.NearestPassagesParams{Query: vec, Model: model, Locales: locales, ChannelID: channelID, Lim: int32(limit * len(locales))})
	if err != nil {
		return nil, fmt.Errorf("embedding: nearest passages: %w", err)
	}
	own := map[uuid.UUID]bool{}
	for _, r := range rows {
		if len(locales) > 0 && r.Locale == locales[0] {
			own[r.SourceID] = true
		}
	}
	out := make([]Passage, 0, limit)
	for _, r := range rows {
		if len(out) == limit {
			break
		}
		if len(locales) > 0 && r.Locale != locales[0] && own[r.SourceID] {
			continue
		}
		out = append(out, Passage{PageID: r.SourceID, Title: r.Title, Path: r.UrlPath, Text: r.Content, Distance: r.Distance})
	}
	return out, nil
}

// queryCache is a small LRU of question vectors, each good for a while
type queryCache struct {
	mu    sync.Mutex
	max   int
	ttl   time.Duration
	order *list.List
	byKey map[[32]byte]*list.Element
}

type cached struct {
	key [32]byte
	vec string
	at  time.Time
}

func newQueryCache(max int, ttl time.Duration) *queryCache {
	return &queryCache{max: max, ttl: ttl, order: list.New(), byKey: map[[32]byte]*list.Element{}}
}

func (c *queryCache) get(key [32]byte) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.byKey[key]
	if !ok {
		return "", false
	}
	e := el.Value.(cached)
	if time.Since(e.at) > c.ttl {
		c.order.Remove(el)
		delete(c.byKey, key)
		return "", false
	}
	c.order.MoveToFront(el)
	return e.vec, true
}

func (c *queryCache) put(key [32]byte, vec string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.byKey[key]; ok {
		el.Value = cached{key: key, vec: vec, at: time.Now()}
		c.order.MoveToFront(el)
		return
	}
	c.byKey[key] = c.order.PushFront(cached{key: key, vec: vec, at: time.Now()})
	for c.order.Len() > c.max {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.byKey, last.Value.(cached).key)
	}
}

// Whoever the request is for: the customer when signed in, else the address - none for work no shopper asked for
func shopperKey(ctx context.Context) string {
	if id := channel.OptionalCustomerID(ctx); id != nil {
		return "customer:" + id.String()
	}
	if req, ok := auth.RequestFromContext(ctx); ok && req != nil {
		return auth.AddressKey(auth.ClientIP(req))
	}
	return ""
}

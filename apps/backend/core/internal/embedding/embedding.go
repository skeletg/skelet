// Package embedding keeps a vector for every product and published page, per locale, so the store can be searched by
// meaning. Saving something queues it; a background pass reads the queue, works out the text, embeds only the passages
// whose words changed, and writes them. The queue empty, the pass picks up whatever has never been embedded
package embedding

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/health"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

const (
	SourceProduct = "product"
	SourcePage    = "cms_page"

	// Sources per pass, and how long one run of the job works before leaving the rest to the next
	passSize  = 32
	runBudget = 45 * time.Second
	// Never embedded, picked up per pass once the queue is empty
	backfillSize = 200
	pruneSize    = 5000
	// A backlog older than this means the job isn't keeping up, or keeps failing
	StaleBacklog = time.Hour
)

// Embedder is the store's embedding provider
type Embedder interface {
	Model(ctx context.Context) string
	Embed(ctx context.Context, purpose ai.Purpose, texts []string) ([][]float32, int64, error)
}

type Indexer struct {
	q      *db.Queries
	tx     *txrunner.Manager
	emb    Embedder
	pruned atomic.Value // the model whose stale rows were last cleared - only a change of model leaves any
}

func New(q *db.Queries, tx *txrunner.Manager, emb Embedder) *Indexer {
	return &Indexer{q: q, tx: tx, emb: emb}
}

// Enqueue marks sources as changed - cheap and safe to call with embeddings off, since the backfill wouldn't know an edit
func (x *Indexer) Enqueue(ctx context.Context, source string, ids ...uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return x.q.EnqueueEmbeddings(ctx, db.EnqueueEmbeddingsParams{Source: source, Ids: ids})
}

func (x *Indexer) EnqueueCategory(ctx context.Context, categoryID uuid.UUID) error {
	return x.q.EnqueueCategoryEmbeddings(ctx, categoryID)
}

func (x *Indexer) EnqueueProductType(ctx context.Context, productTypeID uuid.UUID) error {
	return x.q.EnqueueProductTypeEmbeddings(ctx, productTypeID)
}

// Run works through changed and never-embedded sources until there are none or the run's time is spent
func (x *Indexer) Run(ctx context.Context) error {
	model := x.emb.Model(ctx)
	if model == "" {
		return nil
	}
	if x.pruned.Load() != model {
		n, err := x.q.PruneStaleModelEmbeddings(ctx, db.PruneStaleModelEmbeddingsParams{Model: model, Lim: pruneSize})
		if err != nil {
			return fmt.Errorf("embedding: prune other models: %w", err)
		}
		if n < pruneSize {
			x.pruned.Store(model)
		}
	}
	deadline := time.Now().Add(runBudget)
	var passages, tokens int64
	defer func() {
		if passages > 0 {
			slog.InfoContext(ctx, "embedding: pass done", "model", model, "passages", passages, "tokens", tokens)
		}
	}()
	for time.Now().Before(deadline) {
		pending, err := x.q.PendingEmbeddings(ctx, passSize)
		if err != nil {
			return fmt.Errorf("embedding: read the queue: %w", err)
		}
		if len(pending) == 0 {
			more, err := x.backfill(ctx, model)
			if err != nil || !more {
				return err
			}
			continue
		}
		n, t, err := x.pass(ctx, model, pending)
		passages, tokens = passages+n, tokens+t
		if err != nil {
			return err
		}
	}
	return nil
}

// backfill queues what has never been embedded with this model - the first run, a new model, an item saved while the job was down
func (x *Indexer) backfill(ctx context.Context, model string) (bool, error) {
	products, err := x.q.ProductsMissingEmbeddings(ctx, db.ProductsMissingEmbeddingsParams{Model: model, Lim: backfillSize})
	if err != nil {
		return false, fmt.Errorf("embedding: find products to embed: %w", err)
	}
	pages, err := x.q.PagesMissingEmbeddings(ctx, db.PagesMissingEmbeddingsParams{Model: model, Lim: backfillSize})
	if err != nil {
		return false, fmt.Errorf("embedding: find pages to embed: %w", err)
	}
	if err := x.Enqueue(ctx, SourceProduct, products...); err != nil {
		return false, err
	}
	if err := x.Enqueue(ctx, SourcePage, pages...); err != nil {
		return false, err
	}
	return len(products)+len(pages) > 0, nil
}

// doc is one source in one locale, already split into passages
type doc struct {
	source   string
	id       uuid.UUID
	locale   string
	passages []string
}

type work struct {
	d     *doc
	chunk int
	text  string
	hash  []byte
}

func (x *Indexer) pass(ctx context.Context, model string, pending []db.PendingEmbeddingsRow) (int64, int64, error) {
	ids := map[string][]uuid.UUID{}
	for _, p := range pending {
		ids[p.Source] = append(ids[p.Source], p.SourceID)
	}
	docs, err := x.products(ctx, ids[SourceProduct])
	if err != nil {
		return 0, 0, err
	}
	pages, err := x.pages(ctx, ids[SourcePage])
	if err != nil {
		return 0, 0, err
	}
	docs = append(docs, pages...)

	// Only passages whose words changed go to the provider - an edit to price or stock costs nothing
	held := map[string][]byte{}
	for source, list := range ids {
		rows, err := x.q.EmbeddingHashes(ctx, db.EmbeddingHashesParams{Source: source, Ids: list, Model: model})
		if err != nil {
			return 0, 0, fmt.Errorf("embedding: read hashes: %w", err)
		}
		for _, r := range rows {
			held[key(source, r.SourceID, r.Locale, int(r.Chunk))] = r.ContentHash
		}
	}
	var todo []work
	for i := range docs {
		d := &docs[i]
		for c, text := range d.passages {
			sum := sha256.Sum256([]byte(text))
			if h, ok := held[key(d.source, d.id, d.locale, c)]; !ok || string(h) != string(sum[:]) {
				todo = append(todo, work{d: d, chunk: c, text: text, hash: sum[:]})
			}
		}
	}
	vectors := make([][]float32, len(todo))
	var tokens int64
	embed := func(from, to int) error {
		texts := make([]string, 0, to-from)
		for _, w := range todo[from:to] {
			texts = append(texts, w.text)
		}
		vecs, used, err := x.emb.Embed(ctx, ai.ForDocument, texts)
		if err == nil {
			copy(vectors[from:], vecs)
			tokens += used
		}
		return err
	}
	refused := map[*doc]bool{}
	var firstRefusal error
	for start := 0; start < len(todo); start += ai.MaxEmbedBatch {
		end := min(start+ai.MaxEmbedBatch, len(todo))
		err := embed(start, end)
		if !errors.Is(err, ai.ErrEmbeddingRefused) {
			if err != nil {
				return 0, tokens, fmt.Errorf("embedding: %w", err)
			}
			continue
		}
		// one text the provider won't take mustn't hold the rest - each goes alone, and a refused one waits out a pause
		for i := start; i < end; i++ {
			if end-start > 1 {
				err = embed(i, i+1)
			}
			if errors.Is(err, ai.ErrEmbeddingRefused) {
				refused[todo[i].d], firstRefusal = true, cmp.Or(firstRefusal, err)
			} else if err != nil {
				return 0, tokens, fmt.Errorf("embedding: %w", err)
			}
		}
	}

	bySource := map[string][]*doc{}
	for i := range docs {
		k := docs[i].source + ":" + docs[i].id.String()
		bySource[k] = append(bySource[k], &docs[i])
	}
	changed := map[*doc][]int{}
	for i, w := range todo {
		changed[w.d] = append(changed[w.d], i)
	}
	for _, p := range pending {
		list := bySource[p.Source+":"+p.SourceID.String()]
		if slices.ContainsFunc(list, func(d *doc) bool { return refused[d] }) {
			if err := x.q.DeferEmbedding(ctx, db.DeferEmbeddingParams(p)); err != nil {
				return int64(len(todo)), tokens, fmt.Errorf("embedding: defer %s %s: %w", p.Source, p.SourceID, err)
			}
			continue
		}
		err := x.tx.InTransaction(ctx, func(tx *sql.Tx) error {
			q := x.q.WithTx(tx)
			done := db.FinishEmbeddingParams(p)
			if len(list) == 0 { // gone, unpublished or no longer something a shopper finds
				if err := q.DeleteSourceEmbeddings(ctx, db.DeleteSourceEmbeddingsParams{Source: p.Source, SourceID: p.SourceID}); err != nil {
					return err
				}
				return q.FinishEmbedding(ctx, done)
			}
			locales, chunks := make([]string, len(list)), make([]int16, len(list))
			for i, d := range list {
				locales[i], chunks[i] = d.locale, int16(len(d.passages))
				for _, j := range changed[d] {
					w := todo[j]
					if err := q.UpsertEmbedding(ctx, db.UpsertEmbeddingParams{Source: d.source, SourceID: d.id, Locale: d.locale, Model: model,
						Chunk: int16(w.chunk), Content: w.text, ContentHash: w.hash, Embedding: vectorLiteral(vectors[j])}); err != nil {
						return err
					}
				}
			}
			if err := q.TrimEmbeddings(ctx, db.TrimEmbeddingsParams{Source: p.Source, SourceID: p.SourceID, Model: model, Locales: locales, Chunks: chunks}); err != nil {
				return err
			}
			return q.FinishEmbedding(ctx, done)
		})
		if err != nil {
			return int64(len(todo)), tokens, fmt.Errorf("embedding: write %s %s: %w", p.Source, p.SourceID, err)
		}
	}
	if firstRefusal != nil {
		return int64(len(todo)), tokens, fmt.Errorf("embedding: %d refused by the provider, retried later: %w", len(refused), firstRefusal)
	}
	return int64(len(todo)), tokens, nil
}

func key(source string, id uuid.UUID, locale string, chunk int) string {
	return source + "|" + id.String() + "|" + locale + "|" + strconv.Itoa(chunk)
}

// pgvector's text form, which the query casts - no driver type needed for a value only ever written
func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 10)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (x *Indexer) products(ctx context.Context, ids []uuid.UUID) ([]doc, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := x.q.EmbeddingProductContent(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("embedding: read products: %w", err)
	}
	cats, err := x.q.EmbeddingProductCategories(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("embedding: read categories: %w", err)
	}
	attrs, err := x.q.EmbeddingPublicAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("embedding: read attributes: %w", err)
	}
	values, err := x.q.EmbeddingProductAttributeValues(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("embedding: read attribute values: %w", err)
	}
	localValues, err := x.q.EmbeddingProductLocaleValues(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("embedding: read localized values: %w", err)
	}
	return productDocs(rows, cats, attrs, values, localValues), nil
}

type attribute struct {
	id         uuid.UUID
	slug, unit string
	name       localized
}

// localized is a name with its translations - a locale without one reads the default
type localized struct {
	base  string
	names map[string]string
}

func (l localized) in(locale string) string {
	if n := l.names[locale]; n != "" {
		return n
	}
	return l.base
}

func productDocs(rows []db.EmbeddingProductContentRow, cats []db.EmbeddingProductCategoriesRow, attrRows []db.EmbeddingPublicAttributesRow,
	valueRows []db.EmbeddingProductAttributeValuesRow, localRows []db.EmbeddingProductLocaleValuesRow) []doc {
	var attrs []*attribute
	byID, bySlug := map[uuid.UUID]*attribute{}, map[string]*attribute{}
	for _, r := range attrRows {
		a := byID[r.ID]
		if a == nil {
			a = &attribute{id: r.ID, slug: r.Slug, unit: r.Unit, name: localized{base: r.Name, names: map[string]string{}}}
			byID[r.ID], bySlug[r.Slug] = a, a
			attrs = append(attrs, a)
		}
		if r.Locale != "" {
			a.name.names[r.Locale] = r.LocalName
		}
	}
	// A product's chosen values per attribute, in their order, each with its translations
	type chosen struct {
		attr   uuid.UUID
		values []localized
	}
	picked := map[uuid.UUID][]*chosen{}
	for _, r := range valueRows {
		list := picked[r.ProductID]
		var c *chosen
		if n := len(list); n > 0 && list[n-1].attr == r.AttributeID {
			c = list[n-1]
		} else {
			c = &chosen{attr: r.AttributeID}
			picked[r.ProductID] = append(list, c)
		}
		if n := len(c.values); n > 0 && c.values[n-1].base == r.Name {
			if r.Locale != "" {
				c.values[n-1].names[r.Locale] = r.LocalName
			}
			continue
		}
		v := localized{base: r.Name, names: map[string]string{}}
		if r.Locale != "" {
			v.names[r.Locale] = r.LocalName
		}
		c.values = append(c.values, v)
	}
	local := map[string]string{}
	for _, r := range localRows {
		local[r.ProductID.String()+"|"+r.AttributeID.String()+"|"+r.Locale] = r.Value
	}
	categories := map[string][]string{}
	for _, r := range cats {
		k := r.ProductID.String() + "|" + r.Locale
		if !slices.Contains(categories[k], r.Name) {
			categories[k] = append(categories[k], r.Name)
		}
	}

	out := make([]doc, 0, len(rows))
	for _, r := range rows {
		if tidy(r.Name) == "" {
			continue
		}
		var facts []string
		for _, c := range picked[r.ProductID] {
			if a := byID[c.attr]; a != nil {
				names := make([]string, len(c.values))
				for i, v := range c.values {
					names[i] = v.in(r.Locale)
				}
				facts = append(facts, a.name.in(r.Locale)+": "+strings.Join(names, ", "))
			}
		}
		facts = append(facts, scalarFacts(r.Attributes, attrs, bySlug, local, r.ProductID.String(), r.Locale)...)
		text := productText{
			name: r.Name, kind: r.ProductType, short: richText(r.ShortDescription.RawMessage), long: richText(r.Description.RawMessage),
			categories: categories[r.ProductID.String()+"|"+r.Locale], facts: facts,
		}.String()
		out = append(out, doc{source: SourceProduct, id: r.ProductID, locale: r.Locale, passages: []string{truncate(text)}})
	}
	return out
}

// The values kept on the product itself - numbers, yes/no, free text - for the public attributes only, in the attributes' order
func scalarFacts(raw json.RawMessage, attrs []*attribute, bySlug map[string]*attribute, local map[string]string, productID, locale string) []string {
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	var out []string
	for _, a := range attrs {
		if text := local[productID+"|"+a.id.String()+"|"+locale]; text != "" {
			out = append(out, a.name.in(locale)+": "+tidy(text))
			continue
		}
		v, ok := values[a.slug]
		if !ok || bySlug[a.slug] != a {
			continue
		}
		var s string
		switch t := v.(type) {
		case string:
			s = t
		case float64:
			s = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			if !t {
				continue
			}
			s = "yes"
		default:
			continue
		}
		if s = tidy(s); s == "" {
			continue
		}
		if a.unit != "" {
			s += " " + a.unit
		}
		out = append(out, a.name.in(locale)+": "+s)
	}
	return out
}

func (x *Indexer) pages(ctx context.Context, ids []uuid.UUID) ([]doc, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := x.q.EmbeddingPageContent(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("embedding: read pages: %w", err)
	}
	out := make([]doc, 0, len(rows))
	for _, r := range rows {
		passages := pagePassages(r.Title, r.MetaDescription.String, r.Body.RawMessage)
		for i := range passages {
			passages[i] = truncate(passages[i])
		}
		if len(passages) > 0 {
			out = append(out, doc{source: SourcePage, id: r.PageID, locale: r.Locale, passages: passages})
		}
	}
	return out, nil
}

func truncate(s string) string {
	if r := []rune(s); len(r) > ai.MaxEmbedChars {
		return string(r[:ai.MaxEmbedChars])
	}
	return s
}

// Status is where embedding stands, for one store when a channel is given
type Status struct {
	Model            string
	Queued           int64
	Oldest           time.Time
	Products, Pages  int64 // what the store shows, and how much of it is embedded with the model
	EmbeddedProducts int64
	EmbeddedPages    int64
	Failed           string // the job's last run, when it failed
}

func (x *Indexer) Status(ctx context.Context, channelID *uuid.UUID) (Status, error) {
	st := Status{Model: x.emb.Model(ctx)}
	if st.Model == "" {
		return st, nil
	}
	backlog, err := x.q.EmbeddingBacklog(ctx)
	if err != nil {
		return st, fmt.Errorf("embedding: read the queue: %w", err)
	}
	st.Queued, st.Oldest = backlog.Queued, backlog.Oldest
	run, err := x.q.EmbeddingJobLastRun(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows): // not registered in this process, or not seeded yet
	case err != nil:
		return st, fmt.Errorf("embedding: read the job: %w", err)
	case run.LastStatus == "error":
		st.Failed = run.LastError
	}
	if channelID != nil {
		cov, err := x.q.EmbeddingCoverage(ctx, db.EmbeddingCoverageParams{ChannelID: *channelID, Model: st.Model})
		if err != nil {
			return st, fmt.Errorf("embedding: coverage: %w", err)
		}
		st.Products, st.EmbeddedProducts, st.Pages, st.EmbeddedPages = cov.Products, cov.EmbeddedProducts, cov.Pages, cov.EmbeddedPages
	}
	return st, nil
}

// Health reports the job for System health: off without a model, a warning when it fails or falls behind
func (x *Indexer) Health(ctx context.Context) health.Result {
	st, err := x.Status(ctx, nil)
	switch {
	case err != nil:
		return health.Bad(err.Error(), "")
	case st.Model == "":
		return health.Off("No embedding model set", "Set one under Settings, AI, Embeddings to let the assistant find products and pages by meaning")
	case st.Failed != "":
		return health.Warn(fmt.Sprintf("%d waiting - the last run failed: %s", st.Queued, st.Failed), "Check the embedding provider under Settings, AI, Embeddings")
	case st.Queued > 0 && time.Since(st.Oldest) > StaleBacklog:
		return health.Warn(fmt.Sprintf("%d waiting, the oldest for %s", st.Queued, time.Since(st.Oldest).Round(time.Minute)), "The embedding job isn't keeping up - check the scheduled jobs")
	case st.Queued > 0:
		return health.OK(fmt.Sprintf("%s - %d waiting", st.Model, st.Queued))
	}
	return health.OK(st.Model + " - up to date")
}

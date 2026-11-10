package search

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
)

const (
	pathMeaningMode         = "search/products/meaning"
	pathMeaningStrictness   = "search/products/meaning_strictness"
	OverviewPath            = "search/products/ai_overview"
	OverviewOn              = "meaning"
	MeaningPacePath         = "search/products/meaning_per_minute"
	MeaningPaceDefault      = 60
	MeaningStorePacePath    = "search/products/meaning_store_per_minute"
	MeaningStorePaceDefault = 1200
	meaningOff              = "off"
	meaningNatural          = "natural"
	meaningAlways           = "always"
	strictBalanced          = "balanced"

	meaningTake = 48
	scopedTake  = 200
	FusionK     = 60
	// low damping keeps meaning's order in a sentence - a word match lifts a product a few places
	sentenceK           = 5
	sentenceWordsWeight = 0.25
	meaningTimeout      = 1500 * time.Millisecond
)

type reach struct{ sentence, rescue float64 }

// how far past the nearest a hit may count, as a share of the nearest's distance capped at 0.4 - a model with bunched-up distances
// gets a tighter cut, and a weak best match doesn't widen it
var reaches = map[string]reach{"strict": {0.2, 0.08}, strictBalanced: {0.4, 0.13}, "broad": {0.65, 0.26}}

func reachOf(strictness string) reach {
	if r, ok := reaches[strictness]; ok {
		return r
	}
	return reaches[strictBalanced]
}

// Within is whether a hit is close enough to the nearest one to count
func Within(nearest, distance, share float64) bool { return distance <= nearest+cutOf(nearest, share) }

func cutOf(nearest, share float64) float64 { return min(max(nearest, 0.05), 0.4) * share }

type Near struct {
	ID       uuid.UUID
	Distance float64
}

type Meaning interface {
	Near(ctx context.Context, channelID uuid.UUID, locales []string, query string, limit int) ([]Near, error)
}

type Settings interface {
	GetString(ctx context.Context, path string, scope config.Scope) (string, error)
}

type wordsOnlyKey struct{}

// the assistant blends meaning in itself, so its listings stay on the words
func WordsOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, wordsOnlyKey{}, true)
}

func wordsOnly(ctx context.Context) bool { v, _ := ctx.Value(wordsOnlyKey{}).(bool); return v }

func init() {
	config.Register(config.Definition{Path: pathMeaningMode, Type: config.TypeSelect, Default: meaningNatural, Label: "Search by meaning", Scoping: config.ScopingChannel,
		Description: "Needs embeddings under AI. For sentences, a query that reads like one - \"something warm for winter hikes\" - lists the products closest in what it means, " +
			"and a keyword search that matches nothing tries meaning before giving up. Every search also blends meaning into keyword searches that find few products. Off keeps the assistant on words too.",
		Options: []config.Option{
			{Value: meaningOff, Label: "Off"},
			{Value: meaningNatural, Label: "For sentences"},
			{Value: meaningAlways, Label: "Every search"},
		}})
	config.Register(config.Definition{Path: pathMeaningStrictness, Type: config.TypeSelect, Default: strictBalanced, Label: "Search by meaning strictness", Scoping: config.ScopingChannel,
		Description: "How close to the best match a product must be to be listed by meaning. Strict lists only near twins of what was asked for, broad reaches further for related products.",
		Options: []config.Option{
			{Value: "strict", Label: "Strict"},
			{Value: strictBalanced, Label: "Balanced"},
			{Value: "broad", Label: "Broad"},
		}})
	config.Register(config.Definition{Path: MeaningPacePath, Type: config.TypeInt, Default: strconv.Itoa(MeaningPaceDefault), Scoping: config.ScopingChannel,
		Label:       "Most new questions one shopper's searches send the embedding provider a minute (0 = no limit)",
		Description: "Each is a paid call. A question already asked is free, and past the limit the shopper's searches run on their words. Counted per signed-in customer, else per address - a whole IPv6 /64 as one."})
	config.Register(config.Definition{Path: MeaningStorePacePath, Type: config.TypeInt, Default: strconv.Itoa(MeaningStorePaceDefault), Scoping: config.ScopingChannel,
		Label:       "Most new questions the whole store's searches send the embedding provider a minute (0 = no limit)",
		Description: "A ceiling on what search by meaning can spend however many shoppers are searching, counted across every server. Past it, searches run on their words until the minute is out."})
	config.Register(config.Definition{Path: OverviewPath, Type: config.TypeSelect, Default: meaningOff, Label: "AI overview", Scoping: config.ScopingChannel,
		Description: "A few sentences above the results that answer what the shopper asked, written from the products listed, with a button to carry on in the shopping assistant's chat. Only for searches meaning answers, and only while the assistant is on.",
		Options: []config.Option{
			{Value: meaningOff, Label: "Off"},
			{Value: OverviewOn, Label: "When meaning answers"},
		}})
}

type trialKey struct{}

// Trying makes the searches under ctx read these choices in place of the store's saved ones - empty keeps the saved one
func Trying(ctx context.Context, mode, strictness string) context.Context {
	return context.WithValue(ctx, trialKey{}, map[string]string{pathMeaningMode: mode, pathMeaningStrictness: strictness})
}

func setting(ctx context.Context, s Settings, channelID uuid.UUID, path, fallback string) string {
	if trial, _ := ctx.Value(trialKey{}).(map[string]string); trial[path] != "" {
		return trial[path]
	}
	if s == nil {
		return fallback
	}
	v, err := s.GetString(ctx, path, config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID})
	if err != nil || v == "" {
		return fallback
	}
	return v
}

// MeaningReach is how far meaning reaches on a store's searches, false when the store turned search by meaning off
func MeaningReach(ctx context.Context, s Settings, channelID uuid.UUID) (float64, bool) {
	if setting(ctx, s, channelID, pathMeaningMode, meaningNatural) == meaningOff {
		return 0, false
	}
	return reachOf(setting(ctx, s, channelID, pathMeaningStrictness, strictBalanced)).sentence, true
}

func MeaningLocales(active, storeDefault string) []string {
	out := []string{}
	for _, l := range []string{active, storeDefault} {
		if l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

var sentenceWords = map[string]map[string]bool{
	"en": set("i", "i'm", "im", "me", "my", "we", "you", "need", "want", "looking", "something", "anything", "some", "for", "with",
		"without", "that", "which", "what", "how", "where", "can", "do", "does", "is", "are", "to", "would", "should"),
	"de": set("ich", "mir", "mich", "wir", "suche", "brauche", "möchte", "etwas", "was", "welche", "welcher", "wie", "für", "mit",
		"ohne", "gibt", "habt", "kann", "können"),
	"fr": set("je", "j'ai", "moi", "mon", "ma", "nous", "cherche", "besoin", "veux", "voudrais", "quelque", "pour", "avec", "sans",
		"quel", "quelle", "comment", "avez", "est"),
	"es": set("yo", "me", "mi", "busco", "necesito", "quiero", "algo", "para", "con", "sin", "qué", "cuál", "cómo", "tienen", "hay"),
	"it": set("io", "mi", "mio", "cerco", "ho", "bisogno", "voglio", "vorrei", "qualcosa", "per", "con", "senza", "quale", "come", "avete"),
	"nl": set("ik", "mij", "mijn", "we", "zoek", "nodig", "wil", "iets", "voor", "met", "zonder", "welke", "hoe", "hebben", "is"),
	"pt": set("eu", "me", "meu", "minha", "procuro", "preciso", "quero", "algo", "para", "com", "sem", "qual", "como", "têm", "tem"),
}

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

func NaturalLanguage(query, locale string) bool {
	if strings.ContainsAny(query, "?¿") {
		return true
	}
	lang, _, _ := strings.Cut(strings.ToLower(locale), "-")
	joins := sentenceWords[lang]
	tokens := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
	})
	words, joined := 0, false
	for _, t := range tokens {
		if strings.IndexFunc(t, unicode.IsDigit) >= 0 {
			continue
		}
		words++
		joined = joined || joins[t]
	}
	return words >= 4 || (words >= 2 && joined)
}

type Reading string

const (
	ReadsWords    Reading = "words"
	ReadsKeywords Reading = "keywords"
	ReadsSentence Reading = "sentence"
	ReadsBlended  Reading = "blended"
)

func reading(mode, query, locale string) Reading {
	switch {
	case mode == meaningOff:
		return ReadsWords
	case NaturalLanguage(query, locale):
		return ReadsSentence
	case mode == meaningAlways:
		return ReadsBlended
	}
	return ReadsKeywords
}

type Hybrid struct {
	engine   Engine
	meaning  Meaning
	cfg      *config.Service
	settings Settings
}

func NewHybrid(engine Engine, meaning Meaning, cfg *config.Service) *Hybrid {
	h := &Hybrid{engine: engine, meaning: meaning, cfg: cfg}
	if cfg != nil {
		h.settings = cfg
	}
	return h
}

type BrowseRequest struct {
	ChannelID uuid.UUID
	Locale    string
	Locales   []string
	// Query is as typed, Words has the synonyms expanded
	Query, Words  string
	Filter        BrowseFilter
	Sort          string
	Limit, Offset int
	Boosts        map[uuid.UUID]float64
}

type Answer string

const (
	AnsweredWords   Answer = "words"
	AnsweredMeaning Answer = "meaning"
	AnsweredBlended Answer = "blended"
)

type Considered struct {
	Near
	Counted bool
}

type Explanation struct {
	Reads      Reading
	Answered   Answer
	Margin     float64
	Near       []Considered
	Words      []uuid.UUID
	WordsTotal int64
	Failed     string
}

func (h *Hybrid) Browse(ctx context.Context, r BrowseRequest) (BrowseResult, error) {
	return h.run(ctx, r, nil)
}

func (h *Hybrid) Explain(ctx context.Context, r BrowseRequest) (BrowseResult, Explanation, error) {
	var ex Explanation
	res, err := h.run(ctx, r, &ex)
	return res, ex, err
}

func (h *Hybrid) run(ctx context.Context, r BrowseRequest, ex *Explanation) (BrowseResult, error) {
	query := strings.TrimSpace(r.Query)
	reads := ReadsWords
	if h.meaning != nil && query != "" && !wordsOnly(ctx) && len(r.Filter.ProductIDs) == 0 { // a category page never reads the settings
		reads = reading(setting(ctx, h.settings, r.ChannelID, pathMeaningMode, meaningNatural), query, r.Locale)
	}
	if ex != nil {
		ex.Reads, ex.Answered = reads, AnsweredWords
	}
	words := func() (BrowseResult, error) {
		res, err := h.engine.BrowseProducts(ctx, r.ChannelID, r.Locale, r.Words, r.Filter, r.Sort, r.Limit, r.Offset, r.Boosts)
		if ex != nil && err == nil {
			ex.Words, ex.WordsTotal = res.IDs, res.Total
		}
		return res, err
	}
	if reads == ReadsWords {
		return words()
	}
	rc := reachOf(setting(ctx, h.settings, r.ChannelID, pathMeaningStrictness, strictBalanced))
	if reads == ReadsSentence {
		if blended, ok := h.blend(ctx, r, false, rc.sentence, ex); ok {
			return blended, nil
		}
		return words()
	}
	res, err := words()
	// keywords stay on words that found something; every search leaves alone words that found plenty, or the blend would cap the listing
	if err != nil || (reads == ReadsKeywords && res.Total > 0) || res.Total > meaningTake {
		return res, err
	}
	share := rc.rescue
	if res.Total > 0 {
		share = rc.sentence
	}
	if blended, ok := h.blend(ctx, r, res.Total > 0, share, ex); ok {
		blended.Rescued = res.Total == 0 && blended.Total > 0
		return blended, nil
	}
	return res, nil
}

func (h *Hybrid) blend(ctx context.Context, r BrowseRequest, union bool, share float64, ex *Explanation) (BrowseResult, bool) {
	scope := r.Filter // the shopper's filters hold for both lists, or the words' list would be cut before they apply
	scope.ProductIDs, scope.NoFacets = nil, true
	scoped := len(scope.CategorySet()) > 0 || scope.CollectionID != nil || len(scope.Kinds) > 0 || len(scope.AttributeIn) > 0 ||
		scope.PriceMinCents != nil || scope.PriceMaxCents != nil
	take := meaningTake
	if scoped {
		take = scopedTake
	}
	nctx, cancel := context.WithTimeout(ctx, meaningTimeout)
	near, err := h.meaning.Near(nctx, r.ChannelID, r.Locales, strings.TrimSpace(r.Query), take)
	cancel()
	if err != nil {
		if !errors.Is(err, ratelimit.ErrLimited) {
			slog.WarnContext(ctx, "search: by meaning failed, words only", "channel", r.ChannelID, "err", err)
		}
		if ex != nil {
			ex.Failed = err.Error()
		}
		return BrowseResult{}, false
	}
	if scoped && len(near) > 0 {
		in, err := h.members(ctx, r, scope, idsOf(near))
		if err != nil {
			return BrowseResult{}, false
		}
		near = slices.DeleteFunc(near, func(n Near) bool { return !in[n.ID] })
	}
	var reached []uuid.UUID
	for _, n := range near {
		counted := Within(near[0].Distance, n.Distance, share)
		if counted && len(reached) < meaningTake {
			reached = append(reached, n.ID)
		}
		if ex != nil {
			ex.Near = append(ex.Near, Considered{Near: n, Counted: counted})
		}
	}
	if ex != nil && len(near) > 0 {
		ex.Margin = cutOf(near[0].Distance, share)
	}
	if len(reached) == 0 {
		return BrowseResult{}, false
	}
	var matched []uuid.UUID
	if w, err := h.engine.BrowseProducts(ctx, r.ChannelID, r.Locale, r.Words, scope, "", meaningTake, 0, r.Boosts); err == nil {
		matched = w.IDs
		if ex != nil {
			ex.Words, ex.WordsTotal = w.IDs, w.Total
		}
	}
	var order []uuid.UUID
	if union {
		order = rank(matched, reached, 1, FusionK, r.Boosts)
	} else {
		// ranked among what meaning found, or a product's variants crowding the words' list push it down
		matched = slices.DeleteFunc(slices.Clone(matched), func(id uuid.UUID) bool { return !slices.Contains(reached, id) })
		order = rank(reached, matched, sentenceWordsWeight, sentenceK, r.Boosts)
	}
	res, err := h.narrowed(ctx, r, order)
	if err != nil || res.Total == 0 {
		if err != nil {
			slog.WarnContext(ctx, "search: listing what meaning found failed, words only", "channel", r.ChannelID, "err", err)
		}
		return BrowseResult{}, false
	}
	res.Meaning = !union
	if ex != nil {
		ex.Answered = AnsweredMeaning
		if union {
			ex.Answered = AnsweredBlended
		}
	}
	return res, true
}

// members is which of the given products a listing filter keeps, read through the engine so it means what the listing means
func (h *Hybrid) members(ctx context.Context, r BrowseRequest, f BrowseFilter, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	f.ProductIDs, f.NoFacets = ids, true
	in := map[uuid.UUID]bool{}
	for {
		page, err := h.engine.BrowseProducts(ctx, r.ChannelID, r.Locale, "", f, "", len(ids), len(in), nil)
		if err != nil {
			return nil, err
		}
		before := len(in)
		for _, id := range page.IDs {
			in[id] = true
		}
		if int64(len(in)) >= page.Total || len(in) == before {
			return in, nil
		}
	}
}

// the shopper's filters, the facets and any sort they picked apply as on every listing - relevance is the blend's own order, so that page is cut here
func (h *Hybrid) narrowed(ctx context.Context, r BrowseRequest, order []uuid.UUID) (BrowseResult, error) {
	f := r.Filter
	f.ProductIDs = order
	if resolveSortOption(r.Sort, loadSortOptions(ctx, h.cfg)).Field != sortFieldScore {
		return h.engine.BrowseProducts(ctx, r.ChannelID, r.Locale, "", f, r.Sort, r.Limit, r.Offset, nil)
	}
	all, err := h.engine.BrowseProducts(ctx, r.ChannelID, r.Locale, "", f, "", len(order), 0, nil)
	if err != nil {
		return BrowseResult{}, err
	}
	kept := map[uuid.UUID]bool{}
	for _, id := range all.IDs {
		kept[id] = true
	}
	if int64(len(kept)) < all.Total {
		more, err := h.members(ctx, r, f, order)
		if err != nil {
			return BrowseResult{}, err
		}
		kept = more
	}
	ranked := slices.DeleteFunc(slices.Clone(order), func(id uuid.UUID) bool { return !kept[id] })
	if cfgStringOr(ctx, h.cfg, pathOutOfStockDisplay, oosShow) == oosLast {
		inStock := f
		inStock.InStockOnly = true
		if stocked, err := h.members(ctx, r, inStock, ranked); err == nil {
			slices.SortStableFunc(ranked, func(a, b uuid.UUID) int { return boolRank(stocked[b]) - boolRank(stocked[a]) })
		}
	}
	limit := r.Limit
	if limit <= 0 {
		limit = int(cfgIntOr(ctx, h.cfg, pathBrowsePageSizeDefault, 24))
	}
	limit = min(limit, int(cfgIntOr(ctx, h.cfg, pathBrowsePageSizeMax, 100)))
	from := min(max(r.Offset, 0), len(ranked))
	return BrowseResult{IDs: ranked[from:min(from+limit, len(ranked))], Total: int64(len(ranked)), Facets: all.Facets}, nil
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

func idsOf(near []Near) []uuid.UUID {
	out := make([]uuid.UUID, len(near))
	for i, n := range near {
		out[i] = n.ID
	}
	return out
}

// Fuse ranks two lists together, the second weighed by weight: high in either comes out high, in both higher still. Ties keep the first list's order
func Fuse[T comparable](first, second []T, weight float64, k int) []T {
	return fuse(first, second, weight, k, nil)
}

// rank is fuse with the store's merchandising boosts folded in, the way they weigh a words search
func rank(first, second []uuid.UUID, weight float64, k int, boosts map[uuid.UUID]float64) []uuid.UUID {
	return fuse(first, second, weight, k, func(id uuid.UUID) float64 { return merchFactor(boosts[id]) })
}

func fuse[T comparable](first, second []T, weight float64, k int, factor func(T) float64) []T {
	score := map[T]float64{}
	var order []T
	for n, list := range [][]T{first, second} {
		w := 1.0
		if n == 1 {
			w = weight
		}
		for i, id := range list {
			if _, seen := score[id]; !seen {
				order = append(order, id)
			}
			score[id] += w / float64(k+i+1)
		}
	}
	if factor != nil {
		for id := range score {
			score[id] *= factor(id)
		}
	}
	slices.SortStableFunc(order, func(a, b T) int {
		switch {
		case score[a] > score[b]:
			return -1
		case score[a] < score[b]:
			return 1
		}
		return 0
	})
	return order
}

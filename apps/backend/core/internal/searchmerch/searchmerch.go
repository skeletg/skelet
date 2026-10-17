// Package searchmerch is operator-managed search merchandising - synonym groups (expanded into the query at search time, no reindex), curated pins (forced to the top), boosts, and query redirects ("help" -> /pages/faq) - The hot search path reads one cached per-channel snapshot
package searchmerch

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

var (
	ErrRedirectTarget = apperr.New("SEARCH_REDIRECT_TARGET_INVALID", apperr.ClassValidation, "target must be a site path like /pages/faq or an http(s) URL")
	ErrNotListed      = apperr.New("SEARCH_MERCH_PRODUCT_NOT_LISTED", apperr.ClassValidation, "that product isn't listed on this channel")
)

func init() {
	permission.RegisterGroup(permission.Group{
		Slug:      "searchmerch",
		Name:      "Search Merchandising",
		SortOrder: 77,
	})
	permission.Register(permission.Definition{
		Group: "searchmerch", Slug: "searchmerch.manage", Name: "Manage Synonyms / Pins / Redirects", SortOrder: 1,
	})
}

type Synonym struct {
	ID        uuid.UUID
	ChannelID *uuid.UUID // nil = global
	Terms     string
	CreatedAt time.Time
}

type Pin struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Query     string
	ProductID uuid.UUID
	Position  int32
}

// Boost lifts (positive weight) or buries (negative) one product's score for a normalized query - score-based
// merchandising, softer than a pin's forced position
type Boost struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Query     string
	ProductID uuid.UUID
	Weight    float64
}

type Redirect struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Query     string
	TargetURL string
}

type Service struct {
	queries *db.Queries
	ttl     time.Duration

	mu    sync.Mutex
	snaps map[uuid.UUID]*snapshot
	gen   uint64
}

func NewService(queries *db.Queries) *Service {
	return &Service{queries: queries, ttl: 30 * time.Second, snaps: map[uuid.UUID]*snapshot{}}
}

// One channel's merchandising, loaded together - four reads per keystroke became one load per TTL. Writes here drop it at once,
// and the TTL bounds what another replica's write leaves stale
type snapshot struct {
	loaded    time.Time
	redirects map[string]string
	synonyms  [][]string
	pins      map[string][]uuid.UUID
	boosts    map[string]map[uuid.UUID]float64
}

func (s *Service) snapshot(ctx context.Context, channelID uuid.UUID) *snapshot {
	s.mu.Lock()
	snap, gen := s.snaps[channelID], s.gen
	s.mu.Unlock()
	if snap != nil && time.Since(snap.loaded) < s.ttl {
		return snap
	}
	snap, err := s.load(ctx, channelID)
	if err != nil {
		log.Printf("[searchmerch] load channel %s: %v", channelID, err)
		return &snapshot{}
	}
	s.mu.Lock()
	// A write that landed mid-load already dropped what this read - storing it would serve the old rows for a whole TTL
	if s.gen == gen {
		s.snaps[channelID] = snap
	}
	s.mu.Unlock()
	return snap
}

func (s *Service) load(ctx context.Context, channelID uuid.UUID) (*snapshot, error) {
	snap := &snapshot{
		loaded:    time.Now(),
		redirects: map[string]string{},
		pins:      map[string][]uuid.UUID{},
		boosts:    map[string]map[uuid.UUID]float64{},
	}
	redirects, err := s.queries.ListSearchRedirects(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("redirects: %w", err)
	}
	for _, r := range redirects {
		if !validTarget(r.TargetUrl) {
			log.Printf("[searchmerch] redirect %q on channel %s has an unsafe target, skipped", r.Query, channelID)
			continue
		}
		snap.redirects[r.Query] = r.TargetUrl
	}
	synonyms, err := s.queries.ListSearchSynonyms(ctx, uuid.NullUUID{UUID: channelID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("synonyms: %w", err)
	}
	for _, r := range synonyms {
		snap.synonyms = append(snap.synonyms, splitTerms(r.Terms))
	}
	pins, err := s.queries.ListSearchPins(ctx, db.ListSearchPinsParams{ChannelID: channelID})
	if err != nil {
		return nil, fmt.Errorf("pins: %w", err)
	}
	for _, r := range pins {
		snap.pins[r.Query] = append(snap.pins[r.Query], r.ProductID)
	}
	boosts, err := s.queries.ListSearchBoosts(ctx, db.ListSearchBoostsParams{ChannelID: channelID})
	if err != nil {
		return nil, fmt.Errorf("boosts: %w", err)
	}
	for _, r := range boosts {
		if snap.boosts[r.Query] == nil {
			snap.boosts[r.Query] = map[uuid.UUID]float64{}
		}
		snap.boosts[r.Query][r.ProductID] = r.Weight
	}
	return snap, nil
}

func (s *Service) invalidate(channelID *uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	if channelID == nil {
		clear(s.snaps)
		return
	}
	delete(s.snaps, *channelID)
}

// A pin or boost on a product the channel doesn't list is a row that looks live in the dashboard and never shows
func (s *Service) requireListed(ctx context.Context, channelID, productID uuid.UUID) error {
	_, err := s.queries.GetProductChannelListing(ctx, db.GetProductChannelListingParams{ProductID: productID, ChannelID: channelID})
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotListed
	}
	if err != nil {
		return fmt.Errorf("check listing: %w", err)
	}
	return nil
}

// Normalize is the canonical query key: lowercased, whitespace-collapsed
func Normalize(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}

// RedirectFor returns the target URL when the normalized query has a
// redirect configured
func (s *Service) RedirectFor(ctx context.Context, channelID uuid.UUID, query string) (string, bool) {
	target, ok := s.snapshot(ctx, channelID).redirects[Normalize(query)]
	return target, ok
}

// ExpandQuery appends synonym-group members for every term in the query, so "tyre rack" searches "tyre rack tire tires" when a {tyre, tire, tires} group exists
// Terms match as whole phrases, so a "t shirt, tee" group fires on "red t shirt". Failure falls back to the raw query, since merchandising must never break search
func (s *Service) ExpandQuery(ctx context.Context, channelID uuid.UUID, query string) string {
	return expand(query, s.snapshot(ctx, channelID).synonyms)
}

func expand(query string, groups [][]string) string {
	q := Normalize(query)
	if q == "" {
		return query
	}
	padded := " " + q + " "
	added := map[string]bool{}
	has := func(term string) bool { return added[term] || strings.Contains(padded, " "+term+" ") }
	var extra []string
	for _, group := range groups {
		if !slices.ContainsFunc(group, has) {
			continue
		}
		for _, term := range group {
			if !has(term) {
				added[term] = true
				extra = append(extra, term)
			}
		}
	}
	if len(extra) == 0 {
		return query
	}
	return query + " " + strings.Join(extra, " ")
}

// PinsFor returns the pinned product ids for the normalized query, in
// position order
func (s *Service) PinsFor(ctx context.Context, channelID uuid.UUID, query string) []uuid.UUID {
	return slices.Clone(s.snapshot(ctx, channelID).pins[Normalize(query)])
}

// --- admin CRUD ---

func (s *Service) ListSynonyms(ctx context.Context, channelID uuid.UUID) ([]Synonym, error) {
	rows, err := s.queries.ListSearchSynonyms(ctx, uuid.NullUUID{UUID: channelID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list synonyms: %w", err)
	}
	out := make([]Synonym, 0, len(rows))
	for _, r := range rows {
		syn := Synonym{ID: r.ID, Terms: r.Terms, CreatedAt: r.CreatedAt}
		if r.ChannelID.Valid {
			id := r.ChannelID.UUID
			syn.ChannelID = &id
		}
		out = append(out, syn)
	}
	return out, nil
}

func (s *Service) AddSynonym(ctx context.Context, channelID *uuid.UUID, terms string) (Synonym, error) {
	group := splitTerms(terms)
	if len(group) < 2 {
		return Synonym{}, fmt.Errorf("a synonym group needs at least two terms")
	}
	row, err := s.queries.InsertSearchSynonym(ctx, db.InsertSearchSynonymParams{
		ChannelID: sqlnull.UUID(channelID),
		Terms:     strings.Join(group, ", "),
	})
	if err != nil {
		return Synonym{}, fmt.Errorf("insert synonym: %w", err)
	}
	s.invalidate(channelID)
	syn := Synonym{ID: row.ID, Terms: row.Terms, CreatedAt: row.CreatedAt}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		syn.ChannelID = &id
	}
	return syn, nil
}

// DeleteSynonym scopes the delete to the caller's channel - a channel-scoped admin can't reach another channel's row by id, and a global (channel-less) synonym isn't deletable through this per-channel path
func (s *Service) DeleteSynonym(ctx context.Context, channelID, id uuid.UUID) error {
	defer s.invalidate(&channelID)
	return s.queries.DeleteSearchSynonym(ctx, db.DeleteSearchSynonymParams{ID: id, ChannelID: uuid.NullUUID{UUID: channelID, Valid: true}})
}

func (s *Service) ListPins(ctx context.Context, channelID uuid.UUID) ([]Pin, error) {
	rows, err := s.queries.ListSearchPins(ctx, db.ListSearchPinsParams{ChannelID: channelID, Q: ""})
	if err != nil {
		return nil, fmt.Errorf("list pins: %w", err)
	}
	out := make([]Pin, 0, len(rows))
	for _, r := range rows {
		out = append(out, Pin{ID: r.ID, ChannelID: r.ChannelID, Query: r.Query, ProductID: r.ProductID, Position: r.Position})
	}
	return out, nil
}

func (s *Service) UpsertPin(ctx context.Context, channelID uuid.UUID, query string, productID uuid.UUID, position int32) (Pin, error) {
	q := Normalize(query)
	if q == "" {
		return Pin{}, fmt.Errorf("query must not be empty")
	}
	if position <= 0 {
		position = 1
	}
	if err := s.requireListed(ctx, channelID, productID); err != nil {
		return Pin{}, err
	}
	defer s.invalidate(&channelID)
	row, err := s.queries.UpsertSearchPin(ctx, db.UpsertSearchPinParams{
		ChannelID: channelID,
		Query:     q,
		ProductID: productID,
		Position:  position,
	})
	if err != nil {
		return Pin{}, fmt.Errorf("upsert pin: %w", err)
	}
	return Pin{ID: row.ID, ChannelID: row.ChannelID, Query: row.Query, ProductID: row.ProductID, Position: row.Position}, nil
}

func (s *Service) DeletePin(ctx context.Context, channelID, id uuid.UUID) error {
	defer s.invalidate(&channelID)
	return s.queries.DeleteSearchPin(ctx, db.DeleteSearchPinParams{ID: id, ChannelID: channelID})
}

// BoostsFor returns the boost/bury weights configured for a query, keyed by product id - Keyed on the ORIGINAL
// normalized query like pins/redirects - merchandising matches what the shopper typed, not the synonym-expanded form
func (s *Service) BoostsFor(ctx context.Context, channelID uuid.UUID, query string) map[uuid.UUID]float64 {
	boosts := s.snapshot(ctx, channelID).boosts[Normalize(query)]
	if len(boosts) == 0 {
		return nil
	}
	return maps.Clone(boosts)
}

func (s *Service) ListBoosts(ctx context.Context, channelID uuid.UUID) ([]Boost, error) {
	rows, err := s.queries.ListSearchBoosts(ctx, db.ListSearchBoostsParams{ChannelID: channelID, Q: ""})
	if err != nil {
		return nil, fmt.Errorf("list boosts: %w", err)
	}
	out := make([]Boost, 0, len(rows))
	for _, r := range rows {
		out = append(out, Boost{ID: r.ID, ChannelID: r.ChannelID, Query: r.Query, ProductID: r.ProductID, Weight: r.Weight})
	}
	return out, nil
}

func (s *Service) UpsertBoost(ctx context.Context, channelID uuid.UUID, query string, productID uuid.UUID, weight float64) (Boost, error) {
	q := Normalize(query)
	if q == "" {
		return Boost{}, fmt.Errorf("query must not be empty")
	}
	if weight == 0 {
		return Boost{}, fmt.Errorf("weight must be non-zero (positive boosts, negative buries)")
	}
	if err := s.requireListed(ctx, channelID, productID); err != nil {
		return Boost{}, err
	}
	defer s.invalidate(&channelID)
	row, err := s.queries.UpsertSearchBoost(ctx, db.UpsertSearchBoostParams{ChannelID: channelID, Query: q, ProductID: productID, Weight: weight})
	if err != nil {
		return Boost{}, fmt.Errorf("upsert boost: %w", err)
	}
	return Boost{ID: row.ID, ChannelID: row.ChannelID, Query: row.Query, ProductID: row.ProductID, Weight: row.Weight}, nil
}

func (s *Service) DeleteBoost(ctx context.Context, channelID, id uuid.UUID) error {
	defer s.invalidate(&channelID)
	return s.queries.DeleteSearchBoost(ctx, db.DeleteSearchBoostParams{ID: id, ChannelID: channelID})
}

func (s *Service) ListRedirects(ctx context.Context, channelID uuid.UUID) ([]Redirect, error) {
	rows, err := s.queries.ListSearchRedirects(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list redirects: %w", err)
	}
	out := make([]Redirect, 0, len(rows))
	for _, r := range rows {
		out = append(out, Redirect{ID: r.ID, ChannelID: r.ChannelID, Query: r.Query, TargetURL: r.TargetUrl})
	}
	return out, nil
}

func (s *Service) UpsertRedirect(ctx context.Context, channelID uuid.UUID, query, targetURL string) (Redirect, error) {
	q := Normalize(query)
	targetURL = strings.TrimSpace(targetURL)
	if q == "" || targetURL == "" {
		return Redirect{}, fmt.Errorf("query and target URL are required")
	}
	if !validTarget(targetURL) {
		return Redirect{}, ErrRedirectTarget
	}
	defer s.invalidate(&channelID)
	row, err := s.queries.UpsertSearchRedirect(ctx, db.UpsertSearchRedirectParams{
		ChannelID: channelID,
		Query:     q,
		TargetUrl: targetURL,
	})
	if err != nil {
		return Redirect{}, fmt.Errorf("upsert redirect: %w", err)
	}
	return Redirect{ID: row.ID, ChannelID: row.ChannelID, Query: row.Query, TargetURL: row.TargetUrl}, nil
}

func (s *Service) DeleteRedirect(ctx context.Context, channelID, id uuid.UUID) error {
	defer s.invalidate(&channelID)
	return s.queries.DeleteSearchRedirect(ctx, db.DeleteSearchRedirectParams{ID: id, ChannelID: channelID})
}

// A site path or an http(s) URL - "//host" and backslash forms start with "/" yet browsers follow them off-site
func validTarget(raw string) bool {
	if raw == "" || strings.ContainsRune(raw, '\\') {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && u.Scheme == "" && u.Host == ""
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// splitTerms parses "tyre, tire , tires" into a deduped list of normalized terms
func splitTerms(s string) []string {
	parts := strings.Split(s, ",")
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = Normalize(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

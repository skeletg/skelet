// Package searchmerch is operator-managed search merchandising - synonym groups (expanded into the query at search time, no reindex), curated pins (forced to the top), boosts, and query redirects ("help" -> /pages/faq) - All lookups are tiny indexed reads on the hot search path
package searchmerch

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
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
}

func NewService(queries *db.Queries) *Service { return &Service{queries: queries} }

// Normalize is the canonical query key: lowercased, whitespace-collapsed
func Normalize(q string) string {
	return strings.Join(strings.Fields(strings.ToLower(q)), " ")
}

// RedirectFor returns the target URL when the normalized query has a
// redirect configured
func (s *Service) RedirectFor(ctx context.Context, channelID uuid.UUID, query string) (string, bool) {
	r, err := s.queries.GetSearchRedirect(ctx, db.GetSearchRedirectParams{
		ChannelID: channelID,
		Query:     Normalize(query),
	})
	if err != nil {
		return "", false
	}
	return r.TargetUrl, true
}

// ExpandQuery appends synonym-group members for every term in the query, so "tyre rack" searches "tyre rack tire tires" when a {tyre, tire, tires} group exists
// Failure falls back to the raw query, since merchandising must never break search
func (s *Service) ExpandQuery(ctx context.Context, channelID uuid.UUID, query string) string {
	rows, err := s.queries.ListSearchSynonyms(ctx, uuid.NullUUID{UUID: channelID, Valid: true})
	if err != nil || len(rows) == 0 {
		return query
	}
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return query
	}
	present := make(map[string]bool, len(terms))
	for _, t := range terms {
		present[t] = true
	}
	var extra []string
	for _, row := range rows {
		group := splitTerms(row.Terms)
		hit := false
		for _, g := range group {
			if present[g] {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		for _, g := range group {
			if !present[g] {
				present[g] = true
				extra = append(extra, g)
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
	rows, err := s.queries.ListSearchPins(ctx, db.ListSearchPinsParams{
		ChannelID: channelID,
		Q:         Normalize(query),
	})
	if err != nil {
		return nil
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ProductID)
	}
	return out
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
	syn := Synonym{ID: row.ID, Terms: row.Terms, CreatedAt: row.CreatedAt}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		syn.ChannelID = &id
	}
	return syn, nil
}

// DeleteSynonym scopes the delete to the caller's channel - a channel-scoped admin can't reach another channel's row by id, and a global (channel-less) synonym isn't deletable through this per-channel path
func (s *Service) DeleteSynonym(ctx context.Context, channelID, id uuid.UUID) error {
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
	return s.queries.DeleteSearchPin(ctx, db.DeleteSearchPinParams{ID: id, ChannelID: channelID})
}

// BoostsFor returns the boost/bury weights configured for a query, keyed by product id - Keyed on the ORIGINAL
// normalized query like pins/redirects - merchandising matches what the shopper typed, not the synonym-expanded form
func (s *Service) BoostsFor(ctx context.Context, channelID uuid.UUID, query string) map[uuid.UUID]float64 {
	rows, err := s.queries.BoostsForQuery(ctx, db.BoostsForQueryParams{ChannelID: channelID, Query: Normalize(query)})
	if err != nil || len(rows) == 0 {
		return nil
	}
	out := make(map[uuid.UUID]float64, len(rows))
	for _, r := range rows {
		out[r.ProductID] = r.Weight
	}
	return out
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
	row, err := s.queries.UpsertSearchBoost(ctx, db.UpsertSearchBoostParams{ChannelID: channelID, Query: q, ProductID: productID, Weight: weight})
	if err != nil {
		return Boost{}, fmt.Errorf("upsert boost: %w", err)
	}
	return Boost{ID: row.ID, ChannelID: row.ChannelID, Query: row.Query, ProductID: row.ProductID, Weight: row.Weight}, nil
}

func (s *Service) DeleteBoost(ctx context.Context, channelID, id uuid.UUID) error {
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
	if !strings.HasPrefix(targetURL, "/") && !strings.HasPrefix(targetURL, "https://") && !strings.HasPrefix(targetURL, "http://") {
		return Redirect{}, fmt.Errorf("target must be a path or absolute URL")
	}
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
	return s.queries.DeleteSearchRedirect(ctx, db.DeleteSearchRedirectParams{ID: id, ChannelID: channelID})
}

// splitTerms parses "tyre, tire , tires" into a deduped lowercase list
func splitTerms(s string) []string {
	parts := strings.Split(strings.ToLower(s), ",")
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

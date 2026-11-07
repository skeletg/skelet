// Package aiactivity counts what the shopping assistant and shoppers' own AI agents do on a store - the tools they call and
// the words they search, per store and day, never tied to a person
package aiactivity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

const (
	SourceAssistant = signalsapi.AISourceAssistant
	SourceAgent     = signalsapi.AISourceAgent

	MaxTools    = signalsapi.AIMaxTools
	MaxSearches = signalsapi.AIMaxSearches
	maxAgent    = signalsapi.AIMaxAgentName
	maxTool     = 64
	maxTerm     = signalsapi.AIMaxTerm
	// Seen keys only have to outlast a replay
	SeenRetention = 14 * 24 * time.Hour
)

var ErrInvalid = errors.New("aiactivity: invalid activity")

type Search struct {
	Term    string
	Results int
}

type Activity struct {
	ChannelID uuid.UUID
	At        time.Time
	Source    string
	Agent     string
	Tools     []string
	Searches  []Search
	DedupeKey string
}

type Store struct {
	q  *db.Queries
	qr *db.Queries
}

func New(q *db.Queries) *Store { return &Store{q: q} }

func (s *Store) SetReadQueries(qr *db.Queries) { s.qr = qr }

func (s *Store) read() *db.Queries {
	if s.qr != nil {
		return s.qr
	}
	return s.q
}

// Record counts one activity, false when it was counted already
func (s *Store) Record(ctx context.Context, a Activity) (bool, error) {
	if a.ChannelID == uuid.Nil || strings.TrimSpace(a.DedupeKey) == "" || len(a.DedupeKey) > 200 {
		return false, ErrInvalid
	}
	agent := ""
	switch a.Source {
	case SourceAssistant:
	case SourceAgent:
		if agent = clean(a.Agent, maxAgent); agent == "" {
			agent = signalsapi.AIUnknownAgent
		}
	default:
		return false, ErrInvalid
	}
	tools := make([]string, 0, min(len(a.Tools), MaxTools))
	for _, t := range a.Tools[:min(len(a.Tools), MaxTools)] {
		if t = clean(t, maxTool); t != "" {
			tools = append(tools, t)
		}
	}
	var terms []string
	var results []int32
	for _, sr := range a.Searches[:min(len(a.Searches), MaxSearches)] {
		if term := NormalizeTerm(sr.Term); term != "" {
			terms, results = append(terms, term), append(results, int32(max(sr.Results, 0)))
		}
	}
	at := a.At
	if at.IsZero() {
		at = time.Now()
	}
	n, err := s.q.RecordAIActivity(ctx, db.RecordAIActivityParams{
		DedupeKey: a.DedupeKey, ChannelID: a.ChannelID, Day: at.UTC(), Source: a.Source, Agent: agent,
		Tools: tools, Terms: terms, Results: results,
	})
	return n > 0, err
}

// NormalizeTerm is Lens's own: lower-cased, spaces collapsed, at most 80 runes, so the same words group whoever searched them
func NormalizeTerm(q string) string {
	q = strings.ToLower(strings.Join(strings.Fields(q), " "))
	if r := []rune(q); len(r) > maxTerm {
		q = string(r[:maxTerm])
	}
	return q
}

func clean(s string, limit int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s))
	if r := []rune(s); len(r) > limit {
		s = strings.TrimSpace(string(r[:limit]))
	}
	return s
}

type ToolCount struct {
	Tool  string
	Calls int64
}

type Agent struct {
	Name     string
	Calls    int64
	Searches int64
	Empty    int64
	Tools    []ToolCount
}

const topTools = 8

// Agents is the shoppers' own agents seen on a store since a day, the busiest first, each with its most-called tools
func (s *Store) Agents(ctx context.Context, channelID uuid.UUID, since time.Time, limit int) ([]Agent, error) {
	totals, err := s.read().AIAgentTotals(ctx, db.AIAgentTotalsParams{ChannelID: channelID, Since: since, Lim: int32(limit), Source: SourceAgent})
	if err != nil || len(totals) == 0 {
		return nil, err
	}
	names := make([]string, len(totals))
	byName := make(map[string]*Agent, len(totals))
	out := make([]Agent, len(totals))
	for i, t := range totals {
		names[i] = t.Agent
		out[i] = Agent{Name: t.Agent, Calls: t.Calls}
		byName[t.Agent] = &out[i]
	}
	tools, err := s.read().AIAgentTools(ctx, db.AIAgentToolsParams{ChannelID: channelID, Since: since, Agents: names, Source: SourceAgent})
	if err != nil {
		return nil, err
	}
	for _, t := range tools {
		if a := byName[t.Agent]; a != nil && len(a.Tools) < topTools {
			a.Tools = append(a.Tools, ToolCount{Tool: t.Tool, Calls: t.Calls})
		}
	}
	searched, err := s.read().AIAgentSearches(ctx, db.AIAgentSearchesParams{ChannelID: channelID, Since: since, Agents: names, Source: SourceAgent})
	if err != nil {
		return nil, err
	}
	for _, r := range searched {
		if a := byName[r.Agent]; a != nil {
			a.Searches, a.Empty = r.Searches, r.Empty
		}
	}
	return out, nil
}

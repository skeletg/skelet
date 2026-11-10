// Package assistantlog keeps the storefront assistant's conversations for the store to read back: a row per conversation, a row per turn with how the model got to its answer
package assistantlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

const (
	MaxPage     = 100
	DefaultPage = 30
	MaxTurns    = 200
	maxSearch   = 200
	// trigrams need three letters or digits, and a search with fewer would read every turn
	minSearch  = 3
	pruneBatch = 5000
	eraseBatch = 1000
)

type Query struct {
	Limit      int
	BeforeAt   *time.Time
	BeforeID   uuid.UUID
	Search     string
	Customer   *uuid.UUID
	Unanswered string // "" for every conversation, else AnyGap or one of Gaps
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

func (s *Store) Record(ctx context.Context, p db.RecordAssistantTurnParams) (bool, error) {
	n, err := s.q.RecordAssistantTurn(ctx, p)
	return n > 0, err
}

var (
	pastEverything = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	lastID         = uuid.Max
)

func (s *Store) List(ctx context.Context, channelID uuid.UUID, q Query) ([]db.ListAssistantConversationsRow, bool, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	limit = min(limit, MaxPage)
	p := db.ListAssistantConversationsParams{ChannelID: channelID, BeforeAt: pastEverything, BeforeID: lastID, Unanswered: q.Unanswered, Lim: int32(limit + 1)}
	if q.BeforeAt != nil {
		p.BeforeAt, p.BeforeID = *q.BeforeAt, q.BeforeID
	}
	if q.Customer != nil {
		p.CustomerID = uuid.NullUUID{UUID: *q.Customer, Valid: true}
	}
	var rows []db.ListAssistantConversationsRow
	var err error
	if search := searchTerm(q.Search); search != "" {
		var found []db.SearchAssistantConversationsRow
		found, err = s.read().SearchAssistantConversations(ctx, db.SearchAssistantConversationsParams{
			ChannelID: p.ChannelID, BeforeAt: p.BeforeAt, BeforeID: p.BeforeID, CustomerID: p.CustomerID, Unanswered: p.Unanswered, Search: search, Lim: p.Lim,
		})
		rows = make([]db.ListAssistantConversationsRow, len(found))
		for i, r := range found {
			rows[i] = db.ListAssistantConversationsRow(r)
		}
	} else {
		rows, err = s.read().ListAssistantConversations(ctx, p)
	}
	if err != nil || len(rows) <= limit {
		return rows, false, err
	}
	return rows[:limit], true, nil
}

func (s *Store) Conversation(ctx context.Context, channelID, conversationID uuid.UUID) (db.ListAssistantConversationsRow, bool, error) {
	row, err := s.read().GetAssistantConversation(ctx, db.GetAssistantConversationParams{ChannelID: channelID, ConversationID: conversationID})
	if errors.Is(err, sql.ErrNoRows) {
		return db.ListAssistantConversationsRow{}, false, nil
	}
	return db.ListAssistantConversationsRow(row), err == nil, err
}

func (s *Store) Turns(ctx context.Context, channelID, conversationID uuid.UUID) ([]db.ListAssistantTurnsRow, error) {
	rows, err := s.read().ListAssistantTurns(ctx, db.ListAssistantTurnsParams{ChannelID: channelID, ConversationID: conversationID, Lim: MaxTurns})
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, err
}

func (s *Store) TurnRounds(ctx context.Context, channelID, turnID uuid.UUID) (json.RawMessage, bool, error) {
	raw, err := s.read().GetAssistantTurnRounds(ctx, db.GetAssistantTurnRoundsParams{ChannelID: channelID, TurnID: turnID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return raw, err == nil, err
}

// Prune goes a batch at a time, so a shortened retention on a big store gets through its backlog instead of timing out
func (s *Store) Prune(ctx context.Context, before time.Time) (conversations, turns int64, err error) {
	if conversations, err = drain(ctx, before, s.pruneConversations); err != nil {
		return conversations, 0, err
	}
	turns, err = drain(ctx, before, s.pruneTurns)
	return conversations, turns, err
}

func drain(ctx context.Context, before time.Time, step func(context.Context, time.Time, int32) (int64, error)) (int64, error) {
	var total int64
	for {
		n, err := step(ctx, before, pruneBatch)
		total += n
		if err != nil || n < pruneBatch {
			return total, err
		}
	}
}

func (s *Store) pruneConversations(ctx context.Context, before time.Time, lim int32) (int64, error) {
	return s.q.PruneAssistantConversations(ctx, db.PruneAssistantConversationsParams{Before: before, Lim: lim})
}

func (s *Store) pruneTurns(ctx context.Context, before time.Time, lim int32) (int64, error) {
	return s.q.PruneAssistantTurns(ctx, db.PruneAssistantTurnsParams{Before: before, Lim: lim})
}

// Erase goes a slice of ids at a time, so a big erasure is many short statements rather than one that holds everything up
func (s *Store) Erase(ctx context.Context, customers []uuid.UUID, anonIDs []string) error {
	for start := 0; start < max(len(customers), len(anonIDs)); start += eraseBatch {
		p := db.EraseAssistantConversationsParams{CustomerIds: window(customers, start), AnonIds: window(anonIDs, start)}
		if _, err := s.q.EraseAssistantConversations(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func window[T any](ids []T, start int) []T {
	return append([]T{}, ids[min(start, len(ids)):min(start+eraseBatch, len(ids))]...)
}

var Gaps = []string{"no_answer", "cut_short", "found_nothing", "unsure"}

const AnyGap = "any"

func (s *Store) GapCounts(ctx context.Context, channelID uuid.UUID, customer *uuid.UUID) (db.CountAssistantConversationGapsRow, error) {
	p := db.CountAssistantConversationGapsParams{ChannelID: channelID}
	if customer != nil {
		p.CustomerID = uuid.NullUUID{UUID: *customer, Valid: true}
	}
	return s.read().CountAssistantConversationGaps(ctx, p)
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func searchTerm(raw string) string {
	r := []rune(strings.TrimSpace(raw))
	if countFunc(r, isWordRune) < minSearch {
		return ""
	}
	return likeEscaper.Replace(string(r[:min(len(r), maxSearch)]))
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func countFunc(r []rune, f func(rune) bool) int {
	n := 0
	for _, c := range r {
		if f(c) {
			n++
		}
	}
	return n
}

// Package sendlog owns the email send log in core, because every transactional mail writes a row whether or not Signals runs - Signals only reads it
package sendlog

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/archivefile"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
)

// Who sent the message - the send log's own vocabulary, distinct from the spine's behavioural sources
const (
	SourceCampaign      = "campaign"
	SourceFlow          = "flow"
	SourceTransactional = "transactional"
)

// Spec is a send about to be recorded - the fields the log keys, attributes and reports on
type Spec struct {
	ChannelID   uuid.UUID
	Source      string
	Email       string
	CustomerID  *uuid.UUID
	CampaignID  *uuid.UUID
	RecipientID *uuid.UUID
	Variant     string // A/B arm, copied from the recipient so the rollup needn't join commerce
	FlowID      *uuid.UUID
	FlowRunID   *uuid.UUID
	FlowNodeID  string
	MessageKey  string
	OrderID     *uuid.UUID
}

// Send is the resolved send identity the open and click endpoints need at action time
type Send struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	Source      string
	Email       string
	CustomerID  *uuid.UUID
	CampaignID  *uuid.UUID
	RecipientID *uuid.UUID
	SentAt      time.Time
}

type Store struct {
	q       *db.Queries
	qr      *db.Queries // optional read pool
	archive archivefile.Sink
}

func NewStore(q *db.Queries) *Store { return &Store{q: q} }

// Optional - unset means reads use the primary pool
func (s *Store) SetReadQueries(qr *db.Queries) { s.qr = qr }

func (s *Store) read() *db.Queries {
	if s.qr != nil {
		return s.qr
	}
	return s.q
}

// New records a send - the returned ID is the tracking token's subject
func (s *Store) New(ctx context.Context, sp Spec) (Send, error) {
	id, err := s.q.InsertEmailSend(ctx, db.InsertEmailSendParams{
		ChannelID:   sp.ChannelID,
		Source:      sp.Source,
		Email:       sp.Email,
		CustomerID:  nu(sp.CustomerID),
		CampaignID:  nu(sp.CampaignID),
		RecipientID: nu(sp.RecipientID),
		Variant:     sp.Variant,
		FlowID:      nu(sp.FlowID),
		FlowRunID:   nu(sp.FlowRunID),
		FlowNodeID:  sp.FlowNodeID,
		MessageKey:  sp.MessageKey,
		OrderID:     nu(sp.OrderID),
	})
	if err != nil {
		return Send{}, err
	}
	return Send{
		ID: id, ChannelID: sp.ChannelID, Source: sp.Source, Email: sp.Email,
		CustomerID: sp.CustomerID, CampaignID: sp.CampaignID, RecipientID: sp.RecipientID, SentAt: time.Now(),
	}, nil
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (Send, bool, error) {
	row, err := s.read().GetEmailSend(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Send{}, false, nil
	}
	if err != nil {
		return Send{}, false, err
	}
	return Send{
		ID: row.ID, ChannelID: row.ChannelID, Source: row.Source, Email: row.Email,
		CustomerID: pu(row.CustomerID), CampaignID: pu(row.CampaignID), RecipientID: pu(row.RecipientID),
		SentAt: row.SentAt,
	}, true, nil
}

// ByIDs resolves a batch of sends to their origin, for attributing engagement back to a campaign or flow
func (s *Store) ByIDs(ctx context.Context, ids []uuid.UUID) ([]db.EmailSendsByIDsRow, error) {
	return s.read().EmailSendsByIDs(ctx, ids)
}

// MarkOpened and MarkClicked set the per-send first-seen timestamps, which is what makes a send report as read
func (s *Store) MarkOpened(ctx context.Context, id uuid.UUID) error {
	return s.q.MarkSendOpened(ctx, id)
}

func (s *Store) MarkClicked(ctx context.Context, id uuid.UUID) error {
	return s.q.MarkSendClicked(ctx, id)
}

// EnsurePartitions pre-creates the upcoming month partitions - sends land in the DEFAULT partition once the pre-created months run out
func (s *Store) EnsurePartitions(ctx context.Context) error {
	return s.q.EnsureEmailSendPartitions(ctx)
}

func nu(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func pu(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	id := n.UUID
	return &id
}

func pt(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// Engagement is a customer's lifetime email engagement, counted off the log where opens and clicks are bot-filtered at write time
type Engagement struct {
	Sent       int
	Opened     int
	Clicked    int
	LastSentAt *time.Time
	LastOpenAt *time.Time
}

// CustomerEngagement is zero-valued with no error for a customer who has never been sent anything
func (s *Store) CustomerEngagement(ctx context.Context, customerID uuid.UUID) (Engagement, error) {
	row, err := s.read().CustomerEmailEngagement(ctx, uuid.NullUUID{UUID: customerID, Valid: true})
	if err != nil {
		return Engagement{}, err
	}
	return Engagement{
		Sent: int(row.Sent), Opened: int(row.Opened), Clicked: int(row.Clicked),
		LastSentAt: asTime(row.LastSentAt), LastOpenAt: asTime(row.LastOpenAt),
	}, nil
}

// The MAX columns come back untyped, since a customer may have no sends at all
func asTime(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}

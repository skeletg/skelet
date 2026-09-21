// Package sendlog owns the email send log - what core's campaigns and flows sent, plus the human-only first-open and first-click stamps
// Core keeps no copy: it names a send by the id it minted and reads the record back
package sendlog

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// Who sent the message - the send log's own vocabulary, distinct from the spine's behavioural sources
const (
	SourceCampaign      = "campaign"
	SourceFlow          = "flow"
	SourceTransactional = "transactional"
)

// Logged reports whether a source keeps a row here - a transactional send carries no pixel and is
// never asked about again, so the spine alone hears of it
func Logged(source string) bool { return source == SourceCampaign || source == SourceFlow }

// Spec is a send about to be recorded - the fields the log keys, attributes and reports on
// The id is core's, minted before the mail went out so the tracking token names a row that exists
type Spec struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	Source      string
	Email       string
	CustomerID  *uuid.UUID
	CampaignID  *uuid.UUID
	RecipientID *uuid.UUID
	Variant     string // A/B arm, copied from the recipient so the rollup needn't join core's tables
	FlowID      *uuid.UUID
	FlowRunID   *uuid.UUID
	FlowNodeID  string
	MessageKey  string
	OrderID     *uuid.UUID
}

// Send is a logged send as the handlers read it back
type Send struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	Source      string
	Email       string
	CustomerID  *uuid.UUID
	CampaignID  *uuid.UUID
	RecipientID *uuid.UUID
	Variant     string
	FlowID      *uuid.UUID
	FlowRunID   *uuid.UUID
	FlowNodeID  string
	MessageKey  string
	OrderID     *uuid.UUID
	SentAt      time.Time
	// Human hits only - set once, on the first
	FirstOpenAt  *time.Time
	FirstClickAt *time.Time
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

// New records a send under the id the caller minted
func (s *Store) New(ctx context.Context, sp Spec) (Send, error) {
	row, err := s.q.InsertEmailSend(ctx, db.InsertEmailSendParams{
		ID:          sp.ID,
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
		ID: row.ID, ChannelID: sp.ChannelID, Source: sp.Source, Email: sp.Email,
		CustomerID: sp.CustomerID, CampaignID: sp.CampaignID, RecipientID: sp.RecipientID, Variant: sp.Variant,
		FlowID: sp.FlowID, FlowRunID: sp.FlowRunID, FlowNodeID: sp.FlowNodeID, MessageKey: sp.MessageKey, OrderID: sp.OrderID,
		SentAt: row.SentAt,
	}, nil
}

// Get is the pixel and click hot path - a send older than the query's window reads as gone, so an old
// mail stops recording engagement while its links keep working, the destination riding the token
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
		CustomerID: pu(row.CustomerID), CampaignID: pu(row.CampaignID), RecipientID: pu(row.RecipientID), Variant: row.Variant,
		FlowID: pu(row.FlowID), FlowRunID: pu(row.FlowRunID), FlowNodeID: row.FlowNodeID, MessageKey: row.MessageKey, OrderID: pu(row.OrderID),
		SentAt: row.SentAt, FirstOpenAt: nt(row.FirstOpenAt), FirstClickAt: nt(row.FirstClickAt),
	}, true, nil
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

// VariantStat is one A/B arm's sent, opened and clicked, human hits only
type VariantStat struct {
	Variant string
	Sent    int
	Opened  int
	Clicked int
}

// VariantStats groups a campaign's sends by the denormalized variant, which is cheaper than joining
// core's recipients - it is what the A/B results view shows and the winner decision reads
func (s *Store) VariantStats(ctx context.Context, campaignID uuid.UUID) ([]VariantStat, error) {
	rows, err := s.read().CampaignVariantStats(ctx, uuid.NullUUID{UUID: campaignID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]VariantStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, VariantStat{Variant: row.Variant, Sent: int(row.Sent), Opened: int(row.Opened), Clicked: int(row.Clicked)})
	}
	return out, nil
}

// ScrubIdentity redacts the address on everything this customer was sent, for a GDPR erasure - the rows stay, because the send happened
// Matched on the pre-scrub address as well as the id, so sends from before they had an account go too
func (s *Store) ScrubIdentity(ctx context.Context, customerID uuid.UUID, email string) error {
	_, err := s.q.ScrubCustomerEmailSends(ctx, db.ScrubCustomerEmailSendsParams{CustomerID: customerID, Email: email})
	return err
}

func nt(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
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

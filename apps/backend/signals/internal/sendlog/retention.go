package sendlog

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/signals/internal/archivefile"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// One send-log row as archived - customer_id, email and recipient_id stripped, keeping campaign/flow/order linkage for aggregate stats
type archivedSend struct {
	ID           uuid.UUID  `json:"id"`
	ChannelID    uuid.UUID  `json:"channel_id"`
	Source       string     `json:"source"`
	CampaignID   *uuid.UUID `json:"campaign_id,omitempty"`
	Variant      string     `json:"variant,omitempty"`
	FlowID       *uuid.UUID `json:"flow_id,omitempty"`
	FlowRunID    *uuid.UUID `json:"flow_run_id,omitempty"`
	FlowNodeID   string     `json:"flow_node_id,omitempty"`
	MessageKey   string     `json:"message_key,omitempty"`
	OrderID      *uuid.UUID `json:"order_id,omitempty"`
	SentAt       time.Time  `json:"sent_at"`
	FirstOpenAt  *time.Time `json:"first_open_at,omitempty"`
	FirstClickAt *time.Time `json:"first_click_at,omitempty"`
}

// SetArchiveSink wires the retention archive destination - unset leaves archive mode failing closed
func (s *Store) SetArchiveSink(sink archivefile.Sink) { s.archive = sink }

// Prune drops whole months older than the cutoff, exporting each first when archive is set - months because that is the partition unit
func (s *Store) Prune(ctx context.Context, before time.Time, archive bool) (int64, error) {
	if archive && s.archive == nil {
		return 0, archivefile.ErrNoSink
	}
	months, err := s.q.EmailSendMonthsBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, m := range months {
		if archive {
			if err := s.archiveMonth(ctx, m); err != nil {
				return total, err
			}
		}
		n, err := s.q.DropEmailSendMonth(ctx, m)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *Store) archiveMonth(ctx context.Context, month time.Time) error {
	var f *archivefile.File // opened on the first row - a pre-created partition can be empty and deserves no object
	cursor := uuid.Nil
	from, to := month.UTC(), month.UTC().AddDate(0, 1, 0)
	for {
		rows, err := s.q.ListEmailSendsForArchive(ctx, db.ListEmailSendsForArchiveParams{
			FromAt: from, ToAt: to, CursorID: cursor, Lim: archivefile.PageSize,
		})
		if err != nil {
			archivefile.Discard(f)
			return err
		}
		for _, row := range rows {
			if f == nil {
				f = archivefile.New(ctx, s.archive, "email-sends/"+month.UTC().Format("2006-01")+"-"+row.ID.String()[:8]+".jsonl.gz")
			}
			if err := f.Write(archivedSend{
				ID: row.ID, ChannelID: row.ChannelID, Source: row.Source,
				CampaignID: pu(row.CampaignID), Variant: row.Variant,
				FlowID: pu(row.FlowID), FlowRunID: pu(row.FlowRunID), FlowNodeID: row.FlowNodeID,
				MessageKey: row.MessageKey, OrderID: pu(row.OrderID),
				SentAt: row.SentAt, FirstOpenAt: pt(row.FirstOpenAt), FirstClickAt: pt(row.FirstClickAt),
			}); err != nil {
				archivefile.Discard(f)
				return err
			}
		}
		if len(rows) < archivefile.PageSize {
			break
		}
		cursor = rows[len(rows)-1].ID
	}
	if f == nil {
		return nil
	}
	return f.Close()
}

package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type repo struct {
	conn *sql.DB
	q    *db.Queries
}

func (r *repo) list(ctx context.Context, channelID uuid.UUID) ([]Summary, error) {
	rows, err := r.q.ListBoards(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(rows))
	for _, row := range rows {
		out = append(out, Summary{ID: row.ID, Name: row.Name, UpdatedAt: row.UpdatedAt, ReportCount: int(row.ReportCount)})
	}
	return out, nil
}

func (r *repo) get(ctx context.Context, channelID, id uuid.UUID) (Board, error) {
	row, err := r.q.GetBoard(ctx, db.GetBoardParams{ID: id, ChannelID: channelID})
	if errors.Is(err, sql.ErrNoRows) {
		return Board{}, ErrNotFound
	}
	if err != nil {
		return Board{}, err
	}
	b := Board{ID: row.ID, ChannelID: channelID, Name: row.Name, UpdatedAt: row.UpdatedAt}
	reports, err := r.q.BoardReports(ctx, id)
	if err != nil {
		return Board{}, err
	}
	for _, rr := range reports {
		var d Definition
		if err := json.Unmarshal(rr.Definition, &d); err != nil {
			return Board{}, fmt.Errorf("board %s, report %q: %w", id, rr.Title, err)
		}
		b.Reports = append(b.Reports, Report{Title: rr.Title, Definition: d})
	}
	return b, nil
}

func (r *repo) create(ctx context.Context, b Board) (Board, error) {
	err := r.inTx(ctx, func(q *db.Queries) (err error) {
		b, err = insertBoard(ctx, q, b)
		return err
	})
	return b, err
}

// createIfNone adds boards to a shop that has none, under a per-shop lock so concurrent calls add them once
func (r *repo) createIfNone(ctx context.Context, channelID uuid.UUID, boards []Board) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		if err := q.LockChannelBoards(ctx, channelID); err != nil {
			return err
		}
		n, err := q.CountBoards(ctx, channelID)
		if err != nil || n > 0 {
			return err
		}
		for _, b := range boards {
			b.ChannelID = channelID
			if _, err := insertBoard(ctx, q, b); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertBoard(ctx context.Context, q *db.Queries, b Board) (Board, error) {
	row, err := q.CreateBoard(ctx, db.CreateBoardParams{ChannelID: b.ChannelID, Name: b.Name})
	if err != nil {
		return b, err
	}
	b.ID, b.UpdatedAt = row.ID, row.UpdatedAt
	return b, writeReports(ctx, q, b.ID, b.Reports)
}

// save replaces the board's name and reports in one go, refused when someone else saved it after readAt
func (r *repo) save(ctx context.Context, b Board, readAt time.Time) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		n, err := q.TouchBoard(ctx, db.TouchBoardParams{ID: b.ID, ChannelID: b.ChannelID, Name: b.Name, ReadAt: readAt})
		if err != nil {
			return err
		}
		if n == 0 { // missing from this shop, or saved since readAt
			if _, err := q.GetBoard(ctx, db.GetBoardParams{ID: b.ID, ChannelID: b.ChannelID}); errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
			return ErrStale
		}
		if err := q.ClearBoardReports(ctx, b.ID); err != nil {
			return err
		}
		return writeReports(ctx, q, b.ID, b.Reports)
	})
}

func (r *repo) delete(ctx context.Context, channelID, id uuid.UUID) error {
	n, err := r.q.DeleteBoard(ctx, db.DeleteBoardParams{ID: id, ChannelID: channelID})
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func writeReports(ctx context.Context, q *db.Queries, boardID uuid.UUID, reports []Report) error {
	for i, rep := range reports {
		raw, err := json.Marshal(rep.Definition)
		if err != nil {
			return err
		}
		if err := q.AddBoardReport(ctx, db.AddBoardReportParams{
			BoardID: boardID, Position: int32(i), Title: rep.Title, Definition: raw,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *repo) inTx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(r.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

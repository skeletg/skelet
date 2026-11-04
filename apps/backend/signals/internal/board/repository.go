package board

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

type repo struct {
	conn *sql.DB
	q    *db.Queries
}

func (r *repo) list(ctx context.Context, channelID uuid.UUID) ([]Board, error) {
	rows, err := r.q.ListBoards(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rows))
	for _, row := range rows {
		out = append(out, Board{ID: row.ID, ChannelID: channelID, Name: row.Name, UpdatedAt: row.UpdatedAt, ReportCount: int(row.ReportCount)})
	}
	return out, nil
}

func (r *repo) get(ctx context.Context, id uuid.UUID) (Board, error) {
	row, err := r.q.GetBoard(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Board{}, ErrNotFound
	}
	if err != nil {
		return Board{}, err
	}
	b := Board{ID: row.ID, ChannelID: row.ChannelID, Name: row.Name, UpdatedAt: row.UpdatedAt}
	reports, err := r.q.BoardReports(ctx, id)
	if err != nil {
		return Board{}, err
	}
	for _, rr := range reports {
		var d Definition
		if err := json.Unmarshal(rr.Definition, &d); err != nil { // a newer shape than this build reads still shows its title
			d = Definition{}
		}
		b.Reports = append(b.Reports, Report{Title: rr.Title, Definition: d})
	}
	b.ReportCount = len(b.Reports)
	return b, nil
}

func (r *repo) create(ctx context.Context, b Board) (Board, error) {
	err := r.inTx(ctx, func(q *db.Queries) error {
		row, err := q.CreateBoard(ctx, db.CreateBoardParams{ChannelID: b.ChannelID, Name: b.Name})
		if err != nil {
			return err
		}
		b.ID, b.UpdatedAt = row.ID, row.UpdatedAt
		return writeReports(ctx, q, b.ID, b.Reports)
	})
	return b, err
}

// save replaces the board's name and reports in one go, refused when someone else saved it after readAt
func (r *repo) save(ctx context.Context, b Board, readAt time.Time) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		n, err := q.RenameBoard(ctx, db.RenameBoardParams{ID: b.ID, Name: b.Name, ReadAt: readAt})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrStale
		}
		if err := q.ClearBoardReports(ctx, b.ID); err != nil {
			return err
		}
		return writeReports(ctx, q, b.ID, b.Reports)
	})
}

func (r *repo) delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.q.DeleteBoard(ctx, id)
	return err
}

func writeReports(ctx context.Context, q *db.Queries, boardID uuid.UUID, reports []Report) error {
	for i, rep := range reports {
		raw, err := json.Marshal(rep.Definition)
		if err != nil {
			return err
		}
		if err := q.AddBoardReport(ctx, db.AddBoardReportParams{
			BoardID: boardID, Position: int32(i), Title: rep.Title, Definition: raw, DefinitionVersion: DefinitionVersion,
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

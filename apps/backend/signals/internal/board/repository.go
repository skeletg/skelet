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
		out = append(out, Board{
			ID: row.ID, ChannelID: channelID, Name: row.Name, UpdatedAt: row.UpdatedAt, ReportCount: int(row.ReportCount),
			OwnerID: row.OwnerAdminID.UUID, Status: row.Status,
		})
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
	b := Board{ID: row.ID, ChannelID: row.ChannelID, Name: row.Name, UpdatedAt: row.UpdatedAt, OwnerID: row.OwnerAdminID.UUID, Status: row.Status}
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
		row, err := q.CreateBoard(ctx, db.CreateBoardParams{ChannelID: b.ChannelID, Name: b.Name, OwnerAdminID: nullID(b.OwnerID), Status: b.Status})
		if err != nil {
			return err
		}
		b.ID, b.UpdatedAt = row.ID, row.UpdatedAt
		return writeReports(ctx, q, b.ID, b.Reports)
	})
	return b, err
}

// save replaces the board's name and reports in one go, refused when someone else saved it after readAt - and the
// saving admin's draft goes with it, since what it held is now the board
func (r *repo) save(ctx context.Context, b Board, readAt time.Time, by uuid.UUID) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteBoardDraft(ctx, db.DeleteBoardDraftParams{BoardID: b.ID, AdminID: by}); err != nil {
			return err
		}
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

func (r *repo) shares(ctx context.Context, boardID uuid.UUID) ([]Share, error) {
	rows, err := r.q.BoardShares(ctx, boardID)
	if err != nil {
		return nil, err
	}
	out := make([]Share, 0, len(rows))
	for _, row := range rows {
		out = append(out, Share{AdminID: row.AdminID.UUID, RoleID: row.RoleID.UUID, Level: row.Level})
	}
	return out, nil
}

func (r *repo) channelShares(ctx context.Context, channelID uuid.UUID) (map[uuid.UUID][]Share, error) {
	rows, err := r.q.ChannelBoardShares(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]Share{}
	for _, row := range rows {
		out[row.BoardID] = append(out[row.BoardID], Share{AdminID: row.AdminID.UUID, RoleID: row.RoleID.UUID, Level: row.Level})
	}
	return out, nil
}

func (r *repo) setShares(ctx context.Context, boardID, by uuid.UUID, shares []Share) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		if err := q.ClearBoardShares(ctx, boardID); err != nil {
			return err
		}
		for _, s := range shares {
			if err := q.AddBoardShare(ctx, db.AddBoardShareParams{BoardID: boardID, AdminID: nullID(s.AdminID), RoleID: nullID(s.RoleID), Level: s.Level, CreatedBy: by}); err != nil {
				return err
			}
		}
		return nil
	})
}

// setOwner hands the board to an admin, or with uuid.Nil makes it a shop board - whose shares go, since everyone sees it
func (r *repo) setOwner(ctx context.Context, boardID, owner uuid.UUID) error {
	return r.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.SetBoardOwner(ctx, db.SetBoardOwnerParams{ID: boardID, OwnerAdminID: nullID(owner)}); err != nil {
			return err
		}
		if owner == uuid.Nil {
			return q.ClearBoardShares(ctx, boardID)
		}
		return nil
	})
}

func (r *repo) favourites(ctx context.Context, adminID uuid.UUID) (map[uuid.UUID]bool, error) {
	ids, err := r.q.AdminFavouriteBoards(ctx, adminID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func (r *repo) setFavourite(ctx context.Context, adminID, boardID uuid.UUID, on bool) error {
	if on {
		return r.q.AddBoardFavourite(ctx, db.AddBoardFavouriteParams{AdminID: adminID, BoardID: boardID})
	}
	return r.q.RemoveBoardFavourite(ctx, db.RemoveBoardFavouriteParams{AdminID: adminID, BoardID: boardID})
}

func (r *repo) draftTimes(ctx context.Context, adminID uuid.UUID) (map[uuid.UUID]time.Time, error) {
	rows, err := r.q.AdminDraftBoards(ctx, adminID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]time.Time, len(rows))
	for _, row := range rows {
		out[row.BoardID] = row.UpdatedAt
	}
	return out, nil
}

func (r *repo) draft(ctx context.Context, boardID, adminID uuid.UUID) (*Draft, error) {
	row, err := r.q.GetBoardDraft(ctx, db.GetBoardDraftParams{BoardID: boardID, AdminID: adminID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Draft{Payload: row.Payload, BaseUpdatedAt: row.BaseUpdatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (r *repo) putDraft(ctx context.Context, boardID, adminID uuid.UUID, payload []byte, base time.Time) (time.Time, error) {
	return r.q.PutBoardDraft(ctx, db.PutBoardDraftParams{BoardID: boardID, AdminID: adminID, Payload: payload, BaseUpdatedAt: base})
}

func (r *repo) discardDraft(ctx context.Context, boardID, adminID uuid.UUID) error {
	return r.q.DeleteBoardDraft(ctx, db.DeleteBoardDraftParams{BoardID: boardID, AdminID: adminID})
}

func (r *repo) sweepDrafts(ctx context.Context, before time.Time) (int64, error) {
	return r.q.SweepBoardDrafts(ctx, before)
}

func nullID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }

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

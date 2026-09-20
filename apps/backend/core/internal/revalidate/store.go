package revalidate

import (
	"context"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
)

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// dbStore persists purge activity to cache_revalidations
type dbStore struct{ q *db.Queries }

func NewDBStore(q *db.Queries) Store { return &dbStore{q: q} }

func (s *dbStore) Record(ctx context.Context, e LogEntry) error {
	var actor uuid.NullUUID
	if e.ActorID != nil {
		actor = uuid.NullUUID{UUID: *e.ActorID, Valid: true}
	}
	// The driver encodes a nil []string as SQL NULL, which violates the
	// NOT-NULL tags/paths columns - normalize to empty arrays
	return s.q.InsertCacheRevalidation(ctx, db.InsertCacheRevalidationParams{
		Surface: e.Surface,
		Tags:    nonNil(e.Tags),
		Paths:   nonNil(e.Paths),
		Reason:  e.Reason,
		ActorID: actor,
	})
}

func (s *dbStore) List(ctx context.Context, limit int) ([]LoggedPurge, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ListCacheRevalidations(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]LoggedPurge, len(rows))
	for i, r := range rows {
		var actor *uuid.UUID
		if r.ActorID.Valid {
			id := r.ActorID.UUID
			actor = &id
		}
		out[i] = LoggedPurge{
			ID:        r.ID,
			Surface:   r.Surface,
			Tags:      r.Tags,
			Paths:     r.Paths,
			Reason:    r.Reason,
			ActorID:   actor,
			CreatedAt: r.CreatedAt,
		}
	}
	return out, nil
}

package admin

// Helpers for cache.resolvers.go. Kept out of *.resolvers.go so gqlgen doesn't
// evict them on regeneration.

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/revalidate"
)

func toRebuildTaskModel(t db.CacheRebuildTask) *model.RebuildTask {
	return &model.RebuildTask{
		ID:        t.ID.String(),
		Surface:   t.Surface,
		Path:      t.Path,
		Status:    t.Status,
		Attempts:  t.Attempts,
		LastError: t.LastError,
		Reason:    t.Reason,
		UpdatedAt: t.UpdatedAt,
	}
}

func toCacheRevalidationModel(p revalidate.LoggedPurge) *model.CacheRevalidation {
	m := &model.CacheRevalidation{
		ID:        p.ID.String(),
		Surface:   p.Surface,
		Tags:      p.Tags,
		Paths:     p.Paths,
		Reason:    p.Reason,
		CreatedAt: p.CreatedAt,
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if m.Paths == nil {
		m.Paths = []string{}
	}
	if p.ActorID != nil {
		id := p.ActorID.String()
		m.ActorID = &id
	}
	return m
}

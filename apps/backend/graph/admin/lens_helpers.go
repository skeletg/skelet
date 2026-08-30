package admin

// Lives here rather than lens.resolvers.go - gqlgen evicts non-resolver funcs from generated resolver files

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/lens"
)

func lensSince(sinceDays *int32) time.Time { // default window is 30 days
	days := int32(30)
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	return time.Now().AddDate(0, 0, -int(days))
}

// Both gates matter - lens.view globally and again on the channel
func lensViewScope(ctx context.Context, r *queryResolver, channelID string) (uuid.UUID, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return uuid.Nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return uuid.Nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "lens.view", cid, nil); err != nil {
		return uuid.Nil, err
	}
	return cid, nil
}

func signalPages(pages []lens.SignalPage) []*model.LensSignalPage {
	out := make([]*model.LensSignalPage, 0, len(pages))
	for _, p := range pages {
		out = append(out, &model.LensSignalPage{PageKey: p.PageKey, Count: int32(p.Count)})
	}
	return out
}

func sessionLimit(limit *int32) int32 { // default 50, hard max 200
	n := int32(50)
	if limit != nil && *limit > 0 {
		n = *limit
	}
	if n > 200 {
		n = 200
	}
	return n
}

func (r *queryResolver) sessionModels(ctx context.Context, rows []lens.SessionMeta) []*model.LensSession {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, s := range rows {
		if s.CustomerID != nil {
			ids = append(ids, *s.CustomerID)
		}
	}
	custs, _ := r.CustomerSvc.GetByIDs(ctx, ids) // batched so the recordings table shows a customer instead of a bare id

	out := make([]*model.LensSession, 0, len(rows))
	for _, s := range rows {
		m := &model.LensSession{
			ID:         s.ID.String(),
			AnonID:     s.AnonID,
			StartedAt:  s.StartedAt,
			EndedAt:    s.EndedAt,
			DurationMs: int32(s.DurationMs),
			EventCount: int32(s.EventCount),
			EntryPage:  s.EntryPage,
			ExitPage:   s.ExitPage,
			Viewport:   s.Viewport,
			UserAgent:  s.UserAgent,
			RageCount:  int32(s.RageCount),
			ErrorCount: int32(s.ErrorCount),
		}
		if s.CustomerID != nil {
			cid := s.CustomerID.String()
			m.CustomerID = &cid
			if c, ok := custs[*s.CustomerID]; ok {
				if name := strings.TrimSpace(c.FirstName + " " + c.LastName); name != "" {
					m.CustomerName = &name
				}
				if c.Email != "" {
					email := c.Email
					m.CustomerEmail = &email
				}
			}
		}
		out = append(out, m)
	}
	return out
}

func lensCells(cells []lens.Cell) []*model.LensCell {
	out := make([]*model.LensCell, 0, len(cells))
	for _, c := range cells {
		out = append(out, &model.LensCell{X: int32(c.X), Y: int32(c.Y), Count: int32(c.Count)})
	}
	return out
}

func lensLabels(items []lens.LabelCount) []*model.LensLabelCount {
	out := make([]*model.LensLabelCount, 0, len(items))
	for _, i := range items {
		out = append(out, &model.LensLabelCount{Label: i.Label, Views: int32(i.Views)})
	}
	return out
}

package admin

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *queryResolver) AuditLog(ctx context.Context, adminID *string, action *string, limit *int32, offset *int32) (*model.AuditLogPage, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "audit.view"); err != nil {
		return nil, err
	}
	var aid *uuid.UUID
	if adminID != nil && *adminID != "" {
		id, err := uuid.Parse(*adminID)
		if err != nil {
			return nil, errInvalidID("admin id", nil)
		}
		aid = &id
	}
	lim := int32(50)
	if limit != nil {
		lim = *limit
	}
	off := int32(0)
	if offset != nil {
		off = *offset
	}
	rows, total, err := r.AuditSvc.List(ctx, aid, action, lim, off)
	if err != nil {
		return nil, err
	}
	entries := make([]*model.AuditLogEntry, len(rows))
	for i, row := range rows {
		e := &model.AuditLogEntry{
			ID:         row.ID.String(),
			AdminEmail: row.AdminEmail,
			Action:     row.Action,
			IP:         row.IP,
			Success:    row.Success,
			DurationMs: int32(row.DurationMS),
			CreatedAt:  row.CreatedAt,
		}
		if row.AdminID != nil {
			v := row.AdminID.String()
			e.AdminID = &v
		}
		if len(row.Detail) > 0 {
			var v any
			if json.Unmarshal(row.Detail, &v) == nil {
				e.Detail = v
			}
		}
		entries[i] = e
	}
	return &model.AuditLogPage{Entries: entries, Total: int32(total)}, nil
}

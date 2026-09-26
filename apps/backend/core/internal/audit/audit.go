// Package audit persists the admin audit trail: every admin GraphQL mutation (recorded by the gqlgen operation interceptor in main.go) and every admin action run
// Writes are best-effort - an audit insert failure logs but never blocks the underlying operation
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const CfgRetentionDays = "system/audit/retention_days" // drives the daily audit-log-prune job; <= 0 keeps the trail forever

func init() {
	permission.RegisterGroup(permission.Group{Slug: "audit", Name: "Audit log", SortOrder: 95})
	permission.Register(permission.Definition{Group: "audit", Slug: "audit.view", Name: "View the admin audit log"})
	config.RegisterGroup(config.Group{Section: "system", Slug: "audit", Label: "Audit log", SortOrder: 2})
	config.Register(config.Definition{Path: CfgRetentionDays, Type: config.TypeInt, Default: "365", Label: "Audit log retention (days)"})
}

var secretKeyPattern = regexp.MustCompile(`(?i)password|secret|token|code|key|card|cvv|nonce`) // variable names whose VALUES must never be persisted - credentials, tokens, card data

const maxDetailBytes = 8 * 1024 // caps the stored variables JSON so a bulk import doesn't write megabytes per row

type Service struct {
	queries *db.Queries
}

func NewService(queries *db.Queries) *Service { return &Service{queries: queries} }

type Entry struct {
	AdminID    uuid.UUID // uuid.Nil = unknown
	AdminEmail string
	Action     string
	Detail     map[string]any
	IP         string
	Success    bool
	DurationMS int64
}

func (s *Service) Record(ctx context.Context, e Entry) { // redacts secret-ish keys and caps size - best-effort, so failures are logged and never returned
	detail := pqtype.NullRawMessage{}
	if len(e.Detail) > 0 {
		raw, err := json.Marshal(Redact(e.Detail))
		if err == nil {
			if len(raw) > maxDetailBytes {
				raw, _ = json.Marshal(map[string]any{"truncated": true, "bytes": len(raw)})
			}
			detail = pqtype.NullRawMessage{RawMessage: raw, Valid: true}
		}
	}
	adminID := uuid.NullUUID{}
	if e.AdminID != uuid.Nil {
		adminID = uuid.NullUUID{UUID: e.AdminID, Valid: true}
	}
	// Detached, because the request context may already be done (the interceptor records after the response) and audit must not be dropped just because the client hung up
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.queries.InsertAuditLog(wctx, db.InsertAuditLogParams{
		AdminID:    adminID,
		AdminEmail: e.AdminEmail,
		Action:     e.Action,
		Detail:     detail,
		Ip:         e.IP,
		Success:    e.Success,
		DurationMs: e.DurationMS,
	}); err != nil {
		slog.Error("audit: insert failed", "action", e.Action, "err", err)
	}
}

type Row struct {
	ID         uuid.UUID
	AdminID    *uuid.UUID
	AdminEmail string
	Action     string
	Detail     json.RawMessage
	IP         string
	Success    bool
	DurationMS int64
	CreatedAt  time.Time
}

func (s *Service) List(ctx context.Context, adminID *uuid.UUID, action *string, limit, offset int32) ([]Row, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	aid := uuid.NullUUID{}
	if adminID != nil {
		aid = uuid.NullUUID{UUID: *adminID, Valid: true}
	}
	act := sqlnull.StringNonEmpty(action)
	rows, err := s.queries.ListAuditLog(ctx, db.ListAuditLogParams{AdminID: aid, Action: act, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.queries.CountAuditLog(ctx, db.CountAuditLogParams{AdminID: aid, Action: act})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Row, len(rows))
	for i, r := range rows {
		row := Row{
			ID:         r.ID,
			AdminEmail: r.AdminEmail,
			Action:     r.Action,
			IP:         r.Ip,
			Success:    r.Success,
			DurationMS: r.DurationMs,
			CreatedAt:  r.CreatedAt,
		}
		if r.AdminID.Valid {
			v := r.AdminID.UUID
			row.AdminID = &v
		}
		if r.Detail.Valid {
			row.Detail = r.Detail.RawMessage
		}
		out[i] = row
	}
	return out, total, nil
}

func Redact(m map[string]any) map[string]any { // deep-copies, replacing values under secret-ish keys with a placeholder - exported for the interceptor and tests
	out := make(map[string]any, len(m))
	for k, v := range m {
		if secretKeyPattern.MatchString(k) {
			out[k] = "[REDACTED]"
			continue
		}
		switch t := v.(type) {
		case map[string]any:
			out[k] = Redact(t)
		case []any:
			cp := make([]any, len(t))
			for i, item := range t {
				if im, ok := item.(map[string]any); ok {
					cp[i] = Redact(im)
				} else {
					cp[i] = item
				}
			}
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}

func (s *Service) Prune(ctx context.Context, before time.Time) (int64, error) { // operational history, not a legal ledger, so age is the only criterion
	return s.queries.PruneAuditLog(ctx, before)
}

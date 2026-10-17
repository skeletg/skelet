// Package audit persists the admin audit trail: every admin GraphQL mutation (recorded by the operation interceptor in internal/wiring/graphql.go) and every admin action run.
// Writes are best-effort - an audit insert failure logs but never blocks the underlying operation.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
	"github.com/skeletg/skelet/apps/backend/core/internal/permission"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
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
	AdminID       uuid.UUID // uuid.Nil = unknown
	IntegrationID uuid.UUID
	AdminEmail    string
	Action        string
	Detail        map[string]any
	IP            string
	Success       bool
	DurationMS    int64
}

func Identify(ctx context.Context, e *Entry) { // fills who and from where off the request context - machine callers have no admin row, so the email column carries the integration's name instead
	if a, ok := auth.ActorFromContext(ctx); ok {
		e.AdminID = a.AdminID().UUID
		e.IntegrationID = a.IntegrationID().UUID
		e.AdminEmail = a.Name
		if a.Kind == auth.ActorIntegration {
			e.AdminEmail = "integration:" + a.Name
		}
	}
	if r, ok := auth.RequestFromContext(ctx); ok && r != nil {
		e.IP = auth.ClientIP(r)
	}
}

func (s *Service) Record(ctx context.Context, e Entry) { // redacts secret-ish keys and caps size - best-effort, so failures are logged and never returned
	detail := encodeDetail(e.Detail)
	adminID := uuid.NullUUID{}
	if e.AdminID != uuid.Nil {
		adminID = uuid.NullUUID{UUID: e.AdminID, Valid: true}
	}
	// Detached, because the request context may already be done (the interceptor records after the response) and audit must not be dropped just because the client hung up
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.queries.InsertAuditLog(wctx, db.InsertAuditLogParams{
		IntegrationID: uuid.NullUUID{UUID: e.IntegrationID, Valid: e.IntegrationID != uuid.Nil},
		AdminID:       adminID,
		AdminEmail:    e.AdminEmail,
		Action:        e.Action,
		Detail:        detail,
		Ip:            e.IP,
		Success:       e.Success,
		DurationMs:    e.DurationMS,
	}); err != nil {
		slog.Error("audit: insert failed", "action", e.Action, "err", err)
	}
}

type Row struct {
	ID            uuid.UUID
	AdminID       *uuid.UUID
	IntegrationID *uuid.UUID
	AdminEmail    string
	Action        string
	Detail        json.RawMessage
	IP            string
	Success       bool
	DurationMS    int64
	CreatedAt     time.Time
}

func (s *Service) List(ctx context.Context, adminID, integrationID *uuid.UUID, action *string, limit, offset int32) ([]Row, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset = max(offset, 0)
	aid := uuid.NullUUID{}
	if adminID != nil {
		aid = uuid.NullUUID{UUID: *adminID, Valid: true}
	}
	iid := uuid.NullUUID{}
	if integrationID != nil {
		iid = uuid.NullUUID{UUID: *integrationID, Valid: true}
	}
	act := sqlnull.StringNonEmpty(action)
	rows, err := s.queries.ListAuditLog(ctx, db.ListAuditLogParams{AdminID: aid, IntegrationID: iid, Action: act, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.queries.CountAuditLog(ctx, db.CountAuditLogParams{AdminID: aid, IntegrationID: iid, Action: act})
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
		if r.IntegrationID.Valid {
			v := r.IntegrationID.UUID
			row.IntegrationID = &v
		}
		if r.Detail.Valid {
			row.Detail = r.Detail.RawMessage
		}
		out[i] = row
	}
	return out, total, nil
}

const redacted = "[REDACTED]"

// Deep-copies, replacing values under secret-ish keys with a placeholder - exported for the interceptor and tests
// A settings save names its field in "path" and carries it in "value", neither of which looks secret, so a value beside the path of a secret-typed setting is redacted too
func Redact(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if secretKeyPattern.MatchString(k) {
			out[k] = redacted
			continue
		}
		out[k] = redactValue(v)
	}
	if path, ok := m["path"].(string); ok && isSecretSetting(path) {
		if _, has := out["value"]; has {
			out["value"] = redacted
		}
	}
	return out
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return Redact(t)
	case []any:
		cp := make([]any, len(t))
		for i, item := range t {
			cp[i] = redactValue(item)
		}
		return cp
	}
	return v
}

func isSecretSetting(path string) bool {
	def, ok := config.Find(path)
	return ok && def.Type == config.TypeSecret
}

func encodeDetail(d map[string]any) pqtype.NullRawMessage {
	if len(d) == 0 {
		return pqtype.NullRawMessage{}
	}
	raw, err := json.Marshal(Redact(d))
	if err != nil {
		return pqtype.NullRawMessage{}
	}
	if len(raw) > maxDetailBytes { // the field names still say roughly what the mutation touched
		raw, _ = json.Marshal(map[string]any{"truncated": true, "bytes": len(raw), "keys": sortedKeys(d)})
	}
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (s *Service) Prune(ctx context.Context, before time.Time) (int64, error) { // operational history, not a legal ledger, so age is the only criterion
	return s.queries.PruneAuditLog(ctx, before)
}

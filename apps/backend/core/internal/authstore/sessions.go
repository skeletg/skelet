package authstore

import (
	"context"
	"database/sql"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// The admin_sessions table as the refresh store's session log - keyed by the same token hash the refresh backend holds
type sessionLog struct{ q *db.Queries }

func NewSessionLog(q *db.Queries) auth.SessionLog { return sessionLog{q: q} }

func (l sessionLog) Create(ctx context.Context, adminID uuid.UUID, tokenHash, ip, userAgent string) (uuid.UUID, error) {
	var ipArg pqtype.Inet
	if parsed := net.ParseIP(ip); parsed != nil {
		bits := 32
		if parsed.To4() == nil {
			bits = 128
		}
		ipArg = pqtype.Inet{IPNet: net.IPNet{IP: parsed, Mask: net.CIDRMask(bits, bits)}, Valid: true}
	}
	var uaArg sql.NullString
	if userAgent != "" {
		uaArg = sql.NullString{String: userAgent, Valid: true}
	}
	// RETURNING id is atomic and never zero - a readback could miss the row on replica lag
	id, err := l.q.CreateAdminSession(ctx, db.CreateAdminSessionParams{
		AdminID:   adminID,
		TokenHash: tokenHash,
		IpAddress: ipArg,
		UserAgent: uaArg,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert admin session: %w", err)
	}
	return id, nil
}

func (l sessionLog) Delete(ctx context.Context, tokenHash string) ([]uuid.UUID, error) {
	return l.q.DeleteAdminSession(ctx, tokenHash)
}

func (l sessionLog) DeleteAll(ctx context.Context, adminID uuid.UUID) ([]uuid.UUID, error) {
	return l.q.DeleteAdminSessionsByAdminID(ctx, adminID)
}

func (l sessionLog) DeleteExcept(ctx context.Context, adminID uuid.UUID, keepHash string) ([]uuid.UUID, error) {
	return l.q.DeleteAdminSessionsExcept(ctx, db.DeleteAdminSessionsExceptParams{AdminID: adminID, TokenHash: keepHash})
}

func (l sessionLog) List(ctx context.Context, adminID uuid.UUID) ([]auth.AdminSession, error) {
	rows, err := l.q.ListAdminSessionsByAdminID(ctx, adminID)
	if err != nil {
		return nil, err
	}
	out := make([]auth.AdminSession, 0, len(rows))
	for _, r := range rows {
		s := auth.AdminSession{ID: r.ID, AdminID: r.AdminID, TokenHash: r.TokenHash, CreatedAt: r.CreatedAt, LastActiveAt: r.LastActiveAt}
		if r.IpAddress.Valid {
			s.IPAddress = r.IpAddress.IPNet.IP.String()
		}
		if r.UserAgent.Valid {
			s.UserAgent = r.UserAgent.String
		}
		out = append(out, s)
	}
	return out, nil
}

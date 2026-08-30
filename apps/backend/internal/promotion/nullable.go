package promotion

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
)

// Conversion helpers between domain pointer fields and sqlc's sql.NullXxx / uuid.NullUUID types - Kept in one place so
// the repo stays terse

func uuidPtrToNullUUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func nullUUIDToPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	v := n.UUID
	return &v
}

func timePtrToNullTime(p *time.Time) sql.NullTime {
	if p == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *p, Valid: true}
}

func nullTimeToPtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	v := n.Time
	return &v
}

func int32PtrToNullInt32(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *p, Valid: true}
}

func nullInt32ToPtr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}

func stringToNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringToString(n sql.NullString) string {
	if !n.Valid {
		return ""
	}
	return n.String
}

// pqtype.NullRawMessage is sqlc's wrapper for JSONB columns - Empty
// schedule_json round-trips as Valid=false
func rawMessageToNullRaw(m json.RawMessage) pqtype.NullRawMessage {
	if len(m) == 0 {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: m, Valid: true}
}

func nullRawToRawMessage(n pqtype.NullRawMessage) json.RawMessage {
	if !n.Valid {
		return nil
	}
	return n.RawMessage
}

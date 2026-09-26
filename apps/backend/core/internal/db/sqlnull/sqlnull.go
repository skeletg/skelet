// Package sqlnull converts between Go values and sqlc's null types - nil is NULL, and the NonZero/NonEmpty variants also store a zero value as NULL
package sqlnull

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
)

func UUID(p *uuid.UUID) uuid.NullUUID {
	if p == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func UUIDNonZero(p *uuid.UUID) uuid.NullUUID {
	if p == nil || *p == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *p, Valid: true}
}

func UUIDValueNonZero(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

func String(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func StringNonEmpty(p *string) sql.NullString {
	if p == nil || *p == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func StringValueNonEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func Time(p *time.Time) sql.NullTime {
	if p == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *p, Valid: true}
}

func Int16(p *int16) sql.NullInt16 {
	if p == nil {
		return sql.NullInt16{}
	}
	return sql.NullInt16{Int16: *p, Valid: true}
}

func Int32(p *int32) sql.NullInt32 {
	if p == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: *p, Valid: true}
}

func Int64(p *int64) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *p, Valid: true}
}

func Bool(p *bool) sql.NullBool {
	if p == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *p, Valid: true}
}

func Float64(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

// Empty or nil JSON is NULL
func Raw(b []byte) pqtype.NullRawMessage {
	if len(b) == 0 {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: b, Valid: true}
}

func UUIDPtr(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	v := n.UUID
	return &v
}

func StringPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

// NULL reads as ""
func StringValue(n sql.NullString) string {
	if !n.Valid {
		return ""
	}
	return n.String
}

func TimePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	v := n.Time
	return &v
}

func Int32Ptr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}

func Int64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func Float64Ptr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func RawMessage(n pqtype.NullRawMessage) json.RawMessage {
	if !n.Valid {
		return nil
	}
	return n.RawMessage
}

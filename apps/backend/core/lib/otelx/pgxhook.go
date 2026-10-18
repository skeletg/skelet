package otelx

import (
	"strings"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
)

// Emits an OTel span per query - nil when OTel is off, which Connect accepts
func PGXTracer() pgx.QueryTracer {
	if !enabled() {
		return nil
	}
	// statement text but NOT param values, which carry PII
	return otelpgx.NewTracer(otelpgx.WithTrimSQLInSpanName(), otelpgx.WithSpanNameFunc(QueryName))
}

// The sqlc query name from a "-- name: X :kind" header, else the statement's first word - the full SQL as a span
// name made every trace list and per-operation metric unreadable, and still lands on the span as db.query.text
func QueryName(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if rest, ok := strings.CutPrefix(stmt, "-- name:"); ok {
		if f := strings.Fields(rest); len(f) > 0 {
			return f[0]
		}
	}
	if f := strings.Fields(stmt); len(f) > 0 {
		return strings.ToLower(f[0])
	}
	return "unknown"
}

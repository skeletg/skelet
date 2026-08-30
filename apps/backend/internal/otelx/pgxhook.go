package otelx

import (
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
)

// Emits an OTel span per query - nil when OTel is off, which Connect accepts
func PGXTracer() pgx.QueryTracer {
	if !enabled() {
		return nil
	}
	return otelpgx.NewTracer() // statement text but NOT param values, which carry PII
}

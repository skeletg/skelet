package otelx_test

import (
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
)

func TestQueryName(t *testing.T) {
	for stmt, want := range map[string]string{
		"-- name: ClaimDueCampaigns :many\nUPDATE email_campaigns SET status = 'sending'": "ClaimDueCampaigns",
		"  -- name: LockChannelRoster :exec\nSELECT pg_advisory_xact_lock(1)":             "LockChannelRoster",
		"begin":      "begin",
		"SELECT 1":   "select",
		"\n\tCOMMIT": "commit",
		"":           "unknown",
	} {
		if got := otelx.QueryName(stmt); got != want {
			t.Errorf("QueryName(%q) = %q, want %q", stmt, got, want)
		}
	}
}

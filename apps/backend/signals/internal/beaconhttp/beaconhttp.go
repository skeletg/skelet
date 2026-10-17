// Package beaconhttp is the suite's public edge - /t, the storefront tracker's beacons and SDK reads, and /e, the mail pixel and
// click redirect - mounted alike by cmd/signals and the cmd/ingest tier, so core's proxy contract never changes
package beaconhttp

import (
	"context"
	"net/http"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Limiter bounds these paths per IP and route - nil admits, rather than refusing every beacon
type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

func allow(l Limiter, r *http.Request, route string) bool {
	return l == nil || l.Allow(r.Context(), route+":"+auth.ClientIP(r))
}

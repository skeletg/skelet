package wiring

import (
	"net/http"
	"time"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/tracking"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func newTrackingService(q *db.Queries, cipher *secrets.Cipher, cfg *config.Service) *tracking.Service {
	var sealer tracking.Sealer // a nil *Cipher in the interface would read as a key that panics on use
	if cipher != nil {
		sealer = cipher
	}
	tracking.RegisterPluginKinds(sdk.Destinations(), plugin.Enabled)
	return tracking.NewService(q, sealer, cfg)
}

// The same SSRF-safe transport webhooks use - a destination's endpoint (a server-side GTM container, say) is typed by an admin
func newTrackingWorker(svc *tracking.Service, allow AllowPrivateTargets) *tracking.Worker {
	return tracking.NewWorker(svc, trackingHTTPClient(allow))
}

// Shared by the worker and the dashboard's test sends, so a test reaches exactly what a real delivery would
func trackingHTTPClient(allow AllowPrivateTargets) *http.Client {
	return &http.Client{Transport: otelx.HTTPTransport(webhook.SafeTransport(bool(allow))), Timeout: 30 * time.Second}
}

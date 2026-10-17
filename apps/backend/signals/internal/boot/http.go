package boot

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
	"github.com/skeletg/skelet/apps/backend/signals/internal/geo"
	"github.com/skeletg/skelet/apps/backend/signals/internal/ingesthttp"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
	"github.com/skeletg/skelet/apps/backend/signals/internal/pulsehttp"
	"github.com/skeletg/skelet/apps/backend/signals/internal/segment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/signalsboot"
)

// Beacon is the public surface both entrypoints mount - core proxies /t and /e here rather than
// serving them from a spine stack of its own
var Beacon = fx.Module("beacon", fx.Provide(newBeaconMux))

type beaconDeps struct {
	fx.In

	Config     Config
	Pool       SignalsPool
	Limits     LimitsValkey
	Geo        *geo.Resolver
	Stack      *signalsboot.Stack
	Lens       *lens.Service
	Experiment *experiment.Service
	Segment    *segment.Service
	Channels   *analytics.ChannelRoster
	Rollup     *analytics.TrafficRollup
	Flow       *flow.Service
	Campaign   *campaign.Service
}

func newBeaconMux(d beaconDeps) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterPprof(mux)
	mux.Handle("/t/", ingesthttp.New(ingesthttp.Options{
		Channels: d.Channels, Lens: d.Lens, Experiments: d.Experiment, Segments: d.Segment,
		Geo: d.Geo, Rollup: d.Rollup,
		Limiter:   ratelimit.NewRedis(d.Limits.Client, RateLimitPerMin("INGEST_RATE_LIMIT_PER_MIN", 300), time.Minute),
		Customers: d.Config.Customers,
	}))
	// The open pixel and click redirect every marketing mail carries, so the hit lands beside the send log it names
	mux.Handle("/e/", pulsehttp.New(pulsehttp.Options{
		Campaigns: d.Campaign, Flows: d.Flow,
		Limiter: ratelimit.NewRedis(d.Limits.Client, RateLimitPerMin("TOKEN_RATE_LIMIT_PER_MIN", 60), time.Minute),
	}))
	chPing := d.Stack.CHPing
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := d.Pool.PingContext(r.Context()); err != nil { // every datastore this process prunes and forwards against - a reachable engine alone read healthy while the work errored forever
			http.Error(w, "signals db unavailable", http.StatusServiceUnavailable)
			return
		}
		if chPing != nil {
			if err := chPing(r.Context()); err != nil {
				http.Error(w, "clickhouse unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// IngestFallback hands the commerce beacons to core, which has the subscriber graph they land in
var IngestFallback = fx.Module("ingestfallback", fx.Invoke(mountCommerceFallback))

func mountCommerceFallback(mux *http.ServeMux) error {
	raw := os.Getenv("INGEST_FALLBACK_URL")
	if raw == "" { // unset they 404 here, which the SDKs fail soft on
		slog.Warn("INGEST_FALLBACK_URL unset - the commerce beacons will 404 here, route them to the app", "paths", signalsapi.CommerceBeaconPaths)
		return nil
	}
	target, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse INGEST_FALLBACK_URL: %w", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	for _, p := range signalsapi.CommerceBeaconPaths {
		mux.Handle(p, proxy)
	}
	return nil
}

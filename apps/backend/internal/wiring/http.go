package wiring

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/rs/cors"

	"go.uber.org/fx"

	graphadmin "github.com/skeletg/skelet/apps/backend/graph/admin"
	graphfront "github.com/skeletg/skelet/apps/backend/graph/front"
	"github.com/skeletg/skelet/apps/backend/graph/front/dataloaders"
	authpkg "github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/blog"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/campaign"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/download"
	"github.com/skeletg/skelet/apps/backend/internal/httpx"
	"github.com/skeletg/skelet/apps/backend/internal/ingesthttp"
	"github.com/skeletg/skelet/apps/backend/internal/integration"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/media"
	"github.com/skeletg/skelet/apps/backend/internal/mediahttp"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/otelx"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	pluginhost "github.com/skeletg/skelet/apps/backend/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
	"github.com/skeletg/skelet/apps/backend/internal/review"
	"github.com/skeletg/skelet/apps/backend/internal/sentryx"
	"github.com/skeletg/skelet/apps/backend/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/internal/suppression"
)

type RootHandler struct{ http.Handler } // the fully wrapped HTTP entrypoint - every route tree behind the recover / security-headers / body-limit / request-id / sentry / CORS chain

var HTTP = fx.Module("http", fx.Provide(newRootHandler)) // the GraphQL endpoints, the public HTTP routes and the outer middleware chain

func newRootHandler(infra Infra, s Svcs, tel observability.Telemetry, rl RateLimiters, g GQL) RootHandler {
	// Resolved once: a slug typo must surface here, not as a dashboard that 404s
	adminPath, frontPath, pathErr := APIPaths(os.Getenv)
	if pathErr != nil {
		log.Fatalf("api path: %v", pathErr)
	}
	adminSrv := g.Admin.Server
	frontSrv := g.Front.Server
	port := infra.Config.Port

	// Gateways POST here and HandleWebhook verifies the signature with its own webhook_secret - the /api/v1/ prefix is historical (it predates the partner REST tier's removal) and stays because provider dashboards have the URL configured
	paymentWebhookHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		slug := strings.TrimPrefix(r.URL.Path, "/api/v1/payments/webhook/")
		slug = strings.Trim(slug, "/")
		if slug == "" {
			http.Error(w, "provider slug required", http.StatusBadRequest)
			return
		}
		if err := s.Payment.DispatchWebhook(r.Context(), slug, r); err != nil {
			if errors.Is(err, payment.ErrProviderNotFound) {
				http.Error(w, "unknown provider", http.StatusNotFound)
				return
			}
			if pluginhost.IsTransientHookError(err) { // transient (kill switch, rate limit, DB blip) answers 5xx so the gateway retries - 400 reads as permanent, and a provider's own reject stays 400

				slog.Warn("payment webhook deferred (transient)", "provider", slug, "err", err)
				w.Header().Set("Retry-After", "60")
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			slog.Warn("payment webhook rejected", "provider", slug, "err", err) // response is deliberately opaque - unauthenticated, and the error carries the plugin slug, kill-switch state and whatever the provider said, so echoing it lets anyone enumerate installed gateways
			http.Error(w, "webhook rejected", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux := http.NewServeMux()
	// Gates on the CORE datastores only (a dead DB must report unhealthy, an unconfigured optional plugin must not, since dependent services wait on this to boot) - memoized to one real ping per second (the mutex collapses concurrent callers) so a flood can't become backend load, on a background-derived ctx so a caller hanging up mid-probe doesn't cache "context canceled" for the TTL
	var (
		healthMu  sync.Mutex
		healthAt  time.Time
		healthErr error
		healthTTL = time.Second
	)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthMu.Lock()
		if time.Since(healthAt) >= healthTTL {
			pctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := infra.DB.PingContext(pctx)
			if err != nil {
				err = fmt.Errorf("db not ready: %w", err)
			} else if infra.Valkey != nil {
				if verr := infra.Valkey.Ping(pctx).Err(); verr != nil { // An absent Valkey is a choice, not a fault - only ping a configured one
					err = fmt.Errorf("valkey not ready: %w", verr)
				}
			}
			cancel()
			healthErr = err
			healthAt = time.Now()
		}
		err := healthErr
		healthMu.Unlock()
		if err != nil {
			slog.Warn("health check failed", "err", err) // unauthenticated probe - the cause names the datastore and often its host, so the body stays generic
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		// Search is a SOFT dependency - a downed cluster must not 503 the app (that would take the whole storefront down to fix a degraded one) - but it can't be invisible either: browse is the only listing path, so a silent outage reads as an empty catalogue - report it in the body for monitoring to alarm on
		body := "ok"
		if s.SearchTier.Client != nil {
			sctx, scancel := context.WithTimeout(r.Context(), 2*time.Second)
			if serr := s.SearchTier.Client.Ping(sctx); serr != nil {
				body = "ok (degraded: search unavailable - product listings are empty)"
			}
			scancel()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	// Plugin liveness (/healthz, dependencies reachable) and readiness (/readyz, configured correctly) - 503 when any probe fails, JSON body per plugin - operator probes, NOT the container healthcheck, since an unconfigured optional gateway legitimately fails readiness
	mux.Handle("/healthz", pluginhost.HealthHandler())
	mux.Handle("/readyz", pluginhost.ReadinessHandler())
	mux.Handle("/api/admin-ui/i18n/", otelx.WrapHTTP(pluginhost.AdminTranslationsHandler(), "adminui.i18n"))
	// auth (JWT → claims) → channel (X-Channel-Id / Host → store) → locale (header → tag), all best-effort: resolvers needing one call FromContext and surface their own error, so introspection and admin queries still work without them - devDebug adds per-request DB-query counting + X-DB-Queries / X-Trace-Id in dev, identity otherwise, and sits inside otelx.WrapHTTP so the trace id exists
	devDebug := func(h http.Handler) http.Handler {
		if infra.Config.IsDev() {
			return httpx.DevDebugMiddleware(h)
		}
		return h
	}
	// Complexity limits bound query SHAPE, not raw payload bytes - cap the body so a multi-megabyte POST can't tie up parsing - 1MB comfortably fits any legitimate mutation (rich text, bulk CSV goes elsewhere)
	// integration.Middleware makes this endpoint double as the machine API: an X-Api-Token integration token authorizes through the same resolver permission gates as an admin role, with its grant set global - there is no separate REST tier
	mux.Handle(adminPath, otelx.WrapHTTP(devDebug(maxBody(1<<20)(authpkg.Middleware(infra.Config.JWTSecret)(integration.Middleware(s.Integration)(authpkg.AdminSessionGuard(s.AdminRefresh.RefreshStore)(channel.AdminMiddleware(s.Channel)(locale.Middleware()(graphadmin.RenderCacheMiddleware(adminSrv)))))))), "graphql.admin"))
	voterResolverFn := func(ctx context.Context) (review.Voter, error) { // the MyReviewVote loader's voter closure lives here because dataloaders can't import graph/front (cycle) - it delegates to s.Review.ResolveVoter, so the policy chain is the resolver's own
		customerID := authpkg.OptionalCustomerID(ctx)
		var rawToken string
		if req, ok := authpkg.RequestFromContext(ctx); ok {
			rawToken = req.Header.Get("X-Review-Voter-Token")
		}
		return s.Review.ResolveVoter(ctx, customerID, rawToken, infra.Config.VoterTokenSecret)
	}
	dataloaderDeps := dataloaders.Deps{
		Customer:      s.Customer,
		Review:        s.Review,
		Channel:       s.Channel,
		Catalog:       s.Catalog,
		Promotion:     s.Promotion,
		MediaLibrary:  s.MediaLibrary,
		Voter:         voterResolverFn,
		PluginLoaders: pluginhost.DataLoaderRegistrations(),
	}
	// LocaleClampMiddleware sits inside both channel + locale middleware (it needs the resolved channel AND the extracted header) and snaps unsupported locales to the channel default - front surface only - admin reads must keep accepting arbitrary locales for translation tooling
	frontRespCache := frontResponseCache(cache.New(infra.Cache.Client, "frontresp"), 60*time.Second) // sits INSIDE channel + locale resolution so its key uses the RESOLVED channel id and clamped locale - keying on the raw headers bled cached responses across host-resolved siblings that send no X-Channel-Id at all
	mux.Handle(frontPath, otelx.WrapHTTP(frontRateLimit(rl.Front)(devDebug(maxBody(256<<10)(authpkg.Middleware(infra.Config.JWTSecret)(authpkg.CustomerSessionGuard(s.Customer.IsActive)(channel.Middleware(s.Channel)(locale.Middleware()(channel.LocaleClampMiddleware()(frontRespCache(dataloaders.Middleware(dataloaderDeps)(graphfront.RenderCacheMiddleware(frontSrv))))))))))), "graphql.front"))

	mux.Handle("/api/v1/payments/webhook/", otelx.WrapHTTP(paymentWebhookHandler, "payments.webhook"))
	// Plugin-registered routes, namespaced per plugin under /api/v1/plugins/<slug>/ - public and unauthenticated like the storefront GraphQL endpoint, so they share its rate limiter; each handler is wrapped in the plugin's kill switch, timeout and panic isolation by the host - this is the seam that replaced the GraphQL bridge for plugin-owned browser endpoints
	if pluginRoutes := pluginhost.RouteHandler(); pluginRoutes != nil {
		mux.Handle(pluginhost.RoutePrefix, otelx.WrapHTTP(
			frontRateLimit(rl.Front)(maxBody(64<<10)(channel.Middleware(s.Channel)(pluginRoutes))),
			"plugins.routes"))
	}
	// No separate REST tier - machine callers hit the admin GraphQL endpoint above with an X-Api-Token integration token; a dedicated /api/v1/ REST surface may still come back once a real consumer needs one

	transformLimiter := ratelimit.NewRedis(infra.Cache.Client, RateLimitPerMin("MEDIA_TRANSFORM_RATE_LIMIT_PER_MIN", 120), time.Minute) // the /media/t/* transform handler is shared with cmd/media, which serves the same route standalone - route a proxy path there and this goes cold
	mediaTransformer := media.NewTransformer(s.MediaStorage)
	mediaTransformer.SetMaxSourcePixels(EnvInt("MEDIA_MAX_SOURCE_PIXELS", 0))
	mediaTransformer.SetRenderConcurrency(EnvInt("MEDIA_RENDER_SLOTS", 0), 0)
	mux.Handle("/media/t/", mediahttp.NewTransformHandler(mediaTransformer, mediahttp.Options{
		AllowRateLimit: func(r *http.Request) bool {
			return transformLimiter.Allow(r.Context(), "media-transform:"+authpkg.ClientIP(r))
		},
		Focal: func(r *http.Request, key string) (float64, float64, bool) {
			asset, found := s.MediaLibrary.AssetByObjectKey(r.Context(), key)
			if !found || asset.FocalX == nil || asset.FocalY == nil {
				return 0, 0, false
			}
			return *asset.FocalX, *asset.FocalY, true
		},
	}))

	// Blog public routes - XML responses don't fit the GraphQL surface so they live as plain HTTP - the channel middleware resolves the active store from the X-Channel-Id header / Host so the handlers can scope posts via per-store listings; non-store requests fall back to global (admin / single-store deployments)
	mux.Handle("/blog/feed.rss.xml", channel.Middleware(s.Channel)(blogFeedHandler(s.Blog, blog.FeedKindRSS)))
	mux.Handle("/blog/feed.atom.xml", channel.Middleware(s.Channel)(blogFeedHandler(s.Blog, blog.FeedKindAtom)))
	mux.Handle("/sitemap.xml", channel.Middleware(s.Channel)(blogSitemapHandler(s.Blog)))
	mux.Handle("/feeds/products.xml", channel.Middleware(s.Channel)(merchantFeedHandler(catalog.NewMerchantFeed(infra.Queries, s.MediaStorage)))) // Google Merchant product feed: channel-priced, channel-scoped - operators point Merchant Center at https://<store-host>/feeds/products.xml

	// consume enforces the grant's use-count + expiry under a row lock before the 302; the per-IP budget is shared with the other token-keyed public endpoints - tokens are unguessable so forgery isn't the threat, replay-flooding is, since each hit costs a token verify plus DB work and doubles as a brute-force oracle
	tokenLimiter := ratelimit.NewRedis(infra.Cache.Client, RateLimitPerMin("TOKEN_RATE_LIMIT_PER_MIN", 60), time.Minute)
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/download/")
		if token == "" {
			http.NotFound(w, r)
			return
		}
		if !tokenLimiter.Allow(r.Context(), "download:"+authpkg.ClientIP(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		grant, err := s.Download.Consume(r.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, download.ErrNotFound):
				http.NotFound(w, r)
			case errors.Is(err, download.ErrExpired) || errors.Is(err, download.ErrLimitReached):
				http.Error(w, err.Error(), http.StatusGone)
			default:
				http.Error(w, "download error", http.StatusInternalServerError)
			}
			return
		}
		http.Redirect(w, r, grant.URL, http.StatusFound)
	})

	mux.HandleFunc("/unsubscribe/", func(w http.ResponseWriter, r *http.Request) { // one-click unsubscribe - the signed token (HMAC over the customer id) lets the link work without auth; consuming it records a marketing_email opt-out - public, reached via the storefront /unsubscribe rewrite (mirrors /download)
		token := strings.TrimPrefix(r.URL.Path, "/unsubscribe/")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if !tokenLimiter.Allow(r.Context(), "unsub:"+authpkg.ClientIP(r)) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:48px;"><h1>Too many requests</h1><p>Please try again in a minute.</p></body></html>`))
			return
		}
		if token == "" || s.Campaign.Unsubscribe(r.Context(), token) != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:48px;"><h1>Link invalid</h1><p>This unsubscribe link is invalid or has expired.</p></body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:48px;"><h1>You're unsubscribed</h1><p>You will no longer receive marketing emails from us.</p></body></html>`))
	})

	mux.HandleFunc("/e/o/", func(w http.ResponseWriter, r *http.Request) { // email open pixel - a mail client rendering <img src=".../e/o/<token>"> reports the open - best-effort: always return the 1x1 GIF, record only a valid token - public, reached via the storefront /e rewrite (like /unsubscribe)
		// Over-limit: still serve the pixel (a mail client must never see an error), just skip the recording - same best-effort stance as /t
		if tokenLimiter.Allow(r.Context(), "eopen:"+authpkg.ClientIP(r)) {
			if ev := s.Campaign.RecordOpen(r.Context(), strings.TrimPrefix(r.URL.Path, "/e/o/"),
				signalsclient.Hit{UA: r.UserAgent(), At: time.Now()}); ev != nil {
				s.Bus.Publish(r.Context(), ev)
			}
		}
		w.Header().Set("Content-Type", "image/gif")
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		_, _ = w.Write(campaign.TransparentGIF())
	})

	mux.HandleFunc("/e/c/", func(w http.ResponseWriter, r *http.Request) { // click redirect - records the click then 302s to the signed destination baked into the token (so the target can't be tampered into an open redirect)
		if !tokenLimiter.Allow(r.Context(), "eclick:"+authpkg.ClientIP(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		dest, ev := s.Campaign.RecordClick(r.Context(), strings.TrimPrefix(r.URL.Path, "/e/c/"),
			signalsclient.Hit{UA: r.UserAgent(), At: time.Now()})
		if ev != nil {
			s.Bus.Publish(r.Context(), ev)
		}
		if dest == "" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})

	// The seam an ESP's bounce/complaint webhook posts to (SES/SNS, Postmark, SendGrid - transform to this shape) - Bearer-authed via EMAIL_WEBHOOK_TOKEN so only the configured provider can suppress addresses, 404 when unset - payload: {"type":"bounce"|"complaint","subtype":"hard"|"soft","email":"...","detail":"..."}
	emailWebhookToken := strings.TrimSpace(os.Getenv("EMAIL_WEBHOOK_TOKEN"))
	mux.HandleFunc("/webhooks/email/events", func(w http.ResponseWriter, r *http.Request) {
		if emailWebhookToken == "" {
			http.NotFound(w, r) // feature off until a token is configured
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(emailWebhookToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if on, _ := s.Config.GetBool(r.Context(), "pulse/deliverability/auto_suppress", config.DefaultScope); !on {
			w.WriteHeader(http.StatusNoContent) // ingest accepted but auto-suppress off
			return
		}
		var ev struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Email   string `json:"email"`
			Detail  string `json:"detail"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&ev); err != nil || ev.Email == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// A dropped suppression keeps mailing an address that complained, so a write failure answers 5xx and the ESP redelivers rather than reading the 204 as handled
		var suppressErr error
		switch {
		case ev.Type == "complaint":
			suppressErr = s.Suppression.Suppress(r.Context(), ev.Email, suppression.ReasonComplaint, ev.Detail)
		case ev.Type == "bounce" && ev.Subtype == "soft":
			threshold := 3
			if n, e := s.Config.GetInt(r.Context(), "pulse/deliverability/soft_bounce_threshold", config.DefaultScope); e == nil && n > 0 {
				threshold = int(n)
			}
			_, _, suppressErr = s.Suppression.RecordSoftBounce(r.Context(), ev.Email, ev.Detail, threshold)
		case ev.Type == "bounce":
			suppressErr = s.Suppression.Suppress(r.Context(), ev.Email, suppression.ReasonHardBounce, ev.Detail)
		default:
			http.Error(w, "unknown event type", http.StatusBadRequest)
			return
		}
		if suppressErr != nil {
			slog.Error("suppression write failed", "type", ev.Type, "subtype", ev.Subtype, "err", suppressErr)
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Bulk transfer files move over HTTP, not GraphQL - same auth chain as admin GraphQL, same permission rules as the resolvers
	adminAuth := func(h http.Handler) http.Handler {
		return authpkg.Middleware(infra.Config.JWTSecret)(authpkg.AdminSessionGuard(s.AdminRefresh.RefreshStore)(h))
	}
	mux.Handle(dataImportPath, otelx.WrapHTTP(adminAuth(dataImportHandler(s.DataJob, s.Admin)), "data.import"))
	mux.Handle("/api/admin/data/template", otelx.WrapHTTP(adminAuth(dataTemplateHandler(s.DataJob, s.Admin)), "data.template"))
	mux.Handle("/api/admin/data/jobs/", otelx.WrapHTTP(adminAuth(dataDownloadHandler(s.DataJob, s.Admin)), "data.download"))
	// SVG can't use the GraphQL presign path - see media_svg_http.go for why the bytes come here first
	mux.Handle(mediaSVGPath, otelx.WrapHTTP(adminAuth(mediaSVGHandler(s.MediaLibrary, s.Admin)), "media.svg"))

	// The /t surface lives in ingesthttp, shared with cmd/ingest, so high-volume beacon traffic can move to its own tier without changing the proxy contract - the app keeps serving it, same as the media tier
	mux.Handle("/t/", ingesthttp.New(ingesthttp.Options{
		Channels: s.Channel, Lens: s.Lens, Experiments: s.Experiment, Segments: s.Segment,
		Config: s.Config, Geo: s.Geo, Rollup: s.TrafficRollup,
		Limiter:   ratelimit.NewRedis(infra.Cache.Client, RateLimitPerMin("INGEST_RATE_LIMIT_PER_MIN", 300), time.Minute),
		JWTSecret: infra.Config.JWTSecret, SignalsEnabled: bool(s.SignalsEnabled),
		BackInStock: s.BackInStock, Forms: s.Form,
	}))

	if infra.Config.IsDev() { // playground never ships to production - it exposes schema + an unauthenticated query runner against /api/graphql
		mux.Handle("/playground/admin", playground.Handler("Admin GraphQL", adminPath))
		mux.Handle("/playground/front", playground.Handler("Storefront GraphQL", frontPath))
		log.Printf("Admin GraphQL playground: http://localhost:%s/playground/admin", port)
		log.Printf("Storefront GraphQL playground: http://localhost:%s/playground/front", port)
	}

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   AllowedOrigins(infra.Config),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Apollo-Require-Preflight", "Locale", "X-Channel-Id"},
		AllowCredentials: false, // JWT travels in Authorization header, not cookies
		MaxAge:           300,
	}).Handler(mux)

	// Outer-first: Recover → RequestID → Sentry → cors → mux - Recover outermost so it catches panics from anywhere downstream, including sentryhttp itself (Repanic=true) - RequestID stamps before Sentry so the per-request Hub scope can be tagged with it; Sentry is the identity wrapper when Reporter is nil, so wiring stays unconditional
	sentryMw := sentryx.Middleware(tel.Reporter != nil)
	maxBodyBytes := int64(8 << 20) // DoS guard - API payloads are small; media uploads bypass the server via presigned S3 URLs - override with REQUEST_MAX_BODY_BYTES
	if v := os.Getenv("REQUEST_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBodyBytes = n
		}
	}
	rootHandler := httpx.Recover(httpx.SecurityHeaders(httpx.LimitBody(maxBodyBytes, httpx.RequestID(sentryMw(corsHandler)), dataImportPath)))
	return RootHandler{rootHandler}
}

func maxBody(n int64) func(http.Handler) http.Handler { // GraphQL's complexity limit bounds resolver work but not raw payload size; MaxBytesReader makes oversized POSTs fail fast with 413 semantics
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// frontRateLimit caps requests to the public storefront GraphQL endpoint per client IP (keyed via auth.ClientIP, so it respects TRUSTED_PROXIES) - sits outermost so a flood is rejected before body parsing / auth / dataloaders - fail-open: the underlying limiter degrades open on a Valkey outage - replies with a GraphQL-shaped error envelope so storefront clients parse it cleanly
// A crawl's per-IP bucket would refuse thousands of SSR renders halfway through - the crawler carries this token so the map measures the real site
const crawlBypassHeader = "X-Skelet-Crawl"

func frontRateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	bypass := os.Getenv("SEO_CRAWL_BYPASS_TOKEN")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bypass != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get(crawlBypassHeader)), []byte(bypass)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			if !limiter.Allow(r.Context(), "front:ip:"+authpkg.ClientIP(r)) {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"errors":[{"message":"rate limit exceeded, retry after 60s"}]}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// blogFeedHandler returns an HTTP handler that renders a blog RSS or Atom feed - locale, channel, and base URL are pulled from the channel context; the `locale` query parameter still wins if provided - without a resolved store the feed is global (suitable for single-store deployments)
func blogFeedHandler(svc *blog.Service, kind blog.FeedKind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		locale := r.URL.Query().Get("locale")
		baseURL := os.Getenv("STOREFRONT_URL")
		opts := blog.FeedOptions{BaseURL: baseURL, Locale: locale}
		if sv, ok := channel.FromContext(ctx); ok && sv != nil {
			id := sv.ID
			opts.ChannelID = &id
			if locale == "" {
				opts.Locale = sv.DefaultLocale
			}
			if sv.PublicURL != "" {
				opts.BaseURL = sv.PublicURL
			}
			if opts.Title == "" {
				opts.Title = sv.Name
			}
		}
		body, contentType, err := svc.RenderFeed(ctx, kind, opts)
		if err != nil {
			slog.Error("render feed", "kind", kind, "err", err) // public endpoint - the cause names queries, tables and channel internals
			http.Error(w, "feed unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(body)
	}
}

func merchantFeedHandler(feed *catalog.MerchantFeed) http.HandlerFunc { // prices and currency are channel-bound, so a request that doesn't resolve to a channel gets 404 rather than a mispriced global feed
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sv, ok := channel.FromContext(ctx)
		if !ok || sv == nil {
			http.NotFound(w, r)
			return
		}
		baseURL := sv.PublicURL
		if baseURL == "" {
			baseURL = os.Getenv("STOREFRONT_URL")
		}
		locale := r.URL.Query().Get("locale")
		if locale == "" {
			locale = sv.DefaultLocale
		}
		body, contentType, err := feed.Render(ctx, catalog.MerchantFeedOptions{
			ChannelID:    sv.ID,
			BaseURL:      baseURL,
			CurrencyCode: sv.CurrencyCode,
			Locale:       locale,
			Title:        sv.Name,
		})
		if err != nil {
			slog.Error("render merchant feed", "channel", sv.ID, "err", err)
			http.Error(w, "feed unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=3600") // feeds are pulled by Merchant Center on a schedule; an hour of staleness is fine and keeps the flat query off the hot path
		_, _ = w.Write(body)
	}
}

func blogSitemapHandler(svc *blog.Service) http.HandlerFunc { // scoped to the resolved channel when present so each store gets its own URL set
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		baseURL := os.Getenv("STOREFRONT_URL")
		opts := blog.SitemapOptions{BaseURL: baseURL}
		if sv, ok := channel.FromContext(ctx); ok && sv != nil {
			id := sv.ID
			opts.ChannelID = &id
			if sv.PublicURL != "" {
				opts.BaseURL = sv.PublicURL
			}
		}
		body, contentType, err := svc.RenderSitemap(ctx, opts)
		if err != nil {
			slog.Error("render sitemap", "err", err)
			http.Error(w, "sitemap unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=600")
		_, _ = w.Write(body)
	}
}

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
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/rs/cors"

	"go.uber.org/fx"

	graphadmin "github.com/skeletg/skelet/apps/backend/core/graph/admin"
	graphfront "github.com/skeletg/skelet/apps/backend/core/graph/front"
	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalogio"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/media"
	"github.com/skeletg/skelet/apps/backend/core/internal/mediahttp"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	pluginhost "github.com/skeletg/skelet/apps/backend/core/internal/plugin"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/internal/review"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/cache"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/httpx"
	"github.com/skeletg/skelet/apps/backend/core/lib/objstore"
	"github.com/skeletg/skelet/apps/backend/core/lib/observability"
	"github.com/skeletg/skelet/apps/backend/core/lib/otelx"
	"github.com/skeletg/skelet/apps/backend/core/lib/ratelimit"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
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
	mux.Handle(adminPath, otelx.WrapHTTP(devDebug(maxBody(1<<20)(authpkg.AdminMiddleware(infra.Config.AdminKeys)(integration.Middleware(s.Integration)(authpkg.AdminSessionGuard(s.AdminRefresh.RefreshStore)(channel.AdminMiddleware(s.Channel)(locale.Middleware()(remoteprovider.RequestCacheMiddleware(graphadmin.RenderCacheMiddleware(adminSrv))))))))), "graphql.admin"))
	voterResolverFn := func(ctx context.Context) (review.Voter, error) { // the MyReviewVote loader's voter closure lives here because dataloaders can't import graph/front (cycle) - it delegates to s.Review.ResolveVoter, so the policy chain is the resolver's own
		customerID := channel.OptionalCustomerID(ctx)
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
	mux.Handle(frontPath, otelx.WrapHTTP(frontRateLimit(rl.Front, infra.Config.SignalsServiceKeys)(devDebug(maxBody(256<<10)(authpkg.CustomerMiddleware(infra.Config.CustomerKeys.Verifier())(authpkg.CustomerSessionGuard(s.Customer.IsActive)(channel.Middleware(s.Channel)(s.Cart.OfferGrantMiddleware(locale.Middleware()(channel.LocaleClampMiddleware()(frontRespCache(dataloaders.Middleware(dataloaderDeps)(remoteprovider.RequestCacheMiddleware(graphfront.RenderCacheMiddleware(frontSrv))))))))))))), "graphql.front"))

	mux.Handle("/api/v1/payments/webhook/", otelx.WrapHTTP(paymentWebhookHandler, "payments.webhook"))
	// Plugin-registered routes, namespaced per plugin under /api/v1/plugins/<slug>/ - public like the storefront GraphQL endpoint, with its customer session so sdk.CustomerID and ReadCart see a signed-in shopper, so they share its rate limiter; each handler is wrapped in the plugin's kill switch, timeout and panic isolation by the host - this is the seam that replaced the GraphQL bridge for plugin-owned browser endpoints
	if pluginRoutes := pluginhost.RouteHandler(); pluginRoutes != nil {
		mux.Handle(pluginhost.RoutePrefix, otelx.WrapHTTP(
			frontRateLimit(rl.Front, infra.Config.SignalsServiceKeys)(maxBody(64<<10)(pluginRouteAuth(infra.Config.CustomerKeys.Verifier(), s.Customer.IsActive)(channel.Middleware(s.Channel)(pluginRoutes)))),
			"plugins.routes"))
	}
	// No separate REST tier - machine callers hit the admin GraphQL endpoint above with an X-Api-Token integration token; a dedicated /api/v1/ REST surface may still come back once a real consumer needs one

	transformLimiter := ratelimit.NewRedis(infra.Limits.Client, RateLimitPerMin("MEDIA_TRANSFORM_RATE_LIMIT_PER_MIN", 120), time.Minute) // the /media/t/* transform handler is shared with cmd/media, which serves the same route standalone - route a proxy path there and this goes cold
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

	// The filesystem driver serves its own objects and takes the browser's uploads, which S3 would have answered
	if files, ok := s.MediaStorage.(interface{ Handler() http.Handler }); ok {
		mux.Handle(objstore.FilesPath, otelx.WrapHTTP(files.Handler(), "media.files"))
	}

	// XML doesn't fit the GraphQL surface, so the product feed is plain HTTP, scoped by the channel middleware
	feed := catalogio.NewMerchantFeed(infra.Queries, s.MediaStorage, s.Inventory, s.Catalog)
	mux.Handle("/feeds/products.xml", channel.Middleware(s.Channel)(merchantFeedHandler(feed))) // Google Merchant product feed: channel-priced, channel-scoped - operators point Merchant Center at https://<store-host>/feeds/products.xml

	// consume enforces the grant's use-count + expiry under a row lock before the 302; the per-IP budget is shared with the other token-keyed public endpoints - tokens are unguessable so forgery isn't the threat, replay-flooding is, since each hit costs a token verify plus DB work and doubles as a brute-force oracle
	tokenLimiter := ratelimit.NewRedis(infra.Limits.Client, RateLimitPerMin("TOKEN_RATE_LIMIT_PER_MIN", 60), time.Minute)
	var downloadFiles download.FileStore
	if s.MediaStorage != nil {
		downloadFiles = storedDownloads{s.MediaStorage}
	}
	mux.Handle("/download/", s.Download.Handler(downloadFiles, func(r *http.Request) bool {
		return tokenLimiter.Allow(r.Context(), "download:"+authpkg.ClientIP(r))
	}))

	// The suite signs the unsubscribe link over the customer id with the shared key, so it works without auth, and the opt-out lands where consent lives, which is here
	mux.HandleFunc(unsubscribePath, unsubscribeHandler(infra.Config.EmailLinks.VerifyUnsubscribe, s.Consent, func(r *http.Request) bool {
		return tokenLimiter.Allow(r.Context(), "unsub:"+authpkg.ClientIP(r))
	}))
	mux.HandleFunc(customer.SignupConfirmPath, signupConfirmHandler(s.Form, func(r *http.Request) bool {
		return tokenLimiter.Allow(r.Context(), "subconfirm:"+authpkg.ClientIP(r))
	}))

	// The open pixel and click redirect are the suite's, beside the send log they name - proxied there
	// like /t, and reached via the storefront /e rewrite
	if trackingProxy := pulseProxy(bool(s.SignalsEnabled)); trackingProxy != nil {
		mux.Handle("/e/", trackingProxy)
	}

	mux.HandleFunc("/webhooks/email/events", emailEventsHandler(strings.TrimSpace(os.Getenv("EMAIL_WEBHOOK_TOKEN")), bool(s.SignalsEnabled), s.Config, s.SignalsClient))

	// Bulk transfer files move over HTTP, not GraphQL - same auth chain as admin GraphQL, same permission rules as the resolvers
	// The same credentials the admin GraphQL endpoint takes, so a token that can queue an export can also fetch the file
	adminAuth := func(h http.Handler) http.Handler {
		return authpkg.AdminMiddleware(infra.Config.AdminKeys)(integration.Middleware(s.Integration)(authpkg.AdminSessionGuard(s.AdminRefresh.RefreshStore)(h)))
	}
	mux.Handle(dataImportPath, otelx.WrapHTTP(adminAuth(dataImportHandler(s.DataJob, s.Admin)), "data.import"))
	mux.Handle("/api/admin/data/template", otelx.WrapHTTP(adminAuth(dataTemplateHandler(s.DataJob, s.Admin)), "data.template"))
	mux.Handle("/api/admin/data/jobs/", otelx.WrapHTTP(adminAuth(dataDownloadHandler(s.DataJob, s.Admin)), "data.download"))
	// SVG can't use the GraphQL presign path - see media_svg_http.go for why the bytes come here first
	mux.Handle(mediaSVGPath, otelx.WrapHTTP(adminAuth(mediaSVGHandler(s.MediaLibrary, s.Admin)), "media.svg"))

	// One limiter for the whole /t surface, so a bucket means the same thing whichever tier answers
	ingestLimiter := ratelimit.NewRedis(infra.Limits.Client, RateLimitPerMin("INGEST_RATE_LIMIT_PER_MIN", 300), time.Minute)
	// Ahead of the /t/ catch-all below
	registerCommerceBeacons(mux, commerceBeaconDeps{
		Channels: s.Channel, BackInStock: s.BackInStock, Forms: s.Form,
		Config: s.Config, Limiter: ingestLimiter, Customers: infra.Config.CustomerKeys.Verifier(),
	})
	// The beacon surface is the suite's, so this is a proxy - one public entry point for a simple
	// install, and a storefront that would rather skip the hop can point at the tier directly
	if beacons := beaconProxy(bool(s.SignalsEnabled)); beacons != nil {
		mux.Handle("/t/", beacons)
	}

	if infra.Config.IsDev() { // playground never ships to production - it exposes schema + an unauthenticated query runner against /api/graphql
		mux.Handle("/playground/admin", playground.Handler("Admin GraphQL", adminPath))
		mux.Handle("/playground/front", playground.Handler("Storefront GraphQL", frontPath))
		log.Printf("Admin GraphQL playground: http://localhost:%s/playground/admin", port)
		log.Printf("Storefront GraphQL playground: http://localhost:%s/playground/front", port)
	}

	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   AllowedOrigins(infra.Config),
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Apollo-Require-Preflight", "Locale", channel.HeaderChannelID},
		AllowCredentials: false, // JWT travels in Authorization header, not cookies
		MaxAge:           300,
	}).Handler(mux)

	// Outer-first: Recover → RequestID → Sentry → cors → mux - Recover outermost so it catches panics from anywhere downstream, including sentryhttp itself (Repanic=true) - RequestID stamps before Sentry so the per-request Hub scope can be tagged with it; Sentry is the identity wrapper when Reporter is nil, so wiring stays unconditional
	sentryMw := observability.SentryMiddleware(tel.Reporter != nil)
	maxBodyBytes := int64(8 << 20) // DoS guard - API payloads are small; media uploads bypass it via presigned URLs, and the filesystem driver caps its own - override with REQUEST_MAX_BODY_BYTES
	if v := os.Getenv("REQUEST_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			maxBodyBytes = n
		}
	}
	rootHandler := httpx.Recover(httpx.SecurityHeaders(httpx.LimitBody(maxBodyBytes, httpx.RequestID(sentryMw(corsHandler)), dataImportPath, objstore.FilesPath)))
	return RootHandler{rootHandler}
}

// The consent kind a marketing mail's unsubscribe link opts out of, as consent names it

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

// serviceKeys are the ones the Signals suite signs with - its builder's catalog reads from one address are not a flood
func frontRateLimit(limiter ratelimit.Limiter, serviceKeys authpkg.ServiceKeys) func(http.Handler) http.Handler {
	bypass := os.Getenv("SEO_CRAWL_BYPASS_TOKEN")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if bypass != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get(crawlBypassHeader)), []byte(bypass)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
			if tok := r.Header.Get(signalsapi.ServiceTokenHeader); tok != "" && serviceKeys.Configured() {
				if c, err := authpkg.ValidateServiceToken(tok, signalsapi.CoreAudience, serviceKeys); err == nil && c.System {
					next.ServeHTTP(w, r)
					return
				}
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

// Feeds are pulled by Merchant Center on a schedule - an hour of staleness is fine and keeps the flat query off the hot path
const merchantFeedCacheControl = "public, max-age=3600"

func merchantFeedHandler(feed *catalogio.MerchantFeed) http.HandlerFunc { // prices and currency are channel-bound, so a request that doesn't resolve to a channel gets 404 rather than a mispriced global feed
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
		locale := channel.ClampLocale(*sv, r.URL.Query().Get("locale")) // an unknown code renders the default - the render cache is keyed on it, so arbitrary values must not each mint an entry
		body, contentType, err := feed.Render(ctx, catalogio.MerchantFeedOptions{
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
		w.Header().Set("Cache-Control", merchantFeedCacheControl)
		_, _ = w.Write(body)
	}
}

// SIGNALS_URL is where cmd/signals serves /e - the pixel and the click redirect live beside the send
// log. Unset, the pixel 404s and links in already-sent mail break, which is why compose sets it
func pulseProxy(enabled bool) http.Handler {
	if !enabled {
		return nil // no suite to reach, so /e answers as any unmounted route does
	}
	raw := os.Getenv("SIGNALS_URL")
	if raw == "" {
		slog.Warn("SIGNALS_URL unset - /e is served by the Signals process, so email opens and clicks 404 here until it is set")
		return nil
	}
	target, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("parse SIGNALS_URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// A pixel is best-effort and a click must land somewhere - the process being down answers the
	// pixel with nothing and the click with a plain error rather than a broken page
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Warn("tracking proxy", "path", r.URL.Path, "err", err)
		http.Error(w, "temporarily unavailable", http.StatusBadGateway)
	}
	return proxy
}

// SIGNALS_INGEST_URL is where the suite serves /t - unset, beacons 404 here and the tracker fails soft
func beaconProxy(enabled bool) http.Handler {
	if !enabled {
		return nil
	}
	raw := os.Getenv("SIGNALS_INGEST_URL")
	if raw == "" {
		slog.Warn("SIGNALS_INGEST_URL unset - /t is served by the Signals tier, so beacons 404 here until it is set")
		return nil
	}
	target, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("parse SIGNALS_INGEST_URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// A beacon is best-effort: the tier being down answers exactly as the suite switched off does, so a
	// shopper's page never sees a tracking hiccup and the SDKs fall back on the GETs
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Warn("beacon proxy", "path", r.URL.Path, "err", err)
		if r.Method == http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	return proxy
}

// Plugin routes are a storefront surface - the customer session rides along, and CustomerMiddleware never reads an admin token, so none reaches a plugin's admin gates
func pluginRouteAuth(customers authpkg.CustomerVerifier, active authpkg.CustomerActiveFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return authpkg.CustomerMiddleware(customers)(authpkg.CustomerSessionGuard(active)(next))
	}
}

type suppressionWriter interface {
	Suppress(ctx context.Context, email, reason, detail string) error
	RecordSoftBounce(ctx context.Context, email, detail string, threshold int) (bool, int, error)
}

type deliverabilityConfig interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
	GetInt(ctx context.Context, path string, scope config.Scope) (int64, error)
}

// The seam an ESP's bounce/complaint webhook posts to (SES/SNS, Postmark, SendGrid - transform to this shape) - Bearer-authed via EMAIL_WEBHOOK_TOKEN so only the configured provider can suppress addresses, 404 when unset - payload: {"type":"bounce"|"complaint","subtype":"hard"|"soft","email":"...","detail":"..."}
func emailEventsHandler(token string, signalsOn bool, cfg deliverabilityConfig, suppress suppressionWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" || !signalsOn {
			http.NotFound(w, r) // feature off until a token is configured, and with no suite there is no suppression list to write - a 503 would have the provider redeliver forever
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if on, _ := cfg.GetBool(r.Context(), "signals/deliverability/auto_suppress", config.DefaultScope); !on {
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
			suppressErr = suppress.Suppress(r.Context(), ev.Email, signalsclient.SuppressionComplaint, ev.Detail)
		case ev.Type == "bounce" && ev.Subtype == "soft":
			threshold := 3
			if n, e := cfg.GetInt(r.Context(), "signals/deliverability/soft_bounce_threshold", config.DefaultScope); e == nil && n > 0 {
				threshold = int(n)
			}
			_, _, suppressErr = suppress.RecordSoftBounce(r.Context(), ev.Email, ev.Detail, threshold)
		case ev.Type == "bounce":
			suppressErr = suppress.Suppress(r.Context(), ev.Email, signalsclient.SuppressionHardBounce, ev.Detail)
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
	}
}

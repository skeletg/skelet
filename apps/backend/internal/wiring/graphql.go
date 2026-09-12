package wiring

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	coderws "github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/fx"

	graphadmin "github.com/skeletg/skelet/apps/backend/graph/admin"
	graphfront "github.com/skeletg/skelet/apps/backend/graph/front"
	"github.com/skeletg/skelet/apps/backend/internal/audit"
	authpkg "github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/cache"
	"github.com/skeletg/skelet/apps/backend/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/internal/health"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/otelx"
	"github.com/skeletg/skelet/apps/backend/internal/ratelimit"
)

// RateLimiters are the Valkey-backed request limiters - Nil client (no VALKEY_URL) degrades each to per-process counting, which is correct for one node
type RateLimiters struct {
	General ratelimit.Limiter
	Auth    ratelimit.Limiter
	Front   ratelimit.Limiter
	Login   *authpkg.LoginThrottle
}

type ( // two *handler.Server values, so the graph needs them told apart
	AdminGQL struct{ *handler.Server }
	FrontGQL struct{ *handler.Server }
)

type GQL struct { // bundles both surfaces so the entrypoint pulls one value out of the graph
	Admin AdminGQL
	Front FrontGQL
}

var GraphQL = fx.Module("graphql", // builds both executable schemas with their resolvers, transports, extensions and error presenters
	fx.Provide(
		newRateLimiters,
		newGQL,
	),
)

// login lockout rides the cache client too - its per-(scope,email) keys could otherwise fill the noeviction primary - allkeys-lru keeps the hot keys an active attack touches while reclaiming only cold ones
func newRateLimiters(cache CacheRedis) RateLimiters { // every budget goes through RateLimitPerMin, so all four are off until RATE_LIMIT_ENABLED
	return RateLimiters{
		General: ratelimit.NewRedis(cache.Client, RateLimitPerMin("GENERAL_RATE_LIMIT_PER_MIN", 30), time.Minute),
		Auth:    ratelimit.NewRedis(cache.Client, RateLimitPerMin("AUTH_RATE_LIMIT_PER_MIN", 10), time.Minute),
		Front:   ratelimit.NewRedis(cache.Client, RateLimitPerMin("FRONT_RATE_LIMIT_PER_MIN", 600), time.Minute),
		Login:   newLoginThrottle(cache.Client),
	}
}

// Split from RATE_LIMIT_ENABLED (which it defaults to) because brute-force lockout is worth keeping where per-IP budgets are not
func newLoginThrottle(client *redis.Client) *authpkg.LoginThrottle { // nil disables the lockout end to end - every LoginThrottle method is nil-receiver safe, so no call site branches
	if !EnvBool("LOGIN_LOCKOUT_ENABLED", RateLimitEnabled()) {
		return nil
	}
	return authpkg.NewLoginThrottle(client, EnvInt("LOGIN_LOCKOUT_THRESHOLD", 10), 15*time.Minute, 15*time.Minute)
}

func newGQL(infra Infra, s Svcs, tel observability.Telemetry, rl RateLimiters, healthReg *health.Registry) GQL {
	adminResolver := &graphadmin.Resolver{
		DevMode:           infra.Config.IsDev(),
		CacheClient:       s.RevalidateCli,
		CacheQueue:        s.RevalidateQueue,
		AdminSvc:          s.Admin,
		IntegrationSvc:    s.Integration,
		CustomerSvc:       s.Customer,
		StoreCreditSvc:    s.StoreCredit,
		LoyaltySvc:        s.Loyalty,
		QnaSvc:            s.QnA,
		SearchMerchSvc:    s.SearchMerch,
		SignalsEnabled:    bool(s.SignalsEnabled),
		CartSvc:           s.Cart,
		OrderSvc:          s.Order,
		OrderMailSvc:      s.OrderMail,
		ConfigSvc:         s.Config,
		ConsentSvc:        s.Consent,
		ActionSvc:         s.Action,
		DataJobSvc:        s.DataJob,
		AuditSvc:          s.Audit,
		PrefsSvc:          s.AdminPrefs,
		Queries:           infra.Queries,
		WebhookSvc:        s.Webhook,
		RemoteProviderSvc: s.RemoteProvider,
		CatalogSvc:        s.Catalog,
		BlogSvc:           s.Blog,
		CMSSvc:            s.CMS,
		AISvc:             s.AI,
		UrlRewriteSvc:     s.URLRewrite,
		LinkGraphSvc:      s.LinkGraph,
		SiteCrawlSvc:      s.SiteCrawl,
		ChannelSvc:        s.Channel,
		InventorySvc:      s.Inventory,
		LocaleSvc:         s.Locale,
		CurrencySvc:       s.Currency,
		CountrySvc:        s.Country,
		MediaLibrarySvc:   s.MediaLibrary,
		MenuSvc:           s.Menu,
		ReviewSvc:         s.Review,
		TaxSvc:            s.Tax,
		ShippingSvc:       s.Shipping,
		PaymentSvc:        s.Payment,
		PromotionSvc:      s.Promotion,
		GiftcardSvc:       s.Giftcard,
		SubscriptionSvc:   s.Subscription,
		SalesSvc:          s.Sales,
		EmailTmplSvc:      s.EmailTemplate,
		CampaignSvc:       s.Campaign,
		NotificationSvc:   s.Notification,
		NotificationHub:   s.NotifHub,
		SuppressionSvc:    s.Suppression,
		FormSvc:           s.Form,
		WebFormSvc:        s.WebForm,
		ChatSvc:           s.Chat,
		SendLog:           s.SendLog,
		Health:            healthReg,
		SignalsAPI:        s.SignalsClient,
		FlowSvc:           s.Flow,
		RefreshStore:      s.AdminRefresh.RefreshStore,
		BlogPreviewStore:  s.BlogPreview.Store,
		CMSPreviewStore:   s.CMSPreview.Store,
		JWTSecret:         infra.Config.JWTSecret,
		JWTTTL:            AccessTokenTTL,
		RateLimiter:       rl.General,
		AuthLimiter:       rl.Auth,
		LoginThrottle:     rl.Login,
		Scheduler:         s.Scheduler,
	}

	adminSrv := handler.New(graphadmin.NewExecutableSchema(graphadmin.Config{Resolvers: adminResolver}))
	// Audit interceptor: one row per admin MUTATION (queries are reads and would swamp the table), plus the handful of QUERIES that are really bulk PII egress - a whole-table customer CSV or a DSAR bundle is the most sensitive thing an admin can pull, and auditing only mutations made the trail look complete while missing exactly that - records operation name, redacted variables, admin identity, client IP, success + duration - best-effort insert after the response completes
	auditedExports := map[string]bool{
		"exportCustomerData": true, // DSAR bundle - the bulk CSV egress is audited on the job row instead
	}
	// UI-pref writes are chatty and carry layout blobs - auditing them buries the real trail; keyed on the FIELD so a renamed operation can't dodge auditing of anything else
	unauditedMutations := map[string]bool{
		"setMyPreference": true,
	}
	adminSrv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc == nil || oc.Operation == nil {
			return next(ctx)
		}
		if oc.Operation.Operation != ast.Mutation && !operationSelectsAny(oc.Operation, auditedExports) {
			return next(ctx)
		}
		if oc.Operation.Operation == ast.Mutation && operationSelectsOnly(oc.Operation, unauditedMutations) {
			return next(ctx)
		}
		opName := oc.OperationName
		if opName == "" {
			opName = "(anonymous " + string(oc.Operation.Operation) + ")"
		}
		// The login mutation itself is audited too - identity resolves to nil there (no claims yet), variables redact the password
		start := time.Now()
		handler := next(ctx)
		return func(ctx context.Context) *graphql.Response {
			resp := handler(ctx)
			entry := audit.Entry{
				Action:     opName,
				Detail:     oc.Variables,
				Success:    resp == nil || len(resp.Errors) == 0,
				DurationMS: time.Since(start).Milliseconds(),
			}
			if claims, ok := authpkg.AdminFromContext(ctx); ok && claims != nil {
				entry.AdminID = claims.AdminID
				entry.AdminEmail = claims.Email
			} else if tok, ok := authpkg.ApiTokenFromContext(ctx); ok && tok != nil { // machine callers have no admin row - the email column carries the integration's name so the trail still says who
				entry.AdminEmail = "integration:" + tok.Name
			}
			if r, ok := authpkg.RequestFromContext(ctx); ok && r != nil {
				entry.IP = authpkg.ClientIP(r)
			}
			s.Audit.Record(ctx, entry)
			return resp
		}
	})
	// WebSocket transport for subscriptions (live admin notifications) - the HTTP auth middleware never sees per-message traffic, so the JWT rides the graphql-ws connection_init payload instead: InitFunc validates it and stamps the admin claims into the connection's root context - every subscription resolver then authorizes exactly like a query resolver, and unauthenticated sockets are refused at init (no anonymous parking)
	adminSrv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Implementation: originCheckedWebsocket{
			allow: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // non-browser client; auth still gates in InitFunc
				}
				if u, err := url.Parse(origin); err == nil && u.Host == r.Host {
					return true // same-origin (vite dev proxy / same-host deploys)
				}
				for _, allowed := range AllowedOrigins(infra.Config) {
					if strings.EqualFold(strings.TrimSuffix(allowed, "/"), strings.TrimSuffix(origin, "/")) {
						return true
					}
				}
				return false
			},
		},
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			tokenStr := strings.TrimPrefix(initPayload.Authorization(), "Bearer ")
			if tokenStr == "" {
				return ctx, nil, errors.New("missing authorization in connection_init")
			}
			claims, err := authpkg.ValidateToken(tokenStr, infra.Config.JWTSecret)
			if err != nil {
				return ctx, nil, errors.New("invalid token")
			}
			ctx = authpkg.WithAdmin(ctx, claims)
			ctx = authpkg.WithRequestCache(ctx)
			return ctx, nil, nil
		},
	})
	adminSrv.AddTransport(transport.Options{})
	adminSrv.AddTransport(transport.GET{})
	adminSrv.AddTransport(transport.POST{})
	adminSrv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	// Introspection follows the CALLER, not the environment - gating on APP_ENV left the dashboard playground, which ships in prod behind developer.playground, with no docs or autocomplete anywhere it runs - dev keeps the unauthenticated path so /playground/admin still works
	if infra.Config.IsDev() {
		adminSrv.Use(extension.Introspection{})
	} else {
		adminSrv.Use(graphqlx.PermittedIntrospection{Checker: s.Admin, Slug: "developer.playground"})
	}
	adminSrv.Use(extension.AutomaticPersistedQuery{
		Cache: cache.NewAPQ(infra.Cache.Client, "apq:admin", 24*time.Hour),
	})
	// DoS guard: 500 clears the dashboard's heaviest joined fetch (product detail with content + listings + categories + reviews + inventory) and rejects the pathological
	adminSrv.Use(extension.FixedComplexityLimit(500))
	// Map domain errors → GraphQL extensions {code, class, op, origin, request_id}; recover panics into ClassInternal apperr errors that flow through the same presenter - admin surface, so trusted callers see op/origin for triage - Sentry reporter is nil when the SDK isn't initialised, in which case the presenter behaves as before
	adminSrv.SetErrorPresenter(graphqlx.Presenter(graphqlx.SurfaceAdmin, graphqlx.WithReporter(tel.Reporter), graphqlx.WithDevDetails(infra.Config.IsDev())))
	adminSrv.SetRecoverFunc(graphqlx.RecoverFunc(graphqlx.SurfaceAdmin, graphqlx.WithReporter(tel.Reporter)))
	// OTel: one span per GraphQL operation + top-level field spans - no-op when OTel is not initialised
	adminSrv.Use(otelx.GraphQLMiddleware("admin"))

	frontResolver := &graphfront.Resolver{
		CustomerSvc:          s.Customer,
		StoreCreditSvc:       s.StoreCredit,
		GiftcardSvc:          s.Giftcard,
		MediaPublicBase:      s.MediaStorage.PublicURL(""),
		LoyaltySvc:           s.Loyalty,
		QnaSvc:               s.QnA,
		SearchMerchSvc:       s.SearchMerch,
		SignalsEnabled:       bool(s.SignalsEnabled),
		ReferralSvc:          s.Referral,
		PaymentVaultSvc:      s.PaymentVault,
		AddressSvc:           s.Address,
		CartSvc:              s.Cart,
		OrderSvc:             s.Order,
		ConfigSvc:            s.Config,
		ConsentSvc:           s.Consent,
		CatalogSvc:           s.Catalog,
		BlogSvc:              s.Blog,
		CMSSvc:               s.CMS,
		ChannelSvc:           s.Channel,
		InventorySvc:         s.Inventory,
		LocaleSvc:            s.Locale,
		CurrencySvc:          s.Currency,
		CountrySvc:           s.Country,
		CustomerRefreshStore: s.CustomerRefresh.RefreshStore,
		BlogPreviewStore:     s.BlogPreview.Store,
		CMSPreviewStore:      s.CMSPreview.Store,
		WishlistSvc:          s.Wishlist,
		URLRewriteSvc:        s.URLRewrite,
		MenuSvc:              s.Menu,
		ProductSearch:        s.SearchTier.Engine,
		BlogPostSearch:       s.SearchTier.Posts,
		ReviewSvc:            s.Review,
		WebFormSvc:           s.WebForm,
		PromotionSvc:         s.Promotion,
		SegmentSvc:           s.Segment,
		ShippingSvc:          s.Shipping,
		TaxSvc:               s.Tax,
		PaymentSvc:           s.Payment,
		MediaLibrarySvc:      s.MediaLibrary,
		DownloadSvc:          s.Download,
		SubscriptionSvc:      s.Subscription,
		JWTSecret:            infra.Config.JWTSecret,
		JWTTTL:               AccessTokenTTL,
		VoterTokenSecret:     infra.Config.VoterTokenSecret,
		RateLimiter:          rl.General,
		AuthLimiter:          rl.Auth,
		LoginThrottle:        rl.Login,
		Bus:                  s.Bus,
	}
	frontSrv := handler.New(graphfront.NewExecutableSchema(graphfront.Config{Resolvers: frontResolver}))
	frontSrv.AddTransport(transport.Options{})
	frontSrv.AddTransport(transport.GET{})
	frontSrv.AddTransport(transport.POST{})
	frontSrv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	// Same caller gate as admin - auth.Middleware runs on both endpoints and injects ADMIN claims for an admin token, so the dashboard reaches this schema while anonymous storefront traffic still gets nothing
	if infra.Config.IsDev() {
		frontSrv.Use(extension.Introspection{})
	} else {
		frontSrv.Use(graphqlx.PermittedIntrospection{Checker: s.Admin, Slug: "developer.playground"})
	}
	frontSrv.Use(extension.AutomaticPersistedQuery{
		Cache: cache.NewAPQ(infra.Cache.Client, "apq:front", 24*time.Hour),
	})
	// DoS guard: same complexity cap as the admin surface, but the storefront is unauthenticated so this is the *primary* defense against query-bomb DoS - every public POST hits this gate before any resolver runs - 300 is comfortably above the heaviest storefront fetch (PDP with reviews + answers + media + per-question aggregates) and well below an exponential blow-up
	frontSrv.Use(extension.FixedComplexityLimit(300))
	// Query-DEPTH guard - the complexity limit counts 1 per field with no nesting multiplier, so a query that nests the recursive catalog edges (linkedProducts / alsoBought / partOf, which can cycle between products) ~300 levels deep passes the complexity gate yet fans out into thousands of un-batched DB reads at runtime - reject anything deeper than the deepest legitimate storefront fetch (well under 15)
	frontSrv.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)
		if oc != nil && oc.Operation != nil && !isIntrospectionOnly(oc.Operation.SelectionSet) {
			if selectionDepth(oc.Operation.SelectionSet, oc.Doc.Fragments, 1) > maxQueryDepth {
				return func(ctx context.Context) *graphql.Response {
					return graphql.ErrorResponse(ctx, "query exceeds maximum nesting depth of %d", maxQueryDepth)
				}
			}
		}
		return next(ctx)
	})
	// Front surface - public callers - presenter scrubs Message for ClassInternal and never includes op/origin in extensions
	frontSrv.SetErrorPresenter(graphqlx.Presenter(graphqlx.SurfaceFront, graphqlx.WithReporter(tel.Reporter), graphqlx.WithDevDetails(infra.Config.IsDev())))
	frontSrv.SetRecoverFunc(graphqlx.RecoverFunc(graphqlx.SurfaceFront, graphqlx.WithReporter(tel.Reporter)))
	frontSrv.Use(otelx.GraphQLMiddleware("front"))
	return GQL{Admin: AdminGQL{adminSrv}, Front: FrontGQL{frontSrv}}
}

// operationSelectsAny reports whether a GraphQL operation selects any of the named top-level fields - used to audit the read operations that are really bulk PII egress, without auditing every query on the admin surface
func operationSelectsAny(op *ast.OperationDefinition, names map[string]bool) bool {
	if op == nil {
		return false
	}
	for _, sel := range op.SelectionSet {
		field, ok := sel.(*ast.Field)
		if !ok {
			continue
		}
		if names[field.Name] {
			return true
		}
	}
	return false
}

// operationSelectsOnly reports whether EVERY top-level field of the operation is in the named set - a batched mutation mixing an unaudited field with anything else still gets audited
func operationSelectsOnly(op *ast.OperationDefinition, names map[string]bool) bool {
	if op == nil || len(op.SelectionSet) == 0 {
		return false
	}
	for _, sel := range op.SelectionSet {
		field, ok := sel.(*ast.Field)
		if !ok || !names[field.Name] {
			return false
		}
	}
	return true
}

type originCheckedWebsocket struct { // coder/websocket checks origins off a static host-pattern list - ours stays per-request so same-host and CORS_ALLOWED_ORIGINS still apply
	allow func(*http.Request) bool
}

func (u originCheckedWebsocket) Accept(w http.ResponseWriter, r *http.Request, options transport.WebsocketAcceptOptions) (transport.WebsocketConn, error) {
	if !u.allow(r) {
		http.Error(w, "websocket origin not allowed", http.StatusForbidden)
		return nil, errors.New("websocket origin not allowed")
	}
	inner := transport.CoderWebsocketImplementation{AcceptOptions: coderws.AcceptOptions{InsecureSkipVerify: true}} // allow() already ran - coder's own check would re-reject what we let through
	return inner.Accept(w, r, options)
}

// Unset CORS_ALLOWED_ORIGINS means "no origins allowed" in production - dev falls back to the usual local origins
func AllowedOrigins(cfg Config) []string {
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if cfg.IsDev() {
		return []string{
			"http://localhost:5173", // Vite dev server (React admin)
			"http://localhost:3000", // Next.js dev server (storefront)
		}
	}
	return nil
}

// isIntrospectionOnly reports whether every root field is a `__` meta-field - the canonical introspection query walks ofType chains way past maxQueryDepth, so the guard would reject the one query codegen and GraphiQL need - costs nothing to exempt, since introspection is dev-only here and the complexity cap still applies
func isIntrospectionOnly(sels ast.SelectionSet) bool {
	seen := 0
	for _, sel := range sels {
		f, ok := sel.(*ast.Field)
		if !ok || !strings.HasPrefix(f.Name, "__") {
			return false
		}
		seen++
	}
	return seen > 0
}

// selectionDepth returns the maximum nesting depth of a selection set, resolving fragment spreads against the document's fragment definitions - `depth` is the current level (root fields are depth 1) - short-circuits once over budget so a pathological query doesn't cost more to measure than to reject
func selectionDepth(sels ast.SelectionSet, frags ast.FragmentDefinitionList, depth int) int {
	if depth > maxQueryDepth {
		return depth
	}
	deepest := depth
	for _, sel := range sels {
		var child int
		switch s := sel.(type) {
		case *ast.Field:
			child = selectionDepth(s.SelectionSet, frags, depth+1)
		case *ast.InlineFragment:
			child = selectionDepth(s.SelectionSet, frags, depth) // A type condition, not a new level
		case *ast.FragmentSpread:
			if def := frags.ForName(s.Name); def != nil {
				child = selectionDepth(def.SelectionSet, frags, depth)
			}
		}
		if child > deepest {
			deepest = child
		}
	}
	return deepest
}

// maxQueryDepth caps how deeply a storefront GraphQL query may nest - guards the recursive catalog edges against depth-amplification DoS (see the AroundOperations hook on frontSrv) - 15 clears the deepest legitimate fetch with headroom
const maxQueryDepth = 15

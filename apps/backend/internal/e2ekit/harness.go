// Package e2ekit is the end-to-end harness: one real fx graph, the fixtures to build a store on it,
// and the outbox drain that makes a scenario's assertions deterministic
//
// It is a NORMAL package rather than _test.go files so that code outside internal/e2e can use it -
// a plugin owning part of the money path has e2e tests of its own, and _test.go files can't be imported
//
// It takes a TB rather than *testing.T for the same reason: nothing here needs the testing package,
// and a non-test package importing it would register test flags into anything that linked it
package e2ekit

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/internal/admin"
	"github.com/skeletg/skelet/apps/backend/internal/adminprefs"
	authpkg "github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/country"
	"github.com/skeletg/skelet/apps/backend/internal/currency"
	"github.com/skeletg/skelet/apps/backend/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/download"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
	"github.com/skeletg/skelet/apps/backend/internal/form"
	"github.com/skeletg/skelet/apps/backend/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/internal/integration"
	"github.com/skeletg/skelet/apps/backend/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/internal/locale"
	"github.com/skeletg/skelet/apps/backend/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/internal/notification"
	"github.com/skeletg/skelet/apps/backend/internal/observability"
	"github.com/skeletg/skelet/apps/backend/internal/order"
	"github.com/skeletg/skelet/apps/backend/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/internal/payment"
	"github.com/skeletg/skelet/apps/backend/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/internal/referral"
	"github.com/skeletg/skelet/apps/backend/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/internal/tax"
	"github.com/skeletg/skelet/apps/backend/internal/testdb"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
	"github.com/skeletg/skelet/apps/backend/internal/wiring"
)

// The slice of *testing.T the kit needs - a local interface so this package imports no testing
type TB interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
}

// Everything the scenarios drive or assert against, resolved from the real graph
type Deps struct {
	fx.In

	DB      *sql.DB
	Queries *db.Queries
	Tx      *tx.Manager
	Bus     *events.Bus
	Outbox  *outbox.Outbox

	Verify       *authpkg.VerificationStore
	Config       *config.Service
	Channel      *channel.Service
	Locale       *locale.Service
	Currency     *currency.Service
	Country      *country.Service
	Catalog      *catalog.Service
	Inventory    *inventory.Service
	Cart         *cart.Service
	Order        *order.Service
	Payment      *payment.Service
	PaymentVault *paymentvault.Service
	Promotion    *promotion.Service
	Customer     *customer.Service
	Giftcard     *giftcard.Service
	StoreCredit  *storecredit.Service
	Loyalty      *loyalty.Service
	Referral     *referral.Service
	Download     *download.Service
	Subscription *subscription.Service
	Tax          *tax.Service
	Shipping     *shipping.Service
	Experiment   *experiment.Service
	Flow         *flow.Service // scenarios drive ProcessRuns directly - flow runs advance on a scheduled job the harness doesn't run
	OrderMail    *ordermail.Service
	Form         *form.Service
	BackInStock  *backinstock.Service
	Integration  *integration.Service
	Admin        *admin.Service
	AdminPrefs   *adminprefs.Service
	Notification *notification.Service
	Root         wiring.RootHandler
}

// Ctx is the graph's background context; D is every service the scenarios drive or assert against
type Harness struct {
	Ctx context.Context
	D   Deps
}

// The server-shaped composition minus the loop modules: full domain graph, subscriber wiring and the real HTTP/GraphQL surface - the
// scenarios drive the outbox and scheduler paths by hand for determinism, and the HTTP tests serve Deps.Root through httptest
func Options(cfg wiring.Config, ctx context.Context, bg *sync.WaitGroup, populate ...any) []fx.Option {
	return []fx.Option{
		wiring.Pools,
		wiring.Cache,
		wiring.Registries,
		wiring.Platform,
		wiring.Integrations,
		wiring.Services,
		wiring.Domain,
		wiring.DataTransfer,
		wiring.Jobs,
		wiring.Search,
		wiring.Health,
		wiring.Wiring,
		wiring.GraphQL,
		wiring.HTTP,
		fx.Supply(cfg, wiring.RootContext{Context: ctx},
			observability.Telemetry{}, wiring.QueueProducer,
			wiring.BackgroundWaitGroup{WaitGroup: bg}),
		fx.Populate(populate...),
		fx.NopLogger,
	}
}

func Config() (wiring.Config, bool) {
	dbURL, sigURL := os.Getenv("DATABASE_URL"), os.Getenv("SIGNALS_DB_URL")
	if dbURL == "" || sigURL == "" {
		return wiring.Config{}, false
	}
	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		jwt = "e2e-test-secret"
	}
	return wiring.Config{
		Env:               os.Getenv("APP_ENV"),
		Role:              wiring.RoleServer,
		JWTSecret:         jwt,
		VoterTokenSecret:  []byte("vote:" + jwt),
		DatabaseURL:       dbURL,
		ReplicaURL:        dbURL,
		SignalsURL:        sigURL,
		SignalsReadURL:    sigURL,
		MaxConns:          8,
		AnalyticsMaxConns: 2, SignalsMaxConns: 2, SignalsReadMaxConns: 2,
	}, true
}

// One graph per process - a second fx.New would replay the plugin/registry bootstrap, which the
// SDK correctly refuses. Tests share it and isolate through their own fixtures instead
var (
	harnessOnce sync.Once
	sharedH     *Harness
	harnessErr  error
)

func New(t TB) *Harness {
	t.Helper()
	cfg, ok := Config()
	if !ok {
		t.Skip("set DATABASE_URL and SIGNALS_DB_URL to run the e2e suite")
	}
	// Both databases must carry the disposable marker - this suite writes to each. testdb.Open is
	// the variant for callers holding no *testing.T, which is exactly what taking a TB makes us
	for _, envVar := range []string{"DATABASE_URL", "SIGNALS_DB_URL"} {
		conn, reason := testdb.Open(envVar)
		if conn == nil {
			t.Skip(reason)
		}
		_ = conn.Close()
	}
	harnessOnce.Do(func() {
		// The graph constructs media storage unconditionally - dummy credentials keep it inert
		// (nothing here touches S3) and the metadata kill switch keeps the AWS chain off IMDS
		setenvDefault("S3_BUCKET_NAME", "e2e-unused")
		setenvDefault("AWS_ACCESS_KEY_ID", "e2e-unused")
		setenvDefault("AWS_SECRET_ACCESS_KEY", "e2e-unused")
		setenvDefault("AWS_EC2_METADATA_DISABLED", "true")
		// rate limiting ships OFF by default now - the ingest throttling scenarios test the ON behavior,
		// so the suite opts in explicitly (memory limiter - no Valkey in the Harness)
		setenvDefault("RATE_LIMIT_ENABLED", "true")

		ctx := context.Background()
		var bg sync.WaitGroup
		var d Deps
		app := fx.New(Options(cfg, ctx, &bg, &d)...)
		if err := app.Err(); err != nil {
			harnessErr = err
			return
		}
		sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := app.Start(sctx); err != nil {
			harnessErr = err
			return
		}
		sharedH = &Harness{Ctx: ctx, D: d}
	})
	if harnessErr != nil {
		t.Fatalf("boot e2e graph: %v", harnessErr)
	}
	return sharedH
}

func setenvDefault(key, val string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, val)
	}
}

// settle drains outbox rows until quiescent, waiting out detached handlers between rounds - after
// this returns, every reaction to the acted-on events has run or will never run
func (h *Harness) Settle(t TB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(h.Ctx, 30*time.Second)
	defer cancel()
	for range 20 {
		n := h.D.Outbox.DrainPending(ctx)
		if err := h.D.Bus.WaitIdle(ctx); err != nil {
			t.Fatalf("settle: %v", err)
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("settle: outbox never quiesced after 20 rounds - a handler is enqueueing in a loop")
}

// AdvanceFlows runs a channel's own flow runs to completion, backdating them first because the engine claims the oldest 100 per tick and every order the suite places leaves one behind
func (h *Harness) AdvanceFlows(t TB, channelID uuid.UUID) {
	h.Settle(t) // enrollment rides the outbox, so the runs don't exist yet until this drains
	for range 20 {
		res, err := h.D.DB.Exec(
			`UPDATE flow_runs SET created_at = 'epoch' WHERE id IN (
			   SELECT r.id FROM flow_runs r JOIN flows f ON f.id = r.flow_id
			   WHERE f.channel_id = $1 AND (r.status = 'active' OR (r.status = 'waiting' AND r.resume_at <= NOW())))`,
			channelID)
		if err != nil {
			t.Fatalf("backdate flow runs: %v", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			t.Fatalf("backdate flow runs: %v", err)
		}
		if n == 0 {
			return
		}
		if err := h.D.Flow.ProcessRuns(h.Ctx); err != nil {
			t.Fatalf("advance flow runs: %v", err)
		}
		h.Settle(t)
	}
	t.Fatal("this channel's flow runs never drained after 20 rounds - a flow is re-enrolling in a loop")
}

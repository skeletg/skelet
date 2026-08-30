package e2e

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/internal/admin"
	"github.com/skeletg/skelet/internal/adminprefs"
	authpkg "github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/b2b"
	"github.com/skeletg/skelet/internal/backinstock"
	"github.com/skeletg/skelet/internal/cart"
	"github.com/skeletg/skelet/internal/catalog"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/country"
	"github.com/skeletg/skelet/internal/currency"
	"github.com/skeletg/skelet/internal/customer"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/download"
	"github.com/skeletg/skelet/internal/events"
	"github.com/skeletg/skelet/internal/experiment"
	"github.com/skeletg/skelet/internal/form"
	"github.com/skeletg/skelet/internal/giftcard"
	"github.com/skeletg/skelet/internal/integration"
	"github.com/skeletg/skelet/internal/inventory"
	"github.com/skeletg/skelet/internal/locale"
	"github.com/skeletg/skelet/internal/loyalty"
	"github.com/skeletg/skelet/internal/notification"
	"github.com/skeletg/skelet/internal/observability"
	"github.com/skeletg/skelet/internal/order"
	"github.com/skeletg/skelet/internal/outbox"
	"github.com/skeletg/skelet/internal/payment"
	"github.com/skeletg/skelet/internal/paymentvault"
	"github.com/skeletg/skelet/internal/promotion"
	"github.com/skeletg/skelet/internal/referral"
	"github.com/skeletg/skelet/internal/shipping"
	"github.com/skeletg/skelet/internal/storecredit"
	"github.com/skeletg/skelet/internal/subscription"
	"github.com/skeletg/skelet/internal/tax"
	"github.com/skeletg/skelet/internal/testdb"
	"github.com/skeletg/skelet/internal/tx"
	"github.com/skeletg/skelet/internal/wiring"
)

// Everything the scenarios drive or assert against, resolved from the real graph
type deps struct {
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
	B2B          *b2b.Service
	Experiment   *experiment.Service
	Form         *form.Service
	BackInStock  *backinstock.Service
	Integration  *integration.Service
	Admin        *admin.Service
	AdminPrefs   *adminprefs.Service
	Notification *notification.Service
	Root         wiring.RootHandler
}

type harness struct {
	ctx context.Context
	d   deps
}

// The server-shaped composition minus the loop modules: full domain graph, subscriber wiring and the real HTTP/GraphQL surface - the
// scenarios drive the outbox and scheduler paths by hand for determinism, and the HTTP tests serve deps.Root through httptest
func e2eOptions(cfg wiring.Config, ctx context.Context, bg *sync.WaitGroup, populate ...any) []fx.Option {
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

func e2eConfig() (wiring.Config, bool) {
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
		B2BEnabled: true,
	}, true
}

// One graph per process - a second fx.New would replay the plugin/registry bootstrap, which the
// SDK correctly refuses. Tests share it and isolate through their own fixtures instead
var (
	harnessOnce sync.Once
	sharedH     *harness
	harnessErr  error
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	cfg, ok := e2eConfig()
	if !ok {
		t.Skip("set DATABASE_URL and SIGNALS_DB_URL to run the e2e suite")
	}
	// Both databases must carry the disposable marker - this suite writes to each
	testdb.Require(t).Close()
	testdb.RequireSignals(t).Close()
	harnessOnce.Do(func() {
		// The graph constructs media storage unconditionally - dummy credentials keep it inert
		// (nothing here touches S3) and the metadata kill switch keeps the AWS chain off IMDS
		setenvDefault("S3_BUCKET_NAME", "e2e-unused")
		setenvDefault("AWS_ACCESS_KEY_ID", "e2e-unused")
		setenvDefault("AWS_SECRET_ACCESS_KEY", "e2e-unused")
		setenvDefault("AWS_EC2_METADATA_DISABLED", "true")
		// rate limiting ships OFF by default now - the ingest throttling scenarios test the ON behavior,
		// so the suite opts in explicitly (memory limiter - no Valkey in the harness)
		setenvDefault("RATE_LIMIT_ENABLED", "true")

		ctx := context.Background()
		var bg sync.WaitGroup
		var d deps
		app := fx.New(e2eOptions(cfg, ctx, &bg, &d)...)
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
		sharedH = &harness{ctx: ctx, d: d}
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
func (h *harness) settle(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()
	for range 20 {
		n := h.d.Outbox.DrainPending(ctx)
		if err := h.d.Bus.WaitIdle(ctx); err != nil {
			t.Fatalf("settle: %v", err)
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("settle: outbox never quiesced after 20 rounds - a handler is enqueueing in a loop")
}

// Runs everywhere, no database - proves this package's composition and populate struct resolve,
// the same guarantee wiring_test gives the real entrypoints
func TestE2EGraphResolves(t *testing.T) {
	var bg sync.WaitGroup
	var d deps
	if err := fx.ValidateApp(e2eOptions(wiring.Config{}, context.Background(), &bg, &d)...); err != nil {
		t.Fatalf("e2e graph does not resolve: %v", err)
	}
}

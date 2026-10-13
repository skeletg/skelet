// Package e2ekit is the end-to-end harness: one real fx graph, the fixtures to build a store on it, and the outbox drain that makes assertions deterministic
// A normal package rather than _test.go files, so a plugin owning part of the money path can import it - and it takes a TB for the same reason
package e2ekit

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/admin"
	"github.com/skeletg/skelet/apps/backend/core/internal/adminprefs"
	"github.com/skeletg/skelet/apps/backend/core/internal/backinstock"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/catalog"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/country"
	"github.com/skeletg/skelet/apps/backend/core/internal/currency"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/download"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/giftcard"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/internal/inventory"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	"github.com/skeletg/skelet/apps/backend/core/internal/loyalty"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/internal/ordermail"
	"github.com/skeletg/skelet/apps/backend/core/internal/outbox"
	"github.com/skeletg/skelet/apps/backend/core/internal/payment"
	"github.com/skeletg/skelet/apps/backend/core/internal/paymentvault"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	"github.com/skeletg/skelet/apps/backend/core/internal/referral"
	"github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient"
	"github.com/skeletg/skelet/apps/backend/core/internal/signalsclient/enroll"
	"github.com/skeletg/skelet/apps/backend/core/internal/signupform"
	"github.com/skeletg/skelet/apps/backend/core/internal/storecredit"
	"github.com/skeletg/skelet/apps/backend/core/internal/subscription"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/internal/wiring"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/observability"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// The slice of *testing.T the kit needs - a local interface so this package imports no testing
type TB interface {
	Helper()
	Cleanup(func())
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Skip(args ...any)
}

// Everything the scenarios drive or assert against, resolved from the real graph
type Deps struct {
	fx.In

	DB      *sql.DB
	Queries *db.Queries
	Tx      *txrunner.Manager
	Bus     *events.Bus
	Outbox  *outbox.Outbox

	Verify        *authpkg.VerificationStore
	Config        *config.Service
	Channel       *channel.Service
	Locale        *locale.Service
	Currency      *currency.Service
	Country       *country.Service
	Catalog       *catalog.Service
	Inventory     *inventory.Service
	Cart          *cart.Service
	Order         *order.Service
	Payment       *payment.Service
	PaymentVault  *paymentvault.Service
	Promotion     *promotion.Service
	Customer      *customer.Service
	Giftcard      *giftcard.Service
	StoreCredit   *storecredit.Service
	Loyalty       *loyalty.Service
	Referral      *referral.Service
	Download      *download.Service
	Subscription  *subscription.Service
	Tax           *tax.Service
	Shipping      *shipping.Service
	OrderMail     *ordermail.Service
	Form          *signupform.Service
	BackInStock   *backinstock.Service
	Integration   *integration.Service
	Admin         *admin.Service
	AdminPrefs    *adminprefs.Service
	Notification  *notification.Service
	SignalsClient *signalsclient.Client // core's plug into the suite, which bin/test-e2e runs beside this harness - flows and saved emails are driven through it
	Flows         *enroll.Enroller
	Root          wiring.RootHandler
}

// Ctx is the graph's background context; D is every service the scenarios drive or assert against
type Harness struct {
	Ctx  context.Context
	D    Deps
	Mail *MailSink
}

// Stands in for the configured sender so a scenario can see what core actually delivered - core keeps
// no send log of its own, so the message is the only evidence a transactional mail went out
type MailSink struct {
	mu   sync.Mutex
	sent []email.Message
}

func (m *MailSink) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// To returns every message delivered to one address since the last Reset
func (m *MailSink) To(addr string) []email.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []email.Message
	for _, msg := range m.sent {
		if msg.To == addr {
			out = append(out, msg)
		}
	}
	return out
}

// SubjectsTo returns what one address received, which is what a failing count wants to show
func (m *MailSink) SubjectsTo(addr string) []string {
	out := []string{}
	for _, msg := range m.To(addr) {
		out = append(out, msg.Subject)
	}
	return out
}

// Scenarios share one graph, so a scenario that counts sends starts by clearing what came before
func (m *MailSink) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = nil
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
			observability.Telemetry{},
			wiring.BackgroundWaitGroup{WaitGroup: bg}),
		// Nothing in the suite should reach a real SMTP server, and the sink is how a scenario proves a mail was sent
		fx.Decorate(func(email.Sender) email.Sender { return sink }),
		fx.Populate(populate...),
		fx.NopLogger,
	}
}

func Config() (wiring.Config, bool) {
	dbURL, _ := testdb.URL("DATABASE_URL")
	if dbURL == "" {
		return wiring.Config{}, false
	}
	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		jwt = "e2e-test-secret"
	}
	// The suite the runner starts beside these tests reads the same variable from the same
	// container, so the two ends agree - the fallback is for a suite-less run
	service := os.Getenv("SIGNALS_SERVICE_SECRET")
	if service == "" {
		service = "e2e-service-secret"
	}
	return wiring.Config{
		Env:                  os.Getenv("APP_ENV"),
		Role:                 wiring.RoleServer,
		JWTSecret:            jwt,
		SignalsServiceSecret: service,
		VoterTokenSecret:     []byte("vote:" + jwt),
		DatabaseURL:          dbURL,
		ReplicaURL:           dbURL,
		MaxConns:             8,
		AnalyticsMaxConns:    2,
	}, true
}

// One graph per process - a second fx.New would replay the plugin/registry bootstrap, which the
// SDK correctly refuses. Tests share it and isolate through their own fixtures instead
var (
	harnessOnce sync.Once
	sharedH     *Harness
	harnessErr  error
	sink        *MailSink
)

func New(t TB) *Harness {
	t.Helper()
	cfg, ok := Config()
	if !ok {
		t.Skip("set DATABASE_URL to run the e2e suite")
	}
	// The commerce database must carry the disposable marker, since this suite writes to it
	// testdb.Open is the variant for callers holding no *testing.T
	if conn, reason := testdb.Open("DATABASE_URL"); conn == nil {
		t.Skip(reason)
	} else {
		_ = conn.Close()
	}
	// The app container's own SIGNALS_GRPC_ADDR is the dev stack's Signals process - reading it would push this run's channels,
	// flows and enrollments into the dev suite, so the address arrives under a name only bin/test-e2e sets
	signalsAddr := os.Getenv("E2E_SIGNALS_GRPC_ADDR")
	if signalsAddr == "" {
		t.Skip("E2E_SIGNALS_GRPC_ADDR is unset - run the e2e suite with ./skelet test e2e, which starts a Signals process of its own")
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

		// Core reaches the suite over the contract even here, so it talks to the Signals process bin/test-e2e started for this run's database
		os.Setenv("SIGNALS_GRPC_ADDR", signalsAddr)

		ctx := context.Background()
		var bg sync.WaitGroup
		var d Deps
		sink = &MailSink{}
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
		sharedH = &Harness{Ctx: ctx, D: d, Mail: sink}
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
		h.D.Customer.FlushMail()
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

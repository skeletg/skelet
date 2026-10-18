package e2e

import (
	"context"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"google.golang.org/grpc"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/boot"
	"github.com/skeletg/skelet/apps/backend/signals/internal/emailtemplate"
	"github.com/skeletg/skelet/apps/backend/signals/internal/experiment"
	"github.com/skeletg/skelet/apps/backend/signals/internal/flow"
)

// The suite's own scenarios: the beacon surface against a real signals database, composed the way cmd/signals composes it
// Core is not here - a channel is pushed the way core's job pushes it, and a customer is a token, which is all /t sees of one
type harness struct {
	ctx     context.Context
	d       harnessDeps
	signer  auth.CustomerSigner
	service string
	mail    *mailSink
}

// What the scenarios reach past the surface for
type harnessDeps struct {
	fx.In

	Mux        *http.ServeMux
	Settings   *config.Service
	Queries    boot.Queries
	Channels   *analytics.ChannelRoster
	Experiment *experiment.Service
	Flow       *flow.Service
	Builder    *emailtemplate.Service
	Contract   *grpc.Server
}

// Stands in for the configured sender so a scenario can see what the engines actually delivered
type mailSink struct {
	mu   sync.Mutex
	sent []email.Message
}

func (m *mailSink) Send(_ context.Context, msg email.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mailSink) to(addr string) []email.Message {
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

// One graph per process - scenarios share it and isolate through their own channel
var (
	harnessOnce sync.Once
	shared      *harness
	harnessErr  error
)

func newHarness(t *testing.T) *harness {
	t.Helper()
	testdb.RequireSignals(t)
	harnessOnce.Do(func() {
		// rate limiting ships OFF - the throttling scenario tests the ON behaviour (memory limiter, no Valkey here)
		if os.Getenv("RATE_LIMIT_ENABLED") == "" {
			os.Setenv("RATE_LIMIT_ENABLED", "true")
		}
		// The same fixed test keys core's e2ekit signs with, unless the runner set real ones
		seed := os.Getenv(auth.EnvCustomerPrivateKey)
		if seed == "" {
			seed = "ZTJlLWN1c3RvbWVyLXNlZWQtMDEyMzQ1Njc4OWFiY2Q="
		}
		signer, err := auth.NewCustomerSigner(seed)
		if err != nil {
			panic(err)
		}
		linkKey := os.Getenv(emailtrack.EnvLinkSecret)
		if linkKey == "" {
			linkKey = "e2e-email-link-secret-0123456789"
		}
		service := os.Getenv("SIGNALS_SERVICE_SECRET")
		if service == "" {
			service = "e2e-service-secret"
		}
		dsn, _ := testdb.URL("SIGNALS_DB_URL")
		cfg := boot.Config{
			Env: os.Getenv("APP_ENV"), Customers: signer.Verifier(), EmailLinks: emailtrack.NewSigner([]byte(linkKey)), ServiceKeys: auth.MustServiceKeys(service),
			SignalsURL: dsn, SignalsReadURL: dsn, SignalsMaxConns: 2, SignalsReadMaxConns: 2,
		}
		ctx := context.Background()
		sink := &mailSink{}
		var d harnessDeps
		app := fx.New(
			fx.Supply(cfg, boot.RootContext{Context: ctx}, boot.QueueConsumer),
			boot.Pools, boot.Services, boot.Pulse, boot.Wiring, boot.Beacon, boot.Contract,
			fx.Decorate(func(email.Sender) email.Sender { return sink }),
			fx.Populate(&d),
			fx.NopLogger,
		)
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
		shared = &harness{ctx: ctx, d: d, signer: signer, service: service, mail: sink}
	})
	if harnessErr != nil {
		t.Fatalf("boot suite graph: %v", harnessErr)
	}
	return shared
}

// channel is one core pushed across - the whole set, as the push job sends it, so the previous
// scenario's channel stops being served the moment this one lands
func (h *harness) channel(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, _, err := h.d.Channels.SyncChannels(h.ctx, []analytics.ChannelEntry{{ID: id, IsDefault: true, IsActive: true, Name: "E2E"}}); err != nil {
		t.Fatalf("push channel: %v", err)
	}
	return id
}

// customerToken is what the storefront sends for a signed-in shopper - minted with the secret the
// surface verifies with, for a customer core would hold
func (h *harness) customerToken(t *testing.T, customerID, channelID uuid.UUID, email string) string {
	t.Helper()
	tok, err := auth.GenerateCustomerToken(customerID, channelID, email, h.signer, time.Hour)
	if err != nil {
		t.Fatalf("mint customer token: %v", err)
	}
	return tok
}

// beaconServer is the surface cmd/signals mounts
func beaconServer(t *testing.T, h *harness) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(h.d.Mux)
	t.Cleanup(ts.Close)
	return ts
}

// Runs everywhere, no database - proves the suite's composition resolves, the same guarantee core's
// wiring test gives its entrypoints
func TestSuiteGraphResolves(t *testing.T) {
	for name, deps := range map[string]any{"signals": &boot.Deps{}, "ingest": &boot.IngestDeps{}} {
		pools, mode := boot.Pools, boot.QueueConsumer
		surface := []fx.Option{boot.Contract}
		if name == "ingest" {
			pools, mode = boot.IngestPools, boot.QueueProducer // Pulse rides along for /e
			surface = []fx.Option{boot.IngestFallback}
		}
		if err := fx.ValidateApp(append(surface,
			fx.Supply(boot.Config{}, boot.RootContext{Context: context.Background()}, mode),
			pools, boot.Services, boot.Pulse, boot.Wiring, boot.Beacon,
			fx.Populate(deps),
			fx.NopLogger,
		)...); err != nil {
			t.Errorf("%s graph does not resolve: %v", name, err)
		}
	}
}

package wiring

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/emailtrack"
)

// server answers HTTP, worker runs the background layer behind nothing but /healthz - both at once is safe, just redundant, since the loops are all DB-claim based (SKIP LOCKED + leases)
const (
	RoleServer = "server"
	RoleWorker = "worker"
)

const (
	DefaultPort       = "8080"
	DefaultWorkerPort = "8081"
)

// Read and validated once at boot so no provider re-derives it - single-consumer endpoints (OpenSearch, S3, the storefront URL) stay env-local to whichever provider opens them
type Config struct {
	Env  string
	Role string

	Port       string
	WorkerPort string

	// Each signs one thing and is held only where that thing is checked - see docs/env.md for who holds which
	AdminKeys           auth.AdminKeys      // admin sessions, core alone
	CustomerKeys        auth.CustomerSigner // customer sessions - Signals gets only the public half
	EmailLinks          *emailtrack.Signer  // unsubscribe, open and click links, shared with Signals
	VoterTokenSecret    []byte              // anonymous review votes, shared with the storefront
	SignalsServiceKeys  auth.ServiceKeys    // signs core's calls on the Signals contract - the suite verifies with the same keys, and the storefront never sees them
	DashboardURL        string
	TrustedProxies      string
	ConfigEncryptionKey string // Empty in dev means secret config values are stored as plaintext

	JobsExternal  bool
	RunBackground bool // False only when the server role hands the background layer to a worker

	DatabaseURL     string
	ReplicaURL      string
	ValkeyURL       string
	ValkeyCacheURL  string // evictable Valkey for caches/limiters/APQ - empty falls back to ValkeyURL for a single instance - prod points it at a second instance with an eviction policy so a cache flood can't fill the noeviction primary and take auth-token writes down with it
	ValkeyLimitsURL string

	MaxConns          int
	AnalyticsMaxConns int
}

func (c Config) IsDev() bool    { return c.Env == "development" }
func (c Config) IsWorker() bool { return c.Role == RoleWorker }

func LoadConfig() (Config, error) { // an error here fails the boot before a single connection is opened
	c, err := loadBase()
	if err != nil {
		return Config{}, err
	}
	if err := auth.RefuseRetiredSecret(os.Getenv); err != nil {
		return Config{}, err
	}
	if c.AdminKeys, err = auth.AdminKeysFromEnv(os.Getenv); err != nil {
		return Config{}, err
	}
	if c.CustomerKeys, err = auth.CustomerSignerFromEnv(os.Getenv); err != nil {
		return Config{}, err
	}
	if c.EmailLinks, err = emailtrack.SignerFromEnv(os.Getenv); err != nil {
		return Config{}, err
	}
	// Signs anon review-vote cookie tokens, so a hostile caller can't forge a voter with any UUID - the storefront mints them with the same value
	c.VoterTokenSecret = []byte(strings.TrimSpace(os.Getenv("REVIEW_VOTER_TOKEN_SECRET")))
	if len(c.VoterTokenSecret) == 0 {
		return Config{}, errors.New("REVIEW_VOTER_TOKEN_SECRET is not set - it signs anonymous review votes, shared with the storefront")
	}
	return c, nil
}

// loadBase is what every entrypoint needs - the session, link and voter keys live in LoadConfig because only the HTTP surfaces sign or verify anything
func loadBase() (Config, error) {
	c := Config{
		Env:          os.Getenv("APP_ENV"),
		Role:         strings.TrimSpace(os.Getenv("SKELET_ROLE")),
		Port:         os.Getenv("PORT"),
		WorkerPort:   os.Getenv("WORKER_PORT"),
		DashboardURL: os.Getenv("DASHBOARD_URL"),

		SignalsServiceKeys: auth.ServiceKeysFromEnv(os.Getenv),
		TrustedProxies:     os.Getenv("TRUSTED_PROXIES"),
		JobsExternal:       EnvBool("JOBS_EXTERNAL", false),

		ConfigEncryptionKey: strings.TrimSpace(os.Getenv("CONFIG_ENCRYPTION_KEY")),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		ReplicaURL:  os.Getenv("DATABASE_REPLICA_URL"),

		ValkeyURL:      strings.TrimSpace(os.Getenv("VALKEY_URL")),
		ValkeyCacheURL: strings.TrimSpace(os.Getenv("VALKEY_CACHE_URL")),

		ValkeyLimitsURL: strings.TrimSpace(os.Getenv("VALKEY_LIMITS_URL")),

		MaxConns:          EnvInt("DB_MAX_CONNS", 25),
		AnalyticsMaxConns: EnvInt("ANALYTICS_DB_MAX_CONNS", 8),
	}

	if c.Role == "" { // Unset means server, so existing single-process deploys are unaffected
		c.Role = RoleServer
	}
	if c.Role != RoleServer && c.Role != RoleWorker {
		return Config{}, fmt.Errorf("SKELET_ROLE=%q is not a role (want %q or %q)", c.Role, RoleServer, RoleWorker)
	}
	c.RunBackground = c.Role == RoleWorker || !c.JobsExternal

	if c.Port == "" {
		c.Port = DefaultPort
	}
	if c.WorkerPort == "" { // The worker gets its own port so both roles can share a host network namespace without colliding
		c.WorkerPort = DefaultWorkerPort
	}

	if c.ReplicaURL == "" { // The read pool falls back to the primary - a replica is an optimisation, not a requirement
		c.ReplicaURL = c.DatabaseURL
	}

	return c, nil
}

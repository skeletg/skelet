package wiring

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

	JWTSecret           string
	VoterTokenSecret    []byte
	DashboardURL        string
	TrustedProxies      string
	ConfigEncryptionKey string // Empty in dev means secret config values are stored as plaintext

	JobsExternal        bool
	SignalsJobsExternal bool // SIGNALS_JOBS_EXTERNAL hands retention/forwarding/trait-compute to cmd/signals
	B2BEnabled          bool // B2B_ENABLED=false switches off the whole wholesale surface
	RunBackground       bool // False only when the server role hands the background layer to a worker

	DatabaseURL    string
	ReplicaURL     string
	SignalsURL     string
	SignalsReadURL string
	ValkeyURL      string
	ValkeyCacheURL string // evictable Valkey for caches/limiters/APQ - empty falls back to ValkeyURL for a single instance - prod points it at a second instance with an eviction policy so a cache flood can't fill the noeviction primary and take auth-token writes down with it

	DirectDatabaseURL string // pooler bypass for pg_dump, whose snapshot and SETs need a session a transaction pooler won't hold - unset means there is no pooler and the pooled URL IS direct
	DirectSignalsURL  string

	MaxConns            int
	AnalyticsMaxConns   int
	SignalsMaxConns     int
	SignalsReadMaxConns int
}

func (c Config) IsDev() bool    { return c.Env == "development" }
func (c Config) IsWorker() bool { return c.Role == RoleWorker }

func LoadConfig() (Config, error) { // an error here fails the boot before a single connection is opened
	c, err := loadBase()
	if err != nil {
		return Config{}, err
	}
	if c.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is not set")
	}

	// Signs anon review-vote cookie tokens, so a hostile caller can't forge a voter with any UUID - derived from JWT_SECRET when unset to keep existing deploys working, production should set both
	c.VoterTokenSecret = []byte(os.Getenv("REVIEW_VOTER_TOKEN_SECRET"))
	if len(c.VoterTokenSecret) == 0 {
		c.VoterTokenSecret = []byte("vote:" + c.JWTSecret)
	}
	return c, nil
}

// loadBase is what every entrypoint needs - the JWT and voter-token rules live in LoadConfig because only the HTTP surfaces sign or verify anything
func loadBase() (Config, error) {
	c := Config{
		Env:                 os.Getenv("APP_ENV"),
		Role:                strings.TrimSpace(os.Getenv("SKELET_ROLE")),
		Port:                os.Getenv("PORT"),
		WorkerPort:          os.Getenv("WORKER_PORT"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		DashboardURL:        os.Getenv("DASHBOARD_URL"),
		TrustedProxies:      os.Getenv("TRUSTED_PROXIES"),
		JobsExternal:        EnvBool("JOBS_EXTERNAL", false),
		SignalsJobsExternal: EnvBool("SIGNALS_JOBS_EXTERNAL", false),
		B2BEnabled:          EnvBool("B2B_ENABLED", true),

		ConfigEncryptionKey: strings.TrimSpace(os.Getenv("CONFIG_ENCRYPTION_KEY")),

		DatabaseURL:    os.Getenv("DATABASE_URL"),
		ReplicaURL:     os.Getenv("DATABASE_REPLICA_URL"),
		SignalsURL:     os.Getenv("SIGNALS_DB_URL"),
		SignalsReadURL: os.Getenv("SIGNALS_REPLICA_URL"),

		DirectDatabaseURL: EnvOr("DATABASE_DIRECT_URL", os.Getenv("DATABASE_URL")),
		DirectSignalsURL:  EnvOr("SIGNALS_DIRECT_URL", os.Getenv("SIGNALS_DB_URL")),
		ValkeyURL:         strings.TrimSpace(os.Getenv("VALKEY_URL")),
		ValkeyCacheURL:    strings.TrimSpace(os.Getenv("VALKEY_CACHE_URL")),

		MaxConns:            EnvInt("DB_MAX_CONNS", 25),
		AnalyticsMaxConns:   EnvInt("ANALYTICS_DB_MAX_CONNS", 8),
		SignalsMaxConns:     EnvInt("SIGNALS_DB_MAX_CONNS", 8),
		SignalsReadMaxConns: EnvInt("SIGNALS_READ_DB_MAX_CONNS", 8),
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

	if c.SignalsURL == "" { // required even at SIGNALS_ENABLED=false - email_sends lives here, so verification and reset mail would hit a nil pool
		return Config{}, errors.New("SIGNALS_DB_URL is not set - Signals lives in its own database, and it is required even when SIGNALS_ENABLED=false because transactional email records its sends there (a second database on the commerce instance is fine)")
	}

	if c.ReplicaURL == "" { // Both read pools fall back to their own primary - a replica is an optimisation, not a requirement
		c.ReplicaURL = c.DatabaseURL
	}
	if c.SignalsReadURL == "" {
		c.SignalsReadURL = c.SignalsURL
	}

	return c, nil
}

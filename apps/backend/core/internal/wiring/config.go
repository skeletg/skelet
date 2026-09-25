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

	JWTSecret            string
	VoterTokenSecret     []byte
	SignalsServiceSecret string // signs core's calls on the Signals contract - the suite verifies with the same value, and the storefront never sees it
	DashboardURL         string
	TrustedProxies       string
	ConfigEncryptionKey  string // Empty in dev means secret config values are stored as plaintext

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
		Env:          os.Getenv("APP_ENV"),
		Role:         strings.TrimSpace(os.Getenv("SKELET_ROLE")),
		Port:         os.Getenv("PORT"),
		WorkerPort:   os.Getenv("WORKER_PORT"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		DashboardURL: os.Getenv("DASHBOARD_URL"),

		SignalsServiceSecret: strings.TrimSpace(os.Getenv("SIGNALS_SERVICE_SECRET")),
		TrustedProxies:       os.Getenv("TRUSTED_PROXIES"),
		JobsExternal:         EnvBool("JOBS_EXTERNAL", false),

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

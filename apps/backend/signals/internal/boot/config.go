// Package boot is the suite's composition root - the config both binaries read, the pools they open and the services they build
// Nothing here reaches core's internal tree, only lib/ and the contract
package boot

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
)

// Config is what the two suite processes read from the environment, once, before a connection opens
// No commerce database here - everything of core's is pushed across or asked for over the contract
type Config struct {
	Env string
	// The session secret - verifies the customer sessions /t reads and signs the unsubscribe links core verifies
	JWTSecret string
	// Verifies the tokens core signs its contract calls with - shared with core alone, empty in the beacon tier
	ServiceSecret string

	SignalsURL     string
	SignalsReadURL string
	// The pooler bypass pg_dump and pg_restore need - a real session, which a transaction pooler
	// won't hold. Unset means there is no pooler and the pooled URL IS direct
	DirectURL string
	// The evictable Valkey the rate limiter and the settings cache ride - optional, both fail open without it
	ValkeyURL string
	// The primary Valkey, where core publishes killed admin sessions - optional, without it revoked tokens live out their two minutes
	PrimaryValkeyURL string

	SignalsMaxConns     int
	SignalsReadMaxConns int
}

func (c Config) IsDev() bool { return c.Env == "development" }

// Load is what cmd/signals reads: the contract server and the maintenance work core schedules
func Load() (Config, error) {
	c, err := load(EnvInt("SIGNALS_DB_MAX_CONNS", 8))
	if err != nil {
		return Config{}, err
	}
	// Without it the contract refuses every call silently, so fail at boot
	if c.ServiceSecret == "" {
		return Config{}, errors.New("SIGNALS_SERVICE_SECRET is not set - the contract verifies core's calls with it, and core signs them with the same value")
	}
	return c, nil
}

// LoadIngest sizes the pool for the beacon tier - 16 keeps concurrent /t writes inside signals-db's connection budget, past that the queue is the lever
func LoadIngest() (Config, error) {
	return load(EnvInt("INGEST_SIGNALS_MAX_CONNS", 16))
}

func load(maxConns int) (Config, error) {
	c := Config{
		Env:                 os.Getenv("APP_ENV"),
		JWTSecret:           os.Getenv("JWT_SECRET"),
		ServiceSecret:       strings.TrimSpace(os.Getenv("SIGNALS_SERVICE_SECRET")),
		SignalsURL:          os.Getenv("SIGNALS_DB_URL"),
		SignalsReadURL:      os.Getenv("SIGNALS_REPLICA_URL"),
		DirectURL:           EnvOr("SIGNALS_DIRECT_URL", os.Getenv("SIGNALS_DB_URL")),
		ValkeyURL:           EnvOr("VALKEY_CACHE_URL", strings.TrimSpace(os.Getenv("VALKEY_URL"))),
		PrimaryValkeyURL:    strings.TrimSpace(os.Getenv("VALKEY_URL")),
		SignalsMaxConns:     maxConns,
		SignalsReadMaxConns: EnvInt("SIGNALS_READ_DB_MAX_CONNS", 8),
	}
	if c.SignalsURL == "" {
		return Config{}, errors.New("SIGNALS_DB_URL is not set - the suite has no other database")
	}
	// /t and the unsubscribe links need it, so an empty value fails at boot and not at the first shopper
	if c.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is not set - /t verifies the customer sessions with it, and the unsubscribe links are signed with it")
	}
	if c.SignalsReadURL == "" { // a replica is an optimisation, not a requirement
		c.SignalsReadURL = c.SignalsURL
	}
	return c, nil
}

func EnvInt(key string, def int) int { // only accepts positives - every caller sizes a pool or a limit where a stray 0 means "unset", so it falls back to def
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func EnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func EnvOr(key, def string) string { // a set-but-empty var means "unset" here, since compose writes empty strings for anything it can't interpolate
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func EnvInt64(name string, def int64) int64 { // logs bad input instead of swallowing it - these are byte sizes, where a typo silently capping uploads is worth a line in the log
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Printf("env %s=%q not a valid int64; using default %d", name, raw, def)
		return def
	}
	return n
}

// RateLimitPerMin is a limiter's per-minute budget, 0 while the layer is off - ratelimit.AllowN admits
// everything at 0 before touching Valkey, so disabled costs nothing per request. Ships OFF, like core's
func RateLimitPerMin(key string, def int) int {
	if !EnvBool("RATE_LIMIT_ENABLED", false) {
		return 0
	}
	return EnvInt(key, def)
}

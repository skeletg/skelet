// Package wiring is the composition root - it builds the graph both roles run on, so cmd/server stays thin
package wiring

import (
	"log"
	"os"
	"strconv"
	"strings"
)

func EnvInt(key string, def int) int { // only accepts positives - every caller sizes a pool or a limit where a stray 0 means "unset", so it falls back to def
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func RateLimitEnabled() bool { // rate limiting and the login lockout ship OFF - a fresh install shouldn't 429 anyone clicking around, and a dev box has no proxy in front to key IPs on anyway
	return EnvBool("RATE_LIMIT_ENABLED", false)
}

func RateLimitPerMin(key string, def int) int { // a limiter's per-minute budget, 0 while the layer is off - ratelimit.AllowN admits everything at 0 before touching Valkey, so disabled costs nothing per request
	if !RateLimitEnabled() {
		return 0
	}
	return EnvInt(key, def)
}

func EnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
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

func EnvOr(key, def string) string { // a set-but-empty var means "unset" here, since compose writes empty strings for anything it can't interpolate
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

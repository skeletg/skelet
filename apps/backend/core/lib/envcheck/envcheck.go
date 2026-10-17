// Package envcheck is the boot-time production config gate, composed per process so every entrypoint runs the same checks
package envcheck

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Getenv is os.Getenv in production and a map in tests
type Getenv func(string) string

// A Check reports hard errors, which abort the boot, and warnings, which log loudly and let the process up with a
// broken or insecure feature
type Check func(Getenv) (errs, warnings []string)

// Production runs the checks a given process cares about - a no-op outside production, so development keeps working
// with dev defaults
func Production(getenv Getenv, checks ...Check) (errs, warnings []string) {
	if getenv("APP_ENV") != "production" {
		return nil, nil
	}
	for _, c := range checks {
		e, w := c(getenv)
		errs = append(errs, e...)
		warnings = append(warnings, w...)
	}
	return errs, warnings
}

// Report logs everything found and returns an error only when something is unsafe to run with, so each entrypoint
// handles the outcome the same way
func Report(errs, warnings []string) error {
	for _, w := range warnings {
		slog.Warn("production config warning", "issue", w)
	}
	for _, e := range errs {
		slog.Error("production config error", "issue", e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("refusing to start: %d production config error(s) - see logs above", len(errs))
	}
	return nil
}

// Each process holds only the keys its job needs - see docs/env.md for the map - and every secret shared between two tiers must differ from every other one, or holding one would hand over the rest

// CoreSessionKeys guards the server, which mints both session types and verifies email links and review votes
func CoreSessionKeys(getenv Getenv) (errs, warnings []string) {
	errs = append(errs, retired(getenv)...)
	errs = append(errs, strongSecret(getenv, "ADMIN_JWT_SECRET")...)
	errs = append(errs, ed25519Key(getenv, "CUSTOMER_JWT_PRIVATE_KEY")...)
	errs = append(errs, strongSecret(getenv, "EMAIL_LINK_SECRET")...)
	errs = append(errs, strongSecret(getenv, "REVIEW_VOTER_TOKEN_SECRET")...)
	errs = append(errs, distinct(getenv, "ADMIN_JWT_SECRET", "EMAIL_LINK_SECRET", "REVIEW_VOTER_TOKEN_SECRET", "SIGNALS_SERVICE_SECRET")...)
	return errs, nil
}

// SignalsSessionKeys guards cmd/signals - customer sessions verified with the public key alone, and the email links it signs
func SignalsSessionKeys(getenv Getenv) (errs, warnings []string) {
	errs = append(errs, retired(getenv)...)
	errs = append(errs, ed25519Key(getenv, "CUSTOMER_JWT_PUBLIC_KEY")...)
	errs = append(errs, strongSecret(getenv, "EMAIL_LINK_SECRET")...)
	errs = append(errs, distinct(getenv, "EMAIL_LINK_SECRET", "SIGNALS_SERVICE_SECRET")...)
	return errs, nil
}

// IngestSessionKeys guards cmd/ingest, the internet-facing beacon tier - it verifies customer sessions and holds nothing that signs
func IngestSessionKeys(getenv Getenv) (errs, warnings []string) {
	errs = append(errs, retired(getenv)...)
	errs = append(errs, ed25519Key(getenv, "CUSTOMER_JWT_PUBLIC_KEY")...)
	return errs, nil
}

func retired(getenv Getenv) []string {
	if strings.TrimSpace(getenv("JWT_SECRET")) != "" {
		return []string{"JWT_SECRET is retired - remove it; ADMIN_JWT_SECRET, CUSTOMER_JWT_PRIVATE_KEY / CUSTOMER_JWT_PUBLIC_KEY and EMAIL_LINK_SECRET replace it (docs/env.md)"}
	}
	return nil
}

func strongSecret(getenv Getenv, name string) []string {
	s := strings.TrimSpace(getenv(name))
	switch {
	case s == "" || strings.HasPrefix(s, "change-me"):
		return []string{name + " must be a strong secret (not empty or a dev default)"}
	case len(s) < 32:
		// HMAC keys below the 256-bit hash output add no security and are brute-forceable
		return []string{name + " must be at least 32 characters (use a 256-bit random secret)"}
	}
	return nil
}

func ed25519Key(getenv Getenv, name string) []string {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(getenv(name)))
	if err != nil || len(b) != 32 {
		return []string{name + " must be base64 of a 32-byte Ed25519 key - `./skelet setup` generates the pair"}
	}
	return nil
}

func distinct(getenv Getenv, names ...string) []string {
	var errs []string
	for i, a := range names {
		va := strings.TrimSpace(getenv(a))
		for _, b := range names[i+1:] {
			if vb := strings.TrimSpace(getenv(b)); va != "" && va == vb {
				errs = append(errs, a+" and "+b+" must differ - they are held by different tiers, and one value would hand over both")
			}
		}
	}
	return errs
}

// SignalsServiceSecret guards the two ends of the Signals contract
func SignalsServiceSecret(getenv Getenv) (errs, warnings []string) {
	return strongSecret(getenv, "SIGNALS_SERVICE_SECRET"), nil
}

const ErrSignalsUnaddressed = "SIGNALS_ENABLED is true (the default) but SIGNALS_GRPC_ADDR is empty - point it at the signals service (e.g. signals:8092), or set SIGNALS_ENABLED=false to run without the suite"

// SignalsCaller is the same rule for core, which only signs when it has a suite to dial
func SignalsCaller(getenv Getenv) (errs, warnings []string) {
	if v := strings.TrimSpace(getenv("SIGNALS_ENABLED")); v != "" {
		if on, err := strconv.ParseBool(v); err == nil && !on {
			return nil, nil
		}
	}
	if strings.TrimSpace(getenv("SIGNALS_GRPC_ADDR")) == "" {
		return []string{ErrSignalsUnaddressed}, nil
	}
	return SignalsServiceSecret(getenv)
}

// TrustedProxies guards any process that rate-limits or locks out by client IP
func TrustedProxies(getenv Getenv) (errs, warnings []string) {
	// Unset behind a proxy means X-Forwarded-For is ignored and every visitor keys rate limits and the login lockout on the PROXY's IP, so demand the CIDRs or the literal 'none'
	if strings.TrimSpace(getenv("TRUSTED_PROXIES")) == "" {
		errs = append(errs, "TRUSTED_PROXIES must be set in production: your proxy/LB CIDRs (so forwarded client IPs are trusted for rate limiting), or the literal 'none' if the backend is directly internet-facing")
	}
	return errs, nil
}

// ConfigEncryptionKey guards any process that reads secret config values, which is anything building the config service
func ConfigEncryptionKey(getenv Getenv) (errs, warnings []string) {
	s := strings.TrimSpace(getenv("CONFIG_ENCRYPTION_KEY"))
	switch {
	case s == "":
		errs = append(errs, "CONFIG_ENCRYPTION_KEY must be set (base64 of 32 random bytes, e.g. `openssl rand -base64 32`) to encrypt secret config values at rest")
	case !validConfigEncryptionKey(s):
		errs = append(errs, "CONFIG_ENCRYPTION_KEY must base64-decode to exactly 32 bytes (AES-256)")
	}
	return errs, nil
}

// Database guards any process that opens a pool
func Database(getenv Getenv) (errs, warnings []string) {
	return databaseURL("DATABASE_URL", getenv)
}

// SignalsDatabase is the same rule on SIGNALS_DB_URL - the suite's only database, and core's second
func SignalsDatabase(getenv Getenv) (errs, warnings []string) {
	return databaseURL("SIGNALS_DB_URL", getenv)
}

func databaseURL(name string, getenv Getenv) (errs, warnings []string) {
	db := getenv(name)
	// sslmode=disable sends DB traffic in cleartext - fine for the bundled db on the trusted compose network, dangerous for a managed/remote Postgres - require an explicit acknowledgement so it can't slip into a remote-DB deploy
	if strings.Contains(db, "sslmode=disable") && strings.TrimSpace(getenv("ALLOW_INSECURE_DB")) == "" {
		errs = append(errs, name+" has sslmode=disable - use sslmode=require/verify-full, or set ALLOW_INSECURE_DB=1 to acknowledge a trusted private DB link (e.g. the bundled compose db)")
	}
	if strings.Contains(db, "skelet:skelet") {
		warnings = append(warnings, name+" uses the default dev credentials (skelet:skelet) - set a strong DB password")
	}
	return errs, warnings
}

// CORS guards the process that answers browsers
func CORS(getenv Getenv) (errs, warnings []string) {
	if strings.TrimSpace(getenv("CORS_ALLOWED_ORIGINS")) == "" {
		errs = append(errs, "CORS_ALLOWED_ORIGINS must list the deployed frontend origins (empty blocks all browser API calls)")
	}
	return errs, nil
}

// FrontendURLs only affect the links inside outgoing mail, so they warn rather than refuse
func FrontendURLs(getenv Getenv) (errs, warnings []string) {
	if strings.TrimSpace(getenv("STOREFRONT_URL")) == "" {
		warnings = append(warnings, "STOREFRONT_URL is empty - verification/password-reset links will be malformed")
	}
	if strings.TrimSpace(getenv("DASHBOARD_URL")) == "" {
		warnings = append(warnings, "DASHBOARD_URL is empty - admin password-reset links will be malformed")
	}
	return nil, warnings
}

func SMTP(getenv Getenv) (errs, warnings []string) {
	if strings.TrimSpace(getenv("SMTP_HOST")) == "" {
		warnings = append(warnings, "SMTP_HOST is empty - transactional email (verification, password reset) will fail")
	}
	return nil, warnings
}

func S3(getenv Getenv) (errs, warnings []string) {
	if strings.TrimSpace(getenv("S3_BUCKET_NAME")) == "" {
		warnings = append(warnings, "S3_BUCKET_NAME is empty - media uploads will fail")
	}
	return nil, warnings
}

// RateLimiting and LoginLockout warn instead of refusing - both ship off by default, and a proxy in front may already be metering
func RateLimiting(getenv Getenv) (errs, warnings []string) {
	if !truthy(getenv("RATE_LIMIT_ENABLED")) {
		warnings = append(warnings, "RATE_LIMIT_ENABLED is not set - per-IP rate limiting is OFF on every surface this process serves; set it to true unless your proxy already meters requests")
	}
	return nil, warnings
}

// Only for the process that answers logins - LOGIN_LOCKOUT_ENABLED defaults to RATE_LIMIT_ENABLED, so either one turns it on
func LoginLockout(getenv Getenv) (errs, warnings []string) {
	if !truthy(getenv("LOGIN_LOCKOUT_ENABLED")) && !truthy(getenv("RATE_LIMIT_ENABLED")) {
		warnings = append(warnings, "LOGIN_LOCKOUT_ENABLED is not set - failed admin/customer logins are never locked out, so password guessing is unbounded")
	}
	return nil, warnings
}

func truthy(v string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	return err == nil && b
}

func OpenSearch(getenv Getenv) (errs, warnings []string) {
	// Without a cluster the indexers are disabled and every product/blog search silently returns empty - a broken storefront search box with no loud failure - provision a cluster and set this in production
	if strings.TrimSpace(getenv("OPENSEARCH_URL")) == "" {
		warnings = append(warnings, "OPENSEARCH_URL is empty - product & blog search is disabled and will silently return no results")
	}
	return nil, warnings
}

// Mirrors secrets.NewCipher's acceptance so the boot check and the runtime cipher agree
func validConfigEncryptionKey(s string) bool {
	for _, dec := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		if b, err := dec.DecodeString(s); err == nil {
			return len(b) == 32
		}
	}
	return false
}

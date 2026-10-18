package envcheck

import (
	"encoding/base64"
	"maps"
	"strings"
	"testing"
)

// serverChecks is the set cmd/server composes - the port keeps these cases pointed at the same list they
// always covered, so nothing silently stopped being checked when the gate became per-process
func serverChecks(getenv Getenv) (errs, warnings []string) {
	return Production(getenv,
		CoreSessionKeys, TrustedProxies, ConfigEncryptionKey, Database,
		CORS, FrontendURLs, SMTP, Storage, OpenSearch,
		RateLimiting, LoginLockout,
	)
}

// envMap returns a getenv func backed by a map
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// validEncKey is a base64-encoded 32-byte key accepted by the boot check
var validEncKey = base64.StdEncoding.EncodeToString(make([]byte, 32))

// A set of session keys the server accepts - all distinct, all strong
var (
	adminKey     = strings.Repeat("a", 40)
	customerSeed = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32)))
	customerPub  = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32)))
	linkKey      = strings.Repeat("e", 40)
	voterKey     = strings.Repeat("v", 40)
)

func TestCheckProductionEnv_SkippedOutsideProd(t *testing.T) {
	errs, warns := serverChecks(envMap(map[string]string{"APP_ENV": "development"}))
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("non-prod should be a no-op, got errs=%v warns=%v", errs, warns)
	}
}

func TestCheckProductionEnv_HardErrors(t *testing.T) {
	// No session keys (four) + empty CORS + missing CONFIG_ENCRYPTION_KEY + empty TRUSTED_PROXIES → seven hard errors
	errs, _ := serverChecks(envMap(map[string]string{"APP_ENV": "production"}))
	if len(errs) != 7 {
		t.Fatalf("expected 7 hard errors (4 session keys + empty CORS + missing CONFIG_ENCRYPTION_KEY + empty TRUSTED_PROXIES), got %d: %v", len(errs), errs)
	}
}

func TestCheckProductionEnv_InsecureDBRequiresAck(t *testing.T) {
	base := map[string]string{
		"APP_ENV":          "production",
		"ADMIN_JWT_SECRET": adminKey, "CUSTOMER_JWT_PRIVATE_KEY": customerSeed, "EMAIL_LINK_SECRET": linkKey, "REVIEW_VOTER_TOKEN_SECRET": voterKey,
		"CORS_ALLOWED_ORIGINS":  "https://shop.example.com",
		"CONFIG_ENCRYPTION_KEY": validEncKey,
		"TRUSTED_PROXIES":       "172.16.0.0/12",
		"DATABASE_URL":          "postgres://u:strongpw@db:5432/skelet?sslmode=disable",
	}
	sslErr := func(errs []string) bool {
		for _, e := range errs {
			if strings.Contains(e, "sslmode=disable") {
				return true
			}
		}
		return false
	}
	if errs, _ := serverChecks(envMap(base)); !sslErr(errs) {
		t.Errorf("sslmode=disable without ALLOW_INSECURE_DB should be a hard error, got %v", errs)
	}
	base["ALLOW_INSECURE_DB"] = "1"
	if errs, _ := serverChecks(envMap(base)); sslErr(errs) {
		t.Errorf("ALLOW_INSECURE_DB=1 should clear the sslmode error, got %v", errs)
	}
}

func TestCheckProductionEnv_TrustedProxiesNoneAccepted(t *testing.T) {
	// The literal 'none' satisfies the requirement (direct-exposure opt-out)
	errs, _ := serverChecks(envMap(map[string]string{
		"APP_ENV":          "production",
		"ADMIN_JWT_SECRET": adminKey, "CUSTOMER_JWT_PRIVATE_KEY": customerSeed, "EMAIL_LINK_SECRET": linkKey, "REVIEW_VOTER_TOKEN_SECRET": voterKey,
		"CORS_ALLOWED_ORIGINS":  "https://shop.example.com",
		"CONFIG_ENCRYPTION_KEY": validEncKey,
		"TRUSTED_PROXIES":       "none",
		"DATABASE_URL":          "postgres://u:strongpw@db:5432/skelet?sslmode=require",
	}))
	for _, e := range errs {
		if strings.Contains(e, "TRUSTED_PROXIES") {
			t.Fatalf("TRUSTED_PROXIES=none should be accepted, got %v", errs)
		}
	}
}

func TestCheckProductionEnv_ConfigEncryptionKey(t *testing.T) {
	base := map[string]string{
		"APP_ENV":          "production",
		"ADMIN_JWT_SECRET": adminKey, "CUSTOMER_JWT_PRIVATE_KEY": customerSeed, "EMAIL_LINK_SECRET": linkKey, "REVIEW_VOTER_TOKEN_SECRET": voterKey,
		"CORS_ALLOWED_ORIGINS": "https://shop.example.com",
		"DATABASE_URL":         "postgres://u:strongpw@db:5432/skelet?sslmode=require",
		"STOREFRONT_URL":       "https://shop.example.com",
		"SMTP_HOST":            "smtp.example.com",
		"S3_BUCKET_NAME":       "shop-media",
	}
	keyErr := func(errs []string) bool {
		for _, e := range errs {
			if strings.Contains(e, "CONFIG_ENCRYPTION_KEY") {
				return true
			}
		}
		return false
	}

	// Missing → error
	if errs, _ := serverChecks(envMap(base)); !keyErr(errs) {
		t.Errorf("missing key: expected a CONFIG_ENCRYPTION_KEY error, got %v", errs)
	}
	// Wrong length (16 bytes) → error
	short := clone(base)
	short["CONFIG_ENCRYPTION_KEY"] = base64.StdEncoding.EncodeToString(make([]byte, 16))
	if errs, _ := serverChecks(envMap(short)); !keyErr(errs) {
		t.Errorf("short key: expected a CONFIG_ENCRYPTION_KEY error, got %v", errs)
	}
	// Valid 32-byte key → no key error
	ok := clone(base)
	ok["CONFIG_ENCRYPTION_KEY"] = validEncKey
	if errs, _ := serverChecks(envMap(ok)); keyErr(errs) {
		t.Errorf("valid key: expected no CONFIG_ENCRYPTION_KEY error, got %v", errs)
	}
}

func clone(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return out
}

// Each key is held by a different set of tiers, so a weak one, a dev default, a malformed customer key, the retired name or two equal values all refuse the boot
func TestCoreSessionKeys(t *testing.T) {
	valid := map[string]string{"APP_ENV": "production", "ADMIN_JWT_SECRET": adminKey, "CUSTOMER_JWT_PRIVATE_KEY": customerSeed, "EMAIL_LINK_SECRET": linkKey, "REVIEW_VOTER_TOKEN_SECRET": voterKey, "SIGNALS_SERVICE_SECRET": strings.Repeat("s", 40)}
	if errs, _ := Production(envMap(valid), CoreSessionKeys); len(errs) != 0 {
		t.Fatalf("valid keys refused: %v", errs)
	}
	for name, tc := range map[string]struct{ key, value, want string }{
		"short admin":      {"ADMIN_JWT_SECRET", "short", "at least 32"},
		"dev default link": {"EMAIL_LINK_SECRET", "change-me", "strong secret"},
		"bad customer key": {"CUSTOMER_JWT_PRIVATE_KEY", "not-base64", "Ed25519"},
		"retired secret":   {"JWT_SECRET", strings.Repeat("j", 40), "retired"},
		"admin as voter":   {"REVIEW_VOTER_TOKEN_SECRET", adminKey, "must differ"},
		"link as service":  {"SIGNALS_SERVICE_SECRET", linkKey, "must differ"},
		"voter unset":      {"REVIEW_VOTER_TOKEN_SECRET", "", "strong secret"},
	} {
		t.Run(name, func(t *testing.T) {
			env := clone(valid)
			env[tc.key] = tc.value
			errs, _ := Production(envMap(env), CoreSessionKeys)
			if len(errs) != 1 || !strings.Contains(errs[0], tc.want) {
				t.Fatalf("errs = %v, want one containing %q", errs, tc.want)
			}
		})
	}
}

// The Signals tiers hold no admin key and no customer private key - their checks ask only for what they hold
func TestSignalsTiersCheckOnlyWhatTheyHold(t *testing.T) {
	signals := map[string]string{"APP_ENV": "production", "CUSTOMER_JWT_PUBLIC_KEY": customerPub, "EMAIL_LINK_SECRET": linkKey, "SIGNALS_SERVICE_SECRET": strings.Repeat("s", 40)}
	if errs, _ := Production(envMap(signals), SignalsSessionKeys); len(errs) != 0 {
		t.Fatalf("cmd/signals refused a valid set: %v", errs)
	}
	ingest := map[string]string{"APP_ENV": "production", "CUSTOMER_JWT_PUBLIC_KEY": customerPub}
	if errs, _ := Production(envMap(ingest), IngestSessionKeys); len(errs) != 0 {
		t.Fatalf("cmd/ingest refused a valid set: %v", errs)
	}
	ingest["JWT_SECRET"] = strings.Repeat("j", 40)
	if errs, _ := Production(envMap(ingest), IngestSessionKeys); len(errs) != 1 {
		t.Fatalf("cmd/ingest accepted the retired JWT_SECRET: %v", errs)
	}
	signals["SIGNALS_SERVICE_SECRET"] = linkKey
	if errs, _ := Production(envMap(signals), SignalsSessionKeys); len(errs) != 1 {
		t.Fatalf("cmd/signals accepted its link key as the contract secret: %v", errs)
	}
}

func TestCheckProductionEnv_CleanConfigNoErrors(t *testing.T) {
	errs, warns := serverChecks(envMap(map[string]string{
		"APP_ENV":          "production",
		"ADMIN_JWT_SECRET": adminKey, "CUSTOMER_JWT_PRIVATE_KEY": customerSeed, "EMAIL_LINK_SECRET": linkKey, "REVIEW_VOTER_TOKEN_SECRET": voterKey,
		"CORS_ALLOWED_ORIGINS":  "https://shop.example.com",
		"CONFIG_ENCRYPTION_KEY": validEncKey,
		"DATABASE_URL":          "postgres://u:strongpw@db:5432/skelet?sslmode=require",
		"STOREFRONT_URL":        "https://shop.example.com",
		"SMTP_HOST":             "smtp.example.com",
		"S3_BUCKET_NAME":        "shop-media",
		"TRUSTED_PROXIES":       "172.16.0.0/12",
		"OPENSEARCH_URL":        "https://search.example.com",
		"DASHBOARD_URL":         "https://admin.example.com",
		"RATE_LIMIT_ENABLED":    "true", // off by default, so a clean production config has to say so
	}))
	if len(errs) != 0 {
		t.Fatalf("clean prod config should have no errors, got %v", errs)
	}
	if len(warns) != 0 {
		t.Fatalf("clean prod config should have no warnings, got %v", warns)
	}
}

func TestCheckProductionEnv_Warnings(t *testing.T) {
	// Valid hard requirements but insecure/incomplete operational config
	errs, warns := serverChecks(envMap(map[string]string{
		"APP_ENV":          "production",
		"ADMIN_JWT_SECRET": adminKey, "CUSTOMER_JWT_PRIVATE_KEY": customerSeed, "EMAIL_LINK_SECRET": linkKey, "REVIEW_VOTER_TOKEN_SECRET": voterKey,
		"CORS_ALLOWED_ORIGINS":  "https://shop.example.com",
		"CONFIG_ENCRYPTION_KEY": validEncKey,
		"TRUSTED_PROXIES":       "172.16.0.0/12",
		"ALLOW_INSECURE_DB":     "1",
		"DATABASE_URL":          "postgres://skelet:skelet@db:5432/skelet?sslmode=disable",
		// STOREFRONT_URL / DASHBOARD_URL / SMTP_HOST / S3_BUCKET_NAME /
		// OPENSEARCH_URL omitted
	}))
	if len(errs) != 0 {
		t.Fatalf("hard requirements met, expected no errors, got %v", errs)
	}
	// default DB creds + STOREFRONT_URL + DASHBOARD_URL + SMTP_HOST + S3_BUCKET_NAME + OPENSEARCH_URL = 6, plus the two rate-limit switches this config leaves off - (sslmode is now a hard error gated by ALLOW_INSECURE_DB, not a warning; TRUSTED_PROXIES is now a hard error.)
	if len(warns) != 8 {
		t.Fatalf("expected 8 warnings, got %d: %v", len(warns), warns)
	}
}

// The gap this package closed: cmd/ingest verifies customer sessions and rate-limits by forwarded IP on a public endpoint, but once took a dev-default key and an unset TRUSTED_PROXIES, which auth.SetTrustedProxies turns into no trusted proxies at all rather than an error
func TestIngestRefusesWhatTheServerRefuses(t *testing.T) {
	ingest := func(getenv Getenv) (errs, warnings []string) {
		return Production(getenv, IngestSessionKeys, TrustedProxies, Database)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
	}{
		{"no public key", map[string]string{"TRUSTED_PROXIES": "none"}},
		{"a short public key", map[string]string{"CUSTOMER_JWT_PUBLIC_KEY": "c2hvcnQ=", "TRUSTED_PROXIES": "none"}},
		{"unset trusted proxies", map[string]string{"CUSTOMER_JWT_PUBLIC_KEY": customerPub}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := maps.Clone(tc.env)
			env["APP_ENV"] = "production"
			if errs, _ := ingest(envMap(env)); len(errs) == 0 {
				t.Error("accepted in production - the server would refuse this")
			}
		})
	}
}

// Development keeps booting on dev defaults, or nobody can run the stack locally
func TestNoGateOutsideProduction(t *testing.T) {
	env := envMap(map[string]string{"APP_ENV": "development", "ADMIN_JWT_SECRET": "change-me"})
	if errs, warns := Production(env, CoreSessionKeys, TrustedProxies, Database); len(errs) > 0 || len(warns) > 0 {
		t.Fatalf("development is gated: errs=%v warns=%v", errs, warns)
	}
}

// Rate limiting ships off, so production has to say so out loud - a warning, never a hard error, since the
// proxy in front may be doing the metering
func TestRateLimitingWarnsWhenOff(t *testing.T) {
	errs, warns := Production(envMap(map[string]string{"APP_ENV": "production"}), RateLimiting, LoginLockout)
	if len(errs) != 0 {
		t.Fatalf("rate limiting must never block a boot, got errs=%v", errs)
	}
	if len(warns) != 2 {
		t.Fatalf("want a warning for each of the two switches, got %v", warns)
	}
}

func TestRateLimitingQuietWhenOn(t *testing.T) {
	// The master switch alone covers the lockout too, which is how it defaults in the wiring
	_, warns := Production(envMap(map[string]string{"APP_ENV": "production", "RATE_LIMIT_ENABLED": "true"}), RateLimiting, LoginLockout)
	if len(warns) != 0 {
		t.Fatalf("RATE_LIMIT_ENABLED=true should silence both, got %v", warns)
	}
}

func TestLoginLockoutOnItsOwnSilencesOnlyItself(t *testing.T) {
	_, warns := Production(envMap(map[string]string{"APP_ENV": "production", "LOGIN_LOCKOUT_ENABLED": "true"}), RateLimiting, LoginLockout)
	if len(warns) != 1 || !strings.Contains(warns[0], "RATE_LIMIT_ENABLED") {
		t.Fatalf("want only the per-IP warning left, got %v", warns)
	}
}

// The contract's secret is checked only where core has a suite to dial - CoreSessionKeys refuses it as a copy of another key
func TestSignalsCallerSecret(t *testing.T) {
	strong := strings.Repeat("s", 40)
	for name, tc := range map[string]struct {
		env  map[string]string
		want string // substring of the one error expected, empty for none
	}{
		"enabled, unaddressed": {map[string]string{"SIGNALS_SERVICE_SECRET": strong}, "SIGNALS_GRPC_ADDR is empty"},
		"off, unaddressed":     {map[string]string{"SIGNALS_ENABLED": "false"}, ""},
		"switched off":         {map[string]string{"SIGNALS_GRPC_ADDR": "signals:8092", "SIGNALS_ENABLED": "false"}, ""},
		"missing":              {map[string]string{"SIGNALS_GRPC_ADDR": "signals:8092"}, "must be a strong secret"},
		"short":                {map[string]string{"SIGNALS_GRPC_ADDR": "signals:8092", "SIGNALS_SERVICE_SECRET": "short"}, "at least 32"},
		"strong":               {map[string]string{"SIGNALS_GRPC_ADDR": "signals:8092", "SIGNALS_SERVICE_SECRET": strong}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			errs, _ := SignalsCaller(envMap(tc.env))
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatalf("errs = %v, want none", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], tc.want) {
				t.Fatalf("errs = %v, want one containing %q", errs, tc.want)
			}
		})
	}
}

// The suite seals destination credentials under its own key - missing, short or shared with core's config key all refuse
func TestSignalsEncryptionKey(t *testing.T) {
	other := base64.StdEncoding.EncodeToString(bytes32('o'))
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"missing":     {map[string]string{}, "must be set"},
		"short":       {map[string]string{"SIGNALS_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 16))}, "32 bytes"},
		"core's key":  {map[string]string{"SIGNALS_ENCRYPTION_KEY": validEncKey, "CONFIG_ENCRYPTION_KEY": validEncKey}, "must differ"},
		"its own key": {map[string]string{"SIGNALS_ENCRYPTION_KEY": other, "CONFIG_ENCRYPTION_KEY": validEncKey}, ""},
		"no core key": {map[string]string{"SIGNALS_ENCRYPTION_KEY": other}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.env["APP_ENV"] = "production"
			errs, _ := Production(envMap(tc.env), SignalsEncryptionKey)
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatalf("refused a valid key: %v", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], tc.want) {
				t.Fatalf("errs = %v, want one containing %q", errs, tc.want)
			}
		})
	}
}

func bytes32(c byte) []byte {
	b := make([]byte, 32)
	for i := range b {
		b[i] = c
	}
	return b
}

// The filesystem driver signs browser uploads itself, so a missing or dev key is a refusal, not a warning
func TestStorageFilesystemDriver(t *testing.T) {
	env := map[string]string{"STORAGE_DRIVER": "filesystem", "STORAGE_DIR": "/data", "STORAGE_PUBLIC_URL": "https://api.shop", "STORAGE_SIGNING_KEY": strings.Repeat("s", 40)}
	if errs, warns := Storage(envMap(env)); len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("a complete filesystem config: errs=%v warns=%v", errs, warns)
	}
	for name, bad := range map[string]string{"STORAGE_SIGNING_KEY": "", "STORAGE_DIR": ""} {
		e := maps.Clone(env)
		e[name] = bad
		if errs, _ := Storage(envMap(e)); len(errs) == 0 {
			t.Errorf("empty %s passed", name)
		}
	}
	e := maps.Clone(env)
	e["STORAGE_SIGNING_KEY"] = "skelet-dev-storage-signing-key-not-for-prod"
	if errs, _ := Storage(envMap(e)); len(errs) == 0 {
		t.Error("the development signing key passed")
	}
	delete(env, "STORAGE_PUBLIC_URL")
	if _, warns := Storage(envMap(env)); len(warns) != 1 {
		t.Errorf("no STORAGE_PUBLIC_URL warns once, got %v", warns)
	}
}

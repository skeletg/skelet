package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testAdminSecret = "test-admin-secret-please-ignore-0123"

func testAdminKeys(t *testing.T, previous ...string) AdminKeys {
	t.Helper()
	k, err := NewAdminKeys(testAdminSecret, previous...)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func testCustomerSigner(t *testing.T, previousPublic ...string) CustomerSigner {
	t.Helper()
	seed, _, err := GenerateCustomerKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewCustomerSigner(seed, previousPublic...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGenerateToken_AdminRoundTrip(t *testing.T) {
	adminID, sessionID := uuid.New(), uuid.New()
	keys := testAdminKeys(t)
	tok, err := GenerateToken(adminID, sessionID, "admin@example.com", true, keys, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	claims, err := ValidateToken(tok, keys)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.AdminID != adminID || claims.SessionID != sessionID || claims.Email != "admin@example.com" || !claims.IsSuperuser {
		t.Errorf("claims = %+v", claims)
	}
}

func TestGenerateCustomerToken_RoundTrip(t *testing.T) {
	customerID, channelID := uuid.New(), uuid.New()
	signer := testCustomerSigner(t)
	tok, err := GenerateCustomerToken(customerID, channelID, "shopper@example.com", signer, time.Hour)
	if err != nil {
		t.Fatalf("GenerateCustomerToken: %v", err)
	}
	claims, err := ValidateCustomerToken(tok, signer.Verifier())
	if err != nil {
		t.Fatalf("ValidateCustomerToken: %v", err)
	}
	if claims.CustomerID != customerID || claims.ChannelID != channelID || claims.Email != "shopper@example.com" {
		t.Errorf("claims = %+v", claims)
	}
}

// The Signals tier holds only the public key, so it must verify a session and be unable to mint one
func TestTheSignalsTierVerifiesWithThePublicKeyAlone(t *testing.T) {
	signer := testCustomerSigner(t)
	verifier, err := NewCustomerVerifier(signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := GenerateCustomerToken(uuid.New(), uuid.New(), "s@example.com", signer, time.Hour)
	if _, err := ValidateCustomerToken(tok, verifier); err != nil {
		t.Fatalf("a public-key verifier refused core's token: %v", err)
	}
	if _, err := GenerateCustomerToken(uuid.New(), uuid.New(), "s@example.com", CustomerSigner{verifier: verifier}, time.Hour); err == nil {
		t.Fatal("a verifier-only signer minted a token")
	}
}

func TestValidateToken_WrongKeyRejected(t *testing.T) {
	tok, _ := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, testAdminKeys(t), time.Hour)
	other, _ := NewAdminKeys("a-different-admin-secret-0123456789")
	if _, err := ValidateToken(tok, other); err == nil {
		t.Fatal("a token verified under another key")
	}
	ctok, _ := GenerateCustomerToken(uuid.New(), uuid.New(), "c@d.com", testCustomerSigner(t), time.Hour)
	if _, err := ValidateCustomerToken(ctok, testCustomerSigner(t).Verifier()); err == nil {
		t.Fatal("a customer token verified under another key pair")
	}
}

func TestValidateToken_ExpiredRejected(t *testing.T) {
	keys := testAdminKeys(t)
	tok, _ := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, keys, -time.Minute) // negative TTL, so it's already expired at issue time
	if _, err := ValidateToken(tok, keys); err == nil {
		t.Fatal("expected error validating expired token, got nil")
	}
}

// The two types never cross - different algorithms, different keys and different audiences, each enough on its own
func TestValidateToken_TypesDontCross(t *testing.T) {
	keys, signer := testAdminKeys(t), testCustomerSigner(t)
	adminTok, _ := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, keys, time.Hour)
	customerTok, _ := GenerateCustomerToken(uuid.New(), uuid.New(), "c@d.com", signer, time.Hour)
	if _, err := ValidateToken(customerTok, keys); err == nil {
		t.Error("ValidateToken accepted a customer token")
	}
	if _, err := ValidateCustomerToken(adminTok, signer.Verifier()); err == nil {
		t.Error("ValidateCustomerToken accepted an admin token")
	}
}

// A deploy that reused one HMAC value for everything must still not turn a customer-audience token into an admin one
func TestTheAudienceIsCheckedEvenUnderTheRightKey(t *testing.T) {
	keys := testAdminKeys(t)
	adminID := uuid.New()
	forged := jwtClaims{AdminID: adminID, Type: "admin", IsSuperuser: true, Subject: adminID.String(), Audience: jwt.ClaimStrings{audienceCustomer}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	tok, err := signWith(jwt.SigningMethodHS256, keys.id, keys.secret, forged)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateToken(tok, keys); err == nil {
		t.Fatal("an admin claim under the customer audience validated")
	}
	forged.Audience = nil
	tok, _ = signWith(jwt.SigningMethodHS256, keys.id, keys.secret, forged)
	if _, err := ValidateToken(tok, keys); err == nil {
		t.Fatal("an audience-less admin token validated")
	}
}

// A token with no "type" claim must be REJECTED as admin - an empty type falling through to admin would be a path to forge admin access if anything ever minted a typeless token
func TestValidateToken_MissingTypeRejected(t *testing.T) {
	keys := testAdminKeys(t)
	adminID := uuid.New()
	claims := jwtClaims{AdminID: adminID, Email: "no-type@example.com", Subject: adminID.String(), Audience: jwt.ClaimStrings{audienceAdmin}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	tok, err := signWith(jwt.SigningMethodHS256, keys.id, keys.secret, claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateToken(tok, keys); err == nil {
		t.Error("ValidateToken accepted a typeless token as admin")
	}
}

func TestValidateToken_MalformedRejected(t *testing.T) {
	keys := testAdminKeys(t)
	for _, tok := range []string{"", "not-a-jwt", "a.b.c", "a.b"} {
		if _, err := ValidateToken(tok, keys); err == nil {
			t.Errorf("ValidateToken(%q): expected error, got nil", tok)
		}
	}
}

// alg=none is the canonical signature-stripping attack - only the one algorithm each type is minted with is accepted
func TestValidateToken_AlgNoneRejected(t *testing.T) {
	keys := testAdminKeys(t)
	claims := jwtClaims{AdminID: uuid.New(), Type: "admin", Audience: jwt.ClaimStrings{audienceAdmin}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tok.Header["kid"] = keys.id
	unsigned, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build alg=none token: %v", err)
	}
	if _, err := ValidateToken(unsigned, keys); err == nil {
		t.Fatal("ValidateToken accepted an alg=none token")
	}
}

// The classic confusion: a verifier handed an HS256 token whose "secret" is the public key, which everyone in the Signals tier knows
func TestACustomerTokenHMACedWithThePublicKeyIsRefused(t *testing.T) {
	signer := testCustomerSigner(t)
	pub, _ := base64.StdEncoding.DecodeString(signer.PublicKey())
	claims := jwtClaims{CustomerID: uuid.New(), Type: "customer", Audience: jwt.ClaimStrings{audienceCustomer}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	tok, err := signWith(jwt.SigningMethodHS256, signer.id, pub, claims)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCustomerToken(tok, signer.Verifier()); err == nil {
		t.Fatal("an HS256 token keyed with the public key validated as a customer session")
	}
}

// Rotation: the new key signs, and a token from the retiring one still verifies until it expires - an unlisted key never does
func TestARotatedKeyKeepsItsTokensVerifying(t *testing.T) {
	old, _ := NewAdminKeys("the-old-admin-secret-0123456789abcd")
	oldTok, _ := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, old, time.Hour)
	rotated := testAdminKeys(t, "the-old-admin-secret-0123456789abcd")
	if _, err := ValidateToken(oldTok, rotated); err != nil {
		t.Errorf("a token from the previous admin key stopped verifying: %v", err)
	}
	if _, err := ValidateToken(oldTok, testAdminKeys(t)); err == nil {
		t.Error("a token from a key no longer listed still verified")
	}

	oldSigner := testCustomerSigner(t)
	oldCustomer, _ := GenerateCustomerToken(uuid.New(), uuid.New(), "c@d.com", oldSigner, time.Hour)
	newSigner := testCustomerSigner(t, oldSigner.PublicKey())
	if _, err := ValidateCustomerToken(oldCustomer, newSigner.Verifier()); err != nil {
		t.Errorf("a session from the previous customer key stopped verifying: %v", err)
	}
	fresh, _ := GenerateCustomerToken(uuid.New(), uuid.New(), "c@d.com", newSigner, time.Hour)
	if _, err := ValidateCustomerToken(fresh, oldSigner.Verifier()); err == nil {
		t.Error("the old verifier accepted a token from the new key")
	}
}

func TestKeysRefuseBadMaterial(t *testing.T) {
	if _, err := NewAdminKeys("  "); err == nil {
		t.Error("an empty admin secret was accepted")
	}
	if _, err := NewCustomerSigner("not-base64"); err == nil {
		t.Error("a malformed customer seed was accepted")
	}
	if _, err := NewCustomerVerifier(base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1))); err == nil {
		t.Error("a short public key was accepted")
	}
	if _, err := NewCustomerVerifier(); err == nil {
		t.Error("a verifier with no keys was accepted")
	}
}

// Each surface's middleware recognises only its own token type
func TestEachSurfaceReadsOnlyItsOwnSessions(t *testing.T) {
	keys, signer := testAdminKeys(t), testCustomerSigner(t)
	adminTok, _ := GenerateToken(uuid.New(), uuid.New(), "a@b.com", true, keys, time.Hour)
	customerTok, _ := GenerateCustomerToken(uuid.New(), uuid.New(), "c@d.com", signer, time.Hour)
	see := func(mw func(http.Handler) http.Handler, tok string) (admin, customer bool) {
		h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			_, admin = AdminFromContext(r.Context())
			_, customer = CustomerFromContext(r.Context())
		}))
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		h.ServeHTTP(httptest.NewRecorder(), req)
		return admin, customer
	}
	if a, c := see(AdminMiddleware(keys), adminTok); !a || c {
		t.Errorf("admin surface with an admin token: admin %v customer %v", a, c)
	}
	if a, c := see(AdminMiddleware(keys), customerTok); a || c {
		t.Errorf("admin surface with a customer token: admin %v customer %v, want anonymous", a, c)
	}
	if a, c := see(CustomerMiddleware(signer.Verifier()), customerTok); a || !c {
		t.Errorf("storefront with a customer token: admin %v customer %v", a, c)
	}
	if a, c := see(CustomerMiddleware(signer.Verifier()), adminTok); a || c {
		t.Errorf("storefront with an admin token: admin %v customer %v, want anonymous", a, c)
	}
}

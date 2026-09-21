package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "test-secret-please-ignore"

func TestGenerateToken_AdminRoundTrip(t *testing.T) {
	adminID := uuid.New()
	sessionID := uuid.New()

	tok, err := GenerateToken(adminID, sessionID, "admin@example.com", true, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ValidateToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.AdminID != adminID {
		t.Errorf("AdminID: got %s, want %s", claims.AdminID, adminID)
	}
	if claims.SessionID != sessionID {
		t.Errorf("SessionID: got %s, want %s", claims.SessionID, sessionID)
	}
	if claims.Email != "admin@example.com" {
		t.Errorf("Email: got %q, want %q", claims.Email, "admin@example.com")
	}
	if !claims.IsSuperuser {
		t.Error("IsSuperuser: got false, want true")
	}
}

func TestGenerateCustomerToken_RoundTrip(t *testing.T) {
	customerID := uuid.New()
	channelID := uuid.New()

	tok, err := GenerateCustomerToken(customerID, channelID, "shopper@example.com", testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateCustomerToken: %v", err)
	}

	claims, err := ValidateCustomerToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ValidateCustomerToken: %v", err)
	}
	if claims.CustomerID != customerID {
		t.Errorf("CustomerID: got %s, want %s", claims.CustomerID, customerID)
	}
	if claims.ChannelID != channelID {
		t.Errorf("ChannelID: got %s, want %s", claims.ChannelID, channelID)
	}
	if claims.Email != "shopper@example.com" {
		t.Errorf("Email: got %q, want %q", claims.Email, "shopper@example.com")
	}
}

func TestValidateToken_WrongSecretRejected(t *testing.T) {
	tok, err := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := ValidateToken(tok, "a-different-secret"); err == nil {
		t.Fatal("expected error validating with wrong secret, got nil")
	}
}

func TestValidateToken_ExpiredRejected(t *testing.T) {
	tok, err := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, testSecret, -time.Minute) // negative TTL, so it's already expired at issue time
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if _, err := ValidateToken(tok, testSecret); err == nil {
		t.Fatal("expected error validating expired token, got nil")
	}
}

func TestValidateToken_TypeMismatch(t *testing.T) {
	adminTok, err := GenerateToken(uuid.New(), uuid.New(), "a@b.com", false, testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	customerTok, err := GenerateCustomerToken(uuid.New(), uuid.New(), "c@d.com", testSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateCustomerToken: %v", err)
	}

	if _, err := ValidateToken(customerTok, testSecret); err == nil { // a customer token must not validate as an admin token
		t.Error("ValidateToken accepted a customer token; want type mismatch error")
	}
	if _, err := ValidateCustomerToken(adminTok, testSecret); err == nil { // an admin token must not validate as a customer token
		t.Error("ValidateCustomerToken accepted an admin token; want type mismatch error")
	}
}

// A token with no "type" claim must be REJECTED as admin - an empty type falling through to admin would be a path to forge admin access if anything ever minted a typeless token
func TestValidateToken_MissingTypeRejected(t *testing.T) {
	adminID := uuid.New()
	claims := jwtClaims{
		AdminID:   adminID,
		Email:     "no-type@example.com", // Type deliberately left empty
		Subject:   adminID.String(),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok, err := sign(claims, testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	if _, err := ValidateToken(tok, testSecret); err == nil {
		t.Error("ValidateToken accepted a typeless token as admin; want type mismatch error")
	}
	if _, err := ValidateCustomerToken(tok, testSecret); err == nil {
		t.Error("ValidateCustomerToken accepted a typeless token; want type mismatch error")
	}
}

func TestValidateToken_MalformedRejected(t *testing.T) {
	for _, tok := range []string{"", "not-a-jwt", "a.b.c", "a.b"} {
		if _, err := ValidateToken(tok, testSecret); err == nil {
			t.Errorf("ValidateToken(%q): expected error, got nil", tok)
		}
	}
}

// alg=none is the canonical JWT signature-stripping attack, a token with its signature removed - parse must reject it because the keyfunc only accepts HMAC signing methods
func TestValidateToken_AlgNoneRejected(t *testing.T) {
	claims := jwtClaims{
		AdminID:   uuid.New(),
		Email:     "attacker@example.com",
		Type:      "admin",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	unsigned, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build alg=none token: %v", err)
	}
	if _, err := ValidateToken(unsigned, testSecret); err == nil {
		t.Fatal("ValidateToken accepted an alg=none token; signature stripping not prevented")
	}
}

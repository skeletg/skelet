package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// The audience each token type is minted for - checked on every parse, so even a deploy that reused one value for both keys can't cross a customer token over to the admin surface
const (
	audienceAdmin    = "skelet-admin"
	audienceCustomer = "skelet-customer"
)

type jwtClaims struct {
	AdminID     uuid.UUID `json:"admin_id,omitempty"`
	SessionID   uuid.UUID `json:"session_id,omitempty"`
	CustomerID  uuid.UUID `json:"customer_id,omitempty"`
	ChannelID   uuid.UUID `json:"channel_id,omitempty"`
	Email       string    `json:"email"`
	IsSuperuser bool      `json:"is_superuser,omitempty"`
	Type        string    `json:"type"` // "admin" | "customer"
	jwt.RegisteredClaims
}

func GenerateToken(adminID, sessionID uuid.UUID, email string, isSuperuser bool, keys AdminKeys, ttl time.Duration) (string, error) { // sessionID is the admin_sessions row id, so resolvers can mark the current session as isCurrent
	if !keys.configured() {
		return "", ErrNoAdminKey
	}
	claims := jwtClaims{
		AdminID:     adminID,
		SessionID:   sessionID,
		Email:       email,
		IsSuperuser: isSuperuser,
		Type:        "admin",
		Subject:     adminID.String(),
		Audience:    jwt.ClaimStrings{audienceAdmin},
		IssuedAt:    jwt.NewNumericDate(time.Now()),
		ExpiresAt:   jwt.NewNumericDate(time.Now().Add(ttl)),
	}
	return signWith(jwt.SigningMethodHS256, keys.id, keys.secret, claims)
}

func GenerateCustomerToken(customerID, channelID uuid.UUID, email string, signer CustomerSigner, ttl time.Duration) (string, error) { // channelID is the store the customer logged into - guard.go matches it against the resolved store, so a wrong-store token reads as signed-out
	if signer.priv == nil {
		return "", ErrNoCustomerKey
	}
	claims := jwtClaims{
		CustomerID: customerID,
		ChannelID:  channelID,
		Email:      email,
		Type:       "customer",
		Subject:    customerID.String(),
		Audience:   jwt.ClaimStrings{audienceCustomer},
		IssuedAt:   jwt.NewNumericDate(time.Now()),
		ExpiresAt:  jwt.NewNumericDate(time.Now().Add(ttl)),
	}
	return signWith(jwt.SigningMethodEdDSA, signer.id, signer.priv, claims)
}

func ValidateToken(tokenStr string, keys AdminKeys) (*Claims, error) {
	c, err := parseAdmin(tokenStr, keys)
	if err != nil {
		return nil, err
	}
	return &Claims{AdminID: c.AdminID, SessionID: c.SessionID, Email: c.Email, IsSuperuser: c.IsSuperuser}, nil
}

func ValidateCustomerToken(tokenStr string, v CustomerVerifier) (*CustomerClaims, error) {
	c, err := parseCustomer(tokenStr, v)
	if err != nil {
		return nil, err
	}
	return &CustomerClaims{CustomerID: c.CustomerID, ChannelID: c.ChannelID, Email: c.Email}, nil
}

func signWith(method jwt.SigningMethod, kid string, key any, claims jwtClaims) (string, error) {
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

var errUnknownKey = errors.New("token signed by an unknown key")

// parseAdmin accepts only HS256 tokens for the admin audience, signed by a key in the admin set - an explicit admin type is required too, since a type-less token used to fall through to admin
func parseAdmin(tokenStr string, keys AdminKeys) (*jwtClaims, error) {
	c, err := parseWith(tokenStr, audienceAdmin, jwt.SigningMethodHS256.Alg(), func(kid string) (any, bool) {
		k, ok := keys.verify[kid]
		return k, ok
	})
	if err != nil {
		return nil, err
	}
	if c.Type != "admin" {
		return nil, fmt.Errorf("token type mismatch")
	}
	return c, nil
}

func parseCustomer(tokenStr string, v CustomerVerifier) (*jwtClaims, error) {
	c, err := parseWith(tokenStr, audienceCustomer, jwt.SigningMethodEdDSA.Alg(), func(kid string) (any, bool) {
		k, ok := v.keys[kid]
		return k, ok
	})
	if err != nil {
		return nil, err
	}
	if c.Type != "customer" {
		return nil, fmt.Errorf("token type mismatch")
	}
	return c, nil
}

func parseWith(tokenStr, audience, alg string, key func(kid string) (any, bool)) (*jwtClaims, error) {
	var c jwtClaims
	_, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		k, ok := key(kid)
		if !ok {
			return nil, errUnknownKey
		}
		return k, nil
	}, jwt.WithValidMethods([]string{alg}), jwt.WithAudience(audience), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}
	return &c, nil
}

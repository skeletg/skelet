package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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

func GenerateToken(adminID, sessionID uuid.UUID, email string, isSuperuser bool, secret string, ttl time.Duration) (string, error) { // sessionID is the admin_sessions row id, so resolvers can mark the current session as isCurrent
	claims := jwtClaims{
		AdminID:     adminID,
		SessionID:   sessionID,
		Email:       email,
		IsSuperuser: isSuperuser,
		Type:        "admin",
		Subject:     adminID.String(),
		IssuedAt:    jwt.NewNumericDate(time.Now()),
		ExpiresAt:   jwt.NewNumericDate(time.Now().Add(ttl)),
	}
	return sign(claims, secret)
}

func GenerateCustomerToken(customerID, channelID uuid.UUID, email, secret string, ttl time.Duration) (string, error) { // channelID is the store the customer logged into - guard.go matches it against the resolved store, so a wrong-store token reads as signed-out
	claims := jwtClaims{
		CustomerID: customerID,
		ChannelID:  channelID,
		Email:      email,
		Type:       "customer",
		Subject:    customerID.String(),
		IssuedAt:   jwt.NewNumericDate(time.Now()),
		ExpiresAt:  jwt.NewNumericDate(time.Now().Add(ttl)),
	}
	return sign(claims, secret)
}

func ValidateToken(tokenStr, secret string) (*Claims, error) {
	c, err := parse(tokenStr, secret)
	if err != nil {
		return nil, err
	}
	if c.Type != "admin" { // an explicit type is required - a type-less token would fall through to admin here, a forge-admin path, and every minted token sets Type anyway
		return nil, fmt.Errorf("token type mismatch")
	}
	return &Claims{
		AdminID:     c.AdminID,
		SessionID:   c.SessionID,
		Email:       c.Email,
		IsSuperuser: c.IsSuperuser,
	}, nil
}

func ValidateCustomerToken(tokenStr, secret string) (*CustomerClaims, error) {
	c, err := parse(tokenStr, secret)
	if err != nil {
		return nil, err
	}
	if c.Type != "customer" {
		return nil, fmt.Errorf("token type mismatch")
	}
	return &CustomerClaims{
		CustomerID: c.CustomerID,
		ChannelID:  c.ChannelID,
		Email:      c.Email,
	}, nil
}

func sign(claims jwtClaims, secret string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func parse(tokenStr, secret string) (*jwtClaims, error) {
	var c jwtClaims
	_, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("validate token: %w", err)
	}
	return &c, nil
}

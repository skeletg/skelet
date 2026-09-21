package auth

import (
	"fmt"
	"slices"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// A token for another skelet service acting on an admin's behalf - it carries the caller's grants so that service authorizes without the commerce database
// Scoped to one audience and one slug set, so a leaked token authorizes that service's surface rather than the whole admin API and stays small enough to ride every request

// The wire shape of a grant - short keys because the whole set is serialized into every request's token
type wireGrant struct {
	S string     `json:"s"`
	C *uuid.UUID `json:"c,omitempty"`
	L *string    `json:"l,omitempty"`
}

type serviceJWTClaims struct {
	AdminID     uuid.UUID   `json:"admin_id"`
	SessionID   uuid.UUID   `json:"session_id,omitempty"`
	Email       string      `json:"email"`
	IsSuperuser bool        `json:"is_superuser,omitempty"`
	Grants      []wireGrant `json:"grants,omitempty"`
	System      bool        `json:"sys,omitempty"`
	CustomerID  uuid.UUID   `json:"cust,omitempty"`
	Type        string      `json:"type"`
	jwt.RegisteredClaims
}

// ServiceClaims is everything the receiving service knows about the caller
type ServiceClaims struct {
	AdminID     uuid.UUID
	SessionID   uuid.UUID
	Email       string
	IsSuperuser bool
	Grants      []Grant // empty for a superuser, who bypasses the checks before any grant is read
	// System marks core's own process rather than an admin it acts for - it carries no grants, so it
	// passes no permission check, and only the methods that ask for it by name accept it
	System bool
	// CustomerID names the one shopper a storefront request is asking about - it authorizes reads
	// about that customer and nobody else, which is all a cart or a gated CMS block needs
	CustomerID uuid.UUID
}

// Admin returns the identity half, the shape the request guards already work in
func (c *ServiceClaims) Admin() *Claims {
	return &Claims{AdminID: c.AdminID, SessionID: c.SessionID, Email: c.Email, IsSuperuser: c.IsSuperuser}
}

// Checker answers the permission interfaces from the carried grants
func (c *ServiceClaims) Checker() *GrantChecker {
	return NewGrantChecker(c.AdminID, NewGrantSet(c.Grants))
}

// FilterGrants keeps only what an audience actually checks, expanded through the implication rule so a manage grant still answers its view check
func FilterGrants(grants []Grant, slugs []string) []Grant {
	keep := map[string]struct{}{}
	for _, slug := range slugs {
		for _, effective := range EffectiveSlugs(slug) {
			keep[effective] = struct{}{}
		}
	}
	out := make([]Grant, 0, len(grants))
	for _, g := range grants {
		if _, ok := keep[g.Slug]; ok {
			out = append(out, g)
		}
	}
	return out
}

func GenerateServiceToken(c ServiceClaims, audience, secret string, ttl time.Duration) (string, error) {
	if audience == "" {
		return "", fmt.Errorf("service token needs an audience") // an audience-less token would validate at every service
	}
	if c.System && (c.IsSuperuser || len(c.Grants) > 0) {
		return "", fmt.Errorf("a system token carries no authority of its own") // it would otherwise open the read surface too
	}
	if c.CustomerID != uuid.Nil && (c.IsSuperuser || len(c.Grants) > 0 || c.System) {
		return "", fmt.Errorf("a customer token carries no authority beyond that customer")
	}
	wire := make([]wireGrant, len(c.Grants))
	for i, g := range c.Grants {
		wire[i] = wireGrant{S: g.Slug, C: g.ChannelID, L: g.Locale}
	}
	claims := serviceJWTClaims{
		AdminID: c.AdminID, SessionID: c.SessionID, Email: c.Email, IsSuperuser: c.IsSuperuser,
		Grants:     wire,
		System:     c.System,
		CustomerID: c.CustomerID,
		Type:       "service",
		Subject:    c.AdminID.String(),
		Audience:   jwt.ClaimStrings{audience},
		IssuedAt:   jwt.NewNumericDate(time.Now()),
		ExpiresAt:  jwt.NewNumericDate(time.Now().Add(ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign service token: %w", err)
	}
	return signed, nil
}

// ValidateServiceToken refuses anything not minted for this audience, so one service's token is not a credential at another
func ValidateServiceToken(tokenStr, audience, secret string) (*ServiceClaims, error) {
	var c serviceJWTClaims
	_, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("validate service token: %w", err)
	}
	if c.Type != "service" { // an admin or customer token must never authorize here, and vice versa
		return nil, fmt.Errorf("token type mismatch")
	}
	if audience == "" || !slices.Contains(c.Audience, audience) {
		return nil, fmt.Errorf("token audience mismatch")
	}
	grants := make([]Grant, len(c.Grants))
	for i, g := range c.Grants {
		grants[i] = Grant{Slug: g.S, ChannelID: g.C, Locale: g.L}
	}
	return &ServiceClaims{
		AdminID: c.AdminID, SessionID: c.SessionID, Email: c.Email, IsSuperuser: c.IsSuperuser,
		Grants: grants, System: c.System, CustomerID: c.CustomerID,
	}, nil
}

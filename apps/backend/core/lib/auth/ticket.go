package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// A link an admin's browser opens to save one file straight to disk. It stands in for the bearer a navigation can't carry -
// for that path alone, briefly, on the admin's own session. What they may see is still decided by the route
func GenerateDownloadTicket(c *Claims, path string, keys AdminKeys, ttl time.Duration) (string, error) {
	if !keys.configured() {
		return "", ErrNoAdminKey
	}
	return signWith(jwt.SigningMethodHS256, keys.id, keys.secret, jwtClaims{
		AdminID:     c.AdminID,
		SessionID:   c.SessionID,
		Email:       c.Email,
		IsSuperuser: c.IsSuperuser,
		Type:        "download",
		Path:        path,
		Subject:     c.AdminID.String(),
		Audience:    jwt.ClaimStrings{audienceDownload},
		IssuedAt:    jwt.NewNumericDate(time.Now()),
		ExpiresAt:   jwt.NewNumericDate(time.Now().Add(ttl)),
	})
}

// Lets a GET with no bearer sign in with a ticket minted for its own path - only on the routes it wraps, and before the
// session guard, which then checks the session the ticket was minted on
func DownloadTicket(keys AdminKeys) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, signedIn := AdminFromContext(r.Context()); !signedIn && r.Method == http.MethodGet {
				if ticket := r.URL.Query().Get("ticket"); ticket != "" {
					if c, err := parseDownloadTicket(ticket, keys); err == nil && c.Path == r.URL.Path {
						r = r.WithContext(WithAdmin(r.Context(), &Claims{AdminID: c.AdminID, SessionID: c.SessionID, Email: c.Email, IsSuperuser: c.IsSuperuser}))
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseDownloadTicket(tokenStr string, keys AdminKeys) (*jwtClaims, error) {
	c, err := parseWith(tokenStr, audienceDownload, jwt.SigningMethodHS256.Alg(), func(kid string) (any, bool) {
		k, ok := keys.verify[kid]
		return k, ok
	})
	if err != nil {
		return nil, err
	}
	if c.Type != "download" || c.Path == "" {
		return nil, fmt.Errorf("not a download ticket")
	}
	return c, nil
}

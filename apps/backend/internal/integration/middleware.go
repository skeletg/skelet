package integration

import (
	"net/http"
	"strings"

	"github.com/skeletg/skelet/internal/auth"
)

// The header carrying the integration's raw token on admin API requests - named for consumer familiarity and stability, though the domain-level name is "integration", not "api token"
const HeaderAPIToken = "X-Api-Token"

// Reads the API token from the X-Api-Token header and resolves it to an integration, injecting auth.ApiTokenClaims for the permission guards to authorize against
// Non-blocking like the JWT middleware - a missing or invalid token reads as anonymous and each resolver's own gate refuses
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get(HeaderAPIToken)
			if !strings.HasPrefix(raw, "sk_") {
				next.ServeHTTP(w, r)
				return
			}
			if i, err := svc.Validate(r.Context(), raw); err == nil {
				perms := make(map[string]struct{}, len(i.Permissions))
				for _, p := range i.Permissions {
					perms[p.Slug] = struct{}{}
				}
				r = r.WithContext(auth.WithApiToken(r.Context(), &auth.ApiTokenClaims{
					TokenID:     i.ID,
					Name:        i.Name,
					Permissions: perms,
					ChannelID:   i.ChannelID,
				}))
			}
			next.ServeHTTP(w, r)
		})
	}
}

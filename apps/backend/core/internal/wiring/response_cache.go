package wiring

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/graphqlx"
	"github.com/skeletg/skelet/apps/backend/core/internal/locale"
	cachepkg "github.com/skeletg/skelet/apps/backend/core/lib/cache"
)

// Allowlist for ANONYMOUS requests - strictly catalog/content reads, nothing customer-, cart- or session-scoped, and mutations never reach this path
var frontCacheableOps = map[string]bool{
	"Product": true, "ProductWithReviews": true, "ProductsByIds": true,
	"BrowseProducts": true, "SearchProducts": true, "ProductRecommendations": true,
	"ProductQuestions": true, "ProductSortOptions": true,
	"Category": true, "Collection": true, "Collections": true,
	"Menus": true, "Header": true, "HomePage": true, "Route": true,
	"CurrentChannel": true, "PublicConfig": true, "StoreFeatures": true, "ConsentDefinitions": true,
	"Countries": true, "CountrySettings": true, "Regions": true, "PhoneSettings": true,
	"BlogPost": true, "BlogPostById": true, "BlogPosts": true, "SearchBlogPosts": true,
	"CmsPageById": true, "SynapseAttributes": true,
	"SitemapCounts": true, "SitemapEntries": true,
	"PresentmentCurrencies": true,
	"BlogPostsForSitemap":   true, "CmsPagesForSitemap": true,
	"PaymentMethods": true,
}

type cachedGraphQLResponse struct {
	Body []byte `json:"body"`
}

type respCapture struct { // buffers the downstream response so a successful, error-free payload can be stored after it is sent
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (rc *respCapture) WriteHeader(code int) {
	rc.status = code
	rc.ResponseWriter.WriteHeader(code)
}

func (rc *respCapture) Write(b []byte) (int, error) {
	rc.buf.Write(b)
	return rc.ResponseWriter.Write(b)
}

// Keys on the RESOLVED channel and clamped locale, never raw headers - every alias of one channel must share one entry, so this requires channel + locale middleware
func frontRespCacheKey(r *http.Request, opName string, body []byte) string {
	chID := "none"
	if sv, ok := channel.FromContext(r.Context()); ok {
		chID = sv.ID.String()
	}
	return fmt.Sprintf("frontresp:%s:%s:%s:%x",
		chID, locale.FromContext(r.Context()), opName, sha256.Sum256(body))
}

// Only clean 200s without GraphQL errors - a scrubbed internal error must not go sticky for a minute; the substring probe over-matches a data field named errors, which fails safe
func storableFrontResponse(status int, body []byte) bool {
	return status == http.StatusOK && !bytes.Contains(body, []byte(`"errors"`))
}

// Keyed on store + locale + operation + exact body hash, so any variable change is a distinct entry - short TTL keeps admin edits fresh without tag plumbing (ISR-level staleness, not truth) and fails open
func frontResponseCache(c *cachepkg.Cache, ttl time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c == nil || r.Method != http.MethodPost || r.Header.Get("Authorization") != "" || r.Header.Get("X-Review-Voter-Token") != "" { // Anything carrying credentials or a voter identity is personal - bypass
				next.ServeHTTP(w, r)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var env struct {
				OperationName string `json:"operationName"`
			}
			_ = json.Unmarshal(body, &env)
			if !frontCacheableOps[env.OperationName] {
				next.ServeHTTP(w, r)
				return
			}
			key := frontRespCacheKey(r, env.OperationName, body)
			if hit, ok := cachepkg.GetJSON[cachedGraphQLResponse](r.Context(), c, key); ok {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "HIT")
				_, _ = w.Write(hit.Body)
				return
			}
			w.Header().Set("X-Cache", "MISS")
			rec := &respCapture{ResponseWriter: w, status: http.StatusOK}
			ctx := graphqlx.WithUncacheableFlag(r.Context()) // resolvers that degrade to an empty result rather than erroring mark the request here - a soft-failed search is a clean 200 with no "errors", so without the flag an OpenSearch blip would pin an empty catalogue for the TTL
			r = r.WithContext(ctx)
			next.ServeHTTP(rec, r)
			if storableFrontResponse(rec.status, rec.buf.Bytes()) && !graphqlx.IsUncacheable(ctx) {
				c.SetJSON(ctx, key, cachedGraphQLResponse{Body: rec.buf.Bytes()}, ttl)
			}
		})
	}
}

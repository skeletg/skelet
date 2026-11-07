package wiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/mcpserver"
	"github.com/skeletg/skelet/apps/backend/core/internal/search"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	authpkg "github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// Hosted agents reach a store from a handful of shared addresses, so the per-address cap is far wider than the admin endpoint's
const (
	frontMCPMaxInFlight = 32
	frontMCPCallTimeout = 20 * time.Second
)

// frontGraphQL runs documents against the front schema in-process, on the caller's context - its channel, locale and client address ride along, so every resolver rule applies as it does over HTTP
type frontGraphQL struct{ h http.Handler }

func (g frontGraphQL) Do(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	// The tools blend meaning into their own searches, so the listings they read stay on the words
	req, err := http.NewRequestWithContext(search.WordsOnly(ctx), http.MethodPost, "/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	w := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	g.h.ServeHTTP(w, req)
	var res struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(w.body.Bytes(), &res); err != nil {
		return fmt.Errorf("front graphql answered %d: %w", w.status, err)
	}
	if len(res.Errors) > 0 {
		return &shoptools.Error{Code: res.Errors[0].Extensions.Code, Message: res.Errors[0].Message}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(res.Data, out)
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }
func (b *bufferedResponse) WriteHeader(status int)      { b.status = status }

// The storefront MCP endpoint is anonymous - a bearer token signs nobody in, but the request still rides the context for the client address
func anonymousRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(authpkg.WithRequestCache(authpkg.WithRequest(r.Context(), r))))
	})
}

// A channel's switch that can't be read keeps agents out rather than letting them in
func storefrontTools(shop *shoptools.Service) func(context.Context, channel.Channel, string) (mcpserver.ToolSet, *mcpserver.Refusal) {
	return func(ctx context.Context, ch channel.Channel, agent string) (mcpserver.ToolSet, *mcpserver.Refusal) {
		open, err := shop.Open(ctx, ch)
		switch {
		case err != nil:
			slog.ErrorContext(ctx, "mcp: storefront switch unreadable", "channel", ch.ID, "err", err)
			return nil, &mcpserver.Refusal{Status: http.StatusServiceUnavailable, Message: "the store couldn't be reached - try again shortly"}
		case !open:
			return nil, &mcpserver.Refusal{Status: http.StatusNotFound, Message: "this store doesn't take AI shopping agents"}
		}
		return shop.AgentToolSet(ch, agent), nil
	}
}

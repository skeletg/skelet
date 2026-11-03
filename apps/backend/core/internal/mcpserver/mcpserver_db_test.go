package mcpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

func TestARealIntegrationTokenReachesTheTools(t *testing.T) {
	conn := testdb.Require(t)
	svc := integration.NewService(integration.NewRepository(db.New(conn), txrunner.New(conn)))
	f := newFixture(t, nil)
	if _, err := conn.Exec(`INSERT INTO channels (id, slug, name, currency_code) VALUES ($1, $2, 'MCP probe', 'USD')`, f.pin, "mcp-"+uuid.NewString()[:8]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, f.pin) })
	created, err := svc.Create(context.Background(), integration.CreateIntegrationInput{Name: "mcp-" + uuid.NewString()[:8], ChannelID: &f.pin})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Delete(context.Background(), created.Integration.ID) })

	url := serve(t, integration.Middleware(svc)(f.handler), func(ctx context.Context) context.Context { return ctx })
	withToken := func(tok string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set(integration.HeaderAPIToken, tok) }
	}
	if got := send(t, url, http.MethodPost, initBody, withToken(created.RawToken)); got != http.StatusOK {
		t.Fatalf("a live token pinned to the channel should get in, got %d", got)
	}
	if got := send(t, url, http.MethodPost, initBody, withToken("sk_"+uuid.NewString())); got != http.StatusUnauthorized {
		t.Fatalf("an unknown token should be refused, got %d", got)
	}
	if _, err := svc.SetActive(context.Background(), created.Integration.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := send(t, url, http.MethodPost, initBody, withToken(created.RawToken)); got != http.StatusUnauthorized {
		t.Fatalf("a deactivated token should be refused, got %d", got)
	}
}

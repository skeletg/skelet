package admin

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/aiusage"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
)

func states(rows []*model.AgenticStatus) map[string]model.AgenticState {
	out := map[string]model.AgenticState{}
	for _, r := range rows {
		out[r.Key] = r.State
	}
	return out
}

func TestTheOverviewSaysWhatEachPieceStillNeeds(t *testing.T) {
	conn := testdb.Require(t)
	ctx := context.Background()
	q := db.New(conn)
	cfg := config.NewService(configstore.New(q))
	var id uuid.UUID
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code) VALUES ($1, 'agentic', 'USD') RETURNING id`, "agentic-"+uuid.NewString()[:8]).Scan(&id); err != nil {
		t.Fatal(err)
	}
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
	for path, v := range map[string]string{shoptools.AssistantPath: "true", shoptools.TranscriptsPath: "true"} {
		if _, err := cfg.Set(ctx, path, v, scope); err != nil {
			t.Fatal(err)
		}
	}
	r := &Resolver{ConfigSvc: cfg, AIUsageSvc: aiusage.New(q, cfg), MCP: MCPEndpoints{Storefront: true}}

	bare, err := r.agenticStatus(ctx, channel.Channel{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.AgenticState{
		"provider": model.AgenticStateOff, "assistant": model.AgenticStateAttention, "voice": model.AgenticStateOff, "transcripts": model.AgenticStateAttention,
		"agents": model.AgenticStateAttention, "pricing": model.AgenticStateOff,
	}
	if got := states(bare); len(got) != len(want) {
		t.Fatalf("rows %v", got)
	} else {
		for k, s := range want {
			if got[k] != s {
				t.Errorf("%s = %s, want %s - switched on without what it needs should ask for attention", k, got[k], s)
			}
		}
	}

	t.Setenv("AI_API_KEY", "k")
	if _, err := cfg.Set(ctx, "ai/llm/enabled", "true", config.DefaultScope); err != nil {
		t.Fatal(err)
	}
	r.AISvc, r.SignalsEnabled = ai.NewService(cfg), true
	for path, v := range map[string]string{shoptools.VoiceNamePath: "Ava", shoptools.VoiceInstructionsPath: "Jeans run small."} {
		if _, err := cfg.Set(ctx, path, v, scope); err != nil {
			t.Fatal(err)
		}
	}
	ready, err := r.agenticStatus(ctx, channel.Channel{ID: id, PublicURL: "https://shop.example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range ready {
		if row.Key != "pricing" && row.State != model.AgenticStateOn {
			t.Errorf("%s = %s (%s), want on once everything is in place", row.Key, row.State, row.Detail)
		}
		if row.Key == "agents" && row.Detail != "Any AI agent can shop at https://shop.example/api/mcp" {
			t.Errorf("agents detail = %q", row.Detail)
		}
		if row.Key == "voice" && row.Detail != "Called Ava, friendly and warm, with 16 characters of instructions" {
			t.Errorf("voice detail = %q", row.Detail)
		}
		if row.Key == "assistant" && row.Detail != "On the storefront, answering up to 500 messages a day" {
			t.Errorf("assistant detail = %q", row.Detail)
		}
	}
}

func TestThousandsGroupsDigits(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 100022: "100,022", 1234567: "1,234,567", -4200: "-4,200"} {
		if got := thousands(n); got != want {
			t.Errorf("thousands(%d) = %q, want %q", n, got, want)
		}
	}
}

package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// stubCfg is a minimal ConfigReader for tests
type stubCfg struct {
	strs  map[string]string
	bools map[string]bool
}

func (s stubCfg) GetString(_ context.Context, path string, _ config.Scope) (string, error) {
	return s.strs[path], nil
}
func (s stubCfg) GetBool(_ context.Context, path string, _ config.Scope) (bool, error) {
	return s.bools[path], nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// captureServer records request bodies and replies with the queued responses
type captureServer struct {
	mu        sync.Mutex
	bodies    []map[string]any
	responses []string
}

func (c *captureServer) handler(t *testing.T, wantPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("unexpected path %s (want %s)", r.URL.Path, wantPath)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		c.mu.Lock()
		n := len(c.bodies)
		c.bodies = append(c.bodies, body)
		resp := c.responses[n]
		c.mu.Unlock()
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(resp))
	}
}

func TestChatWithToolsAnthropic(t *testing.T) {
	cap := &captureServer{responses: []string{
		// round 1: model asks to call a tool
		`{"content":[{"type":"text","text":"let me check"},{"type":"tool_use","id":"tu_1","name":"get_sales_summary","input":{"window_days":7}}]}`,
		// round 2: final text answer
		`{"content":[{"type":"text","text":"Sales look healthy."}]}`,
	}}
	srv := httptest.NewServer(cap.handler(t, "/v1/messages"))
	defer srv.Close()

	// The SSRF guard blocks loopback targets outside development; a local
	// stub server is the local-LLM case the dev path exists for
	t.Setenv("APP_ENV", "development")
	svc := NewService(stubCfg{
		strs:  map[string]string{cfgProvider: providerAnthropic, cfgAPIKey: "k", cfgModel: "m", cfgBaseURL: srv.URL},
		bools: map[string]bool{cfgEnabled: true, cfgTools: true},
	})
	ctx := context.Background()
	tools := []Tool{{Name: "get_sales_summary", Description: "sales", Schema: map[string]any{"type": "object"}}}

	turn, err := svc.ChatWithTools(ctx, "sys", []ChatMessage{{Role: "user", Text: "how are sales?"}}, tools, 100)
	if err != nil {
		t.Fatalf("round1: %v", err)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "get_sales_summary" {
		t.Fatalf("expected 1 tool call, got %+v", turn.ToolCalls)
	}
	if string(turn.ToolCalls[0].Input) != `{"window_days":7}` {
		t.Fatalf("bad tool args: %s", turn.ToolCalls[0].Input)
	}
	if _, ok := cap.bodies[0]["tools"]; !ok {
		t.Fatalf("request 1 missing tools")
	}

	msgs := []ChatMessage{
		{Role: "user", Text: "how are sales?"},
		{Role: "assistant", Text: turn.Text, ToolCalls: turn.ToolCalls},
		{Role: "tool", ToolResults: []ToolResult{{ID: "tu_1", Content: "8 orders"}}},
	}
	turn2, err := svc.ChatWithTools(ctx, "sys", msgs, tools, 100)
	if err != nil {
		t.Fatalf("round2: %v", err)
	}
	if turn2.Text != "Sales look healthy." {
		t.Fatalf("expected final text, got %q", turn2.Text)
	}
	got := mustJSON(cap.bodies[1]["messages"])
	for _, want := range []string{"tool_use", "tool_result", "tu_1", "8 orders"} {
		if !strings.Contains(got, want) {
			t.Fatalf("request 2 missing %q in messages: %s", want, got)
		}
	}
}

func TestChatWithToolsOpenAI(t *testing.T) {
	cap := &captureServer{responses: []string{
		`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_top_products","arguments":"{\"limit\":3}"}}]}}]}`,
		`{"choices":[{"message":{"content":"Top product is X."}}]}`,
	}}
	srv := httptest.NewServer(cap.handler(t, "/chat/completions"))
	defer srv.Close()

	t.Setenv("APP_ENV", "development")
	svc := NewService(stubCfg{
		strs:  map[string]string{cfgProvider: providerOpenAI, cfgAPIKey: "k", cfgModel: "m", cfgBaseURL: srv.URL},
		bools: map[string]bool{cfgEnabled: true, cfgTools: true},
	})
	ctx := context.Background()
	tools := []Tool{{Name: "get_top_products", Description: "top", Schema: map[string]any{"type": "object"}}}

	turn, err := svc.ChatWithTools(ctx, "sys", []ChatMessage{{Role: "user", Text: "best sellers?"}}, tools, 100)
	if err != nil {
		t.Fatalf("round1: %v", err)
	}
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Name != "get_top_products" {
		t.Fatalf("expected tool call, got %+v", turn.ToolCalls)
	}
	if string(turn.ToolCalls[0].Input) != `{"limit":3}` {
		t.Fatalf("bad args: %s", turn.ToolCalls[0].Input)
	}

	msgs := []ChatMessage{
		{Role: "user", Text: "best sellers?"},
		{Role: "assistant", ToolCalls: turn.ToolCalls},
		{Role: "tool", ToolResults: []ToolResult{{ID: "call_1", Content: "X sold 50"}}},
	}
	turn2, err := svc.ChatWithTools(ctx, "sys", msgs, tools, 100)
	if err != nil {
		t.Fatalf("round2: %v", err)
	}
	if turn2.Text != "Top product is X." {
		t.Fatalf("expected final text, got %q", turn2.Text)
	}
	got := mustJSON(cap.bodies[1]["messages"])
	for _, want := range []string{`"role":"tool"`, "call_1", "X sold 50"} {
		if !strings.Contains(got, want) {
			t.Fatalf("request 2 missing %q in messages: %s", want, got)
		}
	}
}

package ai

import (
	"context"
	"encoding/json"
	"errors"
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

func TestToolRequestsCacheWhatTheNextRoundRepeats(t *testing.T) {
	cap := &captureServer{responses: []string{
		`{"content":[{"type":"tool_use","id":"tu_1","name":"lookup","input":{}}]}`,
		`{"content":[{"type":"text","text":"done"}]}`,
	}}
	srv := httptest.NewServer(cap.handler(t, "/v1/messages"))
	defer srv.Close()
	t.Setenv("APP_ENV", "development")
	svc := NewService(stubCfg{
		strs:  map[string]string{cfgProvider: providerAnthropic, cfgAPIKey: "k", cfgModel: "m", cfgBaseURL: srv.URL},
		bools: map[string]bool{cfgEnabled: true, cfgTools: true},
	})
	calls := 0
	run := func(_ context.Context, tc []ToolCall) []ToolResult {
		calls++
		return []ToolResult{{ID: tc[0].ID, Content: "found"}}
	}
	reply, _, err := svc.Run(context.Background(), []ChatMessage{{Role: "user", Text: "find it"}}, Loop{System: "sys", Tools: []Tool{{Name: "lookup"}}, Call: run, MaxRounds: 1, MaxTokens: 100})
	if err != nil || reply != "done" || calls != 1 || len(cap.bodies) != 2 {
		t.Fatalf("reply %q, %v after %d calls and %d requests - one round of tools, then the answer", reply, err, calls, len(cap.bodies))
	}
	if !strings.Contains(mustJSON(cap.bodies[0]["system"]), `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("the prompt and tools aren't cached: %s", mustJSON(cap.bodies[0]["system"]))
	}
	for i, body := range cap.bodies {
		msgs := body["messages"].([]any)
		last := mustJSON(msgs[len(msgs)-1])
		if !strings.Contains(last, "cache_control") {
			t.Errorf("request %d leaves its newest message uncached: %s", i, last)
		}
		if earlier := mustJSON(msgs[:len(msgs)-1]); strings.Contains(earlier, "cache_control") {
			t.Errorf("request %d marks more than its newest message: %s", i, earlier)
		}
	}
	if last := mustJSON(cap.bodies[1]["messages"]); !strings.Contains(last, finalRoundNudge) {
		t.Errorf("the last round didn't ask for an answer: %s", last)
	}
	if _, _, err := svc.Run(context.Background(), nil, Loop{System: "sys", Call: run, MaxRounds: 1, MaxTokens: 100}); err == nil {
		t.Error("an empty conversation reached the provider")
	}
}

func TestRunAddsUpEveryRoundsUsage(t *testing.T) {
	for _, tc := range []struct {
		provider, path string
		responses      []string
	}{
		{providerAnthropic, "/v1/messages", []string{
			`{"content":[{"type":"tool_use","id":"tu_1","name":"lookup","input":{}}],"usage":{"input_tokens":100,"output_tokens":20,"cache_creation_input_tokens":900,"cache_read_input_tokens":0}}`,
			`{"stop_reason":"max_tokens","content":[{"type":"text","text":"cut"}],"usage":{"input_tokens":30,"output_tokens":1024,"cache_creation_input_tokens":40,"cache_read_input_tokens":900}}`,
		}},
		{providerOpenAI, "/chat/completions", []string{
			`{"choices":[{"message":{"tool_calls":[{"id":"c1","function":{"name":"lookup","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1000,"completion_tokens":20,"prompt_tokens_details":{"cached_tokens":0}}}`,
			`{"choices":[{"finish_reason":"length","message":{"content":"cut"}}],"usage":{"prompt_tokens":970,"completion_tokens":1024,"prompt_tokens_details":{"cached_tokens":900}}}`,
		}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			cap := &captureServer{responses: tc.responses}
			srv := httptest.NewServer(cap.handler(t, tc.path))
			defer srv.Close()
			t.Setenv("APP_ENV", "development")
			svc := NewService(stubCfg{
				strs:  map[string]string{cfgProvider: tc.provider, cfgAPIKey: "k", cfgModel: "m", cfgBaseURL: srv.URL},
				bools: map[string]bool{cfgEnabled: true, cfgTools: true},
			})
			run := func(_ context.Context, calls []ToolCall) []ToolResult {
				return []ToolResult{{ID: calls[0].ID, Content: "found"}}
			}
			_, used, err := svc.Run(context.Background(), []ChatMessage{{Role: "user", Text: "find it"}}, Loop{System: "sys", Tools: []Tool{{Name: "lookup"}}, Call: run, MaxRounds: 3, MaxTokens: 100})
			if !errors.Is(err, ErrTruncated) {
				t.Fatalf("err = %v, want the cut-off reply's error", err)
			}
			want := Usage{Input: 130, Output: 1044, CacheRead: 900, CacheWrite: 940}
			if tc.provider == providerOpenAI {
				want = Usage{Input: 1070, Output: 1044, CacheRead: 900}
			}
			if used != want {
				t.Fatalf("usage = %+v, want %+v - the failed round was billed too", used, want)
			}
		})
	}
}

func TestALoopCanAskForTheModelsThinking(t *testing.T) {
	cap := &captureServer{responses: []string{
		`{"content":[{"type":"thinking","thinking":"They want a mug, so search first.","signature":"s1"},{"type":"tool_use","id":"tu_1","name":"lookup","input":{"q":"mug"}}]}`,
		`{"content":[{"type":"thinking","thinking":"","signature":"s2"},{"type":"text","text":"Found one."}]}`,
	}}
	srv := httptest.NewServer(cap.handler(t, "/v1/messages"))
	defer srv.Close()
	t.Setenv("APP_ENV", "development")
	svc := NewService(stubCfg{
		strs:  map[string]string{cfgProvider: providerAnthropic, cfgAPIKey: "k", cfgModel: "claude-opus-5-5", cfgBaseURL: srv.URL},
		bools: map[string]bool{cfgEnabled: true, cfgTools: true},
	})
	var rounds []ChatTurn
	reply, _, err := svc.Run(context.Background(), []ChatMessage{{Role: "user", Text: "a mug?"}}, Loop{
		System: "sys", Tools: []Tool{{Name: "lookup"}}, MaxRounds: 3, MaxTokens: 100, Thinking: true,
		Call: func(_ context.Context, calls []ToolCall) []ToolResult {
			return []ToolResult{{ID: calls[0].ID, Content: "Big mug"}}
		},
		OnRound: func(turn ChatTurn) { rounds = append(rounds, turn) },
	})
	if err != nil || reply != "Found one." || len(rounds) != 2 {
		t.Fatalf("reply %q, %v, %d rounds", reply, err, len(rounds))
	}
	if rounds[0].Thinking != "They want a mug, so search first." || rounds[0].ToolCalls[0].Name != "lookup" || rounds[1].Thinking != "" {
		t.Errorf("rounds = %+v", rounds)
	}
	for i, body := range cap.bodies {
		if got := mustJSON(body["thinking"]); got != `{"display":"summarized","type":"adaptive"}` {
			t.Errorf("request %d thinking = %s", i, got)
		}
	}
	if !strings.Contains(mustJSON(cap.bodies[1]["messages"]), `"signature":"s1"`) {
		t.Error("the first round's thinking wasn't replayed as it came")
	}

	cap.bodies, cap.responses = nil, []string{`{"content":[{"type":"text","text":"ok"}]}`}
	if _, _, err := svc.Run(context.Background(), []ChatMessage{{Role: "user", Text: "hi"}}, Loop{System: "sys", MaxRounds: 1, MaxTokens: 100}); err != nil {
		t.Fatal(err)
	}
	if _, asked := cap.bodies[0]["thinking"]; asked {
		t.Error("a loop that didn't ask for thinking got it switched on")
	}
}

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func stubService(t *testing.T, provider, model, baseURL string) *Service {
	t.Helper()
	t.Setenv("APP_ENV", "development") // the stub server is on loopback, which the SSRF guard only admits in development
	return NewService(stubCfg{
		strs:  map[string]string{cfgProvider: provider, cfgAPIKey: "k", cfgModel: model, cfgBaseURL: baseURL},
		bools: map[string]bool{cfgEnabled: true, cfgTools: true},
	})
}

func serve(t *testing.T, path string, responses ...string) (*captureServer, string) {
	t.Helper()
	cap := &captureServer{responses: responses}
	srv := httptest.NewServer(cap.handler(t, path))
	t.Cleanup(srv.Close)
	return cap, srv.URL
}

// A reply cut off at its limit used to come back as if it were whole - a half-rewritten email, or half a JSON object in the meta description
func TestCutOffAndDeclinedRepliesAreErrors(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name, provider, path, reply string
		want                        error
	}{
		{"anthropic max_tokens", providerAnthropic, "/v1/messages", `{"stop_reason":"max_tokens","content":[{"type":"text","text":"{\"metaTitle\":\"Hal"}]}`, ErrTruncated},
		{"anthropic refusal", providerAnthropic, "/v1/messages", `{"stop_reason":"refusal","content":[]}`, ErrRefused},
		{"openai length", providerOpenAI, "/chat/completions", `{"choices":[{"finish_reason":"length","message":{"content":"{\"metaTitle\":\"Hal"}}]}`, ErrTruncated},
		{"openai content_filter", providerOpenAI, "/chat/completions", `{"choices":[{"finish_reason":"content_filter","message":{"content":""}}]}`, ErrRefused},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, url := serve(t, c.path, c.reply, c.reply, c.reply)
			svc := stubService(t, c.provider, "m", url)
			if _, err := svc.GenerateSEO(ctx, "Title", "body"); !errors.Is(err, c.want) {
				t.Errorf("GenerateSEO err = %v, want %v", err, c.want)
			}
			if _, err := svc.Chat(ctx, "sys", []ChatMessage{{Role: "user", Text: "hi"}}, 100); !errors.Is(err, c.want) {
				t.Errorf("Chat err = %v, want %v", err, c.want)
			}
			if _, err := svc.ChatWithTools(ctx, "sys", []ChatMessage{{Role: "user", Text: "hi"}}, nil, 100); !errors.Is(err, c.want) {
				t.Errorf("ChatWithTools err = %v, want %v", err, c.want)
			}
		})
	}
}

// The current models carry their reasoning between tool rounds in signed thinking blocks, which only survive if they come back unchanged
func TestThinkingBlocksAreReplayedVerbatim(t *testing.T) {
	round1 := `{"stop_reason":"tool_use","content":[{"type":"thinking","thinking":"","signature":"sig-abc"},{"type":"text","text":"checking"},{"type":"tool_use","id":"tu_1","name":"get_sales_summary","input":{"window_days":7}}]}`
	cap, url := serve(t, "/v1/messages", round1, `{"stop_reason":"end_turn","content":[{"type":"text","text":"Fine."}]}`)
	svc := stubService(t, providerAnthropic, "m", url)
	ctx := context.Background()

	msgs := []ChatMessage{{Role: "user", Text: "how are sales?"}}
	turn, err := svc.ChatWithTools(ctx, "sys", msgs, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	msgs = append(msgs, turn.AsMessage(), ChatMessage{Role: "tool", ToolResults: []ToolResult{{ID: "tu_1", Content: "8 orders"}}})
	if _, err := svc.ChatWithTools(ctx, "sys", msgs, nil, 100); err != nil {
		t.Fatal(err)
	}

	var sent struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	b, _ := json.Marshal(cap.bodies[1])
	_ = json.Unmarshal(b, &sent)
	var want, got any
	_ = json.Unmarshal([]byte(round1), &want)
	_ = json.Unmarshal(sent.Messages[1].Content, &got)
	if mustJSON(want.(map[string]any)["content"]) != mustJSON(got) {
		t.Errorf("assistant turn sent back as %s, want the reply's blocks unchanged", sent.Messages[1].Content)
	}
}

// How the chat loop tells the model to wrap up without editing the system prompt, which would invalidate earlier thinking
func TestTextOnAToolTurnFollowsTheResults(t *testing.T) {
	ctx := context.Background()
	msgs := []ChatMessage{
		{Role: "user", Text: "q"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "t1", Name: "x", Input: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolResults: []ToolResult{{ID: "t1", Content: "r"}}, Text: "answer now"},
	}

	cap, url := serve(t, "/v1/messages", `{"stop_reason":"end_turn","content":[{"type":"text","text":"ok"}]}`)
	if _, err := stubService(t, providerAnthropic, "m", url).ChatWithTools(ctx, "sys", msgs, nil, 100); err != nil {
		t.Fatal(err)
	}
	last := mustJSON(cap.bodies[0]["messages"].([]any)[2])
	if !strings.Contains(last, `"tool_result"`) || !strings.Contains(last, `{"text":"answer now","type":"text"}],"role":"user"`) {
		t.Errorf("anthropic tool turn = %s, want the results then the text", last)
	}

	cap, url = serve(t, "/chat/completions", `{"choices":[{"finish_reason":"stop","message":{"content":"ok"}}]}`)
	if _, err := stubService(t, providerOpenAI, "m", url).ChatWithTools(ctx, "sys", msgs, nil, 100); err != nil {
		t.Fatal(err)
	}
	sent := cap.bodies[0]["messages"].([]any)
	tail := mustJSON(sent[len(sent)-2:])
	if tail != `[{"content":"r","role":"tool","tool_call_id":"t1"},{"content":"answer now","role":"user"}]` {
		t.Errorf("openai tail = %s, want the tool result then a user message", tail)
	}
}

func TestAnthropicRequestFieldsFollowTheModel(t *testing.T) {
	anthropicAPI := providerCfg{apiKey: "k"}
	proxy := providerCfg{apiKey: "k", baseConfigured: true}
	for _, c := range []struct {
		name          string
		pc            providerCfg
		model, effort string
		wantMax       int
		wantEffort    bool
		wantFallbacks bool
	}{
		{"current opus on the API", anthropicAPI, "claude-opus-5-5", "low", thinkingMaxTokens, true, true},
		{"current opus behind a proxy", proxy, "claude-opus-5-5", "low", thinkingMaxTokens, true, false},
		{"bedrock-style id", proxy, "anthropic.claude-sonnet-5-5", "low", thinkingMaxTokens, true, false},
		{"sonnet 4.6 thinks only when asked", anthropicAPI, "claude-sonnet-4-6", "low", 300, true, false},
		{"haiku 4.5 rejects effort", anthropicAPI, "claude-haiku-4-5", "low", 300, false, false},
		{"no effort requested", anthropicAPI, "claude-opus-5-5", "", thinkingMaxTokens, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			payload := map[string]any{}
			headers := c.pc.anthropicRequest(payload, c.model, 300, c.effort)
			if payload["max_tokens"] != c.wantMax {
				t.Errorf("max_tokens = %v, want %d", payload["max_tokens"], c.wantMax)
			}
			if _, ok := payload["output_config"]; ok != c.wantEffort {
				t.Errorf("output_config present = %v, want %v", ok, c.wantEffort)
			}
			_, fb := payload["fallbacks"]
			if fb != c.wantFallbacks || (headers["anthropic-beta"] != "") != c.wantFallbacks {
				t.Errorf("fallbacks = %v with beta %q, want %v", fb, headers["anthropic-beta"], c.wantFallbacks)
			}
		})
	}
}

func TestOpenAIMaxTokensFieldFollowsTheEndpoint(t *testing.T) {
	if f := (providerCfg{}).openaiMaxTokensField(); f != "max_completion_tokens" {
		t.Errorf("OpenAI's own API: %s", f)
	}
	if f := (providerCfg{baseConfigured: true}).openaiMaxTokensField(); f != "max_tokens" {
		t.Errorf("a compatible server: %s", f)
	}
}

type failingCfg struct{ stubCfg }

func (failingCfg) GetBool(context.Context, string, config.Scope) (bool, error) {
	return false, errors.New("db down")
}

func TestCustomerDataSwitchFailsClosed(t *testing.T) {
	ctx := context.Background()
	if (&Service{cfg: failingCfg{}}).AllowCustomerData(ctx) {
		t.Error("a failed read let customer data through")
	}
	if (&Service{cfg: stubCfg{bools: map[string]bool{cfgAllowCustomerData: false}}}).AllowCustomerData(ctx) {
		t.Error("switched off, still allowed")
	}
	if !(&Service{cfg: stubCfg{bools: map[string]bool{cfgAllowCustomerData: true}}}).AllowCustomerData(ctx) {
		t.Error("switched on, refused")
	}
}

func TestChatReportsTheServerKillSwitch(t *testing.T) {
	svc := &Service{cfg: stubCfg{bools: map[string]bool{cfgEnabled: true}}}
	if _, err := svc.Chat(context.Background(), "sys", []ChatMessage{{Role: "user", Text: "hi"}}, 100); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
}

func TestProviderErrorBodyIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(strings.Repeat("x", 50000)))
	}))
	t.Cleanup(srv.Close)
	_, err := stubService(t, providerAnthropic, "m", srv.URL).Chat(context.Background(), "sys", []ChatMessage{{Role: "user", Text: "hi"}}, 100)
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err = %v, want the status", err)
	}
	if n := len(err.Error()); n > maxErrorSnippet+200 {
		t.Errorf("error is %d characters, want it capped near %d", n, maxErrorSnippet)
	}
}

func TestStripFences(t *testing.T) {
	for in, want := range map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n[1]\n```":           `[1]`,
		"  {\"a\":1}  ":           `{"a":1}`,
	} {
		if got := stripFences(in); got != want {
			t.Errorf("stripFences(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateSlugsKeepsOnlyKnownOnesOnce(t *testing.T) {
	allowed := []string{"spam", "off-topic"}
	got := validateSlugs([]string{" spam", "made-up", "spam", "", "off-topic"}, func(i int) string { return allowed[i] }, len(allowed))
	if strings.Join(got, ",") != "spam,off-topic" {
		t.Errorf("got %v, want [spam off-topic]", got)
	}
}

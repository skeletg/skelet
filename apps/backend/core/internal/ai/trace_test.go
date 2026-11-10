package ai

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
)

func spanAttrs(s sdktrace.ReadOnlySpan) map[attribute.Key]string {
	out := map[attribute.Key]string{}
	for _, a := range s.Attributes() {
		out[a.Key] = a.Value.String()
	}
	return out
}

func TestEachRoundAndToolCallIsTraced(t *testing.T) {
	for _, kept := range []bool{true, false} {
		cap := &captureServer{responses: []string{
			`{"content":[{"type":"tool_use","id":"tu_1","name":"lookup","input":{"q":"mugs"}}],"usage":{"input_tokens":100,"output_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}`,
			`{"content":[{"type":"text","text":"Two mugs."}],"usage":{"input_tokens":30,"output_tokens":8,"cache_creation_input_tokens":0,"cache_read_input_tokens":90}}`,
		}}
		srv := httptest.NewServer(cap.handler(t, "/v1/messages"))
		t.Setenv("APP_ENV", "development")
		svc := NewService(stubCfg{
			strs:  map[string]string{cfgProvider: providerAnthropic, cfgAPIKey: "k", cfgModel: "claude-test", cfgBaseURL: srv.URL},
			bools: map[string]bool{cfgEnabled: true, cfgTools: true},
		})
		rec := tracetest.NewSpanRecorder()
		aitrace.Install(aitrace.Fixed(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))))
		run := func(_ context.Context, calls []ToolCall) []ToolResult {
			return []ToolResult{{ID: calls[0].ID, Content: "found 2"}}
		}
		ctx := aitrace.WithContent(context.Background(), kept)
		if _, _, err := svc.Run(ctx, []ChatMessage{{Role: "user", Text: "any mugs?"}}, Loop{System: "be brief", Tools: []Tool{{Name: "lookup"}}, Call: run, MaxRounds: 3, MaxTokens: 100}); err != nil {
			t.Fatal(err)
		}
		aitrace.Install(nil)
		srv.Close()

		var kinds []string
		for _, s := range rec.Ended() {
			kinds = append(kinds, s.Name()+":"+spanAttrs(s)["langfuse.observation.type"])
		}
		if strings.Join(kinds, ",") != "Model call:generation,lookup:tool,Model call:generation" {
			t.Fatalf("kept=%v: observations %v", kept, kinds)
		}
		first, tool, second := spanAttrs(rec.Ended()[0]), spanAttrs(rec.Ended()[1]), spanAttrs(rec.Ended()[2])
		if first["gen_ai.request.model"] != "claude-test" || first["gen_ai.usage.output_tokens"] != "20" || second["gen_ai.usage.input_tokens"] != "120" {
			t.Errorf("kept=%v: model and tokens %v / %v", kept, first, second)
		}
		if kept {
			if !strings.Contains(first["langfuse.observation.input"], "be brief") || !strings.Contains(first["langfuse.observation.input"], "any mugs?") {
				t.Errorf("the model call's input: %s", first["langfuse.observation.input"])
			}
			if tool["langfuse.observation.input"] != `{"q":"mugs"}` || tool["langfuse.observation.output"] != "found 2" || !strings.Contains(second["langfuse.observation.output"], "Two mugs.") {
				t.Errorf("tool %v, answer %v", tool, second["langfuse.observation.output"])
			}
		} else {
			for _, a := range []map[attribute.Key]string{first, tool, second} {
				if a["langfuse.observation.input"] != "" || a["langfuse.observation.output"] != "" {
					t.Errorf("a conversation the store doesn't keep sent its words: %v", a)
				}
			}
		}
	}
}

func TestAPrivateToolsAnswerNeverReachesATrace(t *testing.T) {
	cap := &captureServer{responses: []string{
		`{"content":[{"type":"tool_use","id":"tu_1","name":"get_my_order","input":{"number":"1042"}}],"usage":{"input_tokens":10,"output_tokens":2}}`,
		`{"content":[{"type":"text","text":"It's on its way."}],"usage":{"input_tokens":10,"output_tokens":2}}`,
	}}
	srv := httptest.NewServer(cap.handler(t, "/v1/messages"))
	defer srv.Close()
	t.Setenv("APP_ENV", "development")
	svc := NewService(stubCfg{
		strs:  map[string]string{cfgProvider: providerAnthropic, cfgAPIKey: "k", cfgModel: "claude-test", cfgBaseURL: srv.URL},
		bools: map[string]bool{cfgEnabled: true, cfgTools: true},
	})
	rec := tracetest.NewSpanRecorder()
	aitrace.Install(aitrace.Fixed(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))))
	defer aitrace.Install(nil)
	run := func(_ context.Context, calls []ToolCall) []ToolResult {
		return []ToolResult{{ID: calls[0].ID, Content: "Order 1042, shipping to Vienna, tracking DHL 0034"}}
	}
	ctx := aitrace.WithContent(context.Background(), true)
	loop := Loop{System: "be brief", Tools: []Tool{{Name: "get_my_order"}}, Call: run, MaxRounds: 3, MaxTokens: 100, Private: map[string]bool{"get_my_order": true}}
	if _, _, err := svc.Run(ctx, []ChatMessage{{Role: "user", Text: "where's my order?"}}, loop); err != nil {
		t.Fatal(err)
	}
	for _, s := range rec.Ended() {
		for k, v := range spanAttrs(s) {
			if strings.Contains(v, "Vienna") {
				t.Errorf("%s %s carried the order: %s", s.Name(), k, v)
			}
		}
	}
}

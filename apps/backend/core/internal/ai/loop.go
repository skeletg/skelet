package ai

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
)

const finalRoundNudge = "You have gathered enough data. Answer the user's question now using the information above - do not request any more tools."

type Loop struct {
	System    string
	Tools     []Tool
	Call      func(context.Context, []ToolCall) []ToolResult
	MaxRounds int
	MaxTokens int
	Thinking  bool // readable summaries of the model's thinking, on the models that think by default
	Effort    string
	OnRound   func(ChatTurn)
	Private   map[string]bool // tools whose answers never reach a trace, only that they answered
}

const privateOutput = "(withheld from traces)"

// Run's usage covers every round that answered, a failed one's included
func (s *Service) Run(ctx context.Context, msgs []ChatMessage, l Loop) (string, Usage, error) {
	if len(msgs) == 0 {
		return "", Usage{}, errors.New("ai: a tool loop needs a message to answer")
	}
	msgs = slices.Clone(msgs)
	var used Usage
	for round := 0; ; round++ {
		last := round >= l.MaxRounds
		if last && msgs[len(msgs)-1].Role == "tool" { // rides on the pending tool results, never the system prompt - editing that mid-loop invalidates the thinking blocks replayed from earlier rounds
			msgs[len(msgs)-1].Text = finalRoundNudge
		}
		gctx, gen := aitrace.Start(ctx, "Model call", aitrace.Generation)
		gen.Model(s.ModelName(ctx))
		gen.Meta("round", round+1)
		if gen.KeepsText() {
			gen.Input(traceMessages(l.System, msgs, l.Private))
		}
		turn, err := s.chatWithTools(gctx, l.System, msgs, l.Tools, l.MaxTokens, l.Thinking, l.Effort)
		if turn.Model != "" {
			gen.Model(turn.Model)
		}
		gen.Usage(turn.Usage.Input, turn.Usage.Output, turn.Usage.CacheRead, turn.Usage.CacheWrite)
		used = used.Add(turn.Usage)
		if err != nil {
			gen.Fail(err)
			gen.End()
			return "", used, err
		}
		if gen.KeepsText() {
			gen.Output(traceTurn(turn))
		}
		gen.End()
		if l.OnRound != nil {
			l.OnRound(turn)
		}
		if len(turn.ToolCalls) == 0 || last {
			return strings.TrimSpace(turn.Text), used, nil
		}
		started := time.Now()
		results := l.Call(ctx, turn.ToolCalls)
		traceTools(ctx, started, turn.ToolCalls, results, l.Private)
		msgs = append(msgs, turn.AsMessage(), ChatMessage{Role: "tool", ToolResults: results})
	}
}

// The tools run as a batch, so each is recorded over the batch's time, with what it was asked and what it answered
func traceTools(ctx context.Context, started time.Time, calls []ToolCall, results []ToolResult, private map[string]bool) {
	ended := time.Now()
	byID := make(map[string]ToolResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}
	for _, c := range calls {
		_, obs := aitrace.StartAt(ctx, c.Name, aitrace.Tool, started)
		obs.Input(string(c.Input))
		if r, ok := byID[c.ID]; ok {
			obs.Output(traceOutput(r, private[c.Name]))
			if r.IsError {
				obs.Meta("refused", true)
			}
		}
		obs.EndAt(ended)
	}
}

type traceMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []traceToolCall  `json:"tool_calls,omitempty"`
	Results   []traceToolReply `json:"tool_results,omitempty"`
}

type traceToolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

type traceToolReply struct {
	Content string `json:"content"`
	Error   bool   `json:"error,omitempty"`
}

// The conversation the way a model call saw it, for the trace's view of the generation
func traceMessages(system string, msgs []ChatMessage, private map[string]bool) []traceMessage {
	out := make([]traceMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, traceMessage{Role: "system", Content: system})
	}
	tool := map[string]string{}
	for _, m := range msgs {
		t := traceMessage{Role: m.Role, Content: m.Text}
		for _, c := range m.ToolCalls {
			tool[c.ID] = c.Name
			t.ToolCalls = append(t.ToolCalls, traceToolCall{Name: c.Name, Input: string(c.Input)})
		}
		for _, r := range m.ToolResults {
			t.Results = append(t.Results, traceToolReply{Content: traceOutput(r, private[tool[r.ID]]), Error: r.IsError})
		}
		out = append(out, t)
	}
	return out
}

func traceOutput(r ToolResult, private bool) string {
	if private && !r.IsError {
		return privateOutput
	}
	return r.Content
}

func traceTurn(turn ChatTurn) traceMessage {
	t := traceMessage{Role: "assistant", Content: turn.Text}
	for _, c := range turn.ToolCalls {
		t.ToolCalls = append(t.ToolCalls, traceToolCall{Name: c.Name, Input: string(c.Input)})
	}
	return t
}

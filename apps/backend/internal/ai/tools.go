package ai

// Tool calling for the agentic Signals chat: instead of pre-stuffing data into the prompt, the caller offers Tools and loops - the model answers or asks for calls, the caller executes them re-checking permissions, until a final text answer - both providers do this with different wire formats, absorbed here so callers see tool calls and results, never provider JSON

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

type Tool struct {
	Name        string
	Description string
	Schema      map[string]any // JSON Schema object for the input - an empty object means "no arguments"
}

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type ToolResult struct { // fed back to the model as ChatMessage{Role:"tool", ToolResults: ...}
	ID      string
	Content string
	IsError bool
}

type ChatTurn struct { // one of the two - final assistant text with ToolCalls empty, or tool calls the caller must execute
	Text      string
	ToolCalls []ToolCall
}

// Both built-in providers support tool calling, so this is really the operator kill switch (ai/llm/tools=false) for models and endpoints that don't - a false drops the caller back to snapshot mode
func (s *Service) SupportsTools(ctx context.Context) bool {
	if s.cfg == nil {
		return false
	}
	v, err := s.cfg.GetBool(ctx, cfgTools, config.DefaultScope)
	if err != nil {
		return true // default-on, matching the config default
	}
	return v
}

// Per-customer profiles and records - defaults true, and operators turn it off to keep personal data out of the LLM provider
func (s *Service) AllowCustomerData(ctx context.Context) bool {
	if s.cfg == nil {
		return false
	}
	v, err := s.cfg.GetBool(ctx, cfgAllowCustomerData, config.DefaultScope)
	if err != nil {
		return true
	}
	return v
}

// ONE turn - to continue, the caller executes the returned calls and appends an assistant ChatMessage carrying the ToolCalls followed by a ChatMessage{Role:"tool", ToolResults: ...} before calling again, and tools=nil gives a plain completion that still reads tool history
func (s *Service) ChatWithTools(ctx context.Context, system string, messages []ChatMessage, tools []Tool, maxTokens int) (ChatTurn, error) {
	if !s.envEnabled {
		return ChatTurn{}, ErrDisabled
	}
	if !s.enabled(ctx) {
		return ChatTurn{}, ErrNotConfigured
	}
	pc := s.providerConfig(ctx)
	if !pc.hasCredentials() {
		return ChatTurn{}, ErrNotConfigured
	}
	if len(messages) == 0 {
		return ChatTurn{}, fmt.Errorf("ai: no messages")
	}
	if pc.provider == providerOpenAI {
		return s.openaiChatTools(ctx, pc, system, messages, tools, maxTokens)
	}
	return s.anthropicChatTools(ctx, pc, system, messages, tools, maxTokens)
}

// ---- Anthropic (Messages API, tool_use / tool_result blocks) ----

func (s *Service) anthropicChatTools(ctx context.Context, pc providerCfg, system string, messages []ChatMessage, tools []Tool, maxTokens int) (ChatTurn, error) {
	msgs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		switch {
		case len(m.ToolResults) > 0:
			blocks := make([]map[string]any, 0, len(m.ToolResults))
			for _, tr := range m.ToolResults {
				blocks = append(blocks, map[string]any{
					"type":        "tool_result",
					"tool_use_id": tr.ID,
					"content":     tr.Content,
					"is_error":    tr.IsError,
				})
			}
			msgs = append(msgs, map[string]any{"role": "user", "content": blocks})
		case len(m.ToolCalls) > 0:
			blocks := make([]map[string]any, 0, len(m.ToolCalls)+1)
			if strings.TrimSpace(m.Text) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Text})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Input)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": blocks})
		default:
			msgs = append(msgs, map[string]any{"role": chatRole(m.Role), "content": m.Text})
		}
	}
	payload := map[string]any{
		"model":      pc.model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   msgs,
	}
	if len(tools) > 0 {
		payload["tools"] = anthropicTools(tools)
	}
	data, err := s.post(ctx, pc.baseURL+"/v1/messages", map[string]string{
		"x-api-key":         pc.apiKey,
		"anthropic-version": anthropicVersion,
	}, payload)
	if err != nil {
		return ChatTurn{}, err
	}
	var out struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return ChatTurn{}, fmt.Errorf("ai: decode: %w", err)
	}
	var turn ChatTurn
	var sb strings.Builder
	for _, c := range out.Content {
		switch c.Type {
		case "text":
			sb.WriteString(c.Text)
		case "tool_use":
			turn.ToolCalls = append(turn.ToolCalls, ToolCall{ID: c.ID, Name: c.Name, Input: c.Input})
		}
	}
	turn.Text = strings.TrimSpace(sb.String())
	return turn, nil
}

func anthropicTools(tools []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		schema := t.Schema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": schema,
		})
	}
	return out
}

// ---- OpenAI-compatible (Chat Completions, tool_calls / role:"tool") ----

func (s *Service) openaiChatTools(ctx context.Context, pc providerCfg, system string, messages []ChatMessage, tools []Tool, maxTokens int) (ChatTurn, error) {
	msgs := make([]map[string]any, 0, len(messages)+1)
	if system != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": system})
	}
	for _, m := range messages {
		switch {
		case len(m.ToolResults) > 0:
			for _, tr := range m.ToolResults {
				msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": tr.ID, "content": tr.Content})
			}
		case len(m.ToolCalls) > 0:
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				args := strings.TrimSpace(string(tc.Input))
				if args == "" {
					args = "{}"
				}
				calls = append(calls, map[string]any{
					"id":       tc.ID,
					"type":     "function",
					"function": map[string]any{"name": tc.Name, "arguments": args},
				})
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": m.Text, "tool_calls": calls})
		default:
			msgs = append(msgs, map[string]any{"role": chatRole(m.Role), "content": m.Text})
		}
	}
	payload := map[string]any{"model": pc.model, "max_tokens": maxTokens, "messages": msgs}
	if len(tools) > 0 {
		payload["tools"] = openaiTools(tools)
	}
	headers := map[string]string{}
	if pc.apiKey != "" {
		headers["Authorization"] = "Bearer " + pc.apiKey
	}
	data, err := s.post(ctx, pc.baseURL+"/chat/completions", headers, payload)
	if err != nil {
		return ChatTurn{}, err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return ChatTurn{}, fmt.Errorf("ai: decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return ChatTurn{}, fmt.Errorf("ai: empty response")
	}
	msg := out.Choices[0].Message
	turn := ChatTurn{Text: strings.TrimSpace(msg.Content)}
	for _, tc := range msg.ToolCalls {
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		turn.ToolCalls = append(turn.ToolCalls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Input: json.RawMessage(args)})
	}
	return turn, nil
}

func openaiTools(tools []Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.Schema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type":     "function",
			"function": map[string]any{"name": t.Name, "description": t.Description, "parameters": params},
		})
	}
	return out
}

package agenttools // the tool menu scoped to one caller and channel - the chat and internal/mcpserver both run tools through it, so they can't disagree on what a caller may read

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
)

type ToolSet struct {
	deps              *Deps
	ch                *channel.Channel
	days              int
	perms             Perms
	allowCustomerData bool
	tools             []tool
}

// A window outside 1-maxWindowDays falls back to the default or the cap, so no caller can ask core for a decade of sales
func (s *Service) ToolSet(ctx context.Context, channelID uuid.UUID, defaultDays int) ToolSet {
	deps := s.deps
	perms := deps.perms(ctx, channelID)
	allow := deps.AI != nil && deps.AI.AllowCustomerData(ctx)
	return ToolSet{
		deps: deps, ch: deps.toolChannel(ctx, channelID), days: clampDays(defaultDays, ai.DefaultWindowDays), perms: perms,
		allowCustomerData: allow, tools: offeredTools(perms, allow),
	}
}

func clampDays(days, fallback int) int {
	if days <= 0 {
		return fallback
	}
	return min(days, maxWindowDays)
}

// Services that scope by the request's channel (reviews among them) must see the tool's channel, not the dashboard's scope or none at all
func (deps *Deps) toolChannel(ctx context.Context, id uuid.UUID) *channel.Channel {
	if ch, ok := channel.FromContext(ctx); ok && ch != nil && ch.ID == id {
		return ch
	}
	if deps.Channel != nil {
		ch, err := deps.Channel.GetByID(ctx, id)
		if err == nil {
			return &ch
		}
		slog.WarnContext(ctx, "tool: channel lookup failed", "channel", id, "err", err)
	}
	return &channel.Channel{ID: id}
}

func (s ToolSet) Perms() Perms { return s.perms }

func (s ToolSet) Days() int { return s.days }

func (s ToolSet) Tools() []ai.Tool {
	out := make([]ai.Tool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t.def)
	}
	return out
}

func (s ToolSet) Call(ctx context.Context, name string, args json.RawMessage) ai.ToolResult {
	for _, t := range s.tools {
		if t.def.Name == name {
			return runTool(channel.WithChannel(ctx, s.ch), s.deps, t, s.ch.ID, s.days, s.allowCustomerData, args)
		}
	}
	return ai.ToolResult{IsError: true, Content: fmt.Sprintf("Unknown tool %q.", name)}
}

// Each result carries its call's id - the provider pairs them up and rejects an answer that doesn't
func (s ToolSet) CallAll(ctx context.Context, calls []ai.ToolCall) []ai.ToolResult {
	out := make([]ai.ToolResult, 0, len(calls))
	for _, call := range calls {
		res := s.Call(ctx, call.Name, call.Input)
		res.ID = call.ID
		out = append(out, res)
	}
	return out
}

package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// SignalsChat answers a Signals question grounded in what this admin may see for the channel - config-gated (AI enabled) + permission-gated per Signals product
func (r *mutationResolver) SignalsChat(ctx context.Context, channelID string, sinceDays *int32, page *model.SignalsChatPage, messages []*model.SignalsChatMessage) (*model.SignalsChatReply, error) {
	if r.AISvc == nil || !r.AISvc.Enabled() {
		return nil, ai.ErrDisabled
	}
	if !r.AISvc.Available(ctx) {
		return nil, ai.ErrNotConfigured
	}
	// hard auth gate - digestPermsFor treats a missing admin as full bypass and the rate limiter skips anonymous callers, so without this an unauthenticated caller could drive the AI tool loop over store data
	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}
	if lim := r.RateLimiter; lim != nil { // per-admin limit - AI calls are expensive, cap runaway use
		if claims, ok := auth.AdminFromContext(ctx); ok {
			if !lim.Allow(ctx, "signalsChat:"+claims.AdminID.String()) {
				return nil, ErrRateLimited
			}
		}
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channel id", nil)
	}

	perms := digestPermsFor(ctx, r.Resolver, cid) // only the Signals sections this admin may view for this channel
	if !perms.any() {
		return nil, fmt.Errorf("you don't have access to any Signals data")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second) // bound the whole exchange - the agentic loop can run many provider round-trips
	defer cancel()

	days := 30
	if sinceDays != nil && *sinceDays > 0 {
		days = int(*sinceDays)
	}
	since := time.Now().AddDate(0, 0, -days)

	useTools := r.AISvc.SupportsTools(ctx)
	digestCap := digestMaxChars // with tools on the model fetches detail on demand, so a compact digest keeps per-round tokens down - snapshot-only needs the full one
	if useTools {
		digestCap = 2500
	}
	digest := buildSignalsDigest(ctx, r.Resolver, cid, since, days, perms, digestCap)
	system := signalsSystemPrompt(days, digest, useTools, pageContextLine(page))

	msgs := make([]ai.ChatMessage, 0, len(messages))
	for _, m := range messages {
		if m == nil || strings.TrimSpace(m.Content) == "" {
			continue
		}
		msgs = append(msgs, ai.ChatMessage{Role: m.Role, Text: m.Content})
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no message provided")
	}

	if !useTools { // no tool calling: one completion grounded in the digest - 4k tokens fits chart-heavy answers
		reply, err := r.AISvc.Chat(ctx, system, msgs, 4096)
		if err != nil {
			return nil, aiProviderError(ctx, err)
		}
		return &model.SignalsChatReply{Reply: reply}, nil
	}

	// agentic loop: execute each tool call (re-gated + channel pinned) and feed results back until a final text answer - bounded by maxRounds, the last round demands a direct answer
	tools := permittedTools(perms, r.AISvc.AllowCustomerData(ctx))
	const maxRounds = 6
	for round := 0; round <= maxRounds; round++ {
		sys := system
		last := round == maxRounds
		if last {
			sys = system + "\n\nYou have gathered enough data. Answer the user's question now using the " +
				"information above - do not request any more tools."
		}
		turn, err := r.AISvc.ChatWithTools(ctx, sys, msgs, tools, 4096)
		if err != nil {
			return nil, aiProviderError(ctx, err)
		}
		if len(turn.ToolCalls) == 0 || last {
			reply := strings.TrimSpace(turn.Text)
			if reply == "" {
				reply = "I gathered the data but couldn't compose a final answer - please ask again, more specifically."
			}
			return &model.SignalsChatReply{Reply: reply}, nil
		}
		msgs = append(msgs, ai.ChatMessage{Role: "assistant", Text: turn.Text, ToolCalls: turn.ToolCalls})
		results := make([]ai.ToolResult, 0, len(turn.ToolCalls))
		for _, call := range turn.ToolCalls {
			results = append(results, runSignalTool(ctx, r.Resolver, perms, cid, days, call))
		}
		msgs = append(msgs, ai.ChatMessage{Role: "tool", ToolResults: results})
	}
	return &model.SignalsChatReply{Reply: ""}, nil // unreachable: last round always returns
}

// RunChatAction runs an operator-confirmed action chat proposed - the dispatcher whitelists actions and each re-checks its own permission
func (r *mutationResolver) RunChatAction(ctx context.Context, action string, argsJSON string) (*model.ChatActionResult, error) {
	ok, msg, err := dispatchChatAction(ctx, r, action, argsJSON)
	if err != nil {
		return nil, err
	}
	return &model.ChatActionResult{Ok: ok, Message: msg}, nil
}

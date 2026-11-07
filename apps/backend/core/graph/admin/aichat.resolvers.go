package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/agenttools"
	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/aitrace"
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
	// hard auth gate - the tool permissions treat a missing admin as full bypass and the rate limiter skips anonymous callers, so without this an unauthenticated caller could drive the AI tool loop over store data
	if _, err := auth.RequireAdmin(ctx); err != nil {
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

	days := 0
	if sinceDays != nil {
		days = int(*sinceDays)
	}
	toolset := r.AgentTools.ToolSet(ctx, cid, days)
	if !toolset.Perms().Any() {
		return nil, fmt.Errorf("you don't have access to any Signals data")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second) // bound the whole exchange - the agentic loop can run many provider round-trips
	defer cancel()

	useTools := r.AISvc.SupportsTools(ctx)
	digestCap := agenttools.SnapshotMaxChars // with tools on the model fetches detail on demand, so a compact digest keeps per-round tokens down - snapshot-only needs the full one
	if useTools {
		digestCap = compactSnapshotChars
	}
	system := signalsSystemPrompt(toolset.Days(), toolset.Snapshot(ctx, digestCap), useTools, pageContextLine(page))

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

	// The admin chat's tools read customer data, so its trace gets the timing and tokens, never what was asked or answered
	ctx, trace := aitrace.Start(ctx, "Admin chat", aitrace.Agent)
	defer trace.End()
	trace.Trace("Admin chat", "", "", "admin")
	trace.Meta("tools", useTools)

	if !useTools { // no tool calling: one completion grounded in the digest - 4k tokens fits chart-heavy answers
		reply, err := r.AISvc.Chat(ctx, system, msgs, adminChatMaxTokens)
		if err != nil {
			return nil, aiProviderError(ctx, err)
		}
		return &model.SignalsChatReply{Reply: reply}, nil
	}

	reply, _, err := r.AISvc.Run(ctx, msgs, ai.Loop{System: system, Tools: toolset.Tools(), Call: toolset.CallAll, MaxRounds: adminChatRounds, MaxTokens: adminChatMaxTokens})
	if err != nil {
		return nil, aiProviderError(ctx, err)
	}
	if reply == "" {
		reply = "I gathered the data but couldn't compose a final answer - please ask again, more specifically."
	}
	return &model.SignalsChatReply{Reply: reply}, nil
}

// RunChatAction runs an operator-confirmed action chat proposed - the dispatcher whitelists actions and each re-checks its own permission
func (r *mutationResolver) RunChatAction(ctx context.Context, action string, argsJSON string) (*model.ChatActionResult, error) {
	ok, msg, err := dispatchChatAction(ctx, r, action, argsJSON)
	if err != nil {
		return nil, err
	}
	return &model.ChatActionResult{Ok: ok, Message: msg}, nil
}

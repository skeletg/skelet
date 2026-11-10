package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/embedding"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// A queue older than this means the embedding job isn't keeping up

func agentic(key, label string, state model.AgenticState, path, detail string, args ...any) *model.AgenticStatus {
	if len(args) > 0 {
		detail = fmt.Sprintf(detail, args...)
	}
	return &model.AgenticStatus{Key: key, Label: label, State: state, SettingsPath: path, Detail: detail}
}

// agenticStatus walks the setup in the order it builds up, each piece saying what it still needs
func (r *Resolver) agenticStatus(ctx context.Context, ch channel.Channel) ([]*model.AgenticStatus, error) {
	scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}
	setting := func(path string) (bool, error) { return r.ConfigSvc.GetBool(ctx, path, scope) }
	hasURL := strings.TrimSpace(ch.PublicURL) != ""
	channelPath := "/channels/" + ch.ID.String()
	var out []*model.AgenticStatus

	provider := r.AISvc != nil && r.AISvc.Enabled() && r.AISvc.Available(ctx)
	switch {
	case r.AISvc == nil || !r.AISvc.Enabled():
		out = append(out, agentic("provider", "AI provider", model.AgenticStateOff, "/settings/ai/llm", "Switched off on this server (AI_ENABLED)"))
	case !provider:
		out = append(out, agentic("provider", "AI provider", model.AgenticStateOff, "/settings/ai/llm", "Not set up - the assistant has nothing to answer with"))
	case !r.AISvc.SupportsTools(ctx):
		provider = false
		out = append(out, agentic("provider", "AI provider", model.AgenticStateAttention, "/settings/ai/llm", "%s is set to answer without tools, and the assistant works only with them", r.AISvc.ModelName(ctx)))
	default:
		out = append(out, agentic("provider", "AI provider", model.AgenticStateOn, "/settings/ai/llm", "%s", r.AISvc.ModelName(ctx)))
	}

	assistant, err := setting(shoptools.AssistantPath)
	if err != nil {
		return nil, err
	}
	switch {
	case !assistant:
		out = append(out, agentic("assistant", "Shopping assistant", model.AgenticStateOff, "/settings/sales/agents", "Hidden on this store"))
	case !provider:
		out = append(out, agentic("assistant", "Shopping assistant", model.AgenticStateAttention, "/settings/ai/llm", "Switched on, but there's no AI provider to answer with, so shoppers don't see it"))
	case !hasURL:
		out = append(out, agentic("assistant", "Shopping assistant", model.AgenticStateAttention, channelPath, "Switched on, but the store has no public URL to build its links on, so shoppers don't see it"))
	default:
		daily, err := r.ConfigSvc.GetInt(ctx, shoptools.AssistantDailyPath, scope)
		if err != nil {
			return nil, err
		}
		limit := "with no daily cap"
		if daily > 0 {
			limit = "up to " + thousands(daily) + " messages a day"
		}
		out = append(out, agentic("assistant", "Shopping assistant", model.AgenticStateOn, "/settings/sales/agents", "On the storefront, answering %s", limit))
	}

	voice, err := r.voiceStatus(ctx, scope)
	if err != nil {
		return nil, err
	}
	out = append(out, voice)

	keep, err := setting(shoptools.TranscriptsPath)
	if err != nil {
		return nil, err
	}
	switch {
	case !keep:
		out = append(out, agentic("transcripts", "Keeping conversations", model.AgenticStateOff, "/settings/sales/agents", "Not kept"))
	case !r.SignalsEnabled:
		out = append(out, agentic("transcripts", "Keeping conversations", model.AgenticStateAttention, "/settings/sales/agents", "Switched on, but Signals is off, so nothing is kept"))
	default:
		out = append(out, agentic("transcripts", "Keeping conversations", model.AgenticStateOn, "/conversations", "Kept for shoppers who are signed in or agreed to analytics"))
	}

	mcp, err := setting(shoptools.MCPEnabledPath)
	if err != nil {
		return nil, err
	}
	switch {
	case !r.MCP.Storefront:
		out = append(out, agentic("agents", "Shoppers' agents", model.AgenticStateOff, "/mcp", "Off on this server (MCP_FRONT_ENABLED)"))
	case !mcp:
		out = append(out, agentic("agents", "Shoppers' agents", model.AgenticStateOff, "/settings/sales/agents", "Turned off for this store"))
	case !hasURL:
		out = append(out, agentic("agents", "Shoppers' agents", model.AgenticStateAttention, channelPath, "On, but the store has no public URL, so agents can't use it"))
	default:
		out = append(out, agentic("agents", "Shoppers' agents", model.AgenticStateOn, "/mcp", "Any AI agent can shop at %s", *storefrontMCPURL(ch.PublicURL)))
	}

	if r.Embeddings != nil {
		st, err := r.Embeddings.Status(ctx, &ch.ID)
		if err != nil {
			return nil, err
		}
		coverage := fmt.Sprintf("%s of %s products and %s of %s pages embedded", thousands(st.EmbeddedProducts), thousands(st.Products), thousands(st.EmbeddedPages), thousands(st.Pages))
		switch {
		case st.Model == "":
			out = append(out, agentic("embeddings", "Search by meaning", model.AgenticStateOff, "/settings/ai/embeddings", "No embedding model, so search runs on words alone"))
		case st.Failed != "":
			out = append(out, agentic("embeddings", "Search by meaning", model.AgenticStateAttention, "/settings/ai/embeddings", "%s - the last run failed: %s", coverage, r.providerError(ctx, st.Failed)))
		case st.Queued > 0 && time.Since(st.Oldest) > embedding.StaleBacklog:
			out = append(out, agentic("embeddings", "Search by meaning", model.AgenticStateAttention, "/scheduled-jobs", "%s - %s waiting since %s ago, so the job isn't keeping up", coverage, thousands(st.Queued), time.Since(st.Oldest).Round(time.Minute)))
		default:
			out = append(out, agentic("embeddings", "Search by meaning", model.AgenticStateOn, "/settings/ai/embeddings", "%s - %s", st.Model, coverage))
		}
	}

	if r.AIUsageSvc != nil {
		if r.AIUsageSvc.PricesSet(ctx) {
			out = append(out, agentic("pricing", "Cost estimate", model.AgenticStateOn, "/settings/ai/usage", "Priced from your provider's rates"))
		} else {
			out = append(out, agentic("pricing", "Cost estimate", model.AgenticStateOff, "/settings/ai/usage", "No prices entered, so the model's cost shows as tokens only"))
		}
	}
	return out, nil
}

// The voice is never broken, only left at its default - which reads as off, so the row says what's still to set
func (r *Resolver) voiceStatus(ctx context.Context, scope config.Scope) (*model.AgenticStatus, error) {
	var v [3]string
	for i, path := range []string{shoptools.VoiceNamePath, shoptools.VoiceTonePath, shoptools.VoiceInstructionsPath} {
		val, err := r.ConfigSvc.GetString(ctx, path, scope)
		if err != nil {
			return nil, err
		}
		v[i] = strings.TrimSpace(val)
	}
	name, tone, instructions := v[0], shoptools.ToneLabel(v[1]), v[2]
	if name == "" && instructions == "" && v[1] == shoptools.DefaultTone {
		return agentic("voice", "Assistant voice", model.AgenticStateOff, "/agentic/voice", "The default voice - no name or instructions of the store's own yet"), nil
	}
	parts := []string{strings.ToLower(tone)}
	if name != "" {
		parts = []string{"Called " + name, parts[0]}
	}
	if n := utf8.RuneCountInString(instructions); n > 0 {
		parts = append(parts, "with "+thousands(int64(n))+" characters of instructions")
	} else {
		parts = append(parts, "no instructions yet")
	}
	return agentic("voice", "Assistant voice", model.AgenticStateOn, "/agentic/voice", "%s", strings.Join(parts, ", ")), nil
}

func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

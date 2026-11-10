package shoptools

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
)

// Search is one catalog search a tool ran and how many products came back, for the AI analytics
type Search struct {
	Term    string
	Results int
}

type searchLog struct {
	mu       sync.Mutex
	searches []Search
}

type searchLogKey struct{}

func withSearchLog(ctx context.Context) (context.Context, *searchLog) {
	l := &searchLog{}
	return context.WithValue(ctx, searchLogKey{}, l), l
}

func logSearch(ctx context.Context, term string, results int) {
	l, ok := ctx.Value(searchLogKey{}).(*searchLog)
	if term = strings.TrimSpace(term); !ok || term == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.searches) < signalsapi.AIMaxSearches {
		if r := []rune(term); len(r) > signalsapi.AIMaxTerm {
			term = string(r[:signalsapi.AIMaxTerm])
		}
		l.searches = append(l.searches, Search{Term: term, Results: results})
	}
}

func (l *searchLog) list() []Search {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.searches)
}

const EventAgentCall = "shoptools.agent_call"

// AgentCall is a tool a shopper's own AI agent called over MCP - counted per store, never tied to a person
type AgentCall struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	Agent     string
	Tool      string
	Searches  []Search
}

func (*AgentCall) Type() string                { return EventAgentCall }
func (e *AgentCall) EventChannelID() uuid.UUID { return e.ChannelID }

// AgentToolSet is the menu as a named agent reaches it, each call counted for the store's AI analytics
func (s *Service) AgentToolSet(ch channel.Channel, agent string) ToolSet {
	return ToolSet{svc: s, ch: ch, agent: agent}
}

func (s *Service) agentCalled(ctx context.Context, ch channel.Channel, agent, tool string, searches []Search) {
	if s.deps.Bus != nil {
		s.deps.Bus.Publish(ctx, &AgentCall{ID: uuid.New(), ChannelID: ch.ID, Agent: agent, Tool: tool, Searches: searches})
	}
}

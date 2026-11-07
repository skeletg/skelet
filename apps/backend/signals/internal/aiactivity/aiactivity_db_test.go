package aiactivity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	db "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

func TestActivityIsCountedOncePerStoreAndDay(t *testing.T) {
	conn := testdb.RequireSignals(t)
	s := New(db.New(conn))
	ctx := context.Background()
	ch, other := uuid.New(), uuid.New()
	now := time.Now().UTC()
	call := func(key, agent, tool string, searches ...Search) Activity {
		return Activity{ChannelID: ch, At: now, Source: SourceAgent, Agent: agent, Tools: []string{tool}, Searches: searches, DedupeKey: key}
	}

	for _, a := range []Activity{
		call("c1", "Claude", "search_products", Search{Term: "Wide  FIT boots", Results: 0}),
		call("c2", "Claude", "search_products", Search{Term: "wide fit boots", Results: 0}),
		call("c3", "Claude", "add_to_cart"),
		call("c4", "  ", "search_products", Search{Term: "linen shirt", Results: 4}),
		{ChannelID: ch, At: now, Source: SourceAssistant, Tools: []string{"search_products", "show_products"}, Searches: []Search{{Term: "wide fit boots"}}, DedupeKey: "t1"},
		{ChannelID: other, At: now, Source: SourceAgent, Agent: "Claude", Tools: []string{"add_to_cart"}, DedupeKey: "o1"},
	} {
		if ok, err := s.Record(ctx, a); err != nil || !ok {
			t.Fatalf("record %s = %v, %v", a.DedupeKey, ok, err)
		}
	}
	if ok, err := s.Record(ctx, call("c1", "Claude", "search_products", Search{Term: "wide fit boots"})); err != nil || ok {
		t.Errorf("a replay = %v, %v - want it counted once", ok, err)
	}

	agents, err := s.Agents(ctx, ch, now.AddDate(0, 0, -1), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 || agents[0].Name != "Claude" || agents[0].Calls != 3 || agents[0].Searches != 2 || agents[0].Empty != 2 {
		t.Fatalf("agents = %+v - want Claude first with its three calls, both searches empty, the other store left out", agents)
	}
	if agents[1].Name != "unknown" || agents[1].Searches != 1 || agents[1].Empty != 0 {
		t.Errorf("an agent without a name = %+v", agents[1])
	}
	if len(agents[0].Tools) != 2 || agents[0].Tools[0].Tool != "search_products" || agents[0].Tools[0].Calls != 2 {
		t.Errorf("tools = %+v - want the most called first", agents[0].Tools)
	}

	gaps, err := db.New(conn).AISearchGaps(ctx, db.AISearchGapsParams{ChannelID: ch, Since: now.AddDate(0, 0, -1), Lim: 10, AssistantSource: SourceAssistant, AgentSource: SourceAgent})
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].Term != "wide fit boots" || gaps[0].Agents != 2 || gaps[0].Assistant != 1 {
		t.Errorf("gaps = %+v - want one term, written two ways, empty for the agent twice and the assistant once", gaps)
	}
}

func TestActivityThatCantBeCountedIsRefused(t *testing.T) {
	s := New(db.New(testdb.RequireSignals(t)))
	ctx := context.Background()
	for name, a := range map[string]Activity{
		"no store":   {Source: SourceAssistant, DedupeKey: "k"},
		"no key":     {ChannelID: uuid.New(), Source: SourceAssistant},
		"bad source": {ChannelID: uuid.New(), Source: "robot", DedupeKey: "k"},
	} {
		if _, err := s.Record(ctx, a); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s = %v, want refused", name, err)
		}
	}
}

func TestAnAgentsNameAndAFloodOfToolsAreBounded(t *testing.T) {
	conn := testdb.RequireSignals(t)
	s := New(db.New(conn))
	ctx := context.Background()
	ch := uuid.New()
	tools := make([]string, MaxTools+30)
	for i := range tools {
		tools[i] = "search_products"
	}
	name := "Agent\x00\x07 " + strings.Repeat("x", 100)
	if _, err := s.Record(ctx, Activity{ChannelID: ch, Source: SourceAgent, Agent: name, Tools: tools, DedupeKey: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	agents, err := s.Agents(ctx, ch, time.Now().AddDate(0, 0, -1), 10)
	if err != nil || len(agents) != 1 {
		t.Fatalf("agents = %+v, %v", agents, err)
	}
	if got := agents[0]; got.Calls != MaxTools || len([]rune(got.Name)) != maxAgent || strings.ContainsAny(got.Name, "\x00\x07") {
		t.Errorf("stored %q with %d calls - want the name cleaned and cut, the calls capped at %d", got.Name, got.Calls, MaxTools)
	}
}

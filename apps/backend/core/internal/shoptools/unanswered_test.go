package shoptools

import (
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

func TestTheUnansweredMarkComesOffWherePutAndIsRemembered(t *testing.T) {
	for in, want := range map[string]string{
		"We don't carry those.\n[[unanswered]]":     "We don't carry those.",
		"We don't carry those. [[unanswered]]":      "We don't carry those.",
		"[[ unanswered ]]\nWe don't carry those.":   "We don't carry those.",
		"First line.\n[[unanswered]]\nSecond line.": "First line.\nSecond line.",
	} {
		got, marked := splitUnanswered(in)
		if !marked || got != want {
			t.Errorf("%q -> %q %v, want %q", in, got, marked, want)
		}
	}
	if got, marked := splitUnanswered("All good."); marked || got != "All good." {
		t.Errorf("an unmarked reply came back %q %v", got, marked)
	}
}

func TestATurnTheStoreCouldntAnswerIsFlagged(t *testing.T) {
	search := func(q string) ai.ToolCall { return toolCall("search_products", map[string]any{"query": q}) }
	pages := toolCall("search_store_pages", map[string]any{"query": "unicorn"})
	catalog := &fakeGraphQL{answers: map[string]func(map[string]any) (any, error){
		"AgentSearch": func(vars map[string]any) (any, error) {
			if vars["query"] == "saddle" {
				return map[string]any{"browseProducts": map[string]any{"totalCount": 1, "items": []any{hit("p1", "Saddle")}}}, nil
			}
			return map[string]any{"browseProducts": map[string]any{"items": []any{}}}, nil
		},
		"AgentPages": func(map[string]any) (any, error) { return map[string]any{"cmsPages": []any{}}, nil },
	}}
	for name, tc := range map[string]struct {
		script               []ai.ChatTurn
		reply                string
		foundNothing, unsure bool
	}{
		"every search came back empty, and the model said so": {
			script: []ai.ChatTurn{round("", search("unicorn saddle"), pages), round("We don't carry those.\n[[unanswered]]\n[[next: Show saddles]]")},
			reply:  "We don't carry those.", foundNothing: true, unsure: true,
		},
		"a later search found something": {
			script: []ai.ChatTurn{round("", search("unicorn")), round("", search("saddle")), round("Here's a saddle.")},
			reply:  "Here's a saddle.",
		},
		"the mark with nothing said": {
			script: []ai.ChatTurn{round("", search("unicorn")), round("[[unanswered]]")},
			reply:  "", foundNothing: true, unsure: true,
		},
		"no search at all": {
			script: []ai.ChatTurn{round("Hello! What are you after?")},
			reply:  "Hello! What are you after?",
		},
	} {
		s, _, bus := settledRig(&fakeModel{script: tc.script}, nil)
		s.deps.GraphQL = catalog
		turn, err := s.Assist(shopperCtx(), shop, guest(nil), ask, "", Notify{})
		if err != nil || turn.Reply != tc.reply || strings.Contains(turn.Reply, "[[") {
			t.Errorf("%s: reply %q, %v", name, turn.Reply, err)
			continue
		}
		ev := bus.published[0].(*AssistantTurn)
		if ev.FoundNothing != tc.foundNothing || ev.Unsure != tc.unsure {
			t.Errorf("%s: found nothing %v, unsure %v", name, ev.FoundNothing, ev.Unsure)
		}
	}
	if !strings.Contains(assistantPrompt(shop, voice{}), "[[unanswered]]") {
		t.Error("the prompt no longer asks for the mark")
	}
}

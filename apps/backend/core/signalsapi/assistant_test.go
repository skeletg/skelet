package signalsapi

import (
	"strings"
	"testing"
	"unicode/utf8"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func TestATrimmedTurnIsOneSignalsKeeps(t *testing.T) {
	big := strings.Repeat("<ü&>", MaxAssistantRoundsBytes/16)
	rec := &signalsv1.AssistantTurnRecord{
		Question:  strings.Repeat("q", MaxAssistantTurnText+10),
		Reply:     strings.Repeat("r", MaxAssistantTurnText+1),
		Recap:     strings.Repeat("c", MaxAssistantTurnText*2),
		Page:      strings.Repeat("/", MaxAssistantTurnMeta+1),
		Model:     strings.Repeat("m", MaxAssistantTurnMeta+1),
		FollowUps: []string{strings.Repeat("f", MaxAssistantTurnMeta+5), "a", "b", "c", "d", "e"},
	}
	for range MaxAssistantRounds + 3 {
		rec.Rounds = append(rec.Rounds, &signalsv1.AssistantRound{Text: "kept", Thinking: big, Calls: []*signalsv1.AssistantToolCall{{Tool: "search_products", Output: big}}})
	}
	FitAssistantTurn(rec)
	rounds, err := AssistantRoundsJSON(rec.Rounds)
	if err != nil || !AssistantTurnFits(rec) || len(rounds) > MaxAssistantRoundsBytes {
		t.Fatalf("fits %v, %d rounds of %d bytes, %v - want a turn Signals keeps", AssistantTurnFits(rec), len(rec.Rounds), len(rounds), err)
	}
	if utf8.RuneCountInString(rec.Question) != MaxAssistantTurnText || !strings.HasSuffix(rec.Question, "…") || rec.Rounds[0].Text != "kept" || !utf8.ValidString(rec.Rounds[0].Calls[0].Output) {
		t.Errorf("trimmed the wrong things: question %d runes, text %q", utf8.RuneCountInString(rec.Question), rec.Rounds[0].Text)
	}
}

func TestAShortTurnIsLeftAlone(t *testing.T) {
	rec := &signalsv1.AssistantTurnRecord{Question: "q", Reply: "short", FollowUps: []string{"x"}}
	FitAssistantTurn(rec)
	if rec.Question != "q" || rec.Reply != "short" || len(rec.FollowUps) != 1 {
		t.Errorf("a turn under every limit was changed: %+v", rec)
	}
}

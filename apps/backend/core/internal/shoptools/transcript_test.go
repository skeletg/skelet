package shoptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
)

func transcriptRig(model *fakeModel, keep bool) (*Service, *fakeBus) {
	s, _, bus := settledRig(model, nil)
	s.deps.Settings = fakeSettings{open: true, transcripts: keep}
	return s, bus
}

func keptTranscript(t *testing.T, bus *fakeBus) *Transcript {
	t.Helper()
	if len(bus.published) != 1 {
		t.Fatalf("published %d events", len(bus.published))
	}
	return bus.published[0].(*AssistantTurn).Transcript
}

func TestAKeptTurnRecordsHowTheModelGotThere(t *testing.T) {
	chat := uuid.New()
	used := ai.Usage{Input: 10, Output: 5}
	model := &fakeModel{calls: []ai.ToolCall{toolCall("get_product", map[string]any{"product": "MUG"})}, thinking: "They named a SKU, so look it up.", reply: "It's a mug.\n[[next: Price? | Stock?]]", used: used}
	s, bus := transcriptRig(model, true)
	turn, err := s.Assist(shopperCtx(), shop, Shopper{Address: "ip", Conversation: chat}, ask, "/shop/mug", Notify{})
	if err != nil || !model.thinkingAsked {
		t.Fatalf("err %v, thinking asked %v", err, model.thinkingAsked)
	}
	tr := keptTranscript(t, bus)
	if tr == nil {
		t.Fatal("no transcript kept")
	}
	if tr.Conversation == chat || tr.Conversation == uuid.Nil {
		t.Errorf("conversation %s - want an id folded from the store and shopper, not the browser's", tr.Conversation)
	}
	if tr.Question != "Two mugs please" || tr.Reply != turn.Reply || tr.Recap != turn.Recap || tr.Page != "/shop/mug" || tr.Model != "fake-model" || tr.Usage != used {
		t.Errorf("transcript = %+v", tr)
	}
	if len(tr.Rounds) != 2 || tr.Rounds[0].Thinking != "They named a SKU, so look it up." || tr.Rounds[1].Text != "It's a mug.\n[[next: Price? | Stock?]]" {
		t.Fatalf("rounds = %+v", tr.Rounds)
	}
	call := tr.Rounds[0].Calls[0]
	var input map[string]string
	if call.Tool != "get_product" || json.Unmarshal(call.Input, &input) != nil || input["product"] != "MUG" || call.Output == "" || call.IsError {
		t.Errorf("call = %+v", call)
	}

	again, bus2 := transcriptRig(&fakeModel{}, true)
	if _, err := again.Assist(shopperCtx(), shop, Shopper{Address: "ip", Conversation: chat}, ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	if got := keptTranscript(t, bus2).Conversation; got != tr.Conversation {
		t.Errorf("the same visitor's next turn filed under %s, want %s", got, tr.Conversation)
	}
	customer := uuid.New()
	other, bus3 := transcriptRig(&fakeModel{}, true)
	if _, err := other.Assist(context.Background(), shop, Shopper{Address: "ip", Customer: &customer, Conversation: chat}, ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	if got := keptTranscript(t, bus3).Conversation; got == tr.Conversation {
		t.Error("another shopper sending the same chat id joined the conversation")
	}
}

func TestATurnIsKeptOnlyWhenTheStoreKeepsThemAndSomeoneOwnsIt(t *testing.T) {
	for name, tc := range map[string]struct {
		keep bool
		ctx  context.Context
		chat uuid.UUID
	}{
		"switched off":      {false, shopperCtx(), uuid.New()},
		"no chat id":        {true, shopperCtx(), uuid.Nil},
		"nobody to file it": {true, context.Background(), uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			model := &fakeModel{}
			s, bus := transcriptRig(model, tc.keep)
			if _, err := s.Assist(tc.ctx, shop, Shopper{Address: "ip", Conversation: tc.chat}, ask, "", Notify{}); err != nil {
				t.Fatal(err)
			}
			if tr := keptTranscript(t, bus); tr != nil || model.thinkingAsked {
				t.Errorf("kept %+v, thinking asked %v", tr, model.thinkingAsked)
			}
		})
	}
}

func TestATranscriptMarksTheWordsTheShopperGotAndTheCallsThatNeverRan(t *testing.T) {
	lookup := toolCall("get_product", map[string]any{"sku": "MUG"})
	script := []ai.ChatTurn{round("Let me check.", lookup), round("We don't ship to Germany.\n[[next: Countries?]]", lookup), round("[[next: Other bags]]")}
	s, bus := transcriptRig(&fakeModel{script: script}, true)
	if _, err := s.Assist(shopperCtx(), shop, Shopper{Address: "ip", Conversation: uuid.New()}, ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	tr := keptTranscript(t, bus)
	if tr.Rounds[0].Shown || !tr.Rounds[1].Shown || tr.Rounds[2].Shown || tr.Reply != "We don't ship to Germany." || len(tr.FollowUps) != 1 || tr.FollowUps[0] != "Other bags" {
		t.Errorf("transcript = %+v", tr)
	}
	if tr.Rounds[0].Calls[0].Skipped || tr.Rounds[1].Calls[0].Skipped {
		t.Error("calls that ran are marked as never run")
	}

	long := make([]ai.ChatTurn, assistantRounds+1)
	for i := range long {
		long[i] = round("", lookup)
	}
	long[assistantRounds] = round("Here's what I found.", lookup)
	s, bus = transcriptRig(&fakeModel{script: long}, true)
	if _, err := s.Assist(shopperCtx(), shop, Shopper{Address: "ip", Conversation: uuid.New()}, ask, "", Notify{}); err != nil {
		t.Fatal(err)
	}
	tr = keptTranscript(t, bus)
	last := tr.Rounds[len(tr.Rounds)-1]
	if !last.Calls[0].Skipped || !last.Shown || tr.Rounds[0].Calls[0].Skipped {
		t.Errorf("the last allowed round = %+v", last)
	}
}

func TestATranscriptKeepsNoOrderDetails(t *testing.T) {
	tr := &Transcript{}
	tr.round(ai.ChatTurn{ToolCalls: []ai.ToolCall{{Name: "get_my_order"}, {Name: "get_my_order"}, {Name: "search_products"}}})
	tr.result(0, ai.ToolResult{Content: "Order 1002 going to Berlin, tracking JD1002"})
	tr.result(1, ai.ToolResult{Content: "There's no order \"999\"", IsError: true})
	tr.result(2, ai.ToolResult{Content: "3 products match"})
	calls := tr.Rounds[0].Calls
	if calls[0].Output != orderOutputWithheld || calls[1].Output != "There's no order \"999\"" || calls[2].Output != "3 products match" {
		t.Errorf("outputs %q / %q / %q - want the order withheld, its refusal and other tools kept", calls[0].Output, calls[1].Output, calls[2].Output)
	}
}

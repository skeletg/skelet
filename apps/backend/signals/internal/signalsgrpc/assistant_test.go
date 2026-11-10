package signalsgrpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/signalsapi"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/assistantlog"
)

func TestRoundsSurviveTheTripThroughStorage(t *testing.T) {
	in := []*signalsv1.AssistantRound{
		{Thinking: "search first", Text: "Let me look\x00.", Calls: []*signalsv1.AssistantToolCall{
			{Tool: "search_products", InputJson: `{"query":"mug"}`, Output: "Big mug"},
			{Tool: "get_product", InputJson: `not json`, Output: "no such product", IsError: true},
		}},
		{Text: "Here's the big mug.", Shown: true, Calls: []*signalsv1.AssistantToolCall{{Tool: "show_products", Skipped: true}}},
	}
	raw, err := roundsIn(in)
	if err != nil {
		t.Fatal(err)
	}
	out := roundsOut(context.Background(), raw)
	if len(out) != 2 || out[0].GetThinking() != "search first" || out[0].GetText() != "Let me look." || out[0].GetShown() || !out[1].GetShown() {
		t.Fatalf("rounds = %+v", out)
	}
	if c := out[0].GetCalls()[0]; c.GetInputJson() != `{"query":"mug"}` || c.GetOutput() != "Big mug" {
		t.Errorf("call = %+v", c)
	}
	if c := out[0].GetCalls()[1]; c.GetInputJson() != "not json" || !c.GetIsError() {
		t.Errorf("a refused call = %+v", c)
	}
	if !out[1].GetCalls()[0].GetSkipped() {
		t.Error("a call that never ran lost its mark")
	}
	huge := []*signalsv1.AssistantRound{{Text: strings.Repeat("x", signalsapi.MaxAssistantRoundsBytes)}}
	if _, err := roundsIn(huge); status.Code(err) != codes.InvalidArgument {
		t.Errorf("an oversized turn = %v", err)
	}
	if roundsOut(context.Background(), []byte("{broken")) != nil {
		t.Error("a broken row answered rounds")
	}
}

func TestRoundsFullOfMarkupAreMeasuredTheWayCoreTrimsThem(t *testing.T) {
	html := strings.Repeat("<p>Tea & <b>cake</b></p>", 1000)
	rounds := []*signalsv1.AssistantRound{}
	for range signalsapi.MaxAssistantRounds {
		rounds = append(rounds, &signalsv1.AssistantRound{Calls: []*signalsv1.AssistantToolCall{{Tool: "get_product", Output: html}}})
	}
	rec := &signalsv1.AssistantTurnRecord{Rounds: rounds}
	signalsapi.FitAssistantTurn(rec)
	raw, err := roundsIn(rec.GetRounds())
	if err != nil {
		t.Fatalf("a turn core trimmed to fit was refused: %v", err)
	}
	if strings.Contains(string(raw), `\u003c`) {
		t.Error("the stored rounds are HTML-escaped, so they measure bigger than core's count")
	}
	if out := roundsOut(context.Background(), raw); out[0].GetCalls()[0].GetOutput() != rec.GetRounds()[0].GetCalls()[0].GetOutput() {
		t.Error("markup didn't survive the trip")
	}
}

func TestATurnThatCantBeKeptIsRefused(t *testing.T) {
	c := signalsv1.NewIngestServiceClient(testConnWith(t, Deps{ServiceKeys: testKeys, Channels: analytics.NewChannelRoster(nil), Assistant: assistantlog.New(nil)}))
	sys := systemCtx(t)
	id, bad := uuid.NewString(), "not-a-uuid"
	for name, turn := range map[string]*signalsv1.AssistantTurnRecord{
		"no id":                {ConversationId: id, ChannelId: id, AnonId: "a"},
		"no conversation":      {Id: id, ChannelId: id, AnonId: "a"},
		"no store":             {Id: id, ConversationId: id, AnonId: "a"},
		"nobody's":             {Id: id, ConversationId: id, ChannelId: id},
		"too long a question":  {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", Question: strings.Repeat("x", signalsapi.MaxAssistantTurnText+1)},
		"too many rounds":      {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", Rounds: make([]*signalsv1.AssistantRound, signalsapi.MaxAssistantRounds+1)},
		"too many follow-ups":  {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", FollowUps: make([]string, signalsapi.MaxAssistantFollowUps+1)},
		"a long follow-up":     {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", FollowUps: []string{strings.Repeat("x", signalsapi.MaxAssistantTurnMeta+1)}},
		"too long a recap":     {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", Recap: strings.Repeat("x", signalsapi.MaxAssistantTurnText+1)},
		"too long a page":      {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", Page: strings.Repeat("x", signalsapi.MaxAssistantTurnMeta+1)},
		"a malformed visitor":  {Id: id, ConversationId: id, ChannelId: id, AnonId: "a b\x00"},
		"a malformed customer": {Id: id, ConversationId: id, ChannelId: id, AnonId: "a", CustomerId: &bad},
	} {
		if _, err := c.RecordAssistantTurn(sys, &signalsv1.RecordAssistantTurnRequest{Turn: turn}); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s = %v, want InvalidArgument", name, err)
		}
	}
}

func TestAMalformedCustomerFilterIsRefusedNotIgnored(t *testing.T) {
	ch := uuid.New()
	c := signalsv1.NewAssistantServiceClient(testConnWith(t, Deps{ServiceKeys: testKeys, Channels: analytics.NewChannelRoster(nil), Assistant: assistantlog.New(nil)}))
	bad := "not-a-uuid"
	_, err := c.ListAssistantConversations(tokenCtx(t, grantOn("assistant.view", ch)), &signalsv1.ListAssistantConversationsRequest{ChannelId: ch.String(), CustomerId: &bad})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("a bad customer id = %v, want InvalidArgument - ignoring it would list every shopper", err)
	}
}

func TestTheUnansweredFilterTakesOnlyKnownKinds(t *testing.T) {
	ch := uuid.New()
	c := signalsv1.NewAssistantServiceClient(testConnWith(t, Deps{ServiceKeys: testKeys, Channels: analytics.NewChannelRoster(nil), Assistant: assistantlog.New(nil)}))
	ctx := tokenCtx(t, grantOn("assistant.view", ch))
	if _, err := c.ListAssistantConversations(ctx, &signalsv1.ListAssistantConversationsRequest{ChannelId: ch.String(), Unanswered: "bored"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("an unknown kind = %v, want InvalidArgument", err)
	}
}

func TestATurnKeepsItsOwnTimeOnlyWhenItIsBelievable(t *testing.T) {
	now := time.Now()
	for name, tc := range map[string]struct {
		stamp *timestamppb.Timestamp
		want  time.Time
	}{
		"none":                 {nil, now},
		"a minute ago":         {timestamppb.New(now.Add(-time.Minute)), now.Add(-time.Minute)},
		"a clock a bit ahead":  {timestamppb.New(now.Add(2 * time.Minute)), now.Add(2 * time.Minute)},
		"an hour ahead":        {timestamppb.New(now.Add(time.Hour)), now},
		"a replay from a year": {timestamppb.New(now.AddDate(-1, 0, 0)), now.AddDate(-1, 0, 0)},
	} {
		if got := turnTime(tc.stamp, now); !got.Equal(tc.want) {
			t.Errorf("%s = %v, want %v", name, got, tc.want)
		}
	}
}

package admin

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func TestAConversationCursorComesBackAsItWent(t *testing.T) {
	at := time.Date(2026, 11, 5, 10, 4, 5, 123456000, time.UTC)
	id := uuid.NewString()
	gotAt, gotID, err := decodeConversationCursor(encodeConversationCursor(at, id))
	if err != nil || !gotAt.Equal(at) || gotID != id {
		t.Errorf("round trip = %v, %s, %v", gotAt, gotID, err)
	}
	for _, bad := range []string{"", "!!", encodeConversationCursor(at, "not-a-uuid"), "bm90LWEtY3Vyc29y"} {
		if _, _, err := decodeConversationCursor(bad); err == nil {
			t.Errorf("%q decoded", bad)
		}
	}
}

func TestAConversationNamesItsLatestUnansweredMessageWithEveryReasonInOrder(t *testing.T) {
	id := uuid.NewString()
	c := conversationSummary(&signalsv1.AssistantConversationSummary{
		Id: uuid.NewString(), StartedAt: timestamppb.Now(), LastAt: timestamppb.Now(), Unanswered: 3,
		LatestUnanswered: &signalsv1.AssistantGapTurn{Id: id, Question: "Mars?", Unsure: true, FoundNothing: true},
	})
	if c.Unanswered != 3 || c.LatestUnanswered == nil || c.LatestUnanswered.TurnID != id || c.LatestUnanswered.Question != "Mars?" {
		t.Fatalf("conversation = %+v", c)
	}
	if got := c.LatestUnanswered.Reasons; len(got) != 2 || got[0] != model.AssistantGapReasonFoundNothing || got[1] != model.AssistantGapReasonUnsure {
		t.Errorf("reasons = %v", got)
	}
	if answered := conversationSummary(&signalsv1.AssistantConversationSummary{StartedAt: timestamppb.Now(), LastAt: timestamppb.Now()}); answered.LatestUnanswered != nil {
		t.Errorf("an answered conversation carries %+v", answered.LatestUnanswered)
	}
	for f, wire := range unansweredWire {
		if !f.IsValid() || wire == "" {
			t.Errorf("%s maps to %q", f, wire)
		}
	}
	if len(unansweredWire) != len(model.AllAssistantUnansweredFilter) {
		t.Errorf("every filter needs its wire name - %d of %d", len(unansweredWire), len(model.AllAssistantUnansweredFilter))
	}
}

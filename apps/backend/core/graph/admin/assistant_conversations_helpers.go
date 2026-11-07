package admin

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/shoptools"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func encodeConversationCursor(at time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(at.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeConversationCursor(cursor string) (time.Time, string, error) {
	bad := apperr.New("INVALID_CURSOR", apperr.ClassValidation, "the cursor isn't one this list gave")
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", bad
	}
	stamp, id, ok := strings.Cut(string(raw), "|")
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if _, idErr := uuid.Parse(id); !ok || err != nil || idErr != nil {
		return time.Time{}, "", bad
	}
	return at, id, nil
}

func conversationSummary(c *signalsv1.AssistantConversationSummary) *model.AssistantConversation {
	return &model.AssistantConversation{
		ID: c.GetId(), CustomerID: c.CustomerId, AnonID: c.GetAnonId(), FirstQuestion: c.GetFirstQuestion(), Turns: c.GetTurns(),
		CartChanged: c.GetCartChanged(), Checkout: c.GetCheckout(), Incomplete: c.GetIncomplete(),
		StartedAt: c.GetStartedAt().AsTime(), LastAt: c.GetLastAt().AsTime(), Unanswered: c.GetUnanswered(), LatestUnanswered: gapOut(c.GetLatestUnanswered()),
	}
}

func transcriptRounds(in []*signalsv1.AssistantRound) []*model.AssistantRound {
	out := make([]*model.AssistantRound, len(in))
	for i, r := range in {
		out[i] = &model.AssistantRound{Thinking: r.GetThinking(), Text: r.GetText(), Shown: r.GetShown(), Calls: make([]*model.AssistantToolCall, len(r.GetCalls()))}
		for j, c := range r.GetCalls() {
			out[i].Calls[j] = &model.AssistantToolCall{Tool: c.GetTool(), Input: c.GetInputJson(), Output: c.GetOutput(), IsError: c.GetIsError(), Skipped: c.GetSkipped()}
		}
	}
	return out
}

var unansweredWire = map[model.AssistantUnansweredFilter]string{
	model.AssistantUnansweredFilterAny:          "any",
	model.AssistantUnansweredFilterNoAnswer:     "no_answer",
	model.AssistantUnansweredFilterCutShort:     "cut_short",
	model.AssistantUnansweredFilterFoundNothing: "found_nothing",
	model.AssistantUnansweredFilterUnsure:       "unsure",
}

// Every reason that holds, in the one order the dashboard shows them
func gapOut(g *signalsv1.AssistantGapTurn) *model.AssistantGap {
	if g == nil {
		return nil
	}
	out := &model.AssistantGap{TurnID: g.GetId(), Question: g.GetQuestion(), Reasons: []model.AssistantGapReason{}}
	for _, r := range []struct {
		on     bool
		reason model.AssistantGapReason
	}{
		{g.GetNoAnswer(), model.AssistantGapReasonNoAnswer}, {g.GetCutShort(), model.AssistantGapReasonCutShort},
		{g.GetFoundNothing(), model.AssistantGapReasonFoundNothing}, {g.GetUnsure(), model.AssistantGapReasonUnsure},
	} {
		if r.on {
			out.Reasons = append(out.Reasons, r.reason)
		}
	}
	return out
}

func gapCounts(c *signalsv1.AssistantGapCounts) *model.AssistantGapCounts {
	return &model.AssistantGapCounts{Any: c.GetAny(), NoAnswer: c.GetNoAnswer(), CutShort: c.GetCutShort(), FoundNothing: c.GetFoundNothing(), Unsure: c.GetUnsure()}
}

// The page's counts cost Signals a second query, so they're asked for only when the caller selected them
func countsSelected(ctx context.Context) bool {
	return slices.Contains(graphql.CollectAllFields(ctx), "counts")
}

var shownKindOut = map[shoptools.ShownKind]model.AssistantShownKind{
	shoptools.ShownCard: model.AssistantShownKindCard, shoptools.ShownPanel: model.AssistantShownKindPanel,
	shoptools.ShownButton: model.AssistantShownKindButton, shoptools.ShownMoved: model.AssistantShownKindMoved,
	shoptools.ShownCart: model.AssistantShownKindCart, shoptools.ShownText: model.AssistantShownKindText,
}

func showedOut(recap string) []*model.AssistantShownItem {
	shown := shoptools.ReadRecap(recap)
	out := make([]*model.AssistantShownItem, 0, len(shown))
	for _, s := range shown {
		item := &model.AssistantShownItem{Kind: shownKindOut[s.Kind], Name: strPtrOrNil(s.Name), Label: strPtrOrNil(s.Label), Path: strPtrOrNil(s.Path), Text: strPtrOrNil(s.Text)}
		if s.ProductID != uuid.Nil {
			id := s.ProductID.String()
			item.ProductID = &id
		}
		out = append(out, item)
	}
	return out
}

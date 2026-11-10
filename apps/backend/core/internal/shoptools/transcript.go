package shoptools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/ai"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

const (
	TranscriptsPath       = "sales/agents/assistant_transcripts"
	maxTranscriptOutput   = 4000
	maxTranscriptThinking = 8000
)

var transcriptSpace = uuid.MustParse("8f1d3c52-6a0e-4c1f-9a37-2b5e7d4c9e10")

// Transcript is a turn as the store keeps it: the message, the answer, and each round of how the model got there
type Transcript struct {
	Conversation uuid.UUID
	Page         string
	Question     string
	Reply        string
	Recap        string
	FollowUps    []string
	Model        string
	Rounds       []Round
	Usage        ai.Usage
}

type Round struct {
	Thinking string
	Text     string
	Calls    []Call
	Shown    bool // these words reached the shopper
}

type Call struct {
	Tool    string
	Input   json.RawMessage
	Output  string
	IsError bool
	Skipped bool // asked for in the last round the turn allowed, so it never ran
}

// The conversation id folds in the store and the shopper, so a chat id sent from one browser can't file turns under another's.
// The storefront sends a chat id only for a shopper who agreed to analytics, signed in or not, so without one nothing is kept
func (s *Service) recorder(ctx context.Context, ch channel.Channel, who Shopper, question, page string) *Transcript {
	var owner string
	if anon := auth.AnonID(ctx); anon != "" {
		owner = "anon:" + anon
	} else if who.Customer != nil {
		owner = "customer:" + who.Customer.String()
	}
	if who.Conversation == uuid.Nil || owner == "" || s.deps.SignalsOff {
		return nil
	}
	if on, err := s.deps.Settings.GetBool(ctx, TranscriptsPath, config.Scope{Kind: config.ScopeChannel, ChannelID: &ch.ID}); err != nil || !on {
		return nil
	}
	return &Transcript{
		Conversation: uuid.NewSHA1(transcriptSpace, []byte(ch.ID.String()+"|"+owner+"|"+who.Conversation.String())),
		Page:         page, Question: strings.TrimSpace(question), Model: s.deps.Model.ModelName(ctx),
	}
}

func (t *Transcript) round(turn ai.ChatTurn) {
	if t == nil {
		return
	}
	rd := Round{Thinking: shorten(turn.Thinking, maxTranscriptThinking), Text: turn.Text}
	for _, tc := range turn.ToolCalls {
		rd.Calls = append(rd.Calls, Call{Tool: tc.Name, Input: tc.Input})
	}
	t.Rounds = append(t.Rounds, rd)
}

func (t *Transcript) result(i int, res ai.ToolResult) {
	if t == nil || len(t.Rounds) == 0 {
		return
	}
	if calls := t.Rounds[len(t.Rounds)-1].Calls; i < len(calls) {
		calls[i].Output, calls[i].IsError = shorten(res.Content, maxTranscriptOutput), res.IsError
		if orderTool[calls[i].Tool] && !res.IsError {
			calls[i].Output = orderOutputWithheld
		}
	}
}

// Order details belong behind the orders permission, not in a transcript staff with assistant.view can read, nor in the AI traces
var orderTool = map[string]bool{"get_my_orders": true, "get_my_order": true, "start_return": true}

const orderOutputWithheld = "(the shopper's order details - not kept in transcripts)"

// settle finishes the record once the turn is answered. A turn that finished stopped on its last round, so any calls that round asked for never ran
func (t *Transcript) settle(turn Turn, used ai.Usage, spoke int) *Transcript {
	t.Reply, t.Recap, t.FollowUps, t.Usage = turn.Reply, turn.Recap, append([]string{}, turn.FollowUps...), used
	if spoke >= 0 && spoke < len(t.Rounds) {
		t.Rounds[spoke].Shown = true
	}
	if n := len(t.Rounds); n > 0 && !turn.Incomplete {
		for i := range t.Rounds[n-1].Calls {
			t.Rounds[n-1].Calls[i].Skipped = true
		}
	}
	return t
}

package signalsapi

import (
	"bytes"
	"encoding/json"
	"slices"
	"unicode/utf8"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// The most a kept assistant turn may hold - core trims a transcript to fit, and Signals refuses one past them
const (
	MaxAssistantTurnText    = 20000 // runes, the question, the reply and the recap each
	MaxAssistantTurnMeta    = 2000  // runes, the page, the model and each follow-up
	MaxAssistantFollowUps   = 5
	MaxAssistantRounds      = 12
	MaxAssistantRoundsBytes = 512 << 10
)

// AssistantTurnFits is the check Signals makes before keeping a turn - the rounds' bytes are AssistantRoundsJSON's to measure
func AssistantTurnFits(t *signalsv1.AssistantTurnRecord) bool {
	long := func(s string, limit int) bool { return utf8.RuneCountInString(s) > limit }
	if long(t.GetQuestion(), MaxAssistantTurnText) || long(t.GetReply(), MaxAssistantTurnText) || long(t.GetRecap(), MaxAssistantTurnText) ||
		long(t.GetPage(), MaxAssistantTurnMeta) || long(t.GetModel(), MaxAssistantTurnMeta) ||
		len(t.GetRounds()) > MaxAssistantRounds || len(t.GetFollowUps()) > MaxAssistantFollowUps {
		return false
	}
	return !slices.ContainsFunc(t.GetFollowUps(), func(f string) bool { return long(f, MaxAssistantTurnMeta) })
}

// AssistantRoundsJSON is the rounds as Signals keeps them, one compact array - both sides measure the cap on this
func AssistantRoundsJSON(rounds []*signalsv1.AssistantRound) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, r := range rounds {
		if i > 0 {
			buf.WriteByte(',')
		}
		raw, err := protojson.Marshal(r)
		if err != nil {
			return nil, err
		}
		if err := json.Compact(&buf, raw); err != nil {
			return nil, err
		}
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

// FitAssistantTurn trims a turn to what Signals keeps rather than have it refused whole. The question and the answer only lose
// a tail past the limit, and when the rounds run long it's the longest tool output or thinking that gives way, a half at a time
func FitAssistantTurn(rec *signalsv1.AssistantTurnRecord) {
	rec.Question, rec.Reply = clipRunes(rec.Question, MaxAssistantTurnText), clipRunes(rec.Reply, MaxAssistantTurnText)
	rec.Recap = clipRunes(rec.Recap, MaxAssistantTurnText)
	rec.Page, rec.Model = clipRunes(rec.Page, MaxAssistantTurnMeta), clipRunes(rec.Model, MaxAssistantTurnMeta)
	rec.FollowUps = rec.FollowUps[:min(len(rec.FollowUps), MaxAssistantFollowUps)]
	for i, f := range rec.FollowUps {
		rec.FollowUps[i] = clipRunes(f, MaxAssistantTurnMeta)
	}
	if len(rec.Rounds) > MaxAssistantRounds {
		rec.Rounds = rec.Rounds[:MaxAssistantRounds]
	}
	for roundsSize(rec.Rounds) > MaxAssistantRoundsBytes {
		longest := longestField(rec.Rounds)
		if longest == nil || len(*longest) < minTrimmed {
			return
		}
		*longest = clipRunes(*longest, utf8.RuneCountInString(*longest)/2)
	}
}

const minTrimmed = 64

func roundsSize(rounds []*signalsv1.AssistantRound) int {
	b, err := AssistantRoundsJSON(rounds)
	if err != nil {
		return 0
	}
	return len(b)
}

func longestField(rounds []*signalsv1.AssistantRound) *string {
	var longest *string
	consider := func(f *string) {
		if longest == nil || len(*f) > len(*longest) {
			longest = f
		}
	}
	for _, r := range rounds {
		consider(&r.Thinking)
		for _, c := range r.Calls {
			consider(&c.Output)
			consider(&c.InputJson)
		}
	}
	return longest
}

// The ellipsis counts, so a clipped string comes out at the limit, never one over
func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return string([]rune(s)[:n-1]) + "…"
}

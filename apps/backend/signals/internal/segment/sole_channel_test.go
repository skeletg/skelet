package segment

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

type rulesStore struct {
	Repository
	channels []uuid.UUID
	stored   json.RawMessage
}

func (r *rulesStore) RosterChannels(context.Context) ([]uuid.UUID, error) { return r.channels, nil }
func (r *rulesStore) FillRules(_ context.Context, _ uuid.UUID, _, raw json.RawMessage) (bool, error) {
	r.stored = raw
	return true, nil
}
func (r *rulesStore) SetRules(_ context.Context, id uuid.UUID, raw json.RawMessage) (Segment, error) {
	r.stored = raw
	return Segment{ID: id, Rules: raw}, nil
}

const oldSpendRule = `{"conditions":[{"kind":"total_spent_cents","op":"gte","value":10000},{"kind":"order_count","op":"gte","value":1}]}`

func TestAnOldSpendRuleIsCompletedOnAOneChannelInstall(t *testing.T) {
	only := uuid.New()
	store := &rulesStore{channels: []uuid.UUID{only}}
	seg, err := (&Service{repo: store}).fillSoleChannel(context.Background(), Segment{ID: uuid.New(), Rules: json.RawMessage(oldSpendRule)})
	if err != nil {
		t.Fatal(err)
	}
	if p := RulesProblem(seg.Rules); p != "" || store.stored == nil {
		t.Fatalf("after the fill the rules still can't run (%q), stored=%s - a one-channel install has only one answer", p, store.stored)
	}
	r, _ := ParseRules(seg.Rules)
	if r.Conditions[0].ChannelID != only.String() || r.Conditions[1].ChannelID != "" {
		t.Fatalf("filled %+v, want only the money condition on the install's channel", r.Conditions)
	}
}

func TestAnOldSpendRuleIsFlaggedNotGuessedAcrossChannels(t *testing.T) {
	store := &rulesStore{channels: []uuid.UUID{uuid.New(), uuid.New()}}
	seg, err := (&Service{repo: store}).fillSoleChannel(context.Background(), Segment{ID: uuid.New(), Rules: json.RawMessage(oldSpendRule)})
	if err != nil {
		t.Fatal(err)
	}
	if store.stored != nil || RulesProblem(seg.Rules) == "" {
		t.Fatalf("with two channels the rule was changed (%s) or stopped reporting its problem - which currency it meant is the merchant's call", store.stored)
	}
}

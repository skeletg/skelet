package segment

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// The switch has to cut exactly the segments Signals feeds - too much drops an operator's hand-built audience, too little leaves promotions gated on a membership nothing recomputes

func TestSignalsMaintainedIdentifiesWhoOwnsMembership(t *testing.T) {
	for name, tc := range map[string]struct {
		rules      string
		refreshSQL string
		want       bool
	}{
		"a trait rule":                    {`{"conditions":[{"kind":"trait","field":"rfm_m","op":"gte","value":4}]}`, "", true},
		"a lifecycle rule":                {`{"conditions":[{"kind":"lifecycle","op":"eq","stage":"at_risk"}]}`, "", true},
		"one synapse rule among others":   {`{"logic":"or","conditions":[{"kind":"group","slug":"wholesale"},{"kind":"lifecycle","op":"eq","stage":"new"}]}`, "", true},
		"a synapse-defined segment":       {"", `{"channelId":"3f1d1e64-0e8f-4a1e-9c4f-2f1f6c9d5a11","conditions":[]}`, true},
		"spend and recency":               {`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":50000,"days":365},{"kind":"last_order_days","op":"lte","value":30}]}`, "", false},
		"order count":                     {`{"conditions":[{"kind":"order_count","op":"gte","value":3,"days":90}]}`, "", false},
		"consent":                         {`{"conditions":[{"kind":"consent","channel":"marketing_email","granted":true}]}`, "", false},
		"a customer group":                {`{"conditions":[{"kind":"group","slug":"wholesale"}]}`, "", false},
		"a static segment":                {"", "", false},
		"the reserved refresh_sql column": {"", "SELECT id FROM customers", false},
		// Rules that no longer parse never compiled, so there is no computed membership to distrust
		"unparseable rules": {`{"conditions":[{"kind":"vibes"}]}`, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := SignalsMaintained(json.RawMessage(tc.rules), tc.refreshSQL); got != tc.want {
				t.Errorf("SignalsMaintained = %v, want %v", got, tc.want)
			}
		})
	}
}

// Only MembershipsForCustomer is reached, so the embedded nil interface is never called
type stubRepo struct {
	Repository
	rows []Membership
}

func (r stubRepo) MembershipsForCustomer(context.Context, uuid.UUID) ([]Membership, error) {
	return r.rows, nil
}

func TestSlugsForCustomerDropsSignalsSegmentsWhenTheSuiteIsOff(t *testing.T) {
	rows := []Membership{
		{Slug: "vip", Rules: json.RawMessage(`{"conditions":[{"kind":"trait","field":"rfm_m","op":"gte","value":4}]}`)},
		{Slug: "big-spenders", Rules: json.RawMessage(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":50000}]}`)},
		{Slug: "browsed-tyres", RefreshSQL: `{"channelId":"3f1d1e64-0e8f-4a1e-9c4f-2f1f6c9d5a11"}`},
		{Slug: "hand-picked"}, // assigned by an operator, nothing recomputes it either way
	}
	for name, tc := range map[string]struct {
		signals bool
		want    []string
	}{
		"with signals on":  {true, []string{"vip", "big-spenders", "browsed-tyres", "hand-picked"}},
		"with signals off": {false, []string{"big-spenders", "hand-picked"}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &Service{repo: stubRepo{rows: rows}}
			svc.SetSignalsEnabled(tc.signals)
			got, err := svc.SlugsForCustomer(context.Background(), uuid.New())
			if err != nil {
				t.Fatalf("SlugsForCustomer: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

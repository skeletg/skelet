package segment

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// Audiences are a Signals capability, so the switch cuts all of them - a promotion or CMS block gated on one simply stops offering it rather than acting on a frozen snapshot

// Only MembershipsForCustomer is reached, so the embedded nil interface is never called
type stubRepo struct {
	Repository
	rows []Membership
}

func (r stubRepo) MembershipsForCustomer(context.Context, uuid.UUID) ([]Membership, error) {
	return r.rows, nil
}

func TestNoSegmentsAtAllWhenTheSuiteIsOff(t *testing.T) {
	rows := []Membership{
		{Slug: "vip", Rules: json.RawMessage(`{"conditions":[{"kind":"trait","field":"rfm_m","op":"gte","value":4}]}`)},
		{Slug: "big-spenders", Rules: json.RawMessage(`{"conditions":[{"kind":"total_spent_cents","op":"gte","value":50000}]}`)},
		{Slug: "hand-picked"}, // assigned by an operator, and still an audience
	}
	for name, tc := range map[string]struct {
		signals bool
		want    int
	}{
		"with signals on":  {true, 3},
		"with signals off": {false, 0},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &Service{repo: stubRepo{rows: rows}}
			svc.SetSignalsEnabled(tc.signals)
			got, err := svc.SlugsForCustomer(context.Background(), uuid.New())
			if err != nil {
				t.Fatalf("SlugsForCustomer: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("got %v, want %d slugs - an audience is a Signals capability, so with the suite off there are none", got, tc.want)
			}
		})
	}
}

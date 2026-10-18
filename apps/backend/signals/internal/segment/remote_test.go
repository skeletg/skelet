package segment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type failingSegments struct{ Repository }

func (failingSegments) SegmentsForCustomer(context.Context, uuid.UUID) ([]MemberSegment, error) {
	return nil, errors.New("connection refused")
}

// An empty answer would read as "in no segment", which a negated promotion condition takes as a match
func TestAFailedMembershipReadIsAnErrorNotAnEmptyList(t *testing.T) {
	svc := &Service{repo: failingSegments{}}
	if refs, err := svc.SegmentRefsForCustomer(context.Background(), uuid.New()); err == nil {
		t.Fatalf("refs = %v with no error", refs)
	}
	if _, err := svc.SlugsForCustomer(context.Background(), uuid.New()); err == nil {
		t.Fatal("slugs answered without the failure")
	}
}

func TestRulesAreCapped(t *testing.T) {
	conds := `{"kind":"group","slug":"g"}`
	raw := `{"conditions":[` + conds
	for range maxConditions {
		raw += "," + conds
	}
	if _, err := ParseRules([]byte(raw + `]}`)); !errors.Is(err, ErrInvalid) {
		t.Errorf("%d conditions: err = %v, want ErrInvalid", maxConditions+1, err)
	}
}

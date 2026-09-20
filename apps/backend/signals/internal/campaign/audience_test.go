package campaign

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type fakeAudience struct {
	contacts []Contact
	optedIn  map[uuid.UUID]bool
	asked    []uuid.UUID
}

func (f *fakeAudience) ListForChannel(_ context.Context, channelID uuid.UUID) ([]Contact, error) {
	out := []Contact{}
	for _, c := range f.contacts {
		if c.ChannelID == channelID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeAudience) Get(context.Context, uuid.UUID) (Contact, bool, error) {
	return Contact{}, false, nil
}

func (f *fakeAudience) OptedInByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	f.asked = append(f.asked, ids...)
	return f.optedIn, nil
}

type fakeSegments struct{ members []uuid.UUID }

func (f fakeSegments) SegmentMemberIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return f.members, nil
}

// Audience resolution is the whole difference between a broadcast reaching the right list and the
// wrong one, and every rule it applies is a reason someone must not be mailed
func TestAudienceResolutionKeepsOnlyWhoCanBeMailed(t *testing.T) {
	ch, other, group := uuid.New(), uuid.New(), uuid.New()
	mailable := Contact{ID: uuid.New(), ChannelID: ch, Email: "ada@test", Active: true, GroupID: group}
	inactive := Contact{ID: uuid.New(), ChannelID: ch, Email: "gone@test", Active: false, GroupID: group}
	noEmail := Contact{ID: uuid.New(), ChannelID: ch, Email: "   ", Active: true, GroupID: group}
	otherShop := Contact{ID: uuid.New(), ChannelID: other, Email: "eu@test", Active: true, GroupID: group}
	ungrouped := Contact{ID: uuid.New(), ChannelID: ch, Email: "solo@test", Active: true}

	aud := &fakeAudience{contacts: []Contact{mailable, inactive, noEmail, otherShop, ungrouped}}
	s := &Service{contacts: aud, segments: fakeSegments{members: []uuid.UUID{mailable.ID}}}

	got, err := s.resolveAudience(context.Background(), ch, AudienceAll, nil)
	if err != nil {
		t.Fatalf("resolve all: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("audience all = %d, want the two mailable contacts on this channel", len(got))
	}
	for _, c := range got {
		if c.email == "gone@test" || c.email == "eu@test" || c.email == "   " {
			t.Errorf("%q made it into the audience", c.email)
		}
	}

	got, err = s.resolveAudience(context.Background(), ch, AudienceGroup, &group)
	if err != nil {
		t.Fatalf("resolve group: %v", err)
	}
	if len(got) != 1 || got[0].customerID != mailable.ID {
		t.Errorf("group audience = %+v, want only the grouped, active, addressable one", got)
	}

	seg := uuid.New()
	got, err = s.resolveAudience(context.Background(), ch, AudienceSegment, &seg)
	if err != nil {
		t.Fatalf("resolve segment: %v", err)
	}
	if len(got) != 1 || got[0].customerID != mailable.ID {
		t.Errorf("segment audience = %+v, want only the member", got)
	}
}

// An estimate that counted the opted-out would tell an operator they are about to reach people they
// are not allowed to reach
func TestTheEstimateCountsOnlyTheOptedIn(t *testing.T) {
	ch := uuid.New()
	yes := Contact{ID: uuid.New(), ChannelID: ch, Email: "yes@test", Active: true}
	no := Contact{ID: uuid.New(), ChannelID: ch, Email: "no@test", Active: true}

	aud := &fakeAudience{
		contacts: []Contact{yes, no},
		optedIn:  map[uuid.UUID]bool{yes.ID: true, no.ID: false},
	}
	s := &Service{contacts: aud}

	total, eligible, err := s.EstimateAudience(context.Background(), ch, AudienceAll, nil)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want both addressable contacts", total)
	}
	if eligible != 1 {
		t.Errorf("eligible = %d, want only the opted-in one", eligible)
	}
	if len(aud.asked) != 2 {
		t.Errorf("consent asked for %d ids, want it resolved fresh for the whole candidate set", len(aud.asked))
	}
}

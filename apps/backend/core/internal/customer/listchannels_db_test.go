package customer

import (
	"testing"

	"github.com/google/uuid"
)

func TestTheCustomerListIsNarrowedToTheChannelsInTheQuery(t *testing.T) {
	f := newFixture(t)
	a, b := f.newChannel(t), f.newChannel(t)
	mine := f.register(t, a, randomEmail(), "password123")
	f.register(t, b, randomEmail(), "password123")

	got, err := f.svc.ListInChannels(f.ctx, []uuid.UUID{a})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != mine.ID {
		t.Fatalf("listed %d customers for channel a, want only its one", len(got))
	}
	if none, err := f.svc.ListInChannels(f.ctx, []uuid.UUID{}); err != nil || len(none) != 0 {
		t.Fatalf("no permitted channels listed %d, %v, want none", len(none), err)
	}
}

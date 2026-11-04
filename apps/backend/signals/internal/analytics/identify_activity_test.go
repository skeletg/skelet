package analytics

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type stitchSpine struct {
	appendSpine
	stitched map[string]uuid.UUID
}

func (s *stitchSpine) UpsertIdentity(_ context.Context, anon string, cust, _ uuid.UUID) error {
	s.stitched[anon] = cust
	return nil
}

func TestASignInWithAVisitorIDStitchesIt(t *testing.T) {
	spine := &stitchSpine{stitched: map[string]uuid.UUID{}}
	rec := NewRecorder(nil, spine)
	cust, ch := uuid.New(), uuid.New()
	ctx := context.Background()

	if _, err := rec.RecordActivity(ctx, ActivitySpec{EventName: "login", Source: SourceAccount, ChannelID: ch, CustomerID: &cust, AnonID: "anon-1", DedupeKey: "a"}); err != nil {
		t.Fatal(err)
	}
	if spine.stitched["anon-1"] != cust {
		t.Fatalf("stitched = %v, want anon-1 on the signed-in customer", spine.stitched)
	}
	// Commerce events carry both ids too, but only an account event is a sign-in
	if _, err := rec.RecordActivity(ctx, ActivitySpec{EventName: "cart_add", Source: SourceCommerce, ChannelID: ch, CustomerID: &cust, AnonID: "anon-2", DedupeKey: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := spine.stitched["anon-2"]; ok {
		t.Fatal("a cart event stitched a visitor")
	}
}

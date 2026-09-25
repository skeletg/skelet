package giftcard

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func TestAWrongChannelCardIsATypedError(t *testing.T) {
	if got := apperr.ClassOf(fmt.Errorf("place: %w", ErrWrongChannel)); got != apperr.ClassNotFound {
		t.Fatalf("class = %v, want NOT_FOUND", got)
	}
	if got := apperr.CodeOf(ErrWrongChannel); got != "GIFT_CARD_NOT_FOUND" {
		t.Fatalf("code = %q", got)
	}
}

func TestAChannelCardSpendsOnlyOnItsChannel(t *testing.T) {
	home, other := uuid.New(), uuid.New()
	card := activeCardCode("GC-C", 3000)
	card.ChannelID = &home
	r := &fakeRepo{byCode: map[string]GiftCard{"GC-C": card}}
	if _, err := redeemUpToTx(context.Background(), r, "GC-C", other, 1000, "USD", time.Now(), nil); !errors.Is(err, ErrWrongChannel) {
		t.Fatalf("another channel redeemed the card: %v", err)
	}
	if r.balanceTo != nil || len(r.txns) != 0 {
		t.Fatal("a refused redemption still moved the balance")
	}
	if applied, err := redeemUpToTx(context.Background(), r, "GC-C", home, 1000, "USD", time.Now(), nil); err != nil || applied != 1000 {
		t.Fatalf("its own channel = %d, %v", applied, err)
	}

	if card.SpendsOn(other) || !card.SpendsOn(home) {
		t.Fatal("a channel card's scope is wrong")
	}
	everywhere := activeCardCode("GC-G", 500)
	if !everywhere.SpendsOn(other) {
		t.Fatal("a card with no channel refused a channel")
	}
}

package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func TestRequireCustomerForCurrentStore_RejectsCrossStoreReplay(t *testing.T) {
	storeA, storeB := uuid.New(), uuid.New()
	customerID := uuid.New()
	tokenA := &auth.CustomerClaims{CustomerID: customerID, ChannelID: storeA, Email: "c@example.com"}

	onA := WithChannel(auth.WithCustomer(context.Background(), tokenA), &Channel{ID: storeA})
	if _, err := RequireCustomerForCurrentStore(onA); err != nil {
		t.Errorf("own store = %v, want nil", err)
	}

	onB := WithChannel(auth.WithCustomer(context.Background(), tokenA), &Channel{ID: storeB})
	if _, err := RequireCustomerForCurrentStore(onB); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("store-A token replayed on store B = %v, want auth.ErrUnauthenticated", err)
	}

	// No resolved channel at all is also unauthenticated - never "trust the token's own channel"
	noStore := auth.WithCustomer(context.Background(), tokenA)
	if _, err := RequireCustomerForCurrentStore(noStore); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("no resolved channel = %v, want auth.ErrUnauthenticated", err)
	}
}

func TestOptionalCustomerID_WrongStoreDegradesToGuest(t *testing.T) {
	storeA, storeB := uuid.New(), uuid.New()
	customerID := uuid.New()
	claims := &auth.CustomerClaims{CustomerID: customerID, ChannelID: storeA}

	onA := WithChannel(auth.WithCustomer(context.Background(), claims), &Channel{ID: storeA})
	if got := OptionalCustomerID(onA); got == nil || *got != customerID {
		t.Errorf("own store = %v, want %s", got, customerID)
	}
	onB := WithChannel(auth.WithCustomer(context.Background(), claims), &Channel{ID: storeB})
	if got := OptionalCustomerID(onB); got != nil {
		t.Errorf("wrong store = %v, want nil (guest)", got)
	}
	if got := OptionalCustomerID(context.Background()); got != nil {
		t.Errorf("no claims = %v, want nil", got)
	}
}

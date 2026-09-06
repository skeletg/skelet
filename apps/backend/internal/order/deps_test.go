package order

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/events"
)

type stubRepricer struct{}

func (stubRepricer) RepriceCartLines(context.Context, cart.Cart) cart.Cart { return cart.Cart{} }

type stubOutbox struct{}

func (stubOutbox) Enqueue(context.Context, *sql.Tx, events.Event) error { return nil }

type stubAuthorizer struct{}

func (stubAuthorizer) Authorize(context.Context, PaymentAuthorizeInput) (PaymentAuthorizeOutcome, error) {
	return PaymentAuthorizeOutcome{}, nil
}

// These three were Set* seams, so leaving one out was legal and silent - a nil Repricer charges the add-time snapshot price, a nil Outbox
// drops placement events on a crash, and a nil PaymentAuthorizer leaves orders unpaid
func TestNewServiceRefusesUnwiredSeams(t *testing.T) {
	seams := []string{"Repricer", "Outbox", "PaymentAuthorizer"}

	_, err := NewService(Deps{})
	if err == nil {
		t.Fatal("an empty Deps built a service")
	}
	for _, name := range seams {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%q missing from %q", name, err)
		}
	}

	// With the seams supplied, only the datastore half is reported - so the check discriminates rather than
	// just refusing everything
	_, err = NewService(Deps{
		Repricer:          stubRepricer{},
		Outbox:            stubOutbox{},
		PaymentAuthorizer: stubAuthorizer{},
	})
	if err == nil {
		t.Fatal("a Deps with no repo built a service")
	}
	for _, name := range seams {
		if strings.Contains(err.Error(), name) {
			t.Errorf("%q reported as unwired though it was supplied: %v", name, err)
		}
	}
	for _, name := range []string{"Repo", "CartRepo", "Customer", "Tx", "Bus"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%q missing from %q", name, err)
		}
	}
}

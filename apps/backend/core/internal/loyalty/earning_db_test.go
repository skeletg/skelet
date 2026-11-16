package loyalty

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

func earnWith(t *testing.T, rule func(*Earning) error) (*Service, *orderState, uuid.UUID, uuid.UUID, *int) {
	t.Helper()
	svc, st, customerID, orderID := earnFixture(t, "USD", 2500)
	st.o.Lines = []order.OrderLine{{ProductSku: "COAT-1", Quantity: 1, TotalPrice: 1500}, {ProductSku: "TEE-1", Quantity: 2, TotalPrice: 1000}}
	st.set("pending", order.PaymentPaid)
	bus := events.NewBus()
	svc.SubscribeToOrderEvents(bus)
	calls := new(int)
	bus.SubscribeSync(EventEarning, 100, func(_ context.Context, e events.Event) error {
		*calls++
		return rule(e.(*Earning))
	})
	return svc, st, customerID, orderID, calls
}

func TestAnEarningRuleChangesThePoints(t *testing.T) {
	svc, _, customerID, orderID, calls := earnWith(t, func(e *Earning) error {
		if e.Points != 25 || e.UnitCents != 100 || e.MerchandiseCents != 2500 || len(e.Lines) != 2 {
			return errors.New("seeded wrong")
		}
		sku, _, cents := e.EarningLine(0)
		if sku == "COAT-1" {
			_ = e.SetEarningPoints(e.Points + cents/e.UnitCents*e.PointsPerUnit)
			e.AddEarningNote("double points on coats")
		}
		return nil
	})
	for range 2 {
		if err := paid(svc, orderID, order.PaymentPaid); err != nil {
			t.Fatal(err)
		}
	}
	if got := orderPoints(t, svc, orderID); got != 40 || *calls != 1 {
		t.Fatalf("%d points after %d calls, want 40 once - the replay is answered by the ledger", got, *calls)
	}
	entries, err := svc.Entries(context.Background(), customerID, 5)
	if err != nil || len(entries) != 1 || entries[0].Reason != "order 1001 - double points on coats" {
		t.Errorf("history %+v, %v", entries, err)
	}
}

func TestAnEarningRuleCanEarnNothing(t *testing.T) {
	svc, _, _, orderID, _ := earnWith(t, func(e *Earning) error { return e.SetEarningPoints(0) })
	if err := paid(svc, orderID, order.PaymentPaid); err != nil {
		t.Fatal(err)
	}
	if got := orderPoints(t, svc, orderID); got != 0 {
		t.Errorf("%d points, want none", got)
	}
}

func TestAFailingEarningRuleIsAskedAgain(t *testing.T) {
	down := true
	svc, _, _, orderID, calls := earnWith(t, func(*Earning) error {
		if down {
			return errors.New("rules service down")
		}
		return nil
	})
	if err := paid(svc, orderID, order.PaymentPaid); err == nil || orderPoints(t, svc, orderID) != 0 {
		t.Fatalf("a failing rule earned anyway: %v", err)
	}
	down = false
	if err := paid(svc, orderID, order.PaymentPaid); err != nil || orderPoints(t, svc, orderID) != 25 || *calls != 2 {
		t.Errorf("the redelivery: %v, %d points after %d calls", err, orderPoints(t, svc, orderID), *calls)
	}
}

func TestAnEarningWebhookAnswer(t *testing.T) {
	for body, want := range map[string]struct {
		points int64
		notes  string
	}{
		``:                                    {25, ""},
		`{"points": 60, "note": "vip"}`:       {60, "vip"},
		`{"note": "thanks"}`:                  {25, "thanks"},
		`{"reject": true, "reason": "staff"}`: {0, "staff"},
		`{"reject": true, "points": 90}`:      {0, ""},
	} {
		e := &Earning{Points: 25}
		if rejected, _, err := e.ApplyWebhookResponse([]byte(body)); rejected || err != nil {
			t.Errorf("%s: rejected %v, %v - a reject earns nothing rather than failing the earn", body, rejected, err)
		}
		if e.Points != want.points || strings.Join(e.Notes, ";") != want.notes {
			t.Errorf("%s: %d points, notes %q", body, e.Points, e.Notes)
		}
	}
	if _, _, err := (&Earning{}).ApplyWebhookResponse([]byte(`{"points": -5}`)); err == nil {
		t.Error("negative points were taken")
	}
}

func TestAnEarningRuleIsHeldToSaneAmounts(t *testing.T) {
	e := &Earning{}
	if err := e.SetEarningPoints(MaxEarningPoints + 1); err == nil {
		t.Error("an order earned past the ceiling")
	}
	for range maxEarningNotes + 2 {
		e.AddEarningNote(strings.Repeat("x", maxEarningNote+50))
	}
	if len(e.Notes) != maxEarningNotes || len([]rune(e.Notes[0])) != maxEarningNote {
		t.Errorf("%d notes, the first %d long", len(e.Notes), len([]rune(e.Notes[0])))
	}
}

// A plugin refusing in words meant for people earns nothing, as a webhook's reject does - only a fault is retried
func TestAnEarningRuleRefusingEarnsNothing(t *testing.T) {
	svc, _, _, orderID, calls := earnWith(t, func(*Earning) error {
		return apperr.New("ACME_NO_POINTS", apperr.ClassValidation, "no points on staff orders")
	})
	if err := paid(svc, orderID, order.PaymentPaid); err != nil || orderPoints(t, svc, orderID) != 0 || *calls != 1 {
		t.Errorf("a refusal: %v, %d points after %d calls", err, orderPoints(t, svc, orderID), *calls)
	}
}

func TestAnEarningLineSaysWhatItSold(t *testing.T) {
	cat := uuid.New()
	e := &Earning{Lines: []EarningLine{{SKU: "COAT-1", ProductID: uuid.New(), CategoryIDs: []uuid.UUID{cat}, Quantity: 2, UnitCents: 4500, TotalCents: 9000},
		{SKU: "SOCK-GIFT", Gift: true}}}
	if _, cats := e.EarningLineProduct(0); len(cats) != 1 || cats[0] != cat || e.EarningLineUnitCents(0) != 4500 || e.EarningLineGift(0) {
		t.Errorf("the coat line: %v, %d, gift %v", cats, e.EarningLineUnitCents(0), e.EarningLineGift(0))
	}
	if id, _ := e.EarningLineProduct(5); id != uuid.Nil || !e.EarningLineGift(1) {
		t.Error("a line past the end, or the gift")
	}
}

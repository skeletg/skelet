package order

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	dbsqlc "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type stageRig struct {
	svc    *Service
	mu     sync.Mutex
	moves  []OrderTransitioning
	refuse func(OrderTransitioning) error
	bus    *events.Bus
	seen   chan OrderStageChanged
}

func newStageRig(t *testing.T) (*stageRig, uuid.UUID) {
	t.Helper()
	conn := testdb.Require(t)
	r := &stageRig{bus: events.NewBus(), seen: make(chan OrderStageChanged, 8)}
	r.svc = &Service{repo: NewRepository(dbsqlc.New(conn)), tx: txrunner.New(conn), bus: r.bus}
	all := []Stage{
		{Key: "awaiting_stock", Label: "Awaiting stock", Within: []string{OrderStatusConfirmed, OrderStatusProcessing}, Plugin: "acme"},
		{Key: "packed", Label: "Packed", Within: []string{OrderStatusProcessing}, Plugin: "other"},
		{Key: "switched_off", Label: "Switched off", Within: []string{OrderStatusConfirmed, OrderStatusProcessing}, Plugin: "gone"},
	}
	r.svc.SetStages(func(context.Context, uuid.UUID) []Stage { return all[:2] }, func() []Stage { return all })
	r.bus.SubscribeSync(EventTransitioning, 100, func(_ context.Context, e events.Event) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		m := *e.(*OrderTransitioning)
		r.moves = append(r.moves, m)
		if r.refuse != nil {
			return r.refuse(m)
		}
		return nil
	})
	r.bus.Subscribe(EventStageChanged, func(_ context.Context, e events.Event) error {
		r.seen <- *e.(*OrderStageChanged)
		return nil
	})
	return r, seedConfirmedOrder(t, conn)
}

func (r *stageRig) lastMove() OrderTransitioning {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.moves[len(r.moves)-1]
}

func (r *stageRig) changed(t *testing.T) OrderStageChanged {
	t.Helper()
	select {
	case c := <-r.seen:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no order.stage_changed")
		return OrderStageChanged{}
	}
}

func TestAnOrderMovesThroughStagesWithItsStatus(t *testing.T) {
	r, id := newStageRig(t)
	ctx := context.Background()
	for key, code := range map[string]string{"packed": "ORDER_STAGE_NOT_HERE", "nope": "ORDER_STAGE_UNKNOWN"} {
		if _, err := r.svc.SetStage(ctx, id, key, ""); conflictCode(err) != code {
			t.Errorf("setting %s on a confirmed order: %v, want %s", key, err, code)
		}
	}

	o, err := r.svc.SetStage(ctx, id, "awaiting_stock", "")
	if err != nil || o.Stage != "awaiting_stock" {
		t.Fatalf("set: %v, stage %q", err, o.Stage)
	}
	if m := r.lastMove(); m.FromStatus != OrderStatusConfirmed || m.ToStatus != OrderStatusConfirmed || m.ToStage != "awaiting_stock" {
		t.Errorf("asked %+v", m)
	}
	if c := r.changed(t); c.FromStage != "" || c.ToStage != "awaiting_stock" {
		t.Errorf("announced %+v", c)
	}
	if !slices.ContainsFunc(o.Events, func(e OrderEvent) bool { return e.Message == "Stage set to Awaiting stock" }) {
		t.Errorf("the timeline has no stage row: %+v", o.Events)
	}

	if o, err = r.svc.UpdateStatus(ctx, id, OrderStatusProcessing); err != nil || o.Stage != "awaiting_stock" {
		t.Fatalf("processing a stage that sits there too: %v, stage %q", err, o.Stage)
	}
	r.refuse = func(m OrderTransitioning) error {
		if m.FromStage == "awaiting_stock" && m.ToStatus == OrderStatusCancelled {
			return errors.New("not while awaiting stock")
		}
		return nil
	}
	if _, err := r.svc.UpdateStatus(ctx, id, OrderStatusCancelled); err == nil {
		t.Fatal("a subscriber's refusal didn't stop the cancel")
	}
	r.refuse = nil

	if _, err := r.svc.SetStage(ctx, id, "packed", ""); err != nil {
		t.Fatal(err)
	}
	r.changed(t)
	if o, err = r.svc.UpdateStatus(ctx, id, OrderStatusCancelled); err != nil || o.Stage != "" {
		t.Fatalf("cancelling: %v, stage %q", err, o.Stage)
	}
	if m := r.lastMove(); m.FromStage != "packed" || m.ToStage != "" {
		t.Errorf("asked %+v", m)
	}
	if c := r.changed(t); c.Status != OrderStatusCancelled || c.FromStage != "packed" || c.ToStage != "" {
		t.Errorf("the cleared stage announced %+v", c)
	}
	if !slices.ContainsFunc(o.Events, func(e OrderEvent) bool { return e.Message == "Left the Packed stage" }) {
		t.Errorf("the timeline doesn't say the move cleared the stage: %+v", o.Events)
	}
}

// A plugin switched off on the channel takes no new orders into its stage, and the ones there stay its own
func TestAStageOutlivesItsPluginBeingSwitchedOff(t *testing.T) {
	r, id := newStageRig(t)
	ctx := context.Background()
	if _, err := r.svc.SetStage(ctx, id, "switched_off", ""); conflictCode(err) != "ORDER_STAGE_UNKNOWN" {
		t.Errorf("entering a stage the channel no longer offers: %v", err)
	}
	if err := r.svc.repo.SetOrderStage(ctx, id, "switched_off"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.SetStage(ctx, id, "", "acme"); conflictCode(err) != "ORDER_STAGE_NOT_YOURS" {
		t.Errorf("another plugin clearing it: %v", err)
	}
	o, err := r.svc.UpdateStatus(ctx, id, OrderStatusProcessing)
	if err != nil || o.Stage != "switched_off" {
		t.Errorf("a status the stage sits in too: %v, stage %q", err, o.Stage)
	}
}

func TestAPluginMovesOnlyItsOwnStages(t *testing.T) {
	r, id := newStageRig(t)
	ctx := context.Background()
	if _, err := r.svc.SetStage(ctx, id, "awaiting_stock", "other"); conflictCode(err) != "ORDER_STAGE_NOT_YOURS" {
		t.Errorf("another plugin's stage: %v", err)
	}
	if _, err := r.svc.SetStage(ctx, id, "awaiting_stock", "acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.SetStage(ctx, id, "", "other"); conflictCode(err) != "ORDER_STAGE_NOT_YOURS" {
		t.Errorf("clearing another plugin's stage: %v", err)
	}
	if o, err := r.svc.SetStage(ctx, id, "", "acme"); err != nil || o.Stage != "" {
		t.Errorf("clearing its own: %v, stage %q", err, o.Stage)
	}
}

func TestAHoldClearsAStageItDoesntHold(t *testing.T) {
	r, id := newStageRig(t)
	ctx := context.Background()
	if _, err := r.svc.SetStage(ctx, id, "awaiting_stock", ""); err != nil {
		t.Fatal(err)
	}
	r.changed(t)
	o, err := r.svc.HoldOrder(ctx, id, "")
	if err != nil || o.Status != OrderStatusOnHold || o.Stage != "" {
		t.Fatalf("hold: %v, %s/%q", err, o.Status, o.Stage)
	}
	if c := r.changed(t); c.Status != OrderStatusOnHold || c.FromStage != "awaiting_stock" {
		t.Errorf("announced %+v", c)
	}
	r.refuse = func(m OrderTransitioning) error {
		if m.FromStatus == OrderStatusOnHold {
			return errors.New("still reviewing")
		}
		return nil
	}
	if _, err := r.svc.ReleaseOrder(ctx, id); err == nil {
		t.Error("a subscriber's refusal didn't keep the hold")
	}
}

// A stage that moves between the veto's answer and the status write means the answer was about another order
func TestAStatusMoveRefusesAStageThatChangedUnderIt(t *testing.T) {
	r, id := newStageRig(t)
	ctx := context.Background()
	r.refuse = func(m OrderTransitioning) error {
		if m.ToStatus == OrderStatusProcessing {
			r.refuse = nil
			return r.svc.repo.SetOrderStage(ctx, id, "awaiting_stock")
		}
		return nil
	}
	if _, err := r.svc.UpdateStatus(ctx, id, OrderStatusProcessing); conflictCode(err) != "ORDER_STAGE_RACED" {
		t.Errorf("a stage set while the move was being decided: %v", err)
	}
}

// A handler moving the order it's being asked about is refused, rather than the two asking each other forever
func TestAMoveDecidedInsideAnotherIsRefused(t *testing.T) {
	r, id := newStageRig(t)
	var nested error
	asked := false
	r.bus.SubscribeSync(EventTransitioning, 200, func(ctx context.Context, e events.Event) error {
		if m := e.(*OrderTransitioning); m.ToStage == "awaiting_stock" && !asked {
			asked = true
			_, nested = r.svc.SetStage(ctx, id, "awaiting_stock", "")
		}
		return nil
	})
	if _, err := r.svc.SetStage(context.Background(), id, "awaiting_stock", ""); err != nil {
		t.Fatal(err)
	}
	if conflictCode(nested) != "ORDER_MOVE_NESTED" {
		t.Errorf("a move asked from inside the answer about it: %v", nested)
	}
}

// A plugin's number taken between its check and the insert names the order again in the store's own format
func TestAnOrderWhoseNumberWasTakenIsNamedAgain(t *testing.T) {
	r, seeded := newStageRig(t)
	ctx := context.Background()
	taken, err := r.svc.repo.GetOrderByID(ctx, seeded)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "T" + strings.ToUpper(uuid.NewString()[:6])
	r.svc.numbering = NewNumbering(fakeSettings{numberFormatPath: prefix + "-{seq}"}, counts{}.next, nil, nil)
	ch := &channel.Channel{ID: taken.ChannelID, Slug: "test"}
	created, err := r.svc.createOrder(ctx, r.svc.repo, ch, Order{ChannelID: taken.ChannelID, CurrencyCode: "USD", Status: OrderStatusPending,
		PaymentStatus: PaymentUnpaid, Number: taken.Number})
	if err != nil || created.Number != prefix+"-1" {
		t.Fatalf("an order given a taken number: %q, %v - want %s-1", created.Number, err, prefix)
	}
	conn := testdb.Require(t)
	t.Cleanup(func() { _, _ = conn.Exec(`DELETE FROM orders WHERE id = $1`, created.ID) })
}

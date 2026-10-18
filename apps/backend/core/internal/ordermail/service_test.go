package ordermail

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
)

type fakeOrders struct {
	o            order.Order
	err          error
	fulfillments []order.Fulfillment
	fulErr       error
}

func (f fakeOrders) GetByID(context.Context, uuid.UUID) (order.Order, error) { return f.o, f.err }
func (f fakeOrders) GetFulfillment(context.Context, uuid.UUID) (order.Fulfillment, error) {
	return order.Fulfillment{}, nil
}

func (f fakeOrders) ListFulfillments(context.Context, uuid.UUID) ([]order.Fulfillment, error) {
	return f.fulfillments, f.fulErr
}

func (f fakeOrders) PaymentLedger(context.Context, uuid.UUID) ([]order.PaymentTransaction, error) {
	return nil, nil
}

type fakeChannels struct {
	ch  channel.Channel
	err error
}

func (f fakeChannels) GetByID(context.Context, uuid.UUID) (channel.Channel, error) {
	return f.ch, f.err
}

// The guest path: a placed order with no customer id keys on the order email, so uuid.Nil is a
// valid value downstream rather than a bug
func TestCustomerIDOrNil(t *testing.T) {
	if got := customerIDOrNil(nil); got != uuid.Nil {
		t.Errorf("guest should resolve to uuid.Nil, got %v", got)
	}
	id := uuid.New()
	if got := customerIDOrNil(&id); got != id {
		t.Errorf("known customer should pass through, got %v want %v", got, id)
	}
}

// A half-known order renders a half-filled receipt, which is worse than a missing one
func TestContextSkipsOnLookupFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		svc  *Service
	}{
		{"order lookup fails", &Service{orders: fakeOrders{err: errors.New("gone")}, channels: fakeChannels{}}},
		{"channel lookup fails", &Service{orders: fakeOrders{}, channels: fakeChannels{err: errors.New("gone")}}},
	} {
		if _, _, err := tc.svc.context(context.Background(), uuid.New()); err == nil || dropSkip(err) == nil {
			t.Errorf("%s: context = %v, want an error the outbox retries", tc.name, err)
		}
	}
}

func TestContextResolvesOrderAndChannel(t *testing.T) {
	chID := uuid.New()
	svc := &Service{
		orders:   fakeOrders{o: order.Order{Email: "guest@example.com", ChannelID: chID}},
		channels: fakeChannels{ch: channel.Channel{ID: chID, Name: "Shop"}},
	}
	o, ch, err := svc.context(context.Background(), uuid.New())
	if err != nil {
		t.Fatal("context failed on two good lookups")
	}
	if o.Email != "guest@example.com" {
		t.Errorf("order email = %q, want the address the receipt goes to", o.Email)
	}
	if ch.ID != chID {
		t.Errorf("channel = %v, want the order's own %v", ch.ID, chID)
	}
}

type fakeDispatcher struct{ took, asked int }

func (f *fakeDispatcher) answer() int { f.asked++; return f.took }

func TestDeliverReadsTheSendLogBeforeAskingAFlow(t *testing.T) {
	log := newFakeSendLog()
	d := &fakeDispatcher{}
	svc := &Service{sendLog: log}
	id := uuid.New()
	sends := 0
	send := func() error { sends++; return nil }
	enroll := func(FlowDispatcher) int { return d.answer() }

	_ = svc.deliver(context.Background(), id, kindConfirmation, "", enroll, send)
	svc.flows = stubDispatcher{}
	d.took = 1
	_ = svc.deliver(context.Background(), id, kindConfirmation, "", enroll, send)
	if sends != 1 || d.asked != 0 {
		t.Fatalf("sends=%d flow asks=%d, want core's one receipt and no flow asked on the replay", sends, d.asked)
	}

	other := uuid.New()
	_ = svc.deliver(context.Background(), other, kindConfirmation, "", enroll, send)
	if sends != 1 || d.asked != 1 {
		t.Fatalf("sends=%d flow asks=%d, want the flow to take a fresh order", sends, d.asked)
	}
}

type stubDispatcher struct{ FlowDispatcher }

// Without a dispatcher nothing else can be sending, so every order email is this package's
func TestFlowTookIsFalseWithoutADispatcher(t *testing.T) {
	s := &Service{}
	if s.flowTook(func(FlowDispatcher) int { return 99 }) {
		t.Error("no dispatcher wired, but flowTook reported a flow had it")
	}
}

// One row per (order, kind, ref), like the table's unique key
type fakeSendLog struct {
	rows     map[string]bool
	readErr  error
	writeErr error
}

func newFakeSendLog() *fakeSendLog { return &fakeSendLog{rows: map[string]bool{}} }

func (f *fakeSendLog) key(orderID uuid.UUID, kind, ref string) string {
	return orderID.String() + "|" + kind + "|" + ref
}

func (f *fakeSendLog) AlreadySent(_ context.Context, orderID uuid.UUID, kind, ref string) (bool, error) {
	if f.readErr != nil {
		return false, f.readErr
	}
	return f.rows[f.key(orderID, kind, ref)], nil
}

func (f *fakeSendLog) RecordSent(_ context.Context, orderID uuid.UUID, kind, ref string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.rows[f.key(orderID, kind, ref)] = true
	return nil
}

// The outbox redelivers an event whose handler finished but whose delivery was never marked done, and
// the customer reads a second receipt as a second order
func TestSendOnceSuppressesAReplay(t *testing.T) {
	log := newFakeSendLog()
	svc := &Service{sendLog: log}
	id := uuid.New()
	sends := 0
	send := func() error { sends++; return nil }

	_ = svc.deliver(context.Background(), id, kindConfirmation, "", nil, send)
	_ = svc.deliver(context.Background(), id, kindConfirmation, "", nil, send)
	if sends != 1 {
		t.Fatalf("a replayed event sent %d times, want 1", sends)
	}
}

// Every shipment on an order is its own email, so the fulfillment id has to reach the key
func TestSendOnceKeepsRefsApart(t *testing.T) {
	svc := &Service{sendLog: newFakeSendLog()}
	id := uuid.New()
	sends := 0
	send := func() error { sends++; return nil }

	_ = svc.deliver(context.Background(), id, kindShipment, uuid.New().String(), nil, send)
	_ = svc.deliver(context.Background(), id, kindShipment, uuid.New().String(), nil, send)
	_ = svc.deliver(context.Background(), id, kindCancellation, "", nil, send)
	if sends != 3 {
		t.Fatalf("distinct emails sent %d times, want 3", sends)
	}
}

// A row written for mail that never left would suppress the real one
func TestSendOnceDoesNotRecordAFailedSend(t *testing.T) {
	log := newFakeSendLog()
	svc := &Service{sendLog: log}
	id := uuid.New()
	sends := 0
	_ = svc.deliver(context.Background(), id, kindConfirmation, "", nil, func() error { sends++; return errors.New("smtp down") })
	if len(log.rows) != 0 {
		t.Fatalf("a failed send was recorded: %v", log.rows)
	}
	_ = svc.deliver(context.Background(), id, kindConfirmation, "", nil, func() error { sends++; return nil })
	if sends != 2 {
		t.Fatalf("the retry sent %d times total, want 2", sends)
	}
}

// A missing receipt costs more than a duplicate, so an unreadable log sends
func TestSendOnceSendsWhenTheLogIsUnreadable(t *testing.T) {
	svc := &Service{sendLog: &fakeSendLog{rows: map[string]bool{}, readErr: errors.New("db down")}}
	sends := 0
	_ = svc.deliver(context.Background(), uuid.New(), kindConfirmation, "", nil, func() error { sends++; return nil })
	if sends != 1 {
		t.Fatalf("sent %d times with an unreadable log, want 1", sends)
	}
}

// A service built without a log still sends, rather than dropping the mail silently
func TestSendOnceSendsWithoutALog(t *testing.T) {
	svc := &Service{}
	sends := 0
	_ = svc.deliver(context.Background(), uuid.New(), kindConfirmation, "", nil, func() error { sends++; return nil })
	if sends != 1 {
		t.Fatalf("sent %d times with no log, want 1", sends)
	}
}

// A transient send failure returned nil, so the outbox never retried and the customer never got the mail
func TestAFailedSendIsRetriedButAMissingAddressIsNot(t *testing.T) {
	svc := &Service{}
	if err := svc.deliver(context.Background(), uuid.New(), kindConfirmation, "", nil, func() error { return errors.New("smtp down") }); err == nil {
		t.Error("a failed send returned nil, so it was never delivered again")
	}
	noAddr := func() error { return fmt.Errorf("%w: 1001", errNoAddress) }
	if err := svc.deliver(context.Background(), uuid.New(), kindConfirmation, "", nil, noAddr); err != nil {
		t.Errorf("an order with no address asked for a retry (%v) - none will find one", err)
	}
}

func TestAGoneOrderSendsNothingAndIsNotRetried(t *testing.T) {
	svc := &Service{orders: fakeOrders{err: order.ErrOrderNotFound}, channels: fakeChannels{}}
	_, _, err := svc.context(context.Background(), uuid.New())
	if dropSkip(err) != nil {
		t.Fatalf("a deleted order = %v, want it skipped without a retry", err)
	}
}

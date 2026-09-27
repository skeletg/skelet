package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/order"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

func freshHost(t *testing.T) *events.Bus {
	t.Helper()
	sdk.ResetForTest()
	resetHostForTest()
	t.Cleanup(func() {
		sdk.ResetForTest()
		resetHostForTest()
	})
	Install()
	bus := events.NewBus()
	SetEventBus(bus)
	return bus
}

// A plain subscriber's failure - returned or panicked - stays its own: handing it to the outbox would replay the event for every other subscriber
func TestAPlainSubscribersFailureNeverReplaysTheEvent(t *testing.T) {
	for name, handler := range map[string]sdk.AsyncHandler{
		"error": func(context.Context, json.RawMessage) error { return errors.New("down") },
		"panic": func(context.Context, json.RawMessage) error { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			bus := freshHost(t)
			sdk.NewPluginForTest("plain").SubscribeAsync("test.event", handler)
			if err := bus.DispatchAsyncNow(context.Background(), &fakeEvent{}); err != nil {
				t.Fatalf("dispatch = %v, want nil - the outbox would replay every subscriber", err)
			}
		})
	}
}

// Once's promise: the failure goes back for redelivery with the claim released, a panic included, and the redelivery runs it
func TestAOnceSubscribersFailureIsRedelivered(t *testing.T) {
	for name, fail := range map[string]func() error{
		"error": func() error { return errors.New("down") },
		"panic": func() error { panic("boom") },
	} {
		t.Run(name, func(t *testing.T) {
			bus := freshHost(t)
			SetEventDedup(&memDedup{claimed: map[string]bool{}})
			runs := 0
			sdk.NewPluginForTest("once").SubscribeAsyncOnce("test.event",
				func(json.RawMessage) (string, error) { return "k", nil },
				func(context.Context, json.RawMessage) error {
					runs++
					if runs == 1 {
						return fail()
					}
					return nil
				})
			if err := bus.DispatchAsyncNow(context.Background(), &fakeEvent{}); err == nil {
				t.Fatal("a failed once-handler reported success - the outbox would mark the event delivered and never retry")
			}
			if err := bus.DispatchAsyncNow(context.Background(), &fakeEvent{}); err != nil {
				t.Fatalf("redelivery: %v", err)
			}
			if runs != 2 {
				t.Fatalf("handler ran %d times, want the failure then the redelivery - a panic must release its claim", runs)
			}
		})
	}
}

func TestAnAlwaysSyncHandlerRunsWhileDisabled(t *testing.T) {
	bus := freshHost(t)
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(context.Context, string, uuid.UUID) bool { return false }

	var ran []string
	p := sdk.NewPluginForTest("veto")
	p.SubscribeSync("test.mutable", 100, func(context.Context, any) error { ran = append(ran, "plain"); return nil })
	p.SubscribeSyncAlways("test.mutable", 200, func(context.Context, any) error { ran = append(ran, "always"); return errors.New("refused") })
	if err := bus.PublishSync(context.Background(), &mutableEvent{}); err == nil {
		t.Fatal("the always-run veto was skipped with the plugin off")
	}
	if len(ran) != 1 || ran[0] != "always" {
		t.Fatalf("ran %v, want only the always-run handler", ran)
	}
}

type channelEvent struct{ ch uuid.UUID }

func (*channelEvent) Type() string                { return "test.channel" }
func (e *channelEvent) EventChannelID() uuid.UUID { return e.ch }

func TestTheKillSwitchReadsTheEventsChannel(t *testing.T) {
	bus := freshHost(t)
	off := uuid.New()
	orig := isEnabledFromContext
	t.Cleanup(func() { isEnabledFromContext = orig })
	isEnabledFromContext = func(_ context.Context, _ string, ch uuid.UUID) bool { return ch != off }

	runs := 0
	sdk.NewPluginForTest("scoped").SubscribeSync("test.channel", 100, func(context.Context, any) error { runs++; return nil })
	_ = bus.PublishSync(context.Background(), &channelEvent{ch: off})
	_ = bus.PublishSync(context.Background(), &channelEvent{ch: uuid.New()})
	if runs != 1 {
		t.Fatalf("handler ran %d times, want only on the channel where the plugin is on", runs)
	}
}

func TestASubscriptionMissingFromConsumesFailsBoot(t *testing.T) {
	freshHost(t)
	p := sdk.RegisterPlugin(sdk.Manifest{Slug: "forgetful", Version: "0.1.0", SDKRange: "^0.1.0", Emits: []string{"test.listed"}, Consumes: []string{"test.listed"}})
	p.SubscribeAsync("test.listed", func(context.Context, json.RawMessage) error { return nil })
	if err := ValidateManifests(); err != nil {
		t.Fatalf("listed subscription: %v", err)
	}
	p.SubscribeAsync("test.unlisted", func(context.Context, json.RawMessage) error { return nil })
	if err := ValidateManifests(); err == nil || !strings.Contains(err.Error(), "test.unlisted") {
		t.Fatalf("err = %v, want the unlisted subscription named", err)
	}
}

type oneCustomer struct{ c customer.Customer }

func (o oneCustomer) GetByIDWithAddresses(context.Context, uuid.UUID) (customer.Customer, error) {
	return o.c, nil
}

type noDrafts struct{ called bool }

func (n *noDrafts) CreateDraftOrder(context.Context, order.CreateDraftOrderInput) (order.Order, error) {
	n.called = true
	return order.Order{}, nil
}

func TestADraftForACustomerOnAnotherStoreIsRefused(t *testing.T) {
	freshHost(t)
	t.Cleanup(func() { draftOrders.Store(nil) })
	drafts := &noDrafts{}
	addr := customer.Address{ID: uuid.New(), FirstName: "A", Street1: "1 Road", City: "Town", CountryCode: "US"}
	SetDraftOrderCreator(drafts, oneCustomer{customer.Customer{ID: uuid.New(), ChannelID: uuid.New(), Email: "a@b.test", Addresses: []customer.Address{addr}}})

	_, err := installedHost.Load().CreateDraftOrder(context.Background(), sdk.DraftOrderRequest{
		ChannelID: uuid.New(), CustomerID: uuid.New(), Lines: []sdk.DraftOrderLine{{SKU: "S", Quantity: 1}},
	})
	if err == nil || drafts.called {
		t.Fatalf("err = %v, created = %v - want a refusal before any draft", err, drafts.called)
	}
}

func TestPluginConfigReaderRefusesAnySlash(t *testing.T) {
	r := &pluginConfigReader{prefix: "shipping/ups/"}
	if _, err := r.Get(context.Background(), "shipping/ups/api_key", uuid.Nil); err == nil {
		t.Error("a shipping full path double-prefixed instead of failing")
	}
}

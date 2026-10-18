package outbox

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

type probeEvent struct {
	ID uuid.UUID `json:"id"`
}

func (*probeEvent) Type() string { return "outbox.probe" }

func TestEnqueueRefusesAnUnregisteredType(t *testing.T) { // the refusal must land in the emitter's transaction at write time - a poisoned row at relay time fails silently at 3am instead
	o := New(nil, nil) // nil queries, so reaching the insert panics and the guard is proven to answer first
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Enqueue reached the insert for an unregistered type (%v)", r)
			}
		}()
		return o.Enqueue(context.Background(), nil, &probeEvent{ID: uuid.New()})
	}()
	if err == nil {
		t.Fatal("err = nil, want the unregistered type refused")
	}
}

func TestRegisteredTypeRematerializesAsTheSamePointerType(t *testing.T) { // subscribers type-assert on the concrete pointer, so a decoder handing back anything else silently drops every handler
	o := New(nil, nil)
	o.RegisterType("outbox.probe", func() events.Event { return &probeEvent{} })

	e, ok := o.newEvent("outbox.probe")
	if !ok {
		t.Fatal("a just-registered type is unknown")
	}
	if _, isProbe := e.(*probeEvent); !isProbe {
		t.Fatalf("newEvent = %T, want *probeEvent", e)
	}
	if a, b := mustNew(t, o), mustNew(t, o); a == b {
		t.Error("two calls returned one pointer, so concurrent dispatches would share and clobber a single decode target")
	}

	if _, ok := o.newEvent("outbox.unknown"); ok {
		t.Error("an unregistered type resolved")
	}
}

func mustNew(t *testing.T, o *Outbox) events.Event {
	t.Helper()
	e, ok := o.newEvent("outbox.probe")
	if !ok {
		t.Fatal("registered type not found")
	}
	return e
}

func TestRegisterTypeIsSafeUnderConcurrency(t *testing.T) { // boot registers from several fx hooks while the relay may already be polling - run with -race this is the actual assertion
	o := New(nil, nil)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			o.RegisterType("outbox.probe", func() events.Event { return &probeEvent{} })
		}()
		go func() {
			defer wg.Done()
			_, _ = o.newEvent("outbox.probe")
		}()
		_ = i
	}
	wg.Wait()
}

func TestSetPollIntervalIgnoresNonPositiveValues(t *testing.T) { // a zero interval would spin the relay's ticker at full speed against Postgres
	o := New(nil, nil)
	o.SetPollInterval(0)
	if o.pollInterval != defaultPollInterval {
		t.Errorf("pollInterval = %s after SetPollInterval(0), want the default kept", o.pollInterval)
	}
	o.SetPollInterval(-time.Second)
	if o.pollInterval != defaultPollInterval {
		t.Errorf("pollInterval = %s after a negative set, want the default kept", o.pollInterval)
	}
	o.SetPollInterval(50 * time.Millisecond)
	if o.pollInterval != 50*time.Millisecond {
		t.Errorf("pollInterval = %s, want the caller's value", o.pollInterval)
	}
}

func TestLeaseComfortablyExceedsTheWebhookRetryChain(t *testing.T) { // the comment promises it, and quietly shrinking the lease below the slowest dispatch would double-deliver every slow webhook
	if leaseSeconds < 8*60 {
		t.Errorf("leaseSeconds = %d, want at least 8 minutes to cover the ~6 minute webhook retry chain plus HTTP timeouts", leaseSeconds)
	}
}

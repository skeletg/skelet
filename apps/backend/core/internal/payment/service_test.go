package payment

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// fakeProvider is a minimal Provider impl that records calls and
// returns configurable results - Tests construct one per scenario
type fakeProvider struct {
	authorizeRes   AuthorizeResult
	authorizeErr   error
	captureRes     CaptureResult
	refundRes      RefundResult
	voidRes        VoidResult
	webhookRes     WebhookResult
	webhookErr     error
	authorizeCalls int
	captureCalls   int
	refundCalls    int
	voidCalls      int
	webhookCalls   int
}

func (p *fakeProvider) Authorize(_ context.Context, _ AuthorizeRequest) (AuthorizeResult, error) {
	p.authorizeCalls++
	return p.authorizeRes, p.authorizeErr
}
func (p *fakeProvider) Capture(_ context.Context, _ CaptureRequest) (CaptureResult, error) {
	p.captureCalls++
	return p.captureRes, nil
}
func (p *fakeProvider) Refund(_ context.Context, _ RefundRequest) (RefundResult, error) {
	p.refundCalls++
	return p.refundRes, nil
}
func (p *fakeProvider) Void(_ context.Context, _ VoidRequest) (VoidResult, error) {
	p.voidCalls++
	return p.voidRes, nil
}
func (p *fakeProvider) HandleWebhook(_ context.Context, _ *http.Request) (WebhookResult, error) {
	p.webhookCalls++
	return p.webhookRes, p.webhookErr
}

// fakeRepo is an in-memory Repository for Service tests - Implements every interface method but only the subset Authorize / Capture / etc.
// touch in tests has interesting behaviour; the rest panic so accidental coverage drift surfaces
type fakeRepo struct {
	methods map[methodKey]PaymentMethod
}
type methodKey struct {
	channel uuid.UUID
	slug    string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{methods: map[methodKey]PaymentMethod{}}
}

func (r *fakeRepo) setMethod(m PaymentMethod) {
	r.methods[methodKey{m.ChannelID, m.ProviderSlug}] = m
}

func (r *fakeRepo) WithTx(_ *sql.Tx) Repository { return r }

func (r *fakeRepo) UpsertPaymentMethod(_ context.Context, channelID uuid.UUID, slug, name string) (PaymentMethod, error) {
	m := PaymentMethod{ID: uuid.New(), ChannelID: channelID, ProviderSlug: slug, DisplayName: name}
	r.setMethod(m)
	return m, nil
}
func (r *fakeRepo) GetPaymentMethod(_ context.Context, channelID uuid.UUID, slug string) (PaymentMethod, error) {
	m, ok := r.methods[methodKey{channelID, slug}]
	if !ok {
		return PaymentMethod{}, ErrProviderNotInstalled
	}
	return m, nil
}
func (r *fakeRepo) ListPaymentMethodsByChannel(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) {
	panic("not used in tests")
}
func (r *fakeRepo) ListEnabledPaymentMethodsByChannel(_ context.Context, _ uuid.UUID) ([]PaymentMethod, error) {
	panic("not used in tests")
}
func (r *fakeRepo) ListPaymentMethodsByProvider(_ context.Context, _ string) ([]PaymentMethod, error) {
	panic("not used in tests")
}
func (r *fakeRepo) SetPaymentMethodEnabled(_ context.Context, channelID uuid.UUID, slug string, enabled bool) (PaymentMethod, error) {
	k := methodKey{channelID, slug}
	m, ok := r.methods[k]
	if !ok {
		return PaymentMethod{}, ErrProviderNotInstalled
	}
	m.IsEnabled = enabled
	r.methods[k] = m
	return m, nil
}
func (r *fakeRepo) UpdatePaymentMethodPresentation(_ context.Context, channelID uuid.UUID, slug, name string, order int32) (PaymentMethod, error) {
	key := methodKey{channelID, slug}
	m, ok := r.methods[key]
	if !ok {
		return PaymentMethod{}, ErrProviderNotInstalled
	}
	m.DisplayName, m.SortOrder = name, order
	r.methods[key] = m
	return m, nil
}
func (r *fakeRepo) SetPaymentMethodSortOrder(_ context.Context, _ uuid.UUID, _ string, _ int32) (PaymentMethod, error) {
	panic("not used in tests")
}
func (r *fakeRepo) SetPaymentMethodCountryMode(_ context.Context, channelID uuid.UUID, slug, mode string) (PaymentMethod, error) {
	k := methodKey{channelID, slug}
	m, ok := r.methods[k]
	if !ok {
		return PaymentMethod{}, ErrProviderNotInstalled
	}
	m.CountryMode = mode
	r.methods[k] = m
	return m, nil
}
func (r *fakeRepo) AddCountryToPaymentMethod(_ context.Context, _ uuid.UUID, _ string) error {
	panic("not used in tests")
}
func (r *fakeRepo) ListCountriesForPaymentMethod(_ context.Context, _ uuid.UUID) ([]string, error) {
	panic("not used in tests")
}
func (r *fakeRepo) ClearPaymentMethodCountries(_ context.Context, _ uuid.UUID) error {
	panic("not used in tests")
}

// eventRecorder subscribes to every payment.* topic on a fresh bus and stores received events for later assertion -
// Used to verify that Service.Authorize publishes PaymentAuthorized, etc
type eventRecorder struct {
	mu       sync.Mutex
	received []events.Event
	done     chan struct{}
	expected int
}

func newRecorder(bus *events.Bus, expected int) *eventRecorder {
	r := &eventRecorder{done: make(chan struct{}, 1), expected: expected}
	for _, topic := range []string{
		EventAuthorized, EventCaptured, EventRefunded, EventVoided, EventProviderWebhook,
	} {
		bus.Subscribe(topic, func(_ context.Context, e events.Event) error {
			r.mu.Lock()
			r.received = append(r.received, e)
			done := len(r.received) >= r.expected
			r.mu.Unlock()
			if done {
				select {
				case r.done <- struct{}{}:
				default:
				}
			}
			return nil
		})
	}
	return r
}

// TestAuthorize_PublishesEvent runs the happy path: registered enabled
// provider, Authorize succeeds, PaymentAuthorized fires on the bus
func TestAuthorize_PublishesEvent(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	store := uuid.New()
	order := uuid.New()
	prov := &fakeProvider{authorizeRes: AuthorizeResult{ProviderReference: "pi_123"}}
	RegisterProvider(Registration{Slug: "stripe", Provider: prov, DisplayName: "Stripe"})

	repo := newFakeRepo()
	repo.setMethod(PaymentMethod{ChannelID: store, ProviderSlug: "stripe", IsEnabled: true})

	bus := events.NewBus()
	rec := newRecorder(bus, 1)

	svc := NewService(repo, nil, nil)
	svc.SetEventBus(bus)

	res, err := svc.Authorize(context.Background(), "stripe", AuthorizeRequest{
		OrderID:      order,
		ChannelID:    store,
		AmountCents:  1500,
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if res.ProviderReference != "pi_123" {
		t.Errorf("ProviderReference: got %q, want pi_123", res.ProviderReference)
	}
	if prov.authorizeCalls != 1 {
		t.Errorf("provider.Authorize calls: got %d, want 1", prov.authorizeCalls)
	}

	// Async events run on a background goroutine; wait briefly
	select {
	case <-rec.done:
	case <-context.Background().Done():
		t.Fatal("ctx cancelled before recorder fired")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.received) != 1 {
		t.Fatalf("events: got %d, want 1", len(rec.received))
	}
	pa, ok := rec.received[0].(*PaymentAuthorized)
	if !ok {
		t.Fatalf("event type: got %T, want *PaymentAuthorized", rec.received[0])
	}
	if pa.OrderID != order || pa.ProviderSlug != "stripe" || pa.ProviderReference != "pi_123" {
		t.Errorf("PaymentAuthorized fields wrong: %+v", pa)
	}
}

// TestAuthorize_DisabledMethod returns ErrProviderDisabled without
// touching the provider or the bus
func TestAuthorize_DisabledMethod(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	store := uuid.New()
	prov := &fakeProvider{}
	RegisterProvider(Registration{Slug: "stripe", Provider: prov})

	repo := newFakeRepo()
	repo.setMethod(PaymentMethod{ChannelID: store, ProviderSlug: "stripe", IsEnabled: false})

	svc := NewService(repo, nil, nil)

	_, err := svc.Authorize(context.Background(), "stripe", AuthorizeRequest{
		ChannelID:    store,
		AmountCents:  100,
		CurrencyCode: "USD",
	})
	if !errors.Is(err, ErrProviderDisabled) {
		t.Errorf("got error %v, want ErrProviderDisabled", err)
	}
	if prov.authorizeCalls != 0 {
		t.Error("disabled method should not invoke provider")
	}
}

// TestAuthorize_ProviderNotFound: unknown slug short-circuits before
// the repo lookup
func TestAuthorize_ProviderNotFound(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	repo := newFakeRepo()
	svc := NewService(repo, nil, nil)

	_, err := svc.Authorize(context.Background(), "missing", AuthorizeRequest{})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("got %v, want ErrProviderNotFound", err)
	}
}

// TestAuthorize_ProviderErrorNoEvent: provider Authorize fails; no
// PaymentAuthorized event should reach the bus
func TestAuthorize_ProviderErrorNoEvent(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	store := uuid.New()
	prov := &fakeProvider{authorizeErr: errors.New("stripe down")}
	RegisterProvider(Registration{Slug: "stripe", Provider: prov})

	repo := newFakeRepo()
	repo.setMethod(PaymentMethod{ChannelID: store, ProviderSlug: "stripe", IsEnabled: true})

	bus := events.NewBus()
	var fired bool
	bus.Subscribe(EventAuthorized, func(_ context.Context, _ events.Event) error {
		fired = true
		return nil
	})

	svc := NewService(repo, nil, nil)
	svc.SetEventBus(bus)

	_, err := svc.Authorize(context.Background(), "stripe", AuthorizeRequest{
		ChannelID:   store,
		AmountCents: 100,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if fired {
		t.Error("PaymentAuthorized fired despite provider error")
	}
}

// TestRegisterProvider_DuplicatePanics: registering the same slug
// twice crashes - same contract as the SDK
func TestRegisterProvider_DuplicatePanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	RegisterProvider(Registration{Slug: "stripe", Provider: &fakeProvider{}})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
	}()
	RegisterProvider(Registration{Slug: "stripe", Provider: &fakeProvider{}})
}

// TestRegisterProvider_NilProviderPanics: nil Provider is a misuse
func TestRegisterProvider_NilProviderPanics(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil Provider")
		}
	}()
	RegisterProvider(Registration{Slug: "stripe", Provider: nil})
}

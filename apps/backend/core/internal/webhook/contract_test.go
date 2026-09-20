package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/circuit"
)

// --- fakes ---

type fakeRepo struct {
	mu         sync.Mutex
	webhooks   []Webhook
	secrets    map[uuid.UUID]string
	deliveries []Delivery
	queue      []*QueueItem
}

func newFakeRepo(webhooks ...Webhook) *fakeRepo {
	r := &fakeRepo{secrets: map[uuid.UUID]string{}}
	for _, w := range webhooks {
		r.webhooks = append(r.webhooks, w)
		r.secrets[w.ID] = "whsec_test"
	}
	return r
}

func (r *fakeRepo) ListActiveByEvent(_ context.Context, eventType string, kind Kind, _ *uuid.UUID) ([]Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Webhook
	for _, w := range r.webhooks {
		if !w.IsActive || w.Kind != kind {
			continue
		}
		for _, entry := range w.EventTypes {
			if MatchesEntry(entry, eventType) {
				out = append(out, w)
				break
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) GetSigningSecret(_ context.Context, id uuid.UUID) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.secrets[id]
	if !ok {
		return "", ErrWebhookNotFound
	}
	return s, nil
}

func (r *fakeRepo) CreateDelivery(_ context.Context, d Delivery) (Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deliveries = append(r.deliveries, d)
	return d, nil
}

func (r *fakeRepo) Create(context.Context, CreateInput, string) (Webhook, error) {
	return Webhook{}, errors.New("not implemented")
}
func (r *fakeRepo) List(context.Context) ([]Webhook, error) { return nil, nil }
func (r *fakeRepo) Update(context.Context, UpdateInput) (Webhook, error) {
	return Webhook{}, errors.New("not implemented")
}
func (r *fakeRepo) SetActive(context.Context, uuid.UUID, bool) (Webhook, error) {
	return Webhook{}, errors.New("not implemented")
}
func (r *fakeRepo) UpdateSigningSecret(context.Context, uuid.UUID, string) (Webhook, error) {
	return Webhook{}, errors.New("not implemented")
}
func (r *fakeRepo) Delete(context.Context, uuid.UUID) error { return nil }
func (r *fakeRepo) ListDeliveries(context.Context, uuid.UUID, int32) ([]Delivery, error) {
	return nil, nil
}
func (r *fakeRepo) DeliveryStats(context.Context, uuid.UUID) (DeliveryStats, error) {
	return DeliveryStats{}, nil
}
func (r *fakeRepo) ClearDeliveries(context.Context, uuid.UUID) (int64, error) { return 0, nil }
func (r *fakeRepo) PruneDeliveries(context.Context, time.Time) (int64, error) { return 0, nil }
func (r *fakeRepo) WithTx(*sql.Tx) Repository                                 { return r }

func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range r.webhooks {
		if w.ID == id {
			return w, nil
		}
	}
	return Webhook{}, ErrWebhookNotFound
}

func (r *fakeRepo) GetDelivery(_ context.Context, id uuid.UUID) (Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.deliveries {
		if d.ID == id {
			return d, nil
		}
	}
	return Delivery{}, ErrWebhookNotFound
}

// --- in-memory queue mirroring the SQL semantics ---

func (r *fakeRepo) Enqueue(_ context.Context, webhookID uuid.UUID, eventType, payload string) (QueueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item := QueueItem{
		ID: uuid.New(), WebhookID: webhookID, EventType: eventType, Payload: payload,
		Status: QueuePending, NextAttemptAt: time.Now(),
	}
	r.queue = append(r.queue, &item)
	return item, nil
}

func (r *fakeRepo) ClaimDueQueue(_ context.Context, batchSize, leaseSeconds int32) ([]QueueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []QueueItem
	now := time.Now()
	for _, item := range r.queue {
		if int32(len(out)) >= batchSize {
			break
		}
		if item.Status == QueuePending && !item.NextAttemptAt.After(now) {
			item.Attempts++
			item.NextAttemptAt = now.Add(time.Duration(leaseSeconds) * time.Second)
			out = append(out, *item)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteQueueItem(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, item := range r.queue {
		if item.ID == id {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *fakeRepo) RescheduleQueueItem(_ context.Context, id uuid.UUID, nextAttemptAt time.Time, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.queue {
		if item.ID == id {
			item.NextAttemptAt = nextAttemptAt
			item.LastError = &lastError
		}
	}
	return nil
}

func (r *fakeRepo) MarkQueueItemDead(_ context.Context, id uuid.UUID, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.queue {
		if item.ID == id {
			item.Status = QueueDead
			item.LastError = &lastError
		}
	}
	return nil
}

func (r *fakeRepo) ListDeadQueueItems(_ context.Context, webhookID uuid.UUID, limit int32) ([]QueueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []QueueItem
	for _, item := range r.queue {
		if item.WebhookID == webhookID && item.Status == QueueDead && int32(len(out)) < limit {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (r *fakeRepo) RetryQueueItem(_ context.Context, id uuid.UUID) (QueueItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.queue {
		if item.ID == id {
			item.Status = QueuePending
			item.Attempts = 0
			item.NextAttemptAt = time.Now()
			item.LastError = nil
			return *item, nil
		}
	}
	return QueueItem{}, ErrWebhookNotFound
}

func (r *fakeRepo) ClearDeadQueueItems(_ context.Context, webhookID uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []*QueueItem
	var n int64
	for _, item := range r.queue {
		if item.WebhookID == webhookID && item.Status == QueueDead {
			n++
			continue
		}
		kept = append(kept, item)
	}
	r.queue = kept
	return n, nil
}

// testEvent is a sync event that accepts webhook mutations via a tag list -
// the same shape most real appliers use
type testEvent struct {
	eventType string
	Tags      []string `json:"tags"`
}

func (e *testEvent) Type() string { return e.eventType }

func (e *testEvent) ApplyWebhookResponse(body []byte) (bool, string, error) {
	var resp struct {
		Reject bool     `json:"reject"`
		Reason string   `json:"reason"`
		Tags   []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, "", err
	}
	e.Tags = append(e.Tags, resp.Tags...)
	return resp.Reject, resp.Reason, nil
}

// plain event without an applier - sync delivery is veto-only for it
type plainEvent struct{ eventType string }

func (e *plainEvent) Type() string { return e.eventType }

func resetCircuits() {
	circuits = circuit.New(breakerTripThreshold, breakerCooldown)
}

func syncWebhook(url string, policy FailurePolicy, timeoutMs int32) Webhook {
	return Webhook{
		ID:             uuid.New(),
		Name:           "test-hook",
		TargetURL:      url,
		Kind:           KindSync,
		IsActive:       true,
		EventTypes:     []string{"test.eventing"},
		FailurePolicy:  policy,
		TimeoutMs:      timeoutMs,
		PayloadVersion: PayloadVersionV1,
	}
}

func dispatcherFor(webhooks ...Webhook) (*Dispatcher, *fakeRepo) {
	repo := newFakeRepo(webhooks...)
	return NewDispatcher(NewService(repo), nil), repo
}

// --- wildcard matching ---

func TestMatchesEntry(t *testing.T) {
	cases := []struct {
		entry, event string
		want         bool
	}{
		{"order.placed", "order.placed", true},
		{"order.placed", "order.cancelled", false},
		{"order.*", "order.placed", true},
		{"order.*", "order.return_requested", true},
		{"order.*", "customer.registered", false},
		{"*", "anything.at_all", true},
		{"cart.line.*", "cart.line.adding", true},
		{"cart.line.*", "cart.bundle.adding", false},
		{"order*", "order.placed", false}, // not a wildcard form, exact mismatch
	}
	for _, c := range cases {
		if got := MatchesEntry(c.entry, c.event); got != c.want {
			t.Errorf("MatchesEntry(%q, %q) = %v, want %v", c.entry, c.event, got, c.want)
		}
	}
}

func TestValidateEventTypesWildcards(t *testing.T) {
	// defaults.go registered the real catalog in init()
	if err := ValidateEventTypes([]string{"order.*"}, KindAsync); err != nil {
		t.Errorf("order.* async: %v", err)
	}
	if err := ValidateEventTypes([]string{"order.*"}, KindSync); err != nil {
		t.Errorf("order.* sync: %v", err)
	}
	if err := ValidateEventTypes([]string{"*"}, KindAsync); err != nil {
		t.Errorf("* async: %v", err)
	}
	if err := ValidateEventTypes([]string{"nosuchdomain.*"}, KindAsync); err == nil {
		t.Error("nosuchdomain.* should be rejected")
	}
	// cms page publishing is sync-only, so the async pool has no cms sync match
	if err := ValidateEventTypes([]string{"auth.login.attempting"}, KindAsync); err == nil {
		t.Error("sync event on async webhook should be rejected")
	}
}

// --- breaker ---

func TestCircuitBreaker(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	now := time.Unix(1000, 0)
	circuits.SetNow(func() time.Time { return now })
	id := uuid.New()

	// closed until the trip threshold
	for i := range breakerTripThreshold - 1 {
		if !circuits.Allow(id) {
			t.Fatalf("blocked before threshold at fail %d", i)
		}
		circuits.Record(id, false)
	}
	if !circuits.Allow(id) {
		t.Fatal("blocked before threshold")
	}
	circuits.Record(id, false) // fail #5 - trips

	if circuits.Allow(id) {
		t.Fatal("open circuit allowed a call inside cooldown")
	}
	if st := CircuitInfoFor(id).State; st != CircuitOpen {
		t.Fatalf("state = %s, want open", st)
	}

	// cooldown elapses - exactly one probe allowed
	now = now.Add(breakerCooldown + time.Second)
	if !circuits.Allow(id) {
		t.Fatal("no probe allowed after cooldown")
	}
	if circuits.Allow(id) {
		t.Fatal("second concurrent probe allowed")
	}

	// failed probe re-arms the cooldown
	circuits.Record(id, false)
	if circuits.Allow(id) {
		t.Fatal("allowed right after failed probe")
	}

	// successful probe closes
	now = now.Add(breakerCooldown + time.Second)
	if !circuits.Allow(id) {
		t.Fatal("no probe allowed after second cooldown")
	}
	circuits.Record(id, true)
	if !circuits.Allow(id) {
		t.Fatal("closed circuit blocked a call")
	}
	if st := CircuitInfoFor(id).State; st != CircuitClosed {
		t.Fatalf("state = %s, want closed", st)
	}
}

// --- sync dispatch policies ---

func TestSyncFailClosedAborts(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d, _ := dispatcherFor(syncWebhook(srv.URL, FailClosed, 0))
	err := d.dispatchSync(context.Background(), &plainEvent{"test.eventing"})
	if err == nil {
		t.Fatal("fail_closed 500 should abort the operation")
	}
}

func TestSyncFailOpenContinues(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d, repo := dispatcherFor(syncWebhook(srv.URL, FailOpen, 0))
	if err := d.dispatchSync(context.Background(), &plainEvent{"test.eventing"}); err != nil {
		t.Fatalf("fail_open 500 should not abort: %v", err)
	}
	if len(repo.deliveries) != 1 || repo.deliveries[0].Success {
		t.Fatal("failed delivery should still be recorded")
	}
}

func TestSyncFailOpenStillHonorsExplicitReject(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"reject": true, "reason": "blocked by fraud rules"}`))
	}))
	defer srv.Close()

	d, _ := dispatcherFor(syncWebhook(srv.URL, FailOpen, 0))
	err := d.dispatchSync(context.Background(), &testEvent{eventType: "test.eventing"})
	if err == nil || !strings.Contains(err.Error(), "fraud rules") {
		t.Fatalf("explicit reject must abort under fail_open, got %v", err)
	}
}

func TestSyncMutationApplied(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tags": ["vip", "b2b"]}`))
	}))
	defer srv.Close()

	d, _ := dispatcherFor(syncWebhook(srv.URL, FailClosed, 0))
	e := &testEvent{eventType: "test.eventing"}
	if err := d.dispatchSync(context.Background(), e); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(e.Tags) != 2 || e.Tags[0] != "vip" {
		t.Fatalf("tags not applied: %v", e.Tags)
	}
}

func TestSyncPerWebhookTimeout(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	d, _ := dispatcherFor(syncWebhook(srv.URL, FailClosed, 50))
	start := time.Now()
	err := d.dispatchSync(context.Background(), &plainEvent{"test.eventing"})
	if err == nil {
		t.Fatal("timeout should abort under fail_closed")
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("timeout_ms=50 not honored, took %s", elapsed)
	}
}

func TestSyncCircuitTripsAndSkipsCalls(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	wh := syncWebhook(srv.URL, FailClosed, 0)
	d, _ := dispatcherFor(wh)
	for range breakerTripThreshold {
		_ = d.dispatchSync(context.Background(), &plainEvent{"test.eventing"})
	}
	if got := hits.Load(); got != breakerTripThreshold {
		t.Fatalf("expected %d upstream hits, got %d", breakerTripThreshold, got)
	}

	// circuit is open now - the next dispatch must fail fast without a call
	err := d.dispatchSync(context.Background(), &plainEvent{"test.eventing"})
	if err == nil || !strings.Contains(err.Error(), "circuit") {
		t.Fatalf("expected circuit-open rejection, got %v", err)
	}
	if got := hits.Load(); got != breakerTripThreshold {
		t.Fatalf("circuit-open dispatch still hit upstream (%d hits)", got)
	}
}

func TestCustomHeadersSentButReservedProtected(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	var gotAuth, gotEvent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotEvent = r.Header.Get("X-Webhook-Event")
	}))
	defer srv.Close()

	wh := syncWebhook(srv.URL, FailClosed, 0)
	wh.CustomHeaders = map[string]string{
		"Authorization":   "Bearer sekrit",
		"X-Webhook-Event": "spoofed.event", // must not shadow the real one
	}
	d, _ := dispatcherFor(wh)
	if err := d.dispatchSync(context.Background(), &plainEvent{"test.eventing"}); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if gotAuth != "Bearer sekrit" {
		t.Fatalf("custom Authorization header not sent, got %q", gotAuth)
	}
	if gotEvent != "test.eventing" {
		t.Fatalf("reserved header was shadowed: %q", gotEvent)
	}
}

// --- async queue worker ---

func asyncWebhook(url string) Webhook {
	return Webhook{
		ID:             uuid.New(),
		Name:           "async-hook",
		TargetURL:      url,
		Kind:           KindAsync,
		IsActive:       true,
		EventTypes:     []string{"test.evented"},
		FailurePolicy:  FailClosed,
		PayloadVersion: PayloadVersionV1,
	}
}

func TestQueueWorkerDeliversAndCompletes(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	wh := asyncWebhook(srv.URL)
	d, repo := dispatcherFor(wh)
	worker := NewQueueWorker(NewService(repo), d)

	// async dispatch enqueues instead of delivering inline
	_ = d.dispatchAsync(context.Background(), &plainEvent{"test.evented"})
	if len(repo.queue) != 1 {
		t.Fatalf("expected 1 queued item, got %d", len(repo.queue))
	}
	if hits.Load() != 0 {
		t.Fatal("dispatch must not deliver inline")
	}

	if n := worker.RunOnce(context.Background()); n != 1 {
		t.Fatalf("RunOnce processed %d, want 1", n)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 delivery, got %d", hits.Load())
	}
	if len(repo.queue) != 0 {
		t.Fatal("successful delivery should delete the queue row")
	}
	if len(repo.deliveries) != 1 || !repo.deliveries[0].Success {
		t.Fatal("delivery history row missing")
	}
}

func TestQueueWorkerDeadLettersAndRedelivery(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	var failing atomic.Bool
	failing.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	wh := asyncWebhook(srv.URL)
	d, repo := dispatcherFor(wh)
	svc := NewService(repo)
	worker := NewQueueWorker(svc, d)
	ctx := context.Background()

	item, err := svc.EnqueueDelivery(ctx, wh.ID, "test.evented", `{"apiVersion":"v1","event":"test.evented"}`)
	if err != nil {
		t.Fatal(err)
	}

	// exhaust the ladder: force the row due again after each failed pass
	for i := 0; i < int(maxQueueAttempts); i++ {
		repo.mu.Lock()
		for _, q := range repo.queue {
			q.NextAttemptAt = time.Now().Add(-time.Second)
		}
		repo.mu.Unlock()
		worker.RunOnce(ctx)
	}

	dead, err := svc.ListDeadLetters(ctx, wh.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(dead) != 1 || dead[0].ID != item.ID {
		t.Fatalf("expected the item dead-lettered, got %+v", dead)
	}
	if dead[0].LastError == nil || !strings.Contains(*dead[0].LastError, "502") {
		t.Fatalf("dead letter should carry the last error, got %v", dead[0].LastError)
	}

	// endpoint recovers - retrying the dead letter delivers it
	failing.Store(false)
	if _, err := svc.RetryDeadLetter(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	worker.RunOnce(ctx)
	if len(repo.queue) != 0 {
		t.Fatalf("retried dead letter should deliver and complete, queue: %d", len(repo.queue))
	}
}

func TestRedeliverFromHistory(t *testing.T) {
	resetCircuits()
	defer resetCircuits()
	wh := asyncWebhook("https://example.com/hook")
	_, repo := dispatcherFor(wh)
	svc := NewService(repo)
	ctx := context.Background()

	recorded, err := svc.RecordDelivery(ctx, Delivery{
		ID: uuid.New(), WebhookID: wh.ID, EventType: "test.evented",
		RequestBody: `{"apiVersion":"v1","event":"test.evented","data":{}}`, Success: false, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.RedeliverDelivery(ctx, recorded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Payload != recorded.RequestBody || item.WebhookID != wh.ID {
		t.Fatalf("redelivered item doesn't mirror the delivery: %+v", item)
	}
}

func TestValidateDeliverySettings(t *testing.T) {
	if err := validateDeliverySettings(FailOpen, 2500, map[string]string{"Authorization": "x"}); err != nil {
		t.Fatalf("valid settings rejected: %v", err)
	}
	if err := validateDeliverySettings("sometimes", 0, nil); !errors.Is(err, ErrInvalidFailurePolicy) {
		t.Fatalf("bad policy: %v", err)
	}
	if err := validateDeliverySettings("", MaxTimeoutMs+1, nil); !errors.Is(err, ErrInvalidTimeout) {
		t.Fatalf("bad timeout: %v", err)
	}
	if err := validateDeliverySettings("", 0, map[string]string{"X-Webhook-Signature-256": "x"}); !errors.Is(err, ErrReservedHeader) {
		t.Fatalf("reserved header: %v", err)
	}
	if err := validateDeliverySettings("", 0, map[string]string{"Content-Type": "text/plain"}); !errors.Is(err, ErrReservedHeader) {
		t.Fatalf("content-type: %v", err)
	}
}

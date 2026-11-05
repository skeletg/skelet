package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/leasequeue"
)

const (
	pollInterval = 2 * time.Second
	batchSize    = 100
	leaseSeconds = 60 // well past sendTimeout, so a slow send isn't claimed twice
	sendTimeout  = 15 * time.Second
	workers      = 8
	logRetention = 30 * 24 * time.Hour
)

// The waits after the 1st, 2nd... failed attempt. A platform that is down for a day loses the events of that day,
// which is what every vendor's own retry window allows for anyway
var retryDelays = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

var maxAttempts = int32(1 + len(retryDelays))

// Worker sends what Track queued. Each row is one event for one destination, so a failure retries or dead-letters that
// event alone and never holds up the rest. At least once: a crash mid-send replays the row once its lease lapses, and
// the event id lets the platform dedupe
type Worker struct {
	store  store
	client *http.Client
	runner leasequeue.Runner[Delivery]
}

// The client must be the SSRF-safe one - a destination's endpoint is configured by an admin and may name anything
func NewWorker(s *Service, client *http.Client) *Worker {
	w := &Worker{store: s.store, client: client}
	w.runner = leasequeue.Runner[Delivery]{
		Name: "tracking", Poll: pollInterval, Batch: batchSize, Lease: leaseSeconds, Workers: workers,
		Claim:   s.store.ClaimDue,
		Prepare: w.prepare,
		Hourly: func(ctx context.Context) {
			if _, err := w.store.Prune(ctx, time.Now().Add(-logRetention)); err != nil {
				slog.Warn("prune tracking deliveries", "err", err)
			}
		},
	}
	return w
}

func (w *Worker) Run(ctx context.Context) { w.runner.Run(ctx) }

// RunOnce sends everything due and returns how many rows it took
func (w *Worker) RunOnce(ctx context.Context) int { return w.runner.RunOnce(ctx) }

// Loads each destination a batch names once, secret included - many rows share one
func (w *Worker) prepare(ctx context.Context, due []Delivery) func(context.Context, Delivery) {
	dests := map[uuid.UUID]Destination{}
	failed := map[uuid.UUID]error{}
	for _, item := range due {
		if _, ok := dests[item.DestinationID]; ok || failed[item.DestinationID] != nil {
			continue
		}
		d, err := w.store.Get(ctx, item.DestinationID)
		if err != nil {
			failed[item.DestinationID] = fmt.Errorf("load destination: %w", err)
			continue
		}
		dests[item.DestinationID] = d
	}
	return func(ctx context.Context, item Delivery) {
		d, ok := dests[item.DestinationID]
		if !ok {
			w.fail(ctx, item, nil, failed[item.DestinationID])
			return
		}
		defer func() {
			if rec := recover(); rec != nil { // a kind's mapping bug must not retry forever - it takes the ladder like any failure
				slog.Error("tracking send panicked", "delivery", item.ID, "kind", d.Kind, "panic", rec, "stack", string(debug.Stack()))
				w.fail(ctx, item, &d, fmt.Errorf("send panicked: %v", rec))
			}
		}()
		w.deliver(ctx, item, d)
	}
}

func (w *Worker) deliver(ctx context.Context, item Delivery, d Destination) {
	k, ok := KindFor(d.Kind)
	switch {
	case !d.Enabled:
		w.dead(ctx, item, d, "destination disabled") // a retry once it's enabled again sends it
		return
	case !ok:
		w.dead(ctx, item, d, "kind "+d.Kind+" is not in this build")
		return
	}
	var ev Event
	if err := json.Unmarshal(item.Payload, &ev); err != nil {
		w.dead(ctx, item, d, "undecodable payload: "+err.Error())
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	if err := k.Send(sendCtx, w.client, d, ev); err != nil {
		w.fail(ctx, item, &d, err)
		return
	}
	if err := w.store.MarkSent(ctx, item.ID); err != nil {
		slog.Warn("mark tracking delivery sent", "delivery", item.ID, "err", err) // the lease replays it, and the platform dedupes on the event id
	}
}

func (w *Worker) fail(ctx context.Context, item Delivery, d *Destination, err error) {
	reason := err.Error()
	if d != nil {
		reason = redact(reason, d)
	}
	delay, retry := leasequeue.Retry(item.Attempts, retryDelays)
	if isPermanent(err) || !retry {
		if d == nil {
			d = &Destination{}
		}
		w.dead(ctx, item, *d, reason)
		return
	}
	if err := w.store.Reschedule(ctx, item.ID, time.Now().Add(delay), reason); err != nil {
		slog.Warn("reschedule tracking delivery", "delivery", item.ID, "err", err)
	}
}

func (w *Worker) dead(ctx context.Context, item Delivery, d Destination, reason string) {
	if err := w.store.MarkDead(ctx, item.ID, redact(reason, &d)); err != nil {
		slog.Warn("mark tracking delivery dead", "delivery", item.ID, "err", err)
	}
}

// The log is shown to admins, and a platform's error can echo the request - a GA4 api_secret rides in the URL, a token
// in a header. Every secret and config value that looks like one is cut out before it is stored
func redact(s string, d *Destination) string {
	if d.Secret != "" {
		s = strings.ReplaceAll(s, d.Secret, "[redacted]")
	}
	for k, v := range d.Config {
		lk := strings.ToLower(k)
		if len(v) >= 8 && (strings.Contains(lk, "secret") || strings.Contains(lk, "token") || strings.Contains(lk, "key")) {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	if len(s) > 1000 {
		s = strings.ToValidUTF8(s[:1000], "") // a cut through a multi-byte rune is invalid text, which Postgres refuses
	}
	return s
}

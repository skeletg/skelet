package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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
	poll   time.Duration
}

// The client must be the SSRF-safe one - a destination's endpoint is configured by an admin and may name anything
func NewWorker(s *Service, client *http.Client) *Worker {
	return &Worker{store: s.store, client: client, poll: pollInterval}
}

func (w *Worker) Run(ctx context.Context) {
	tick := time.NewTicker(w.poll)
	defer tick.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			w.RunOnce(ctx)
		case <-prune.C:
			if _, err := w.store.Prune(ctx, time.Now().Add(-logRetention)); err != nil {
				slog.Warn("prune tracking deliveries", "err", err)
			}
		}
	}
}

// RunOnce sends everything due and returns how many rows it took
func (w *Worker) RunOnce(ctx context.Context) int {
	done := 0
	for ctx.Err() == nil {
		due, err := w.store.ClaimDue(ctx, batchSize, leaseSeconds)
		if err != nil {
			slog.Warn("claim tracking deliveries", "err", err)
			return done
		}
		done += len(due)
		dests := map[uuid.UUID]*Destination{} // many rows share a destination
		var wg sync.WaitGroup
		sem := make(chan struct{}, workers)
		for _, item := range due {
			d, ok := dests[item.DestinationID]
			if !ok {
				loaded, err := w.store.Get(ctx, item.DestinationID)
				if err != nil {
					w.fail(ctx, item, nil, fmt.Errorf("load destination: %w", err))
					continue
				}
				d = &loaded
				dests[item.DestinationID] = d
			}
			sem <- struct{}{}
			wg.Add(1)
			go func(item Delivery, d Destination) {
				defer wg.Done()
				defer func() { <-sem }()
				defer func() {
					if rec := recover(); rec != nil { // a kind's mapping bug must not take the process down; the lease replays the row
						slog.Error("tracking send panicked", "delivery", item.ID, "kind", d.Kind, "panic", rec, "stack", string(debug.Stack()))
					}
				}()
				w.deliver(ctx, item, d)
			}(item, *d)
		}
		wg.Wait()
		if len(due) < batchSize {
			return done
		}
	}
	return done
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
	case k.Available != nil && !k.Available(ctx, d.ChannelID):
		w.dead(ctx, item, d, "plugin disabled on this channel")
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
	if isPermanent(err) || item.Attempts >= maxAttempts {
		if d == nil {
			d = &Destination{}
		}
		w.dead(ctx, item, *d, reason)
		return
	}
	delay := retryDelays[max(1, min(int(item.Attempts), len(retryDelays)))-1]
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
		s = s[:1000]
	}
	return s
}

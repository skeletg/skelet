// Package spinequeue is the optional durable buffer between ingest and the spine engine - JetStream in front of AppendEvent
// Web events have no outbox backstop and the ClickHouse client buffers in memory, so without this a crash loses the unflushed tail; with it an event is on disk before AppendEvent returns and the signals worker drains the stream into whichever engine is selected
// Producers publish with the dedupe key as the message id, so producer retries collapse in the stream's dedupe window before the engine even sees them
package spinequeue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/skeletg/skelet/apps/backend/internal/analytics"
)

const (
	streamName   = "SIGNALS_EVENTS"
	subject      = "signals.events"
	consumerTag  = "spine-ingest"
	fetchBatch   = 500
	nakBaseDelay = 5 * time.Second // redelivery backoff scales with the attempt count, so an engine outage idles the batch instead of spinning it
	nakMaxDelay  = 2 * time.Minute
)

type Queue struct {
	nc *nats.Conn
	js jetstream.JetStream
}

func New(ctx context.Context, url string) (*Queue, error) {
	nc, err := nats.Connect(url, nats.MaxReconnects(-1))
	if err != nil {
		return nil, fmt.Errorf("connect signals queue: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, err
	}
	// Transient transport, not a system of record - a day of headroom for a down consumer then the stream sheds, since the engine is the durable store, and it's created only when missing so an operator's tuning of an existing stream survives boots
	if _, err := js.Stream(ctx, streamName); err != nil {
		if !errors.Is(err, jetstream.ErrStreamNotFound) {
			nc.Close()
			return nil, fmt.Errorf("look up signals stream: %w", err)
		}
		if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
			Name:       streamName,
			Subjects:   []string{subject},
			Storage:    jetstream.FileStorage,
			Retention:  jetstream.LimitsPolicy,
			MaxAge:     24 * time.Hour,
			Duplicates: 2 * time.Minute,
		}); err != nil && !errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
			nc.Close()
			return nil, fmt.Errorf("ensure signals stream: %w", err)
		}
	}
	return &Queue{nc: nc, js: js}, nil
}

func (q *Queue) Close() { q.nc.Close() }

type Stats struct { // what the health panel needs to answer "is the queue actually being drained"
	StreamMessages uint64
	Pending        uint64 // undelivered messages waiting for the consumer
	HasConsumer    bool   // false with the queue configured is the silent-loss case - producers publish happily, nothing drains, and the stream sheds at its MaxAge a day later
	Redelivering   uint64 // in-flight but un-acked - sustained non-zero means the engine is rejecting
}

func (q *Queue) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	stream, err := q.js.Stream(ctx, streamName)
	if err != nil {
		return s, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return s, err
	}
	s.StreamMessages = info.State.Msgs
	cons, err := stream.Consumer(ctx, consumerTag)
	if err != nil {
		if errors.Is(err, jetstream.ErrConsumerNotFound) {
			return s, nil // no worker has ever run - exactly what the panel must surface
		}
		return s, err
	}
	ci, err := cons.Info(ctx)
	if err != nil {
		return s, err
	}
	s.HasConsumer, s.Pending, s.Redelivering = true, ci.NumPending, uint64(ci.NumAckPending)
	return s, nil
}

// Layers the queue over the selected engine - AppendEvent(s) publish durably and return while everything else passes through, and the bool goes optimistic (same slack the ClickHouse driver documents, it only repeats idempotent writes)
func (q *Queue) WrapSpine(inner analytics.SpineStore) analytics.SpineStore {
	return &queueSpine{SpineStore: inner, q: q}
}

type queueSpine struct {
	analytics.SpineStore
	q *Queue
}

func (s *queueSpine) Unwrap() analytics.SpineStore { return s.SpineStore } // lets engine-type checks (GDPR routing, dictionary refresh) see through the decorator

func (s *queueSpine) AppendEvent(ctx context.Context, e analytics.SpineEvent) (bool, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	var opts []jetstream.PublishOpt
	if e.DedupeKey != "" {
		opts = append(opts, jetstream.WithMsgID(e.DedupeKey))
	}
	if _, err := s.q.js.Publish(ctx, subject, raw, opts...); err != nil {
		return false, fmt.Errorf("publish spine event: %w", err)
	}
	return true, nil
}

// Publishes async and waits on every ack - one round-trip window for a whole heatmap flush instead of a publish per cell
func (s *queueSpine) AppendEvents(ctx context.Context, events []analytics.SpineEvent) error {
	futures := make([]jetstream.PubAckFuture, 0, len(events))
	for _, e := range events {
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		var opts []jetstream.PublishOpt
		if e.DedupeKey != "" {
			opts = append(opts, jetstream.WithMsgID(e.DedupeKey))
		}
		f, err := s.q.js.PublishAsync(subject, raw, opts...)
		if err != nil {
			return fmt.Errorf("publish spine event: %w", err)
		}
		futures = append(futures, f)
	}
	for _, f := range futures {
		select {
		case <-f.Ok():
		case err := <-f.Err():
			return fmt.Errorf("publish spine event: %w", err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Consume drains the stream into the engine until ctx ends - at-least-once, a batch acks only after the engine took it (and flushed, when it buffers), and the engines collapse redeliveries on the dedupe key
// Redelivery never exhausts (no MaxDeliver): an engine outage is indistinguishable from a poison batch, and only genuinely malformed payloads Term - shedding is the stream's MaxAge alone
func (q *Queue) Consume(ctx context.Context, inner analytics.SpineStore) error {
	stream, err := q.js.Stream(ctx, streamName)
	if err != nil {
		return err
	}
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:    consumerTag,
		AckPolicy:  jetstream.AckExplicitPolicy,
		MaxDeliver: -1,
	})
	if err != nil {
		return err
	}
	flusher, _ := inner.(interface{ Flush(context.Context) error })
	for ctx.Err() == nil {
		batch, err := cons.Fetch(fetchBatch, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("signals queue fetch", "err", err)
			select { // a down server would otherwise spin this loop
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
			continue
		}
		var pending []jetstream.Msg
		var events []analytics.SpineEvent
		for msg := range batch.Messages() {
			var e analytics.SpineEvent
			if err := json.Unmarshal(msg.Data(), &e); err != nil {
				_ = msg.Term() // malformed never gets better
				continue
			}
			pending = append(pending, msg)
			events = append(events, e)
		}
		if err := batch.Error(); err != nil {
			slog.Error("signals queue batch", "err", err) // partial fetch - the missing messages just redeliver
		}
		if len(pending) == 0 {
			continue
		}
		if err := inner.AppendEvents(ctx, events); err != nil {
			slog.Error("signals queue append", "err", err)
			nakAll(pending)
			continue
		}
		if flusher != nil {
			if err := flusher.Flush(ctx); err != nil {
				// the engine re-buffered (or shed and forgot the keys), so redeliver - duplicates collapse
				slog.Error("signals queue flush", "err", err)
				nakAll(pending)
				continue
			}
		}
		for _, m := range pending {
			_ = m.Ack()
		}
	}
	return nil
}

func nakAll(msgs []jetstream.Msg) {
	for _, m := range msgs {
		delay := nakBaseDelay
		if md, err := m.Metadata(); err == nil {
			delay = min(time.Duration(md.NumDelivered)*nakBaseDelay, nakMaxDelay)
		}
		_ = m.NakWithDelay(delay)
	}
}

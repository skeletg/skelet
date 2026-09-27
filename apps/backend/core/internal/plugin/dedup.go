package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/sdk"
)

// EventDedupStore backs sdk.SubscribeAsyncOnce - Claim answers true exactly once per key (durable, so replays across
// restarts are caught), Release un-claims after a handler error so the next redelivery retries
type EventDedupStore interface {
	Claim(ctx context.Context, pluginSlug, eventType, key string) (first bool, err error)
	Release(ctx context.Context, pluginSlug, eventType, key string) error
}

// SetEventDedup wires the durable store, same single-shot contract as SetConfigService
func SetEventDedup(s EventDedupStore) {
	h := installedHost.Load()
	if h == nil {
		panic("plugin.SetEventDedup called before plugin.Install")
	}
	if !h.eventDedup.CompareAndSwap(nil, &s) {
		panic("plugin.SetEventDedup called twice")
	}
}

// SubscribeAsyncOnce wraps the handler in a claim/run/release-on-error cycle - a duplicate key is skipped, a handler error or panic releases the claim and goes back to the outbox, which redelivers, and a missing store degrades to at-least-once delivery with the same retry
func (h *Host) SubscribeAsyncOnce(slug, eventType string, keyFn sdk.DedupKeyFunc, handler sdk.AsyncHandler) error {
	if keyFn == nil {
		return errors.New("plugin: SubscribeAsyncOnce requires a key function")
	}
	if handler == nil {
		return errors.New("plugin: SubscribeAsyncOnce requires a handler")
	}
	wrapped := func(ctx context.Context, payload json.RawMessage) error {
		key, err := keyFn(payload)
		if err != nil {
			return fmt.Errorf("dedup key: %w", err)
		}
		store := h.eventDedup.Load()
		if store == nil {
			return handler(ctx, payload)
		}
		first, err := (*store).Claim(ctx, slug, eventType, key)
		if err != nil {
			return fmt.Errorf("dedup claim: %w", err)
		}
		if !first {
			return nil
		}
		if err := callAsync(ctx, handler, payload); err != nil {
			// un-claim so the next replay retries - a panic included, or its key would stay claimed and never run - a crash between claim and here loses the key, the narrow window this simpler contract accepts
			if rerr := (*store).Release(ctx, slug, eventType, key); rerr != nil {
				return errors.Join(err, fmt.Errorf("dedup release: %w", rerr))
			}
			return err
		}
		return nil
	}
	return h.subscribeAsync(slug, eventType, wrapped, true)
}

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

// pluginEvent is the bus envelope for a plugin-published event - Type answers the subscriber
// lookup, MarshalJSON hands async handlers the exact payload the plugin published
type pluginEvent struct {
	eventType string
	payload   json.RawMessage
}

func (e *pluginEvent) Type() string                 { return e.eventType }
func (e *pluginEvent) MarshalJSON() ([]byte, error) { return e.payload, nil }

// PublishEvent is the seam that makes Manifest.Emits a checked contract instead of a claim: the boot validator resolves every Consumes against the Emits lists, so a publish of an undeclared type would fire subscribers no manifest accounts for - refused here, always, not just in dev
func (h *Host) PublishEvent(ctx context.Context, slug, eventType string, payload any) error {
	if eventType == "" {
		return fmt.Errorf("plugin %q: publish with an empty event type", slug)
	}
	h.manifestMu.Lock()
	m, known := h.manifestBySlug[slug]
	h.manifestMu.Unlock()
	if !known {
		return fmt.Errorf("plugin %q: publish before RegisterPlugin - no manifest to validate Emits against", slug)
	}
	if !slices.Contains(m.Emits, eventType) {
		return fmt.Errorf("plugin %q publishes %q but its manifest does not declare it in Emits - subscribers resolve against that list at boot", slug, eventType)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("plugin %q: marshal %s payload: %w", slug, eventType, err)
	}
	bus := h.bus.Load()
	if bus == nil {
		return fmt.Errorf("plugin %q: publish %s before the event bus is wired", slug, eventType)
	}
	bus.Publish(ctx, &pluginEvent{eventType: eventType, payload: raw})
	return nil
}

var _ events.Event = (*pluginEvent)(nil)

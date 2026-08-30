package webhook

import (
	"fmt"
	"strings"
	"sync"
)

// Describes a subscribable event type plus the execution mode it requires - async events are post-commit notifications, while sync events fire pre-commit and may be rejected or enriched by the webhook response
type EventDefinition struct {
	Type        string
	Name        string
	Description string
	Kind        Kind
}

var (
	regMu    sync.RWMutex
	eventReg []EventDefinition
)

func RegisterEvent(d EventDefinition) {
	if d.Type == "" {
		panic("webhook: event type is empty")
	}
	if d.Kind != KindAsync && d.Kind != KindSync {
		panic(fmt.Sprintf("webhook: event %q has invalid kind %q", d.Type, d.Kind))
	}
	regMu.Lock()
	defer regMu.Unlock()
	for _, existing := range eventReg {
		if existing.Type == d.Type {
			panic(fmt.Sprintf("webhook: event type %q already registered", d.Type))
		}
	}
	eventReg = append(eventReg, d)
}

func AllEvents() []EventDefinition {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]EventDefinition, len(eventReg))
	copy(out, eventReg)
	return out
}

func FindEvent(eventType string) (EventDefinition, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	for _, d := range eventReg {
		if d.Type == eventType {
			return d, true
		}
	}
	return EventDefinition{}, false
}

// Whether a subscription entry is a pattern rather than an exact type - "*" matches every event of the webhook's kind, and "<prefix>.*" every event of that kind under the prefix
func IsWildcard(entry string) bool {
	return entry == "*" || strings.HasSuffix(entry, ".*")
}

// Whether an event type matches a subscription entry, exact or wildcard - mirrors the SQL in ListActiveWebhooksByEvent, so keep the two in sync
func MatchesEntry(entry, eventType string) bool {
	if entry == eventType || entry == "*" {
		return true
	}
	if strings.HasSuffix(entry, ".*") {
		return strings.HasPrefix(eventType, strings.TrimSuffix(entry, "*"))
	}
	return false
}

// Checks every entry is a registered type, or a wildcard matching at least one, AND matches the given webhook kind - a sync webhook may only subscribe to sync events and vice versa, since the two pools are disjoint
// Wildcards validate against the kind's pool only, so "order.*" is fine on both kinds even though the order.* namespace mixes sync and async events
func ValidateEventTypes(types []string, kind Kind) error {
	if kind != KindAsync && kind != KindSync {
		return fmt.Errorf("%w: %q", ErrInvalidKind, kind)
	}
	if len(types) == 0 {
		return fmt.Errorf("%w: at least one event type is required", ErrInvalidEventTypes)
	}
	regMu.RLock()
	defer regMu.RUnlock()
	known := make(map[string]Kind, len(eventReg))
	for _, d := range eventReg {
		known[d.Type] = d.Kind
	}
	var unknown, mismatched []string
	for _, t := range types {
		if IsWildcard(t) {
			matched := false
			for _, d := range eventReg {
				if d.Kind == kind && MatchesEntry(t, d.Type) {
					matched = true
					break
				}
			}
			if !matched {
				unknown = append(unknown, fmt.Sprintf("%s (matches no %s events)", t, kind))
			}
			continue
		}
		k, ok := known[t]
		if !ok {
			unknown = append(unknown, t)
			continue
		}
		if k != kind {
			mismatched = append(mismatched, fmt.Sprintf("%s (is %s)", t, k))
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: unknown event types: %s", ErrInvalidEventTypes, strings.Join(unknown, ", "))
	}
	if len(mismatched) > 0 {
		return fmt.Errorf("%w: %s webhook cannot subscribe to %s", ErrKindEventMismatch, kind, strings.Join(mismatched, ", "))
	}
	return nil
}

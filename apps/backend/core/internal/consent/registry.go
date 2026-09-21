// Package consent is a code-defined registry of consent kinds a customer can opt in or out of - newsletters, transactional email, future SMS and push - plus the storage and lookup surface for their current state
// The pattern mirrors config and internal/permission: domain packages declare kinds in init() via Register, main.go calls Validate() at startup to fail fast, and the service resolves customer state with a registry-default fallback, so adding a kind is code-only with no migration or backfill
package consent

import (
	"fmt"
	"strings"
	"sync"
)

// Labels the legal regime a consent falls under
type Purpose string

// A future hint for routing - email now, SMS and push later
type Channel string

const (
	PurposeMarketing     Purpose = "marketing"
	PurposeTransactional Purpose = "transactional"

	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
	ChannelPush  Channel = "push"
)

// Describes a single consent kind - immutable once registered
type Definition struct {
	Slug         string  // stable identifier; also the DB key
	Name         string  // human-readable label
	Description  string  // shown to customers in preference UI / register form
	Purpose      Purpose // marketing vs transactional - affects legal rules
	Channel      Channel // email/sms/push
	Required     bool    // true → customer cannot opt out (transactional)
	DefaultOptIn bool    // state when no DB row exists - GDPR: MUST be false for marketing
}

var (
	mu       sync.RWMutex
	registry []Definition
)

// Panics on a duplicate slug, since two packages declaring the same kind is a bug that should surface immediately
func Register(d Definition) {
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range registry {
		if existing.Slug == d.Slug {
			panic(fmt.Sprintf("consent: slug %q already registered", d.Slug))
		}
	}
	registry = append(registry, d)
}

func AllDefinitions() []Definition {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Definition, len(registry))
	copy(out, registry)
	return out
}

func Find(slug string) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, d := range registry {
		if d.Slug == slug {
			return d, true
		}
	}
	return Definition{}, false
}

// Checks every registered kind for structural and legal sanity, called from main.go after all init() functions have run Slugs must be lowercase, non-empty and whitespace-free; Required with Purpose=Marketing is rejected, since marketing consent must be optional under GDPR; Required implies DefaultOptIn, because required-but-off is nonsense Marketing with DefaultOptIn is a compliance red flag, warned through the returned error so reviewers catch it before shipping
func Validate() error {
	mu.RLock()
	defer mu.RUnlock()
	var problems []string
	for _, d := range registry {
		if d.Slug == "" {
			problems = append(problems, "empty slug")
			continue
		}
		if d.Slug != strings.ToLower(d.Slug) {
			problems = append(problems, fmt.Sprintf("slug %q must be lowercase", d.Slug))
		}
		if strings.ContainsAny(d.Slug, " \t\n") {
			problems = append(problems, fmt.Sprintf("slug %q contains whitespace", d.Slug))
		}
		if d.Required && d.Purpose == PurposeMarketing {
			problems = append(problems, fmt.Sprintf("%q: marketing consent cannot be Required", d.Slug))
		}
		if d.Required && !d.DefaultOptIn {
			problems = append(problems, fmt.Sprintf("%q: Required=true implies DefaultOptIn=true", d.Slug))
		}
		if d.Purpose == PurposeMarketing && d.DefaultOptIn {
			problems = append(problems, fmt.Sprintf("%q: marketing consent with DefaultOptIn=true is not GDPR-compliant", d.Slug))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("consent registry invalid: %s", strings.Join(problems, "; "))
}

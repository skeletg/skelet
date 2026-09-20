package consent

import (
	"github.com/google/uuid"
)

const EventChanged = "consent.changed"

// Fires after an opt-in or opt-out is stored - consent is account-global, so it carries no channel and per-channel webhooks never match it
// Marketing tools have to hear this: an unsubscribe that reaches nobody keeps mailing someone who asked you to stop
type ConsentChanged struct {
	CustomerID uuid.UUID
	Kind       string
	OptedIn    bool
	// One of the Source* constants, so a handler can tell an unsubscribe link from an admin edit
	Source string
}

func (*ConsentChanged) Type() string { return EventChanged }

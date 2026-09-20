package flow

import (
	"context"

	"github.com/google/uuid"
)

// What the engine needs from outside its own tables, answered from the rosters core pushes across

// One person a run can reach, with both consents as core last pushed them
// Consent is read here rather than carried on the run: someone who unsubscribes between enrolling and being sent to must not be sent to
type Contact struct {
	ID           uuid.UUID
	ChannelID    uuid.UUID
	Email        string
	FirstName    string
	Phone        string
	EmailOptedIn bool
	SMSOptedIn   bool
}

type contactReader interface {
	Get(ctx context.Context, customerID uuid.UUID) (Contact, bool, error)
}

// The shop a run sends as - the customer-facing name, the storefront the links point at, and the
// sender identity core resolved from its settings, all off the channel roster
type Store struct {
	ID          uuid.UUID
	Name        string
	PublicURL   string
	FromAddress string
	FromName    string
	LogoURL     string
}

type storeReader interface {
	Get(ctx context.Context, channelID uuid.UUID) (Store, error)
}

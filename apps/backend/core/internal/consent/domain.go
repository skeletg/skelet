package consent

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUnknownConsent = errors.New("unknown consent kind")

	ErrConsentRequired = errors.New("consent is required and cannot be opted out of")

	ErrUnknownSource = errors.New("unknown consent source")
)

// Where a consent change came from - SetConsent refuses anything else, so the audit history only ever carries these
const (
	SourceRegistration    = "registration"
	SourceProfile         = "profile"
	SourceCheckout        = "checkout"
	SourceUnsubscribeLink = "unsubscribe_link"
	SourceAdmin           = "admin"
	// A customer.registering plugin decided, where the shopper was shown no choice - kept distinct so an audit can tell it from the shopper's own
	SourcePlugin = "plugin"
	// A signup form, recorded only once the address owner followed the confirmation link
	SourceSignupForm = "signup_form"
)

func validSource(s string) bool {
	switch s {
	case SourceRegistration, SourceProfile, SourceCheckout, SourceUnsubscribeLink, SourceAdmin, SourcePlugin, SourceSignupForm:
		return true
	}
	return false
}

type Consent struct {
	Definition Definition
	OptedIn    bool
	Source     string
	UpdatedAt  time.Time // zero when IsDefault = true
	IsDefault  bool      // true → no DB row; using Definition.DefaultOptIn
}

type StoredConsent struct {
	CustomerID uuid.UUID
	Kind       string
	OptedIn    bool
	Source     string
	UpdatedAt  time.Time
}

// One write to a customer's consent - AdminID is set only when a person in the dashboard made it
type Change struct {
	CustomerID uuid.UUID
	Kind       string
	OptedIn    bool
	Source     string
	AdminID    *uuid.UUID
}

type Event struct {
	ID         uuid.UUID
	CustomerID uuid.UUID
	Kind       string
	OptedIn    bool
	Source     string
	AdminID    *uuid.UUID
	CreatedAt  time.Time
}

type Repository interface {
	// Writes the current state and appends its history entry as one statement
	Upsert(ctx context.Context, c Change) (StoredConsent, error)
	// The same write inside a caller's transaction, so an opt-out and the event carrying it cannot land apart
	UpsertTx(ctx context.Context, sqlTx *sql.Tx, c Change) (StoredConsent, error)
	// Returns a nil pointer when no row exists, and the service falls back to Definition.DefaultOptIn - an error means the DB is actually broken, not "no row"
	Get(ctx context.Context, customerID uuid.UUID, kind string) (*StoredConsent, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]StoredConsent, error)
	ListEvents(ctx context.Context, customerID uuid.UUID, limit int32) ([]Event, error)
}

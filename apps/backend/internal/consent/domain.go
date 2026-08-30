package consent

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUnknownConsent = errors.New("unknown consent kind")

	ErrConsentRequired = errors.New("consent is required and cannot be opted out of")
)

// Labels where a consent change came from - free-form strings, with the well-known values below - Admin UI and marketing tools filter by source when building audit reports
const (
	SourceRegistration    = "registration"
	SourceProfile         = "profile"
	SourceCheckout        = "checkout"
	SourceUnsubscribeLink = "unsubscribe_link"
	SourceAdmin           = "admin"
)

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

type Repository interface {
	Upsert(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) (StoredConsent, error)
	// Returns a nil pointer when no row exists, and the service falls back to Definition.DefaultOptIn - an error means the DB is actually broken, not "no row"
	Get(ctx context.Context, customerID uuid.UUID, kind string) (*StoredConsent, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID) ([]StoredConsent, error)
	ListByCustomerIDs(ctx context.Context, customerIDs []uuid.UUID, kind string) ([]StoredConsent, error)
	DeleteByCustomer(ctx context.Context, customerID uuid.UUID) error
}

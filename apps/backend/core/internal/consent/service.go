package consent

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Registry lookup, default fallback when no DB row exists, and the "required can't be opted out" rule
type Service struct {
	repo Repository
	bus  *events.Bus
	// The change event goes durable through these when wired - see outbox.go
	outbox OutboxEnqueuer
	tx     *txrunner.Manager
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Optional - without it a consent change is silent, which is only safe where nothing acts on one
func (s *Service) SetEventBus(b *events.Bus) { s.bus = b }

// Records an opt-in or opt-out - rejects unknown kinds with ErrUnknownConsent, opting out of required kinds with ErrConsentRequired and a source outside the Source* constants with ErrUnknownSource
func (s *Service) SetConsent(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, source string) (Consent, error) {
	return s.set(ctx, Change{CustomerID: customerID, Kind: kind, OptedIn: optedIn, Source: source})
}

// The dashboard's write - the history names the admin who made it, nil when an integration token did
func (s *Service) SetConsentByAdmin(ctx context.Context, customerID uuid.UUID, kind string, optedIn bool, adminID *uuid.UUID) (Consent, error) {
	return s.set(ctx, Change{CustomerID: customerID, Kind: kind, OptedIn: optedIn, Source: SourceAdmin, AdminID: adminID})
}

func (s *Service) set(ctx context.Context, c Change) (Consent, error) {
	def, ok := Find(c.Kind)
	if !ok {
		return Consent{}, ErrUnknownConsent
	}
	if !validSource(c.Source) {
		return Consent{}, ErrUnknownSource
	}
	if def.Required && !c.OptedIn {
		return Consent{}, ErrConsentRequired
	}
	stored, err := s.upsertAndPublish(ctx, c)
	if err != nil {
		return Consent{}, err
	}
	return Consent{
		Definition: def,
		OptedIn:    stored.OptedIn,
		Source:     stored.Source,
		UpdatedAt:  stored.UpdatedAt,
		IsDefault:  false,
	}, nil
}

func (s *Service) History(ctx context.Context, customerID uuid.UUID, limit int32) ([]Event, error) {
	if limit <= 0 || limit > maxHistory {
		limit = maxHistory
	}
	return s.repo.ListEvents(ctx, customerID, limit)
}

const maxHistory = 500

// A single consent, falling back to the registry default when no DB row exists - an unknown kind returns ErrUnknownConsent, which the resolver surfaces as a GraphQL error, since it's a developer bug rather than a runtime condition to hide
func (s *Service) Get(ctx context.Context, customerID uuid.UUID, kind string) (Consent, error) {
	def, ok := Find(kind)
	if !ok {
		return Consent{}, ErrUnknownConsent
	}
	stored, err := s.repo.Get(ctx, customerID, kind)
	if err != nil {
		return Consent{}, err
	}
	if stored == nil {
		return Consent{
			Definition: def,
			OptedIn:    def.DefaultOptIn,
			Source:     "", // no source yet; we're using the definition default
			IsDefault:  true,
		}, nil
	}
	return Consent{
		Definition: def,
		OptedIn:    stored.OptedIn,
		Source:     stored.Source,
		UpdatedAt:  stored.UpdatedAt,
		IsDefault:  false,
	}, nil
}

// One Consent per registered Definition in registration order, with either the stored state or the definition default
// A customer who has never touched their preferences gets the full registry with IsDefault on every row, which is exactly what a preference-center UI needs to render the initial form
func (s *Service) ListForCustomer(ctx context.Context, customerID uuid.UUID) ([]Consent, error) {
	defs := AllDefinitions()
	rows, err := s.repo.ListByCustomer(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("list stored consents: %w", err)
	}
	byKind := make(map[string]StoredConsent, len(rows))
	for _, row := range rows {
		byKind[row.Kind] = row
	}
	out := make([]Consent, 0, len(defs))
	for _, def := range defs {
		if stored, ok := byKind[def.Slug]; ok {
			out = append(out, Consent{
				Definition: def,
				OptedIn:    stored.OptedIn,
				Source:     stored.Source,
				UpdatedAt:  stored.UpdatedAt,
				IsDefault:  false,
			})
			continue
		}
		out = append(out, Consent{
			Definition: def,
			OptedIn:    def.DefaultOptIn,
			IsDefault:  true,
		})
	}
	return out, nil
}

// The single-customer consent check, with the same default-fallback as Get, so a customer with no row resolves to the GDPR-safe opt-out for marketing
func (s *Service) IsOptedIn(ctx context.Context, customerID uuid.UUID, kind string) (bool, error) {
	c, err := s.Get(ctx, customerID, kind)
	if err != nil {
		return false, err
	}
	return c.OptedIn, nil
}

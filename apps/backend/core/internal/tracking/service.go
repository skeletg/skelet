package tracking

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// store is what the service and the worker need of the database - the postgres repository, or a test's fake
type store interface {
	ListEnabled(ctx context.Context, channelID uuid.UUID) ([]Destination, error)
	List(ctx context.Context, channelID uuid.UUID) ([]Destination, error)
	Get(ctx context.Context, id uuid.UUID) (Destination, error)
	Create(ctx context.Context, d Destination) (Destination, error)
	Update(ctx context.Context, d Destination) (Destination, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Enqueue(ctx context.Context, destinationID uuid.UUID, ev Event) error
	EnqueueCatalog(ctx context.Context, destinationID uuid.UUID) error
	ClaimDue(ctx context.Context, batch, leaseSeconds int32, catalog bool) ([]Delivery, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	Reschedule(ctx context.Context, id uuid.UUID, at time.Time, reason string) error
	MarkDead(ctx context.Context, id uuid.UUID, reason string) error
	Retry(ctx context.Context, id uuid.UUID) error
	ListDeliveries(ctx context.Context, destinationID uuid.UUID, limit int32) ([]Delivery, error)
	Prune(ctx context.Context, before time.Time) (int64, error)
}

// Settings reads a channel's switches - satisfied by *config.Service
type Settings interface {
	GetBool(ctx context.Context, path string, scope config.Scope) (bool, error)
}

type Service struct {
	store    store
	settings Settings // nil: no kind is ever treated as necessary
}

func NewService(q *db.Queries, sealer Sealer, settings Settings) *Service {
	return &Service{store: &repository{q: q, sealer: sealer}, settings: settings}
}

var (
	ErrUnknownKind  = errors.New("unknown tracking destination kind")
	ErrNameMissing  = errors.New("a tracking destination needs a name")
	ErrFieldMissing = errors.New("a required setting is missing")
)

// Track hands one event to every enabled destination on its channel that wants it and that the visitor's consent allows.
// It only enqueues - delivery is the worker's - so a slow platform never holds up the shop
func (s *Service) Track(ctx context.Context, ev Event) error {
	if ev.ID == "" || ev.Name == "" || ev.ChannelID == uuid.Nil {
		return fmt.Errorf("tracking event needs an id, a name and a channel")
	}
	dests, err := s.store.ListEnabled(ctx, ev.ChannelID)
	if err != nil {
		return err
	}
	var errs []error
	for _, d := range dests {
		k, ok := KindFor(d.Kind)
		if !ok || k.Catalog || !wants(d, k, ev.Name) || (k.Available != nil && !k.Available(ctx, ev.ChannelID)) || !Allowed(s.purposes(ctx, d, k), ev.Consent) {
			continue
		}
		if err := s.store.Enqueue(ctx, d.ID, ev); err != nil {
			errs = append(errs, err) // the replay this error triggers is a no-op for the destinations that did enqueue
		}
	}
	return errors.Join(errs...)
}

func wants(d Destination, k Kind, name string) bool {
	events := d.Events
	if len(events) == 0 {
		events = k.DefaultEvents
	}
	return len(events) == 0 || slices.Contains(events, name)
}

// The kind's purposes, unless the merchant made it necessary - a setting that fails to read leaves consent required
func (s *Service) purposes(ctx context.Context, d Destination, k Kind) []Purpose {
	if k.NecessarySetting != "" && s.settings != nil {
		on, err := s.settings.GetBool(ctx, k.NecessarySetting, config.Scope{Kind: config.ScopeChannel, ChannelID: &d.ChannelID})
		if err == nil && on {
			return []Purpose{PurposeNecessary}
		}
	}
	return k.Purposes
}

func (s *Service) List(ctx context.Context, channelID uuid.UUID) ([]Destination, error) {
	return s.store.List(ctx, channelID)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Destination, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, d Destination) (Destination, error) {
	if err := validate(d, true); err != nil {
		return Destination{}, err
	}
	return s.store.Create(ctx, d)
}

// An empty Secret keeps the stored one. The kind and channel never change - a different platform is a new destination
func (s *Service) Update(ctx context.Context, d Destination) (Destination, error) {
	current, err := s.store.Get(ctx, d.ID)
	if err != nil {
		return Destination{}, err
	}
	d.Kind, d.ChannelID = current.Kind, current.ChannelID
	if err := validate(d, false); err != nil {
		return Destination{}, err
	}
	return s.store.Update(ctx, d)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.Delete(ctx, id)
}

func (s *Service) Deliveries(ctx context.Context, destinationID uuid.UUID, limit int32) ([]Delivery, error) {
	return s.store.ListDeliveries(ctx, destinationID, limit)
}

// Redeliver puts a dead letter back in the queue for a fresh round of attempts
func (s *Service) Redeliver(ctx context.Context, deliveryID uuid.UUID) error {
	return s.store.Retry(ctx, deliveryID)
}

// A new destination needs every required field, its secret included; an update may leave the secret empty to keep it
func validate(d Destination, creating bool) error {
	k, ok := KindFor(d.Kind)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKind, d.Kind)
	}
	if strings.TrimSpace(d.Name) == "" {
		return ErrNameMissing
	}
	for _, f := range k.Fields {
		switch {
		case !f.Required:
		case f.Secret && creating && d.Secret == "", !f.Secret && strings.TrimSpace(d.Config[f.Key]) == "":
			return fmt.Errorf("%w: %s", ErrFieldMissing, f.Label)
		}
	}
	return nil
}

// SendTest sends one sample purchase to a destination now, outside the queue, and returns the platform's answer - for
// checking a setup, so it is marked as a test where the platform has a way to say so (GA4's validate, Meta's test code)
func (s *Service) SendTest(ctx context.Context, client *http.Client, id uuid.UUID) (Event, error) {
	d, err := s.store.Get(ctx, id)
	if err != nil {
		return Event{}, err
	}
	k, ok := KindFor(d.Kind)
	if !ok {
		return Event{}, fmt.Errorf("%w: %q", ErrUnknownKind, d.Kind)
	}
	if k.Catalog {
		return Event{}, fmt.Errorf("use tracking catalog-send for a catalogue destination")
	}
	ev := Event{
		ID: "test-" + uuid.NewString(), Name: Purchase, OccurredAt: time.Now().UTC(), ChannelID: d.ChannelID, Source: "server",
		Consent:   &auth.Consent{Analytics: true, Ads: true, Personalization: true},
		User:      User{AnonID: "test-visitor", Email: "test@example.com"},
		Ecommerce: &Ecommerce{Currency: "EUR", Value: 10, TransactionID: "TEST-1", Items: []Item{{ItemID: "TEST-SKU", ItemName: "Test item", Price: 10, Quantity: 1}}},
	}
	return ev, k.Send(ctx, client, d, ev)
}

const CatalogSnapshot = "catalog_snapshot"

// Pending refreshes coalesce. Retries read current catalogue data and account credentials.
func (s *Service) QueueCatalog(ctx context.Context, id uuid.UUID) error {
	d, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	k, ok := KindFor(d.Kind)
	if !ok || !k.Catalog {
		return fmt.Errorf("destination is not a catalogue destination")
	}
	if !d.Enabled || (k.Available != nil && !k.Available(ctx, d.ChannelID)) {
		return fmt.Errorf("catalogue destination is disabled")
	}
	return s.store.EnqueueCatalog(ctx, id)
}

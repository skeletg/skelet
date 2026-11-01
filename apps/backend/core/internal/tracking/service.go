package tracking

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
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
	ClaimDue(ctx context.Context, batch, leaseSeconds int32) ([]Delivery, error)
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
	ErrUnknownKind = errors.New("unknown tracking destination kind")
	ErrNameMissing = errors.New("a tracking destination needs a name")
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
		if !ok || !wants(d, k, ev.Name) || !Allowed(s.purposes(ctx, d, k), ev.Consent) {
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
	if err := validate(d); err != nil {
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
	if err := validate(d); err != nil {
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

func validate(d Destination) error {
	if _, ok := KindFor(d.Kind); !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKind, d.Kind)
	}
	if strings.TrimSpace(d.Name) == "" {
		return ErrNameMissing
	}
	return nil
}

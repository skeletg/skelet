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
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// store is what the service and the worker need of the database - the postgres repository, or a test's fake
type store interface {
	ListEnabled(ctx context.Context, channelID uuid.UUID) ([]Destination, error)
	List(ctx context.Context, channelID uuid.UUID) ([]Destination, error)
	Get(ctx context.Context, id uuid.UUID) (Destination, error)
	Create(ctx context.Context, d Destination) (Destination, error)
	Update(ctx context.Context, d Destination) (Destination, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Enqueue(ctx context.Context, destinationIDs []uuid.UUID, ev Event) error // the event once, a delivery per destination
	ClaimDue(ctx context.Context, batch, leaseSeconds int32) ([]Delivery, error)
	MarkSent(ctx context.Context, id uuid.UUID) error // the row goes - the log is what is pending or failed
	Reschedule(ctx context.Context, id uuid.UUID, at time.Time, reason string) error
	MarkDead(ctx context.Context, id uuid.UUID, reason string) error
	Retry(ctx context.Context, id uuid.UUID) (bool, error)
	GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error)
	ListDeliveries(ctx context.Context, destinationID uuid.UUID, status string, limit int32) ([]Delivery, error)
	Import(ctx context.Context, d Destination, legacyID uuid.UUID) (Destination, bool, error)
	Health(ctx context.Context, channelID uuid.UUID) (map[uuid.UUID]Health, error)
	Prune(ctx context.Context, logBefore, replayBefore time.Time) (int64, error)
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
	ErrFieldUnknown = errors.New("not a setting of this destination - a secret goes in the secret, never the settings")
	ErrNeedsSecret  = errors.New("enter the destination's secret before turning it on")
	ErrNotDead      = errors.New("only a delivery that gave up can be retried")
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
	var to []uuid.UUID
	for _, d := range dests {
		if s.routes(ctx, d, ev) {
			to = append(to, d.ID)
		}
	}
	if len(to) == 0 {
		return nil
	}
	return s.store.Enqueue(ctx, to, ev)
}

// routes is whether an event goes to a destination: a kind this build has, switched on for the channel, wanting the
// event, and allowed by the visitor's consent unless the kind is built to act on it regardless
func (s *Service) routes(ctx context.Context, d Destination, ev Event) bool {
	k, ok := KindFor(d.Kind)
	switch {
	case !ok, !wants(d, k, ev.Name), k.Available != nil && !k.Available(ctx, ev.ChannelID):
		return false
	case k.takesRegardless(ev.Name):
		return true
	}
	return Allowed(s.purposes(ctx, d, k), ev.Consent)
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
	d.Kind, d.ChannelID, d.ImportedFrom = current.Kind, current.ChannelID, current.ImportedFrom
	if err := validate(d, false); err != nil {
		return Destination{}, err
	}
	if d.Enabled && d.Secret == "" && NeedsSecret(current) {
		return Destination{}, ErrNeedsSecret // an imported row would fail every delivery until its secret is entered
	}
	return s.store.Update(ctx, d)
}

// NeedsSecret reports a destination whose kind requires a secret it doesn't have - an imported one, until the admin
// enters it
func NeedsSecret(d Destination) bool {
	k, ok := KindFor(d.Kind)
	if !ok || d.Secret != "" {
		return false
	}
	for _, f := range k.Fields {
		if f.Secret && f.Required {
			return true
		}
	}
	return false
}

// Imported is a destination copied from elsewhere, with the id of the one it was copied from
type Imported struct {
	Destination
	LegacyID uuid.UUID
}

// InTestMode reports a destination whose kind's test-mode setting is on - its sends stay out of the platform's live
// reports. A kind without one is never in test mode
func InTestMode(d Destination) bool {
	k, _ := KindFor(d.Kind)
	for _, f := range k.Fields {
		if f.TestMode {
			v := d.Config[f.Key]
			return v != "" && v != "false"
		}
	}
	return false
}

// Import copies destinations from elsewhere (old Signals ones) - always disabled and without a secret, which the admin
// enters before turning one on. Every one is checked before any is saved, so a refusal imports nothing. It returns the
// newly imported; one imported before is left as it was
func (s *Service) Import(ctx context.Context, items []Imported) ([]Destination, error) {
	for _, it := range items {
		if err := validate(it.Destination, false); err != nil {
			return nil, fmt.Errorf("%s: %w", it.Name, err)
		}
	}
	out := []Destination{}
	for _, it := range items {
		d, imported, err := s.store.Import(ctx, it.Destination, it.LegacyID)
		if err != nil {
			return nil, err
		}
		if imported {
			out = append(out, d)
		}
	}
	return out, nil
}

// Health is each destination of a channel with its last success, last failure and failures in the past day
func (s *Service) Health(ctx context.Context, channelID uuid.UUID) (map[uuid.UUID]Health, error) {
	return s.store.Health(ctx, channelID)
}

func (s *Service) Delivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	return s.store.GetDelivery(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.store.Delete(ctx, id)
}

// Deliveries is a destination's log, newest first - an empty status lists every row
func (s *Service) Deliveries(ctx context.Context, destinationID uuid.UUID, status string, limit int32) ([]Delivery, error) {
	return s.store.ListDeliveries(ctx, destinationID, status, limit)
}

// Redeliver puts a dead letter back in the queue for a fresh round of attempts
func (s *Service) Redeliver(ctx context.Context, deliveryID uuid.UUID) error {
	ok, err := s.store.Retry(ctx, deliveryID)
	if err == nil && !ok {
		return ErrNotDead
	}
	return err
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
	// The settings are stored as plain JSON and only the secret is sealed, so only a declared, non-secret field goes there
	for key := range d.Config {
		if i := slices.IndexFunc(k.Fields, func(f sdk.DestinationField) bool { return f.Key == key }); i < 0 || k.Fields[i].Secret {
			return fmt.Errorf("%w: %s", ErrFieldUnknown, key)
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
	ev := Event{
		ID: "test-" + uuid.NewString(), Name: Purchase, OccurredAt: time.Now().UTC(), ChannelID: d.ChannelID, Source: "server",
		Consent:   &sdk.ShopConsent{Analytics: true, Ads: true, Personalization: true},
		User:      User{AnonID: "test-visitor", Email: "test@example.com"},
		Ecommerce: &Ecommerce{Currency: "EUR", Value: 10, TransactionID: "TEST-1", Items: []Item{{ItemID: "TEST-SKU", ItemName: "Test item", Price: 10, Quantity: 1}}},
	}
	if err := k.Send(ctx, client, d.config(), ev); err != nil {
		// The platform's answer goes back to an admin screen - a transport error can quote the request URL, secret and all
		return ev, errors.New(redact(err.Error(), &d))
	}
	return ev, nil
}

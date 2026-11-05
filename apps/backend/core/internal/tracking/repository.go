package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

// Sealer keeps destination secrets encrypted at rest - satisfied by *secrets.Cipher, whose Decrypt passes unsealed plaintext through
type Sealer interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(stored string) (string, error)
}

// Delivery is one event bound for one destination - pending in the queue, then the log entry once sent or dead
type Delivery struct {
	ID            uuid.UUID
	DestinationID uuid.UUID
	EventID       string
	EventName     string
	Payload       json.RawMessage
	Status        string
	Attempts      int32
	NextAttemptAt time.Time
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type repository struct {
	q      *db.Queries
	sealer Sealer // nil stores secrets as given, for a dev stack with no key
}

func (r *repository) seal(plain string) (string, error) {
	if plain == "" || r.sealer == nil {
		return plain, nil
	}
	return r.sealer.Encrypt(plain)
}

func (r *repository) open(stored string) (string, error) {
	if !secrets.IsEncrypted(stored) {
		return stored, nil
	}
	if r.sealer == nil {
		return "", fmt.Errorf("destination secret is encrypted but no key is configured")
	}
	return r.sealer.Decrypt(stored)
}

func (r *repository) toDestination(row db.TrackingDestination) (Destination, error) {
	d := Destination{
		ID: row.ID, ChannelID: row.ChannelID, Kind: row.Kind, Name: row.Name, Enabled: row.Enabled,
		Events: row.Events, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if err := json.Unmarshal(row.Config, &d.Config); err != nil {
		return Destination{}, fmt.Errorf("destination %s config: %w", row.ID, err)
	}
	secret, err := r.open(row.Secret)
	if err != nil {
		return Destination{}, fmt.Errorf("destination %s secret: %w", row.ID, err)
	}
	d.Secret = secret
	return d, nil
}

func (r *repository) toDestinations(rows []db.TrackingDestination) ([]Destination, error) {
	out := make([]Destination, 0, len(rows))
	for _, row := range rows {
		d, err := r.toDestination(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (r *repository) ListEnabled(ctx context.Context, channelID uuid.UUID) ([]Destination, error) {
	rows, err := r.q.ListEnabledTrackingDestinations(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list tracking destinations: %w", err)
	}
	return r.toDestinations(rows)
}

func (r *repository) List(ctx context.Context, channelID uuid.UUID) ([]Destination, error) {
	rows, err := r.q.ListTrackingDestinations(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list tracking destinations: %w", err)
	}
	return r.toDestinations(rows)
}

func (r *repository) Get(ctx context.Context, id uuid.UUID) (Destination, error) {
	row, err := r.q.GetTrackingDestination(ctx, id)
	if err != nil {
		return Destination{}, fmt.Errorf("get tracking destination: %w", err)
	}
	return r.toDestination(row)
}

func (r *repository) Create(ctx context.Context, d Destination) (Destination, error) {
	cfg, _ := json.Marshal(d.Config) // map[string]string - can't fail
	secret, err := r.seal(d.Secret)
	if err != nil {
		return Destination{}, fmt.Errorf("seal destination secret: %w", err)
	}
	row, err := r.q.CreateTrackingDestination(ctx, db.CreateTrackingDestinationParams{
		ChannelID: d.ChannelID, Kind: d.Kind, Name: d.Name, Enabled: d.Enabled,
		Events: nonNil(d.Events), Config: cfg, Secret: secret,
	})
	if err != nil {
		return Destination{}, fmt.Errorf("create tracking destination: %w", err)
	}
	return r.toDestination(row)
}

// An empty Secret keeps the stored one
func (r *repository) Update(ctx context.Context, d Destination) (Destination, error) {
	cfg, _ := json.Marshal(d.Config)
	secret, err := r.seal(d.Secret)
	if err != nil {
		return Destination{}, fmt.Errorf("seal destination secret: %w", err)
	}
	row, err := r.q.UpdateTrackingDestination(ctx, db.UpdateTrackingDestinationParams{
		ID: d.ID, Name: d.Name, Enabled: d.Enabled, Events: nonNil(d.Events), Config: cfg, Secret: secret,
	})
	if err != nil {
		return Destination{}, fmt.Errorf("update tracking destination: %w", err)
	}
	return r.toDestination(row)
}

func (r *repository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteTrackingDestination(ctx, id)
}

func (r *repository) Enqueue(ctx context.Context, destinationID uuid.UUID, ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", ev.Name, err)
	}
	if err := r.q.EnqueueTrackingDelivery(ctx, db.EnqueueTrackingDeliveryParams{
		DestinationID: destinationID, EventID: ev.ID, EventName: ev.Name, Payload: payload,
	}); err != nil {
		return fmt.Errorf("enqueue %s for %s: %w", ev.Name, destinationID, err)
	}
	return nil
}

func (r *repository) ClaimDue(ctx context.Context, batch, leaseSeconds int32) ([]Delivery, error) {
	rows, err := r.q.ClaimDueTrackingDeliveries(ctx, db.ClaimDueTrackingDeliveriesParams{BatchSize: batch, LeaseSeconds: leaseSeconds})
	if err != nil {
		return nil, fmt.Errorf("claim tracking deliveries: %w", err)
	}
	out := make([]Delivery, len(rows))
	for i, row := range rows {
		out[i] = toDelivery(row)
	}
	return out, nil
}

func (r *repository) MarkSent(ctx context.Context, id uuid.UUID) error {
	return r.q.MarkTrackingDeliverySent(ctx, id)
}

func (r *repository) Reschedule(ctx context.Context, id uuid.UUID, at time.Time, reason string) error {
	return r.q.RescheduleTrackingDelivery(ctx, db.RescheduleTrackingDeliveryParams{ID: id, NextAttemptAt: at, LastError: reason})
}

func (r *repository) MarkDead(ctx context.Context, id uuid.UUID, reason string) error {
	return r.q.MarkTrackingDeliveryDead(ctx, db.MarkTrackingDeliveryDeadParams{ID: id, LastError: reason})
}

func (r *repository) Retry(ctx context.Context, id uuid.UUID) error {
	return r.q.RetryTrackingDelivery(ctx, id)
}

func (r *repository) ListDeliveries(ctx context.Context, destinationID uuid.UUID, limit int32) ([]Delivery, error) {
	rows, err := r.q.ListTrackingDeliveries(ctx, db.ListTrackingDeliveriesParams{DestinationID: destinationID, Lim: limit})
	if err != nil {
		return nil, fmt.Errorf("list tracking deliveries: %w", err)
	}
	out := make([]Delivery, len(rows))
	for i, row := range rows {
		out[i] = toDelivery(row)
	}
	return out, nil
}

func (r *repository) Prune(ctx context.Context, before time.Time) (int64, error) {
	return r.q.PruneTrackingDeliveries(ctx, before)
}

func toDelivery(row db.TrackingDelivery) Delivery {
	return Delivery{
		ID: row.ID, DestinationID: row.DestinationID, EventID: row.EventID, EventName: row.EventName, Payload: row.Payload,
		Status: row.Status, Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt, LastError: row.LastError,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

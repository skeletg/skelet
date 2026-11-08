package tracking

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
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

// toRoute is a destination without its secret - all Track needs to pick where an event goes, so the hot path never
// decrypts one
func toRoute(row db.TrackingDestination) (Destination, error) {
	d := Destination{
		ID: row.ID, ChannelID: row.ChannelID, Kind: row.Kind, Name: row.Name, Enabled: row.Enabled,
		Events: row.Events, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.LastSentAt.Valid {
		d.LastSentAt = &row.LastSentAt.Time
	}
	if err := json.Unmarshal(row.Config, &d.Config); err != nil {
		return Destination{}, fmt.Errorf("destination %s config: %w", row.ID, err)
	}
	return d, nil
}

func (r *repository) toDestination(row db.TrackingDestination) (Destination, error) {
	d, err := toRoute(row)
	if err != nil {
		return Destination{}, err
	}
	secret, err := r.open(row.Secret)
	if err != nil {
		return Destination{}, fmt.Errorf("destination %s secret: %w", row.ID, err)
	}
	d.Secret = secret
	return d, nil
}

func toDestinations(rows []db.TrackingDestination, conv func(db.TrackingDestination) (Destination, error)) ([]Destination, error) {
	out := make([]Destination, 0, len(rows))
	for _, row := range rows {
		d, err := conv(row)
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
	return toDestinations(rows, toRoute)
}

func (r *repository) List(ctx context.Context, channelID uuid.UUID) ([]Destination, error) {
	rows, err := r.q.ListTrackingDestinations(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list tracking destinations: %w", err)
	}
	return toDestinations(rows, r.toDestination)
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

func (r *repository) Enqueue(ctx context.Context, destinationIDs []uuid.UUID, ev Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("encode %s event: %w", ev.Name, err)
	}
	if err := r.q.EnqueueTrackingEvent(ctx, db.EnqueueTrackingEventParams{
		DestinationIds: destinationIDs, ChannelID: ev.ChannelID, EventID: ev.ID, EventName: ev.Name,
		CustomerID: sqlnull.UUID(ev.User.CustomerID), AnonID: ev.User.AnonID,
		Email: strings.ToLower(strings.TrimSpace(ev.User.Email)), Payload: payload,
	}); err != nil {
		return fmt.Errorf("enqueue %s: %w", ev.Name, err)
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
	return r.q.DeleteTrackingDelivery(ctx, id)
}

func (r *repository) Reschedule(ctx context.Context, id uuid.UUID, at time.Time, reason string) error {
	return r.q.RescheduleTrackingDelivery(ctx, db.RescheduleTrackingDeliveryParams{ID: id, NextAttemptAt: at, LastError: reason})
}

func (r *repository) MarkDead(ctx context.Context, id uuid.UUID, reason string) error {
	return r.q.MarkTrackingDeliveryDead(ctx, db.MarkTrackingDeliveryDeadParams{ID: id, LastError: reason})
}

// Reports whether the delivery was dead and went back to the queue
func (r *repository) Retry(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := r.q.RetryTrackingDelivery(ctx, id)
	return n > 0, err
}

func (r *repository) GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	row, err := r.q.GetTrackingDelivery(ctx, id)
	if err != nil {
		return Delivery{}, fmt.Errorf("get tracking delivery: %w", err)
	}
	return toDelivery(db.ClaimDueTrackingDeliveriesRow(row)), nil
}

func (r *repository) Health(ctx context.Context, channelID uuid.UUID) (map[uuid.UUID]Health, error) {
	rows, err := r.q.TrackingDestinationHealth(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("tracking destination health: %w", err)
	}
	out := make(map[uuid.UUID]Health, len(rows))
	for _, row := range rows {
		out[row.ID] = Health{LastError: row.LastError, Dead24h: int(row.Dead24h)}
	}
	return out, nil
}

func (r *repository) ListDeliveries(ctx context.Context, destinationID uuid.UUID, status string, limit int32) ([]Delivery, error) {
	rows, err := r.q.ListTrackingDeliveries(ctx, db.ListTrackingDeliveriesParams{DestinationID: destinationID, Status: status, Lim: limit})
	if err != nil {
		return nil, fmt.Errorf("list tracking deliveries: %w", err)
	}
	out := make([]Delivery, len(rows))
	for i, row := range rows {
		out[i] = toDelivery(db.ClaimDueTrackingDeliveriesRow(row)) // the two queries select the same columns
	}
	return out, nil
}

// Prune drops dead letters past the log window, strips the visitor's data from every event with nothing left to
// deliver, and drops those past the replay window
func (r *repository) Prune(ctx context.Context, logBefore, replayBefore time.Time) (int64, error) {
	n, err := r.q.PruneTrackingDeliveries(ctx, logBefore)
	if err != nil {
		return n, err
	}
	if _, err := r.q.ScrubSettledTrackingEvents(ctx); err != nil {
		return n, err
	}
	m, err := r.q.PruneTrackingEvents(ctx, replayBefore)
	return n + m, err
}

func toDelivery(row db.ClaimDueTrackingDeliveriesRow) Delivery {
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

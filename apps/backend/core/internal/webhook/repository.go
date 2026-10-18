package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *txrunner.Manager
	sealer  Sealer
}

// Seals the signing secret and custom header values at rest - satisfied by *secrets.Cipher, whose Decrypt passes unsealed plaintext through
type Sealer interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(stored string) (string, error)
}

var ErrSealedWithoutKey = errors.New("webhook: value is encrypted but no encryption key is configured")

// A nil sealer stores plaintext - dev without CONFIG_ENCRYPTION_KEY
func NewRepository(queries *db.Queries, txMgr *txrunner.Manager, sealer Sealer) Repository {
	return &postgresRepository{queries: queries, tx: txMgr, sealer: sealer}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t), tx: r.tx, sealer: r.sealer}
}

func (r *postgresRepository) seal(plain string) (string, error) {
	if r.sealer == nil || plain == "" {
		return plain, nil
	}
	return r.sealer.Encrypt(plain)
}

func (r *postgresRepository) open(stored string) (string, error) {
	if !secrets.IsEncrypted(stored) {
		return stored, nil
	}
	if r.sealer == nil {
		return "", ErrSealedWithoutKey
	}
	return r.sealer.Decrypt(stored)
}

// sealHeaders seals each value - a header usually carries the receiver's API token
func (r *postgresRepository) sealHeaders(h map[string]string) (pqtype.NullRawMessage, error) {
	sealed := make(map[string]string, len(h))
	for k, v := range h {
		sv, err := r.seal(v)
		if err != nil {
			return pqtype.NullRawMessage{}, fmt.Errorf("seal header %q: %w", k, err)
		}
		sealed[k] = sv
	}
	return headersToJSON(sealed), nil
}

func (r *postgresRepository) openHeaders(raw pqtype.NullRawMessage) (map[string]string, error) {
	h := headersFromJSON(raw)
	for k, v := range h {
		pv, err := r.open(v)
		if err != nil {
			return nil, fmt.Errorf("open header %q: %w", k, err)
		}
		h[k] = pv
	}
	return h, nil
}

func (r *postgresRepository) Create(ctx context.Context, input CreateInput, signingSecret string) (Webhook, error) {
	secret, err := r.seal(signingSecret)
	if err != nil {
		return Webhook{}, fmt.Errorf("seal signing secret: %w", err)
	}
	headers, err := r.sealHeaders(input.CustomHeaders)
	if err != nil {
		return Webhook{}, err
	}
	var sv uuid.NullUUID
	if input.ChannelID != nil {
		sv = uuid.NullUUID{UUID: *input.ChannelID, Valid: true}
	}
	row, err := r.queries.CreateWebhook(ctx, db.CreateWebhookParams{
		Name:           input.Name,
		TargetUrl:      input.TargetURL,
		SigningSecret:  secret,
		EventTypes:     input.EventTypes,
		Kind:           string(input.Kind),
		ChannelID:      sv,
		FailurePolicy:  string(input.FailurePolicy),
		TimeoutMs:      input.TimeoutMs,
		CustomHeaders:  headers,
		PayloadVersion: input.PayloadVersion,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("create webhook: %w", err)
	}
	return r.toWebhook(row)
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Webhook, error) {
	row, err := r.queries.GetWebhookByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Webhook{}, ErrWebhookNotFound
	}
	if err != nil {
		return Webhook{}, fmt.Errorf("get webhook: %w", err)
	}
	return r.toWebhook(row)
}

func (r *postgresRepository) List(ctx context.Context) ([]Webhook, error) {
	rows, err := r.queries.ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	out := make([]Webhook, len(rows))
	for i, row := range rows {
		if out[i], err = r.toWebhook(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *postgresRepository) ListActiveByEvent(ctx context.Context, eventType string, kind Kind, channelID *uuid.UUID) ([]Webhook, error) {
	var sv uuid.NullUUID
	if channelID != nil {
		sv = uuid.NullUUID{UUID: *channelID, Valid: true}
	}
	rows, err := r.queries.ListActiveWebhooksByEvent(ctx, db.ListActiveWebhooksByEventParams{
		EventType: eventType,
		Kind:      string(kind),
		ChannelID: sv,
	})
	if err != nil {
		return nil, fmt.Errorf("list active webhooks: %w", err)
	}
	out := make([]Webhook, len(rows))
	for i, row := range rows {
		if out[i], err = r.toWebhook(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateInput) (Webhook, error) {
	var updated Webhook
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		current, err := q.GetWebhookByID(ctx, input.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWebhookNotFound
		}
		if err != nil {
			return fmt.Errorf("get webhook: %w", err)
		}

		name := current.Name
		if input.Name != nil {
			name = *input.Name
		}
		targetURL := current.TargetUrl
		if input.TargetURL != nil {
			targetURL = *input.TargetURL
		}
		eventTypes := current.EventTypes
		if input.EventTypes != nil {
			eventTypes = input.EventTypes
		}
		failurePolicy := current.FailurePolicy
		if input.FailurePolicy != nil {
			failurePolicy = string(*input.FailurePolicy)
		}
		timeoutMs := current.TimeoutMs
		if input.TimeoutMs != nil {
			timeoutMs = *input.TimeoutMs
		}
		customHeaders := current.CustomHeaders
		if input.CustomHeaders != nil {
			stored, err := r.openHeaders(current.CustomHeaders)
			if err != nil {
				return err
			}
			if customHeaders, err = r.sealHeaders(keepMasked(input.CustomHeaders, stored)); err != nil {
				return err
			}
		}

		row, err := q.UpdateWebhook(ctx, db.UpdateWebhookParams{
			ID:            input.ID,
			Name:          name,
			TargetUrl:     targetURL,
			EventTypes:    eventTypes,
			FailurePolicy: failurePolicy,
			TimeoutMs:     timeoutMs,
			CustomHeaders: customHeaders,
		})
		if err != nil {
			return fmt.Errorf("update webhook: %w", err)
		}
		updated, err = r.toWebhook(row)
		return err
	})
	if err != nil {
		return Webhook{}, err
	}
	return updated, nil
}

func (r *postgresRepository) SetActive(ctx context.Context, id uuid.UUID, active bool) (Webhook, error) {
	row, err := r.queries.SetWebhookActive(ctx, db.SetWebhookActiveParams{
		ID:       id,
		IsActive: active,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("set webhook active: %w", err)
	}
	return r.toWebhook(row)
}

func (r *postgresRepository) UpdateSigningSecret(ctx context.Context, id uuid.UUID, newSecret string) (Webhook, error) {
	sealed, err := r.seal(newSecret)
	if err != nil {
		return Webhook{}, fmt.Errorf("seal signing secret: %w", err)
	}
	row, err := r.queries.UpdateWebhookSigningSecret(ctx, db.UpdateWebhookSigningSecretParams{
		ID:            id,
		SigningSecret: sealed,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("update signing secret: %w", err)
	}
	return r.toWebhook(row)
}

func (r *postgresRepository) GetSigningSecret(ctx context.Context, id uuid.UUID) (string, error) {
	secret, err := r.queries.GetWebhookSigningSecret(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get signing secret: %w", err)
	}
	return r.open(secret)
}

func (r *postgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteWebhook(ctx, id); err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	return nil
}

func (r *postgresRepository) CreateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	params := db.CreateWebhookDeliveryParams{
		WebhookID:       d.WebhookID,
		EventType:       d.EventType,
		Url:             d.URL,
		RequestBody:     d.RequestBody,
		RequestHeaders:  headersToJSON(d.RequestHeaders),
		ResponseHeaders: headersToJSON(d.ResponseHeaders),
		DurationMs:      d.DurationMs,
		Success:         d.Success,
		Attempt:         d.Attempt,
	}
	if d.ResponseStatus != nil {
		params.ResponseStatus = sql.NullInt32{Int32: *d.ResponseStatus, Valid: true}
	}
	if d.ResponseBody != nil {
		params.ResponseBody = sql.NullString{String: *d.ResponseBody, Valid: true}
	}
	if d.ErrorMessage != nil {
		params.ErrorMessage = sql.NullString{String: *d.ErrorMessage, Valid: true}
	}
	row, err := r.queries.CreateWebhookDelivery(ctx, params)
	if err != nil {
		return Delivery{}, fmt.Errorf("create delivery: %w", err)
	}
	return toDelivery(row), nil
}

func headersToJSON(h map[string]string) pqtype.NullRawMessage {
	if len(h) == 0 {
		return pqtype.NullRawMessage{}
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return pqtype.NullRawMessage{}
	}
	return pqtype.NullRawMessage{RawMessage: raw, Valid: true}
}

func headersFromJSON(raw pqtype.NullRawMessage) map[string]string {
	if !raw.Valid {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw.RawMessage, &out); err != nil {
		return nil
	}
	return out
}

func (r *postgresRepository) ClearDeliveries(ctx context.Context, webhookID uuid.UUID) (int64, error) {
	n, err := r.queries.DeleteWebhookDeliveriesByWebhook(ctx, webhookID)
	if err != nil {
		return 0, fmt.Errorf("clear deliveries: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) PruneEnqueueDedup(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.queries.PruneWebhookEnqueueDedup(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("prune enqueue dedup: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) PruneDeliveries(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.queries.PruneWebhookDeliveries(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("prune deliveries: %w", err)
	}
	return n, nil
}

func (r *postgresRepository) GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error) {
	row, err := r.queries.GetWebhookDelivery(ctx, id)
	if err != nil {
		return Delivery{}, fmt.Errorf("delivery not found")
	}
	return toDelivery(row), nil
}

func (r *postgresRepository) Enqueue(ctx context.Context, webhookID uuid.UUID, eventType, payload, dedupeKey string) (QueueItem, error) {
	if dedupeKey != "" {
		rows, err := r.queries.EnqueueWebhookDeliveryOnce(ctx, db.EnqueueWebhookDeliveryOnceParams{
			WebhookID: webhookID,
			EventType: eventType,
			Payload:   payload,
			DedupeKey: dedupeKey,
		})
		if err != nil {
			return QueueItem{}, fmt.Errorf("enqueue delivery: %w", err)
		}
		if len(rows) == 0 {
			return QueueItem{}, nil
		}
		return toQueueItem(rows[0]), nil
	}
	row, err := r.queries.EnqueueWebhookDelivery(ctx, db.EnqueueWebhookDeliveryParams{
		WebhookID: webhookID,
		EventType: eventType,
		Payload:   payload,
	})
	if err != nil {
		return QueueItem{}, fmt.Errorf("enqueue delivery: %w", err)
	}
	return toQueueItem(row), nil
}

func (r *postgresRepository) ClaimDueQueue(ctx context.Context, batchSize, leaseSeconds int32) ([]QueueItem, error) {
	rows, err := r.queries.ClaimDueWebhookQueue(ctx, db.ClaimDueWebhookQueueParams{
		LeaseSeconds: leaseSeconds,
		BatchSize:    batchSize,
	})
	if err != nil {
		return nil, fmt.Errorf("claim due queue: %w", err)
	}
	out := make([]QueueItem, len(rows))
	for i, row := range rows {
		out[i] = toQueueItem(row)
	}
	return out, nil
}

func (r *postgresRepository) DeleteQueueItem(ctx context.Context, id uuid.UUID) error {
	if err := r.queries.DeleteWebhookQueueItem(ctx, id); err != nil {
		return fmt.Errorf("delete queue item: %w", err)
	}
	return nil
}

func (r *postgresRepository) RescheduleQueueItem(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time, lastError string) error {
	err := r.queries.RescheduleWebhookQueueItem(ctx, db.RescheduleWebhookQueueItemParams{
		ID:            id,
		NextAttemptAt: nextAttemptAt,
		LastError:     sql.NullString{String: lastError, Valid: lastError != ""},
	})
	if err != nil {
		return fmt.Errorf("reschedule queue item: %w", err)
	}
	return nil
}

func (r *postgresRepository) MarkQueueItemDead(ctx context.Context, id uuid.UUID, lastError string) error {
	err := r.queries.MarkWebhookQueueItemDead(ctx, db.MarkWebhookQueueItemDeadParams{
		ID:        id,
		LastError: sql.NullString{String: lastError, Valid: lastError != ""},
	})
	if err != nil {
		return fmt.Errorf("mark queue item dead: %w", err)
	}
	return nil
}

func (r *postgresRepository) ListDeadQueueItems(ctx context.Context, webhookID uuid.UUID, limit int32) ([]QueueItem, error) {
	rows, err := r.queries.ListDeadWebhookQueueItems(ctx, db.ListDeadWebhookQueueItemsParams{
		WebhookID: webhookID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list dead queue items: %w", err)
	}
	out := make([]QueueItem, len(rows))
	for i, row := range rows {
		out[i] = toQueueItem(row)
	}
	return out, nil
}

func (r *postgresRepository) RetryQueueItem(ctx context.Context, id uuid.UUID) (QueueItem, error) {
	row, err := r.queries.RetryWebhookQueueItem(ctx, id)
	if err != nil {
		return QueueItem{}, fmt.Errorf("retry queue item: %w", err)
	}
	return toQueueItem(row), nil
}

func (r *postgresRepository) ClearDeadQueueItems(ctx context.Context, webhookID uuid.UUID) (int64, error) {
	n, err := r.queries.ClearDeadWebhookQueueItems(ctx, webhookID)
	if err != nil {
		return 0, fmt.Errorf("clear dead queue items: %w", err)
	}
	return n, nil
}

func toQueueItem(row db.WebhookQueue) QueueItem {
	q := QueueItem{
		ID:            row.ID,
		WebhookID:     row.WebhookID,
		EventType:     row.EventType,
		Payload:       row.Payload,
		Status:        QueueStatus(row.Status),
		Attempts:      row.Attempts,
		NextAttemptAt: row.NextAttemptAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if row.LastError.Valid {
		q.LastError = &row.LastError.String
	}
	return q
}

func (r *postgresRepository) ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int32) ([]Delivery, error) {
	rows, err := r.queries.ListWebhookDeliveries(ctx, db.ListWebhookDeliveriesParams{
		WebhookID: webhookID,
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	out := make([]Delivery, len(rows))
	for i, row := range rows {
		out[i] = toDelivery(row)
	}
	return out, nil
}

func (r *postgresRepository) DeliveryStats(ctx context.Context, webhookID uuid.UUID) (DeliveryStats, error) {
	row, err := r.queries.WebhookDeliveryStats(ctx, webhookID)
	if err != nil {
		return DeliveryStats{}, fmt.Errorf("delivery stats: %w", err)
	}
	return DeliveryStats{
		Total:          row.Total,
		Succeeded:      row.Succeeded,
		Failed:         row.Failed,
		AvgSuccessMs:   row.AvgSuccessMs,
		LastDeliveryAt: asTimePtr(row.LastDeliveryAt),
		LastSuccessAt:  asTimePtr(row.LastSuccessAt),
		LastFailureAt:  asTimePtr(row.LastFailureAt),
	}, nil
}

// asTimePtr unwraps a MAX(timestamptz) aggregate, which sqlc types as interface{} because it can't prove
// non-nullability - The database/sql driver scans it as time.Time (or nil when the aggregate matched no rows)
func asTimePtr(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}

func (r *postgresRepository) toWebhook(row db.Webhook) (Webhook, error) {
	headers, err := r.openHeaders(row.CustomHeaders)
	if err != nil {
		return Webhook{}, fmt.Errorf("webhook %s: %w", row.ID, err)
	}
	w := Webhook{
		ID:             row.ID,
		Name:           row.Name,
		TargetURL:      row.TargetUrl,
		Kind:           Kind(row.Kind),
		IsActive:       row.IsActive,
		EventTypes:     row.EventTypes,
		FailurePolicy:  FailurePolicy(row.FailurePolicy),
		TimeoutMs:      row.TimeoutMs,
		CustomHeaders:  headers,
		PayloadVersion: row.PayloadVersion,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		w.ChannelID = &id
	}
	return w, nil
}

func toDelivery(row db.WebhookDelivery) Delivery {
	d := Delivery{
		ID:              row.ID,
		WebhookID:       row.WebhookID,
		EventType:       row.EventType,
		URL:             row.Url,
		RequestBody:     row.RequestBody,
		RequestHeaders:  headersFromJSON(row.RequestHeaders),
		ResponseHeaders: headersFromJSON(row.ResponseHeaders),
		DurationMs:      row.DurationMs,
		Success:         row.Success,
		Attempt:         row.Attempt,
		CreatedAt:       row.CreatedAt,
	}
	if row.ResponseStatus.Valid {
		v := row.ResponseStatus.Int32
		d.ResponseStatus = &v
	}
	if row.ResponseBody.Valid {
		d.ResponseBody = &row.ResponseBody.String
	}
	if row.ErrorMessage.Valid {
		d.ErrorMessage = &row.ErrorMessage.String
	}
	return d
}

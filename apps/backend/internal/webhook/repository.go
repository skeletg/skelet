package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	db "github.com/skeletg/skelet/internal/db/sqlc"
	"github.com/skeletg/skelet/internal/tx"
	"github.com/sqlc-dev/pqtype"
)

type postgresRepository struct {
	queries *db.Queries
	tx      *tx.Manager
}

func NewRepository(queries *db.Queries, txMgr *tx.Manager) Repository {
	return &postgresRepository{queries: queries, tx: txMgr}
}

func (r *postgresRepository) WithTx(t *sql.Tx) Repository {
	return &postgresRepository{queries: r.queries.WithTx(t)}
}

func (r *postgresRepository) Create(ctx context.Context, input CreateInput, signingSecret string) (Webhook, error) {
	var sv uuid.NullUUID
	if input.ChannelID != nil {
		sv = uuid.NullUUID{UUID: *input.ChannelID, Valid: true}
	}
	row, err := r.queries.CreateWebhook(ctx, db.CreateWebhookParams{
		Name:           input.Name,
		TargetUrl:      input.TargetURL,
		SigningSecret:  signingSecret,
		EventTypes:     input.EventTypes,
		Kind:           string(input.Kind),
		ChannelID:      sv,
		FailurePolicy:  string(input.FailurePolicy),
		TimeoutMs:      input.TimeoutMs,
		CustomHeaders:  headersToJSON(input.CustomHeaders),
		PayloadVersion: input.PayloadVersion,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("create webhook: %w", err)
	}
	return toWebhook(row), nil
}

func (r *postgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Webhook, error) {
	row, err := r.queries.GetWebhookByID(ctx, id)
	if err != nil {
		return Webhook{}, fmt.Errorf("webhook not found")
	}
	return toWebhook(row), nil
}

func (r *postgresRepository) List(ctx context.Context) ([]Webhook, error) {
	rows, err := r.queries.ListWebhooks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	out := make([]Webhook, len(rows))
	for i, row := range rows {
		out[i] = toWebhook(row)
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
		out[i] = toWebhook(row)
	}
	return out, nil
}

func (r *postgresRepository) Update(ctx context.Context, input UpdateInput) (Webhook, error) {
	var updated Webhook
	err := r.tx.InTransaction(ctx, func(t *sql.Tx) error {
		q := r.queries.WithTx(t)
		current, err := q.GetWebhookByID(ctx, input.ID)
		if err != nil {
			return fmt.Errorf("webhook not found")
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
			customHeaders = headersToJSON(input.CustomHeaders)
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
		updated = toWebhook(row)
		return nil
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
	return toWebhook(row), nil
}

func (r *postgresRepository) UpdateSigningSecret(ctx context.Context, id uuid.UUID, newSecret string) (Webhook, error) {
	row, err := r.queries.UpdateWebhookSigningSecret(ctx, db.UpdateWebhookSigningSecretParams{
		ID:            id,
		SigningSecret: newSecret,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("update signing secret: %w", err)
	}
	return toWebhook(row), nil
}

func (r *postgresRepository) GetSigningSecret(ctx context.Context, id uuid.UUID) (string, error) {
	secret, err := r.queries.GetWebhookSigningSecret(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get signing secret: %w", err)
	}
	return secret, nil
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

func (r *postgresRepository) Enqueue(ctx context.Context, webhookID uuid.UUID, eventType, payload string) (QueueItem, error) {
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

func toWebhook(row db.Webhook) Webhook {
	w := Webhook{
		ID:             row.ID,
		Name:           row.Name,
		TargetURL:      row.TargetUrl,
		Kind:           Kind(row.Kind),
		IsActive:       row.IsActive,
		EventTypes:     row.EventTypes,
		FailurePolicy:  FailurePolicy(row.FailurePolicy),
		TimeoutMs:      row.TimeoutMs,
		CustomHeaders:  headersFromJSON(row.CustomHeaders),
		PayloadVersion: row.PayloadVersion,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.ChannelID.Valid {
		id := row.ChannelID.UUID
		w.ChannelID = &id
	}
	return w
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

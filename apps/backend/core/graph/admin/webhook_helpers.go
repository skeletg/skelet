package admin

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/circuit"
	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

// A global webhook receives every channel's events, so it needs the global grant
func (r *Resolver) requireWebhookScope(ctx context.Context, id uuid.UUID, slug string) (webhook.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return webhook.Webhook{}, err
	}
	w, err := r.WebhookSvc.GetByID(ctx, id)
	if err != nil {
		return webhook.Webhook{}, err
	}
	return w, auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, slug, w.ChannelID)
}

func (r *Resolver) requireWebhookQueueItemScope(ctx context.Context, id uuid.UUID, slug string) (webhook.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return webhook.Webhook{}, err
	}
	webhookID, err := r.Queries.GetWebhookQueueItemWebhookID(ctx, id)
	if err != nil {
		return webhook.Webhook{}, errNotFound("dead letter")
	}
	return r.requireWebhookScope(ctx, webhookID, slug)
}

func (r *Resolver) requireWebhookDeliveryScope(ctx context.Context, id uuid.UUID, slug string) (webhook.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, slug); err != nil {
		return webhook.Webhook{}, err
	}
	d, err := r.Queries.GetWebhookDelivery(ctx, id)
	if err != nil {
		return webhook.Webhook{}, errNotFound("delivery")
	}
	return r.requireWebhookScope(ctx, d.WebhookID, slug)
}

func toWebhookModel(w webhook.Webhook) *model.Webhook {
	m := &model.Webhook{
		ID:             w.ID.String(),
		Name:           w.Name,
		TargetURL:      w.TargetURL,
		Kind:           webhookKindToGQL(w.Kind),
		IsActive:       w.IsActive,
		EventTypes:     w.EventTypes,
		FailurePolicy:  webhookFailurePolicyToGQL(w.FailurePolicy),
		TimeoutMs:      w.TimeoutMs,
		PayloadVersion: w.PayloadVersion,
		CreatedAt:      w.CreatedAt,
		UpdatedAt:      w.UpdatedAt,
	}
	if len(w.CustomHeaders) > 0 {
		m.CustomHeaders = webhook.MaskHeaders(w.CustomHeaders)
	}
	if w.Kind == webhook.KindSync { // breaker state only means something for sync webhooks - async attempts never feed it
		m.Circuit = toCircuitModel(webhook.CircuitInfoFor(w.ID))
	}
	if w.ChannelID != nil {
		m.Channel = &model.Channel{ID: w.ChannelID.String()} // id-only shell - a lookup here would be an N+1 in list views, so callers needing the store go through the channel resolver
	}
	return m
}

// Webhooks and remote providers share the breaker, so they share its admin shape
func toCircuitModel(info circuit.Info) *model.WebhookCircuit {
	state := model.WebhookCircuitStateClosed
	switch info.State {
	case circuit.Open:
		state = model.WebhookCircuitStateOpen
	case circuit.HalfOpen:
		state = model.WebhookCircuitStateHalfOpen
	}
	return &model.WebhookCircuit{
		State:               state,
		ConsecutiveFailures: int32(info.ConsecutiveFailures),
		OpenedAt:            info.OpenedAt,
	}
}

func webhookKindToGQL(k webhook.Kind) model.WebhookKind {
	if k == webhook.KindSync {
		return model.WebhookKindSync
	}
	return model.WebhookKindAsync
}

func webhookKindFromGQL(k model.WebhookKind) webhook.Kind {
	if k == model.WebhookKindSync {
		return webhook.KindSync
	}
	return webhook.KindAsync
}

func webhookFailurePolicyToGQL(p webhook.FailurePolicy) model.WebhookFailurePolicy {
	if p == webhook.FailOpen {
		return model.WebhookFailurePolicyFailOpen
	}
	return model.WebhookFailurePolicyFailClosed
}

func webhookFailurePolicyFromGQL(p model.WebhookFailurePolicy) webhook.FailurePolicy {
	if p == model.WebhookFailurePolicyFailOpen {
		return webhook.FailOpen
	}
	return webhook.FailClosed
}

// Non-string values are rejected up front so a typo'd nested object can't silently serialize as "map[...]"
func webhookHeadersFromGQL(v any) (map[string]string, error) {
	if v == nil {
		return nil, nil
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("customHeaders must be an object of string values")
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("customHeaders[%q] must be a string", k)
		}
		out[k] = s
	}
	return out, nil
}

func toWebhookDeliveryModel(d webhook.Delivery) *model.WebhookDelivery {
	m := &model.WebhookDelivery{
		ID:             d.ID.String(),
		EventType:      d.EventType,
		URL:            d.URL,
		RequestBody:    d.RequestBody,
		ResponseStatus: d.ResponseStatus,
		ResponseBody:   d.ResponseBody,
		DurationMs:     d.DurationMs,
		Success:        d.Success,
		ErrorMessage:   d.ErrorMessage,
		Attempt:        d.Attempt,
		CreatedAt:      d.CreatedAt,
	}
	if len(d.RequestHeaders) > 0 {
		m.RequestHeaders = d.RequestHeaders
	}
	if len(d.ResponseHeaders) > 0 {
		m.ResponseHeaders = d.ResponseHeaders
	}
	return m
}

func toWebhookDeliveryStatsModel(s webhook.DeliveryStats) *model.WebhookDeliveryStats {
	rate := 0.0
	if s.Total > 0 {
		rate = float64(s.Succeeded) / float64(s.Total)
	}
	return &model.WebhookDeliveryStats{
		Total:          int32(s.Total),
		Succeeded:      int32(s.Succeeded),
		Failed:         int32(s.Failed),
		SuccessRate:    rate,
		AvgSuccessMs:   int32(s.AvgSuccessMs),
		LastDeliveryAt: s.LastDeliveryAt,
		LastSuccessAt:  s.LastSuccessAt,
		LastFailureAt:  s.LastFailureAt,
	}
}

func toWebhookQueueItemModel(q webhook.QueueItem) *model.WebhookQueueItem {
	status := model.WebhookQueueStatusPending
	if q.Status == webhook.QueueDead {
		status = model.WebhookQueueStatusDead
	}
	return &model.WebhookQueueItem{
		ID:            q.ID.String(),
		EventType:     q.EventType,
		Status:        status,
		Attempts:      q.Attempts,
		NextAttemptAt: q.NextAttemptAt,
		LastError:     q.LastError,
		Payload:       q.Payload,
		CreatedAt:     q.CreatedAt,
		UpdatedAt:     q.UpdatedAt,
	}
}

func toWebhookEventTypeModel(d webhook.EventDefinition) *model.WebhookEventType {
	return &model.WebhookEventType{
		Type:        d.Type,
		Name:        d.Name,
		Description: d.Description,
		Kind:        webhookKindToGQL(d.Kind),
	}
}

package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/webhook"
)

// Authorization: a global webhook (no channelId) requires global webhooks.manage; a store-scoped webhook only requires
// webhooks.manage on that store
func (r *mutationResolver) CreateWebhook(ctx context.Context, input model.CreateWebhookInput) (*model.CreateWebhookPayload, error) {
	domainInput := webhook.CreateInput{
		Name:       input.Name,
		TargetURL:  input.TargetURL,
		Kind:       webhookKindFromGQL(input.Kind),
		EventTypes: input.EventTypes,
	}
	if input.FailurePolicy != nil {
		domainInput.FailurePolicy = webhookFailurePolicyFromGQL(*input.FailurePolicy)
	}
	if input.TimeoutMs != nil {
		domainInput.TimeoutMs = *input.TimeoutMs
	}
	headers, err := webhookHeadersFromGQL(input.CustomHeaders)
	if err != nil {
		return nil, err
	}
	domainInput.CustomHeaders = headers
	if input.ChannelID != nil {
		sid, err := parseID(*input.ChannelID, "channel")
		if err != nil {
			return nil, err
		}
		domainInput.ChannelID = &sid
		if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "webhooks.manage", sid, nil); err != nil {
			return nil, err
		}
	} else if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return nil, err
	}
	result, err := r.WebhookSvc.Create(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return &model.CreateWebhookPayload{
		RawSecret: result.RawSecret,
		Webhook:   toWebhookModel(result.Webhook),
	}, nil
}

func (r *mutationResolver) UpdateWebhook(ctx context.Context, input model.UpdateWebhookInput) (*model.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return nil, err
	}
	webhookID, err := parseID(input.ID, "webhook")
	if err != nil {
		return nil, err
	}
	updateInput := webhook.UpdateInput{
		ID:         webhookID,
		Name:       input.Name,
		TargetURL:  input.TargetURL,
		EventTypes: input.EventTypes,
	}
	if input.FailurePolicy != nil {
		p := webhookFailurePolicyFromGQL(*input.FailurePolicy)
		updateInput.FailurePolicy = &p
	}
	updateInput.TimeoutMs = input.TimeoutMs
	headers, err := webhookHeadersFromGQL(input.CustomHeaders)
	if err != nil {
		return nil, err
	}
	if headers != nil || input.CustomHeaders != nil {
		// {} clears, absent field leaves headers untouched
		if headers == nil {
			headers = map[string]string{}
		}
		updateInput.CustomHeaders = headers
	}
	w, err := r.WebhookSvc.Update(ctx, updateInput)
	if err != nil {
		return nil, err
	}
	return toWebhookModel(w), nil
}

func (r *mutationResolver) SetWebhookActive(ctx context.Context, id string, active bool) (*model.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return nil, err
	}
	webhookID, err := parseID(id, "webhook")
	if err != nil {
		return nil, err
	}
	w, err := r.WebhookSvc.SetActive(ctx, webhookID, active)
	if err != nil {
		return nil, err
	}
	return toWebhookModel(w), nil
}

func (r *mutationResolver) RotateWebhookSecret(ctx context.Context, id string) (*model.RotateWebhookSecretPayload, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return nil, err
	}
	webhookID, err := parseID(id, "webhook")
	if err != nil {
		return nil, err
	}
	result, err := r.WebhookSvc.RotateSecret(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	return &model.RotateWebhookSecretPayload{
		RawSecret: result.RawSecret,
		Webhook:   toWebhookModel(result.Webhook),
	}, nil
}

func (r *mutationResolver) DeleteWebhook(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return false, err
	}
	webhookID, err := parseID(id, "webhook")
	if err != nil {
		return false, err
	}
	if err := r.WebhookSvc.Delete(ctx, webhookID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) ClearWebhookDeliveries(ctx context.Context, webhookID string) (int32, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return 0, err
	}
	id, err := parseID(webhookID, "webhook")
	if err != nil {
		return 0, err
	}
	n, err := r.WebhookSvc.ClearDeliveries(ctx, id)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *mutationResolver) RetryWebhookDeadLetter(ctx context.Context, id string) (*model.WebhookQueueItem, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return nil, err
	}
	itemID, err := parseID(id, "dead letter")
	if err != nil {
		return nil, err
	}
	item, err := r.WebhookSvc.RetryDeadLetter(ctx, itemID)
	if err != nil {
		return nil, err
	}
	return toWebhookQueueItemModel(item), nil
}

func (r *mutationResolver) ClearWebhookDeadLetters(ctx context.Context, webhookID string) (int32, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return 0, err
	}
	id, err := parseID(webhookID, "webhook")
	if err != nil {
		return 0, err
	}
	n, err := r.WebhookSvc.ClearDeadLetters(ctx, id)
	if err != nil {
		return 0, err
	}
	return int32(n), nil
}

func (r *mutationResolver) RedeliverWebhookDelivery(ctx context.Context, deliveryID string) (*model.WebhookQueueItem, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(deliveryID, "delivery")
	if err != nil {
		return nil, err
	}
	item, err := r.WebhookSvc.RedeliverDelivery(ctx, id)
	if err != nil {
		return nil, err
	}
	return toWebhookQueueItemModel(item), nil
}

func (r *queryResolver) Webhooks(ctx context.Context) ([]*model.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.view"); err != nil {
		return nil, err
	}
	webhooks, err := r.WebhookSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Webhook, len(webhooks))
	for i, w := range webhooks {
		out[i] = toWebhookModel(w)
	}
	return out, nil
}

func (r *queryResolver) Webhook(ctx context.Context, id string) (*model.Webhook, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.view"); err != nil {
		return nil, err
	}
	webhookID, err := parseID(id, "webhook")
	if err != nil {
		return nil, err
	}
	w, err := r.WebhookSvc.GetByID(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	return toWebhookModel(w), nil
}

func (r *queryResolver) WebhookDeliveries(ctx context.Context, webhookID string, limit *int32) ([]*model.WebhookDelivery, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.view"); err != nil {
		return nil, err
	}
	id, err := parseID(webhookID, "webhook")
	if err != nil {
		return nil, err
	}
	lim := int32(25)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	deliveries, err := r.WebhookSvc.ListDeliveries(ctx, id, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.WebhookDelivery, len(deliveries))
	for i, d := range deliveries {
		out[i] = toWebhookDeliveryModel(d)
	}
	return out, nil
}

func (r *queryResolver) WebhookDeliveryStats(ctx context.Context, webhookID string) (*model.WebhookDeliveryStats, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.view"); err != nil {
		return nil, err
	}
	id, err := parseID(webhookID, "webhook")
	if err != nil {
		return nil, err
	}
	stats, err := r.WebhookSvc.DeliveryStats(ctx, id)
	if err != nil {
		return nil, err
	}
	return toWebhookDeliveryStatsModel(stats), nil
}

func (r *queryResolver) WebhookEventTypes(ctx context.Context) ([]*model.WebhookEventType, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.view"); err != nil {
		return nil, err
	}
	defs := webhook.AllEvents()
	out := make([]*model.WebhookEventType, len(defs))
	for i, d := range defs {
		out[i] = toWebhookEventTypeModel(d)
	}
	return out, nil
}

func (r *queryResolver) WebhookDeadLetters(ctx context.Context, webhookID string, limit *int32) ([]*model.WebhookQueueItem, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "webhooks.view"); err != nil {
		return nil, err
	}
	id, err := parseID(webhookID, "webhook")
	if err != nil {
		return nil, err
	}
	lim := int32(50)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	items, err := r.WebhookSvc.ListDeadLetters(ctx, id, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.WebhookQueueItem, len(items))
	for i, item := range items {
		out[i] = toWebhookQueueItemModel(item)
	}
	return out, nil
}

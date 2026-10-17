package admin

import (
	"testing"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/webhook"
)

// webhooks.view is enough to read a webhook, so its stored tokens must never come back in the clear
func TestTheWebhookModelMasksHeaderValues(t *testing.T) {
	m := toWebhookModel(webhook.Webhook{ID: uuid.New(), CustomHeaders: map[string]string{"X-Api-Key": "tok_live"}})
	h, _ := m.CustomHeaders.(map[string]string)
	if h["X-Api-Key"] != webhook.MaskedHeaderValue {
		t.Fatalf("customHeaders = %v, want the value masked", m.CustomHeaders)
	}
}

package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// CfgDeliveryRetentionDays drives the daily webhook-deliveries-prune job; <= 0 keeps the log forever
const CfgDeliveryRetentionDays = "system/webhooks/delivery_retention_days"

func init() {
	config.RegisterGroup(config.Group{Section: "system", Slug: "webhooks", Label: "Webhooks", SortOrder: 4})
	config.Register(config.Definition{Path: CfgDeliveryRetentionDays, Type: config.TypeInt, Default: "90", Label: "Webhook delivery-log retention (days)"})
}

type Service struct {
	repo Repository

	// AllowPrivateTargets disables the SSRF range checks on target URLs (ValidateTargetURL). main.go enables it in
	// development so localhost webhook receivers keep working; leave false in prod
	AllowPrivateTargets bool
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (CreateResult, error) {
	if input.Kind == "" {
		input.Kind = KindAsync
	}
	if input.FailurePolicy == "" {
		input.FailurePolicy = FailClosed
	}
	if input.PayloadVersion == "" {
		input.PayloadVersion = PayloadVersionV1
	}
	if err := validateDeliverySettings(input.FailurePolicy, input.TimeoutMs, input.CustomHeaders); err != nil {
		return CreateResult{}, err
	}
	if input.PayloadVersion != PayloadVersionV1 {
		return CreateResult{}, fmt.Errorf("%w: %q (only %q exists)", ErrInvalidPayloadVersion, input.PayloadVersion, PayloadVersionV1)
	}
	if err := ValidateEventTypes(input.EventTypes, input.Kind); err != nil {
		return CreateResult{}, err
	}
	if err := ValidateTargetURL(input.TargetURL, s.AllowPrivateTargets); err != nil {
		return CreateResult{}, err
	}
	raw, err := generateSecret()
	if err != nil {
		return CreateResult{}, fmt.Errorf("generate secret: %w", err)
	}
	w, err := s.repo.Create(ctx, input, raw)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{RawSecret: raw, Webhook: w}, nil
}

func (s *Service) List(ctx context.Context) ([]Webhook, error) {
	return s.repo.List(ctx)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Webhook, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, input UpdateInput) (Webhook, error) {
	if input.TargetURL != nil {
		if err := ValidateTargetURL(*input.TargetURL, s.AllowPrivateTargets); err != nil {
			return Webhook{}, err
		}
	}
	policy := FailurePolicy("")
	if input.FailurePolicy != nil {
		policy = *input.FailurePolicy
	}
	timeout := int32(0)
	if input.TimeoutMs != nil {
		timeout = *input.TimeoutMs
	}
	if err := validateDeliverySettings(policy, timeout, input.CustomHeaders); err != nil {
		return Webhook{}, err
	}
	if input.EventTypes != nil {
		// Kind is immutable - validate new event types against the webhook's
		// existing kind rather than letting someone slip in mismatched types
		current, err := s.repo.GetByID(ctx, input.ID)
		if err != nil {
			return Webhook{}, err
		}
		if err := ValidateEventTypes(input.EventTypes, current.Kind); err != nil {
			return Webhook{}, err
		}
	}
	w, err := s.repo.Update(ctx, input)
	if err != nil {
		return Webhook{}, err
	}
	// an edit usually means the endpoint got fixed - let it prove it now
	ResetCircuit(w.ID)
	return w, nil
}

func (s *Service) SetActive(ctx context.Context, id uuid.UUID, active bool) (Webhook, error) {
	w, err := s.repo.SetActive(ctx, id, active)
	if err != nil {
		return Webhook{}, err
	}
	if active {
		ResetCircuit(id)
	}
	return w, nil
}

func (s *Service) RotateSecret(ctx context.Context, id uuid.UUID) (CreateResult, error) {
	raw, err := generateSecret()
	if err != nil {
		return CreateResult{}, fmt.Errorf("generate secret: %w", err)
	}
	w, err := s.repo.UpdateSigningSecret(ctx, id, raw)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{RawSecret: raw, Webhook: w}, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int32) ([]Delivery, error) {
	return s.repo.ListDeliveries(ctx, webhookID, limit)
}

func (s *Service) ClearDeliveries(ctx context.Context, webhookID uuid.UUID) (int64, error) {
	return s.repo.ClearDeliveries(ctx, webhookID)
}

func (s *Service) DeliveryStats(ctx context.Context, webhookID uuid.UUID) (DeliveryStats, error) {
	return s.repo.DeliveryStats(ctx, webhookID)
}

func (s *Service) ListActiveByEvent(ctx context.Context, eventType string, kind Kind, channelID *uuid.UUID) ([]Webhook, error) {
	return s.repo.ListActiveByEvent(ctx, eventType, kind, channelID)
}

func (s *Service) GetSigningSecret(ctx context.Context, id uuid.UUID) (string, error) {
	return s.repo.GetSigningSecret(ctx, id)
}

func (s *Service) RecordDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	return s.repo.CreateDelivery(ctx, d)
}

func (s *Service) EnqueueDelivery(ctx context.Context, webhookID uuid.UUID, eventType, payload, dedupeKey string) (QueueItem, error) {
	return s.repo.Enqueue(ctx, webhookID, eventType, payload, dedupeKey)
}

// RedeliverDelivery re-enqueues a historical delivery's exact payload for a fresh delivery cycle through the async queue - Works for sync deliveries too - the redelivered copy is a plain notification (the original operation is long finished, so the response can't mutate anything)
func (s *Service) RedeliverDelivery(ctx context.Context, deliveryID uuid.UUID) (QueueItem, error) {
	d, err := s.repo.GetDelivery(ctx, deliveryID)
	if err != nil {
		return QueueItem{}, err
	}
	return s.repo.Enqueue(ctx, d.WebhookID, d.EventType, d.RequestBody, "")
}

func (s *Service) ListDeadLetters(ctx context.Context, webhookID uuid.UUID, limit int32) ([]QueueItem, error) {
	return s.repo.ListDeadQueueItems(ctx, webhookID, limit)
}

func (s *Service) RetryDeadLetter(ctx context.Context, id uuid.UUID) (QueueItem, error) {
	return s.repo.RetryQueueItem(ctx, id)
}

func (s *Service) ClearDeadLetters(ctx context.Context, webhookID uuid.UUID) (int64, error) {
	return s.repo.ClearDeadQueueItems(ctx, webhookID)
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// headers the dispatcher owns - merchant headers can't shadow the envelope
// metadata or the signature
var reservedHeaderNames = map[string]struct{}{
	"content-type":   {},
	"content-length": {},
	"host":           {},
	"user-agent":     {},
}

const maxCustomHeaders = 20

// validateDeliverySettings checks the shared create/update knobs - Empty
// policy / zero timeout mean "unchanged or default" and pass through
func validateDeliverySettings(policy FailurePolicy, timeoutMs int32, headers map[string]string) error {
	if policy != "" && policy != FailOpen && policy != FailClosed {
		return fmt.Errorf("%w: %q", ErrInvalidFailurePolicy, policy)
	}
	if timeoutMs < 0 || timeoutMs > MaxTimeoutMs {
		return fmt.Errorf("%w: %d ms (0 = default, max %d)", ErrInvalidTimeout, timeoutMs, MaxTimeoutMs)
	}
	if len(headers) > maxCustomHeaders {
		return fmt.Errorf("%w: at most %d custom headers", ErrReservedHeader, maxCustomHeaders)
	}
	for k := range headers {
		name := strings.ToLower(strings.TrimSpace(k))
		if name == "" {
			return fmt.Errorf("%w: empty header name", ErrReservedHeader)
		}
		if _, reserved := reservedHeaderNames[name]; reserved || strings.HasPrefix(name, "x-webhook-") {
			return fmt.Errorf("%w: %q", ErrReservedHeader, k)
		}
	}
	return nil
}

// PruneDeliveries drops delivery-log rows past the retention window, across all webhooks
func (s *Service) PruneDeliveries(ctx context.Context, before time.Time) (int64, error) {
	return s.repo.PruneDeliveries(ctx, before)
}

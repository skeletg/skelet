package webhook

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Async webhooks fire after a business operation commits, can't affect it, and are retried on failure
// Sync webhooks fire during a pre-commit bus hook, block the operation until they respond, can reject or enrich the payload, and never retry
type Kind string

const (
	KindAsync Kind = "async"
	KindSync  Kind = "sync"
)

// What a failed sync delivery does to the emitting operation - fail_closed (the default) aborts it, making the webhook a gate, while fail_open logs and continues, making it advisory
// An explicit {"reject": true} is honored under BOTH policies; fail_open only forgives transport errors, timeouts, non-2xx statuses and malformed bodies - Async webhooks ignore the policy entirely
type FailurePolicy string

const (
	FailClosed FailurePolicy = "fail_closed"
	FailOpen   FailurePolicy = "fail_open"
)

const PayloadVersionV1 = "v1"

const MaxTimeoutMs = 30000

type Webhook struct {
	ID         uuid.UUID
	Name       string
	TargetURL  string
	Kind       Kind
	IsActive   bool
	EventTypes []string
	// nil for global webhooks, which fire for any store's events, and set for store-scoped ones - the dispatcher matches against ChannelScopedEvent
	ChannelID     *uuid.UUID
	FailurePolicy FailurePolicy
	// Overrides the per-attempt HTTP timeout - 0 is the kind default (5s sync, 10s async), capped at MaxTimeoutMs
	TimeoutMs int32
	// Merchant-supplied request headers for auth tokens and routing hints - reserved names (Content-Type, X-Webhook-*) are rejected at write time and skipped at send time
	CustomHeaders  map[string]string
	PayloadVersion string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Implemented by event payloads carrying a store affiliation - the dispatcher filters on it, so global webhooks always fire and per-store ones only when the event's store matches
type ChannelScopedEvent interface {
	EventChannelID() uuid.UUID
}

type Delivery struct {
	ID              uuid.UUID
	WebhookID       uuid.UUID
	EventType       string
	URL             string
	RequestBody     string
	RequestHeaders  map[string]string
	ResponseStatus  *int32
	ResponseBody    *string
	ResponseHeaders map[string]string
	DurationMs      int32
	Success         bool
	ErrorMessage    *string
	Attempt         int32
	CreatedAt       time.Time
}

// The aggregate health of a webhook's stored delivery rows - retries are separate rows, so counts are attempt-level, and the three timestamps are nil until a matching delivery exists
type DeliveryStats struct {
	Total          int64
	Succeeded      int64
	Failed         int64
	AvgSuccessMs   float64
	LastDeliveryAt *time.Time
	LastSuccessAt  *time.Time
	LastFailureAt  *time.Time
}

type CreateInput struct {
	Name           string
	TargetURL      string
	Kind           Kind
	EventTypes     []string
	ChannelID      *uuid.UUID
	FailurePolicy  FailurePolicy
	TimeoutMs      int32
	CustomHeaders  map[string]string
	PayloadVersion string
}

type UpdateInput struct {
	ID            uuid.UUID
	Name          *string
	TargetURL     *string
	EventTypes    []string // nil = no change, [] = clear all
	FailurePolicy *FailurePolicy
	TimeoutMs     *int32
	CustomHeaders map[string]string
}

type CreateResult struct {
	RawSecret string
	Webhook   Webhook
}

var (
	ErrWebhookNotFound       = errors.New("webhook not found")
	ErrInvalidEventTypes     = errors.New("invalid event types")
	ErrInvalidKind           = errors.New("invalid webhook kind")
	ErrKindEventMismatch     = errors.New("event types do not match webhook kind")
	ErrInvalidFailurePolicy  = errors.New("invalid failure policy")
	ErrInvalidTimeout        = errors.New("invalid timeout")
	ErrReservedHeader        = errors.New("reserved header name")
	ErrInvalidPayloadVersion = errors.New("invalid payload version")
)

// A successful delivery deletes its row, since the delivery history is the log - exhausted ones park as dead
type QueueStatus string

const (
	QueuePending QueueStatus = "pending"
	QueueDead    QueueStatus = "dead"
)

type QueueItem struct {
	ID            uuid.UUID
	WebhookID     uuid.UUID
	EventType     string
	Payload       string
	Status        QueueStatus
	Attempts      int32
	NextAttemptAt time.Time
	LastError     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Implemented by sync event payloads that accept mutations from a webhook response The dispatcher type-asserts each incoming sync event and, when it matches, calls ApplyWebhookResponse with the response body so the event decides how to fold it in - append tags, stack discounts, and so on Returning reject=true aborts the sync chain, and the reason propagates back to the emitting service
type SyncResponseApplier interface {
	ApplyWebhookResponse(body []byte) (reject bool, reason string, err error)
}

type Repository interface {
	Create(ctx context.Context, input CreateInput, signingSecret string) (Webhook, error)
	GetByID(ctx context.Context, id uuid.UUID) (Webhook, error)
	List(ctx context.Context) ([]Webhook, error)
	// channelID nil lists global plus all-stores, a value lists global webhooks plus those scoped to that store
	ListActiveByEvent(ctx context.Context, eventType string, kind Kind, channelID *uuid.UUID) ([]Webhook, error)
	Update(ctx context.Context, input UpdateInput) (Webhook, error)
	SetActive(ctx context.Context, id uuid.UUID, active bool) (Webhook, error)
	UpdateSigningSecret(ctx context.Context, id uuid.UUID, newSecret string) (Webhook, error)
	GetSigningSecret(ctx context.Context, id uuid.UUID) (string, error)
	Delete(ctx context.Context, id uuid.UUID) error
	CreateDelivery(ctx context.Context, d Delivery) (Delivery, error)
	GetDelivery(ctx context.Context, id uuid.UUID) (Delivery, error)
	ListDeliveries(ctx context.Context, webhookID uuid.UUID, limit int32) ([]Delivery, error)
	DeliveryStats(ctx context.Context, webhookID uuid.UUID) (DeliveryStats, error)
	ClearDeliveries(ctx context.Context, webhookID uuid.UUID) (int64, error)
	// age-based retention across all webhooks (the daily prune job)
	PruneDeliveries(ctx context.Context, before time.Time) (int64, error)
	// a non-empty dedupeKey enqueues at most once per webhook, returning a zero QueueItem on a repeat
	Enqueue(ctx context.Context, webhookID uuid.UUID, eventType, payload, dedupeKey string) (QueueItem, error)
	PruneEnqueueDedup(ctx context.Context, before time.Time) (int64, error)
	ClaimDueQueue(ctx context.Context, batchSize, leaseSeconds int32) ([]QueueItem, error)
	DeleteQueueItem(ctx context.Context, id uuid.UUID) error
	RescheduleQueueItem(ctx context.Context, id uuid.UUID, nextAttemptAt time.Time, lastError string) error
	MarkQueueItemDead(ctx context.Context, id uuid.UUID, lastError string) error
	ListDeadQueueItems(ctx context.Context, webhookID uuid.UUID, limit int32) ([]QueueItem, error)
	RetryQueueItem(ctx context.Context, id uuid.UUID) (QueueItem, error)
	ClearDeadQueueItems(ctx context.Context, webhookID uuid.UUID) (int64, error)
	WithTx(tx *sql.Tx) Repository
}

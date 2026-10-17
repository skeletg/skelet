package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/db/sqlnull"
)

// Job is the admin-facing view of a scheduled job - Webhook secrets are never
// echoed back - only HasSecret is exposed
type Job struct {
	ID                  uuid.UUID
	Name                string
	Kind                string // "code" | "webhook"
	Schedule            string
	Enabled             bool
	Description         string
	Timezone            string
	MaxAttempts         int32
	RetryBackoffSeconds int32
	CurrentAttempt      int32
	AlertAfterFailures  int32
	ConsecutiveFailures int32
	TimeoutSeconds      int32
	Webhook             *WebhookView // non-nil for kind=webhook
	NextRunAt           *time.Time
	LastRunAt           *time.Time
	LastStatus          string
	LastError           string
	LastDurationMs      *int32
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Run struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string
	Error      string
	DurationMs int32
	Attempt    int32
}

// WebhookView is the safe read projection of a webhook job's config - the host only, since the path and query of a hook URL are often the credential
type WebhookView struct {
	Host      string
	Method    string
	HasSecret bool
}

func toJob(row db.ScheduledJob) Job {
	j := Job{
		ID:                  row.ID,
		Name:                row.Name,
		Kind:                row.Kind,
		Schedule:            row.Schedule,
		Enabled:             row.Enabled,
		Description:         row.Description,
		Timezone:            row.Timezone,
		MaxAttempts:         row.MaxAttempts,
		RetryBackoffSeconds: row.RetryBackoffSeconds,
		CurrentAttempt:      row.CurrentAttempt,
		AlertAfterFailures:  row.AlertAfterFailures,
		ConsecutiveFailures: row.ConsecutiveFailures,
		TimeoutSeconds:      row.TimeoutSeconds,
		LastStatus:          row.LastStatus,
		LastError:           row.LastError,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
	if row.NextRunAt.Valid {
		t := row.NextRunAt.Time
		j.NextRunAt = &t
	}
	if row.LastRunAt.Valid {
		t := row.LastRunAt.Time
		j.LastRunAt = &t
	}
	if row.LastDurationMs.Valid {
		v := row.LastDurationMs.Int32
		j.LastDurationMs = &v
	}
	if row.Kind == "webhook" {
		var cfg WebhookConfig
		_ = json.Unmarshal(row.Payload, &cfg)
		host := ""
		if u, err := url.Parse(cfg.URL); err == nil {
			host = u.Host
		}
		j.Webhook = &WebhookView{Host: host, Method: cfg.Method, HasSecret: cfg.Secret != ""}
	}
	return j
}

func (s *Service) List(ctx context.Context) ([]Job, error) {
	rows, err := s.queries.ListScheduledJobs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Job, len(rows))
	for i, r := range rows {
		out[i] = toJob(r)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Job, error) {
	row, err := s.queries.GetScheduledJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

// UpdateScheduleInput is the admin edit of a job's cadence + execution config (applies to both code and webhook jobs) -
// nil pointers keep the stored value, so a client that omits a field can't silently reset an operator's tuning
type UpdateScheduleInput struct {
	ID                  uuid.UUID
	Schedule            string
	Enabled             bool
	Timezone            string
	MaxAttempts         *int32
	RetryBackoffSeconds *int32
	AlertAfterFailures  *int32
	TimeoutSeconds      *int32
}

// UpdateSchedule validates + applies a job's schedule/timezone/retry/alert
// config and recomputes the next run
func (s *Service) UpdateSchedule(ctx context.Context, in UpdateScheduleInput, by *uuid.UUID) (Job, error) {
	in.Schedule = strings.TrimSpace(in.Schedule)
	in.Timezone = strings.TrimSpace(in.Timezone)
	if err := ValidateScheduleTZ(in.Schedule, in.Timezone); err != nil {
		return Job{}, err
	}
	var next sql.NullTime
	if t, ok := NextRunInTZ(in.Schedule, in.Timezone, time.Now()); ok {
		next = sql.NullTime{Time: t, Valid: true}
	}
	row, err := s.queries.UpdateScheduledJobSchedule(ctx, db.UpdateScheduledJobScheduleParams{
		ID:                  in.ID,
		Schedule:            in.Schedule,
		Enabled:             in.Enabled,
		Timezone:            in.Timezone,
		MaxAttempts:         clampedOrKeep(in.MaxAttempts, 1, 100),
		RetryBackoffSeconds: clampedOrKeep(in.RetryBackoffSeconds, 0, maxRunTimeoutSeconds),
		AlertAfterFailures:  clampedOrKeep(in.AlertAfterFailures, 0, 1000),
		TimeoutSeconds:      clampedOrKeep(in.TimeoutSeconds, 1, maxRunTimeoutSeconds),
		NextRunAt:           next,
		UpdatedBy:           sqlnull.UUID(by),
	})
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

// CreateWebhookInput defines an admin-created webhook job
type CreateWebhookInput struct {
	Name                string
	Schedule            string
	Description         string
	Enabled             bool
	Timezone            string
	MaxAttempts         int32
	RetryBackoffSeconds int32
	AlertAfterFailures  int32
	URL                 string
	Method              string
	Headers             map[string]string
	Body                string
	Secret              string
}

// CreateWebhook validates + persists a new webhook job
func (s *Service) CreateWebhook(ctx context.Context, in CreateWebhookInput, by *uuid.UUID) (Job, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Schedule = strings.TrimSpace(in.Schedule)
	in.Timezone = strings.TrimSpace(in.Timezone)
	if in.Name == "" {
		return Job{}, ErrWebhookNameNeeded
	}
	// Code jobs seed by name, so a webhook holding one's name would keep that job from ever being created
	if s.isCodeJob(in.Name) {
		return Job{}, ErrWebhookNameTaken
	}
	if err := ValidateScheduleTZ(in.Schedule, in.Timezone); err != nil {
		return Job{}, err
	}
	if err := ValidateWebhookURL(in.URL, s.webhook.allowPrivate); err != nil {
		return Job{}, err
	}
	if err := validateMethod(in.Method); err != nil {
		return Job{}, ErrWebhookMethod
	}
	sealed, err := s.webhook.seal(WebhookConfig{
		URL: strings.TrimSpace(in.URL), Method: normalizeMethod(in.Method),
		Headers: in.Headers, Body: in.Body, Secret: in.Secret,
	})
	if err != nil {
		return Job{}, err
	}
	payload, err := json.Marshal(sealed)
	if err != nil {
		return Job{}, err
	}
	var next sql.NullTime
	if t, ok := NextRunInTZ(in.Schedule, in.Timezone, time.Now()); ok {
		next = sql.NullTime{Time: t, Valid: true}
	}
	row, err := s.queries.CreateWebhookJob(ctx, db.CreateWebhookJobParams{
		Name:                in.Name,
		Schedule:            in.Schedule,
		Description:         in.Description,
		Enabled:             in.Enabled,
		Payload:             payload,
		NextRunAt:           next,
		Timezone:            in.Timezone,
		MaxAttempts:         clampMin(in.MaxAttempts, 1),
		RetryBackoffSeconds: clampMin(in.RetryBackoffSeconds, 0),
		AlertAfterFailures:  clampMin(in.AlertAfterFailures, 0),
		CreatedBy:           sqlnull.UUID(by),
	})
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

// UpdateWebhookInput edits a webhook job's request config - A blank Secret
// KEEPS the existing one (the admin never sees it on read, so blank ≠ clear)
type UpdateWebhookInput struct {
	ID          uuid.UUID
	Description string
	URL         string
	Method      string
	Headers     map[string]string
	Body        string
	Secret      string
}

// UpdateWebhook rewrites a webhook job's URL/method/headers/body (and rotates the secret when a new one is supplied) -
// Schedule/timezone/retry/alert are edited via UpdateSchedule
func (s *Service) UpdateWebhook(ctx context.Context, in UpdateWebhookInput, by *uuid.UUID) (Job, error) {
	if err := ValidateWebhookURL(in.URL, s.webhook.allowPrivate); err != nil {
		return Job{}, err
	}
	if err := validateMethod(in.Method); err != nil {
		return Job{}, ErrWebhookMethod
	}
	cur, err := s.queries.GetScheduledJob(ctx, in.ID)
	if err != nil {
		return Job{}, err
	}
	if cur.Kind != "webhook" {
		return Job{}, fmt.Errorf("job %q is not a webhook job", cur.Name)
	}
	var prev WebhookConfig
	_ = json.Unmarshal(cur.Payload, &prev)
	sealed, err := s.webhook.seal(WebhookConfig{
		URL: strings.TrimSpace(in.URL), Method: normalizeMethod(in.Method),
		Headers: in.Headers, Body: in.Body, Secret: in.Secret,
	})
	if err != nil {
		return Job{}, err
	}
	if in.Secret == "" {
		sealed.Secret = prev.Secret // blank = keep existing, already sealed as stored
	}
	payload, err := json.Marshal(sealed)
	if err != nil {
		return Job{}, err
	}
	row, err := s.queries.UpdateWebhookJobPayload(ctx, db.UpdateWebhookJobPayloadParams{
		ID:          in.ID,
		Payload:     payload,
		Description: in.Description,
		UpdatedBy:   sqlnull.UUID(by),
	})
	if err != nil {
		return Job{}, err
	}
	return toJob(row), nil
}

// WebhookDetail is the full editable webhook config (no secret - only whether
// one is set) - Manage-gated at the resolver because headers may carry tokens
type WebhookDetail struct {
	URL       string
	Method    string
	Body      string
	Headers   map[string]string
	HasSecret bool
}

// WebhookDetail loads a webhook job's full config for the edit dialog so a save
// round-trips body/headers instead of wiping them
func (s *Service) WebhookDetail(ctx context.Context, id uuid.UUID) (WebhookDetail, error) {
	row, err := s.queries.GetScheduledJob(ctx, id)
	if err != nil {
		return WebhookDetail{}, err
	}
	if row.Kind != "webhook" {
		return WebhookDetail{}, fmt.Errorf("job %q is not a webhook job", row.Name)
	}
	var stored WebhookConfig
	_ = json.Unmarshal(row.Payload, &stored)
	// Opened for the manage-gated edit dialog only - a save round-trips the headers instead of wiping them
	cfg, err := s.webhook.open(stored)
	if err != nil {
		return WebhookDetail{}, err
	}
	return WebhookDetail{
		URL:       cfg.URL,
		Method:    cfg.Method,
		Body:      cfg.Body,
		Headers:   cfg.Headers,
		HasSecret: cfg.Secret != "",
	}, nil
}

// Delete removes a webhook job (code jobs are owned by code and can't be
// deleted - only disabled)
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteScheduledJob(ctx, id)
}

// Runs returns a job's recent run history (newest first)
func (s *Service) Runs(ctx context.Context, jobID uuid.UUID, limit int32) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.queries.ListScheduledJobRuns(ctx, db.ListScheduledJobRunsParams{JobID: jobID, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Run, len(rows))
	for i, r := range rows {
		out[i] = Run{
			StartedAt:  r.StartedAt,
			FinishedAt: r.FinishedAt,
			Status:     r.Status,
			Error:      r.Error,
			DurationMs: r.DurationMs,
			Attempt:    r.Attempt,
		}
	}
	return out, nil
}

// PruneRuns deletes run-history rows older than `before` (the history-prune
// job) - Returns the number removed
func (s *Service) PruneRuns(ctx context.Context, before time.Time) (int64, error) {
	return s.queries.PruneScheduledJobRuns(ctx, before)
}

func normalizeMethod(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	if m == "" {
		return "POST"
	}
	return m
}

func clampMin(v, min int32) int32 {
	if v < min {
		return min
	}
	return v
}

// nil = keep the stored value (SQL COALESCE); set values are clamped into [min, max]
func clampedOrKeep(v *int32, min, max int32) sql.NullInt32 {
	if v == nil {
		return sql.NullInt32{}
	}
	c := *v
	if c < min {
		c = min
	}
	if c > max {
		c = max
	}
	return sql.NullInt32{Int32: c, Valid: true}
}

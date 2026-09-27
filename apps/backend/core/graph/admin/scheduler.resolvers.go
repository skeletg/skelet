package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"github.com/skeletg/skelet/apps/backend/core/internal/scheduler"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) UpdateScheduledJob(ctx context.Context, input model.UpdateScheduledJobInput) (*model.ScheduledJob, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.manage"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", nil)
	}
	j, err := r.Scheduler.UpdateSchedule(ctx, scheduler.UpdateScheduleInput{
		ID:                  id,
		Schedule:            input.Schedule,
		Enabled:             input.Enabled,
		Timezone:            ptr.Deref(input.Timezone),
		MaxAttempts:         input.MaxAttempts,
		RetryBackoffSeconds: input.RetryBackoffSeconds,
		AlertAfterFailures:  input.AlertAfterFailures,
		TimeoutSeconds:      input.TimeoutSeconds,
	}, currentAdminID(ctx))
	if err != nil {
		return nil, err
	}
	return toScheduledJobModel(j), nil
}

func (r *mutationResolver) CreateWebhookJob(ctx context.Context, input model.CreateWebhookJobInput) (*model.ScheduledJob, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.manage"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	headers := make(map[string]string, len(input.Headers))
	for _, h := range input.Headers {
		headers[h.Name] = h.Value
	}
	j, err := r.Scheduler.CreateWebhook(ctx, scheduler.CreateWebhookInput{
		Name:                input.Name,
		Schedule:            input.Schedule,
		Description:         ptr.Deref(input.Description),
		Enabled:             input.Enabled == nil || *input.Enabled,
		Timezone:            ptr.Deref(input.Timezone),
		MaxAttempts:         derefInt32Default(input.MaxAttempts, 1),
		RetryBackoffSeconds: derefInt32Default(input.RetryBackoffSeconds, 60),
		AlertAfterFailures:  derefInt32Default(input.AlertAfterFailures, 0),
		URL:                 input.URL,
		Method:              ptr.Deref(input.Method),
		Headers:             headers,
		Body:                ptr.Deref(input.Body),
		Secret:              ptr.Deref(input.Secret),
	}, currentAdminID(ctx))
	if err != nil {
		return nil, err
	}
	return toScheduledJobModel(j), nil
}

func (r *mutationResolver) UpdateWebhookJob(ctx context.Context, input model.UpdateWebhookJobInput) (*model.ScheduledJob, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.manage"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	id, err := uuid.Parse(input.ID)
	if err != nil {
		return nil, errInvalidID("id", nil)
	}
	headers := make(map[string]string, len(input.Headers))
	for _, h := range input.Headers {
		headers[h.Name] = h.Value
	}
	j, err := r.Scheduler.UpdateWebhook(ctx, scheduler.UpdateWebhookInput{
		ID:          id,
		Description: ptr.Deref(input.Description),
		URL:         input.URL,
		Method:      ptr.Deref(input.Method),
		Headers:     headers,
		Body:        ptr.Deref(input.Body),
		Secret:      ptr.Deref(input.Secret),
	}, currentAdminID(ctx))
	if err != nil {
		return nil, err
	}
	return toScheduledJobModel(j), nil
}

func (r *mutationResolver) DeleteScheduledJob(ctx context.Context, id string) (bool, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.manage"); err != nil {
		return false, err
	}
	if r.Scheduler == nil {
		return false, errNotConfigured("scheduler")
	}
	jid, err := uuid.Parse(id)
	if err != nil {
		return false, errInvalidID("id", nil)
	}
	if err := r.Scheduler.Delete(ctx, jid); err != nil {
		return false, err
	}
	return true, nil
}

func (r *mutationResolver) RunScheduledJobNow(ctx context.Context, id string) (*model.ScheduledJob, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.manage"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	jid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", nil)
	}
	if err := r.Scheduler.RunNow(ctx, jid); err != nil {
		return nil, err
	}
	j, err := r.Scheduler.Get(ctx, jid)
	if err != nil {
		return nil, err
	}
	return toScheduledJobModel(j), nil
}

func (r *queryResolver) ScheduledJobs(ctx context.Context) ([]*model.ScheduledJob, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.view"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	jobs, err := r.Scheduler.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ScheduledJob, len(jobs))
	for i, j := range jobs {
		out[i] = toScheduledJobModel(j)
	}
	return out, nil
}

func (r *queryResolver) ScheduledJobRuns(ctx context.Context, jobID string, limit *int32) ([]*model.ScheduledJobRun, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.view"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	jid, err := uuid.Parse(jobID)
	if err != nil {
		return nil, errInvalidID("id", nil)
	}
	runs, err := r.Scheduler.Runs(ctx, jid, derefInt32Default(limit, 50))
	if err != nil {
		return nil, err
	}
	out := make([]*model.ScheduledJobRun, len(runs))
	for i, run := range runs {
		out[i] = toScheduledJobRunModel(run)
	}
	return out, nil
}

func (r *queryResolver) PreviewSchedule(ctx context.Context, schedule string, timezone *string, count *int32) (*model.SchedulePreview, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.view"); err != nil {
		return nil, err
	}
	tz := ptr.Deref(timezone)
	if err := scheduler.ValidateScheduleTZ(schedule, tz); err != nil {
		return &model.SchedulePreview{Valid: false, Error: err.Error(), NextRuns: []*time.Time{}}, nil
	}
	runs := scheduler.PreviewRuns(schedule, tz, int(derefInt32Default(count, 5)))
	out := make([]*time.Time, len(runs))
	for i := range runs {
		t := runs[i]
		out[i] = &t
	}
	return &model.SchedulePreview{Valid: true, NextRuns: out}, nil
}

func (r *queryResolver) WebhookJobConfig(ctx context.Context, id string) (*model.WebhookJobConfig, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "scheduler.manage"); err != nil {
		return nil, err
	}
	if r.Scheduler == nil {
		return nil, errNotConfigured("scheduler")
	}
	jid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", nil)
	}
	d, err := r.Scheduler.WebhookDetail(ctx, jid)
	if err != nil {
		return nil, err
	}
	return &model.WebhookJobConfig{
		URL:       d.URL,
		Method:    d.Method,
		Body:      d.Body,
		Headers:   headersToModel(d.Headers),
		HasSecret: d.HasSecret,
	}, nil
}

package admin

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/scheduler"
)

// headersToModel converts a header map to a name-sorted slice for a stable
// edit-dialog ordering.
func headersToModel(h map[string]string) []*model.HTTPHeader {
	out := make([]*model.HTTPHeader, 0, len(h))
	for k, v := range h {
		out = append(out, &model.HTTPHeader{Name: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func toScheduledJobModel(j scheduler.Job) *model.ScheduledJob {
	m := &model.ScheduledJob{
		ID:                  j.ID.String(),
		Name:                j.Name,
		Kind:                j.Kind,
		Schedule:            j.Schedule,
		Enabled:             j.Enabled,
		Description:         j.Description,
		Timezone:            j.Timezone,
		MaxAttempts:         j.MaxAttempts,
		RetryBackoffSeconds: j.RetryBackoffSeconds,
		CurrentAttempt:      j.CurrentAttempt,
		AlertAfterFailures:  j.AlertAfterFailures,
		ConsecutiveFailures: j.ConsecutiveFailures,
		TimeoutSeconds:      j.TimeoutSeconds,
		NextRunAt:           j.NextRunAt,
		LastRunAt:           j.LastRunAt,
		LastStatus:          j.LastStatus,
		LastError:           j.LastError,
		LastDurationMs:      j.LastDurationMs,
		CreatedAt:           j.CreatedAt,
		UpdatedAt:           j.UpdatedAt,
	}
	if j.Webhook != nil {
		m.Webhook = &model.ScheduledJobWebhook{URL: j.Webhook.URL, Method: j.Webhook.Method, HasSecret: j.Webhook.HasSecret}
	}
	return m
}

func toScheduledJobRunModel(r scheduler.Run) *model.ScheduledJobRun {
	return &model.ScheduledJobRun{
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		Status:     r.Status,
		Error:      r.Error,
		DurationMs: r.DurationMs,
		Attempt:    r.Attempt,
	}
}

// currentAdminID returns the authenticated admin's id for audit columns.
func currentAdminID(ctx context.Context) *uuid.UUID {
	if claims, ok := auth.AdminFromContext(ctx); ok {
		id := claims.AdminID
		return &id
	}
	return nil
}

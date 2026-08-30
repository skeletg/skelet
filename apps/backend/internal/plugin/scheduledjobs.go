package plugin

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/sdk"
)

// Plugin-registered scheduled jobs - plugins call sdk.RegisterScheduledJob from init(), the host records them under the calling plugin's slug, and main.go drains them into the scheduler engine at boot, like health probes but consumed once rather than per-request

type ScheduledJobReg struct {
	Name        string // already slug-qualified as "<plugin>.<name>", and persisted as the scheduled_jobs row key
	Schedule    string
	Description string
	Handler     func(ctx context.Context) error
}

type scheduledJobRegistry struct {
	mu   sync.Mutex
	jobs []ScheduledJobReg
}

var schedJobRegistry = &scheduledJobRegistry{}

// Qualified by the manifest slug, matching health-check naming, so plugin jobs can't collide with core jobs or each other
func (h *Host) RegisterScheduledJob(slug, name, defaultSchedule, description string, handler sdk.ScheduledJobHandler) error {
	full := name
	if slug != "" {
		full = slug + "." + name
	}
	schedJobRegistry.mu.Lock()
	defer schedJobRegistry.mu.Unlock()
	schedJobRegistry.jobs = append(schedJobRegistry.jobs, ScheduledJobReg{
		Name:        full,
		Schedule:    defaultSchedule,
		Description: description,
		Handler:     gatedJobHandler(slug, handler),
	})
	return nil
}

// The scheduler holds the handler for the process's life, so the kill switch is checked per RUN - gating at registration would leave a disabled plugin's cron firing until the next deploy
// A skipped run reports success: the job didn't fail, it wasn't supposed to happen
func gatedJobHandler(slug string, handler sdk.ScheduledJobHandler) sdk.ScheduledJobHandler {
	if slug == "" {
		return handler
	}
	return func(ctx context.Context) error {
		if !isEnabledFromContext(ctx, slug, uuid.Nil) {
			return nil
		}
		return handler(ctx)
	}
}

func ScheduledJobs() []ScheduledJobReg {
	schedJobRegistry.mu.Lock()
	defer schedJobRegistry.mu.Unlock()
	out := make([]ScheduledJobReg, len(schedJobRegistry.jobs))
	copy(out, schedJobRegistry.jobs)
	return out
}

package sdk

import "context"

// Runs outside any request, so read config through PluginConfig rather than a request-scoped reader - a returned error records a failed run
type ScheduledJobHandler func(ctx context.Context) error

// name is qualified by the manifest slug ("payment-billing.reconcile") and persisted as the scheduled_jobs row key, so it has to be right the first time - hence the handle rather than inference
// defaultSchedule is cron ("0 2 * * *") or a descriptor ("@every 15m", "@daily") and only seeds the first row - operators retune or disable it from the admin afterwards, like any core code job
func (pl *Plugin) RegisterScheduledJob(name, defaultSchedule, description string, handler ScheduledJobHandler) {
	withHost(func(h Host) error {
		return h.RegisterScheduledJob(pl.slug, name, defaultSchedule, description, handler)
	})
}

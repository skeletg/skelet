package scheduler

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

// Global scheduler settings, stored in the config registry so admins flip them
// on /settings without a redeploy
const (
	cfgEnabled       = "system/scheduler/enabled"
	cfgAlertWebhook  = "system/scheduler/alert_webhook_url"
	cfgHistoryDays   = "system/scheduler/history_retention_days"
	defaultRetention = 30
)

func init() {
	config.RegisterSection(config.Section{Slug: "system", Label: "System", SortOrder: 99})
	config.RegisterGroup(config.Group{Section: "system", Slug: "scheduler", Label: "Scheduled jobs", SortOrder: 1})
	config.Register(config.Definition{Path: cfgEnabled, Type: config.TypeBool, Default: "true", Label: "Scheduler enabled (master pause switch)"})
	// Secret-typed: a chat-hook URL is itself the credential
	config.Register(config.Definition{Path: cfgAlertWebhook, Type: config.TypeSecret, Default: "", Label: "Failure alert webhook URL"})
	config.Register(config.Definition{Path: cfgHistoryDays, Type: config.TypeInt, Default: "30", Label: "Run-history retention (days)"})
}

// ConfigAdapter implements the engine's Config (pause + alert sink) over the
// config service, and exposes the history retention for the prune job
type ConfigAdapter struct{ cfg *config.Service }

func NewConfig(cfg *config.Service) *ConfigAdapter { return &ConfigAdapter{cfg: cfg} }

func (a *ConfigAdapter) SchedulerEnabled(ctx context.Context) bool {
	v, err := a.cfg.GetBool(ctx, cfgEnabled, config.DefaultScope)
	if err != nil {
		return true // fail-open: a config blip must not silently halt every job
	}
	return v
}

func (a *ConfigAdapter) AlertWebhookURL(ctx context.Context) string {
	v, _ := a.cfg.GetString(ctx, cfgAlertWebhook, config.DefaultScope)
	return v
}

func (a *ConfigAdapter) HistoryRetentionDays(ctx context.Context) int {
	v, err := a.cfg.GetInt(ctx, cfgHistoryDays, config.DefaultScope)
	if err != nil || v <= 0 {
		return defaultRetention
	}
	return int(v)
}

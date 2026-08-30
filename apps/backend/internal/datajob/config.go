package datajob

import (
	"context"
	"time"

	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/permission"
)

const (
	cfgArtifactDays = "system/data_transfer/artifact_retention_days"
	cfgHistoryDays  = "system/data_transfer/history_retention_days"

	defaultArtifactDays = 7
	// The row outlives its file so the history stays readable after the artifact is swept
	defaultHistoryDays = 90
)

func init() {
	config.RegisterGroup(config.Group{Section: "system", Slug: "data_transfer", Label: "Import & export", SortOrder: 4})
	config.Register(config.Definition{
		Path: cfgArtifactDays, Type: config.TypeInt, Default: "7", Label: "Keep generated files for (days)",
		Description: "How long an export stays downloadable and an import's source file is kept for re-reading. The job history outlives the file",
	})
	config.Register(config.Definition{
		Path: cfgHistoryDays, Type: config.TypeInt, Default: "90", Label: "Keep job history for (days)",
		Description: "How long finished import and export runs stay listed",
	})

	permission.RegisterGroup(permission.Group{Slug: "data_transfer", Name: "Import & export", SortOrder: 88})
	permission.Register(permission.Definition{
		Group: "data_transfer", Slug: "data.view", Name: "View import & export", SortOrder: 1,
		Description: "See the transfer hub and its job history - what can actually be run is gated by each resource's own permission",
	})
}

// Set by wiring once the config service exists - nil is fine, the defaults stand
func (s *Service) SetConfig(cfg *config.Service) { s.cfg = cfg }

func (s *Service) retention() time.Duration {
	return s.days(context.Background(), cfgArtifactDays, defaultArtifactDays)
}

// How long a finished run stays listed after its file is gone
func (s *Service) historyRetention(ctx context.Context) time.Duration {
	return s.days(ctx, cfgHistoryDays, defaultHistoryDays)
}

func (s *Service) days(ctx context.Context, path string, fallback int64) time.Duration {
	days := fallback
	if s.cfg != nil {
		if v, err := s.cfg.GetInt(ctx, path, config.DefaultScope); err == nil && v > 0 {
			days = int64(v)
		}
	}
	return time.Duration(days) * 24 * time.Hour
}

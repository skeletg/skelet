package signalsboot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/skeletg/skelet/apps/backend/signals/internal/analytics"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

// The retention bodies live here so cmd/server's scheduler jobs and cmd/signals' ticker loops share one implementation, like TraitComputeJob - hand-kept copies of the same config reads drift

// SpinePruneJob is the analytics-prune body: spine hygiene, then event retention (prune or archive per retention_action)
func SpinePruneJob(cfgSvc Settings, rec *analytics.Recorder) func(context.Context) error {
	return func(ctx context.Context) (err error) {
		maintErr := rec.Maintain(ctx) // hygiene first and unconditionally - the GDPR erasure sweep must not die with a keep-forever retention setting
		defer func() { err = errors.Join(err, maintErr) }()
		archive, err := archiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
		days, err := cfgSvc.Int(ctx, "signals/analytics/spine_retention_days", nil)
		if err != nil {
			return err
		}
		if days > 0 {
			before := time.Now().AddDate(0, 0, -int(days))
			var n int64
			if archive {
				n, err = rec.ArchivePrune(ctx, before)
			} else {
				n, err = rec.Prune(ctx, before)
			}
			if err != nil {
				return err
			}
			if n > 0 {
				slog.Info("spine retention", "rows", n, "archived", archive)
			}
		}
		return nil
	}
}

// ArchiveAction reports whether aged data is archived before deletion - the retention_action setting, which every sweep honours
func ArchiveAction(ctx context.Context, cfgSvc Settings) (bool, error) {
	return archiveAction(ctx, cfgSvc)
}

// The slice of the send log the retention sweep drives, narrow so it can be faked
type SendLogMaintainer interface {
	EnsurePartitions(ctx context.Context) error
	Prune(ctx context.Context, before time.Time, archive bool) (int64, error)
}

// SendLogRetentionJob is the send-log body: partitions first and unconditionally, then whole months past the window
func SendLogRetentionJob(cfgSvc Settings, m SendLogMaintainer) func(context.Context) error {
	return func(ctx context.Context) error {
		// Unconditional - sends start landing in the DEFAULT partition once the pre-created months run out, whatever retention says
		if err := m.EnsurePartitions(ctx); err != nil {
			return err
		}
		months, err := cfgSvc.Int(ctx, "signals/analytics/send_log_retention_months", nil)
		if err != nil || months <= 0 {
			return err
		}
		archive, err := archiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
		// Whole months only, the partition unit - anchored to the first of this month so a run is idempotent within a day
		now := time.Now().UTC()
		cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -int(months), 0)
		n, err := m.Prune(ctx, cutoff, archive)
		if err != nil {
			return err
		}
		if n > 0 {
			slog.Info("send-log retention", "rows", n, "archived", archive)
		}
		return nil
	}
}

// LensPruneJob is the lens-prune body: raw web events + aggregates on the main window, per-day rollups on their own longer window, replay on its shorter one
func LensPruneJob(cfgSvc Settings, lensSvc *lens.Service) func(context.Context) error {
	return func(ctx context.Context) error {
		archive, err := archiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
		if days, err := cfgSvc.Int(ctx, "signals/lens/retention_days", nil); err != nil {
			return err
		} else if days > 0 {
			if _, err := lensSvc.Prune(ctx, time.Now().AddDate(0, 0, -int(days)), archive); err != nil {
				return err
			}
		}
		if days, err := cfgSvc.Int(ctx, "signals/lens/rollup_retention_days", nil); err != nil {
			return err
		} else if days > 0 {
			if _, err := lensSvc.PruneRollups(ctx, time.Now().AddDate(0, 0, -int(days))); err != nil {
				return err
			}
		}
		if days, err := cfgSvc.Int(ctx, "signals/lens/replay_retention_days", nil); err != nil {
			return err
		} else if days > 0 {
			if _, err := lensSvc.PruneReplay(ctx, time.Now().AddDate(0, 0, -int(days))); err != nil {
				return err
			}
		}
		return nil
	}
}

func archiveAction(ctx context.Context, cfgSvc Settings) (bool, error) {
	action, err := cfgSvc.String(ctx, "signals/analytics/retention_action", nil)
	if err != nil {
		return false, err
	}
	return action == "archive", nil
}

// archiveSink adapts the BACKUP_* media store to the Recorder's archive seam, verifying every object landed non-empty - the sweep deletes rows on Put's word

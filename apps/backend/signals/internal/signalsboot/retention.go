package signalsboot

import (
	"context"
	"errors"
	"fmt"
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
		archive, err := ArchiveAction(ctx, cfgSvc)
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

// The slice of the send log the retention sweep drives, narrow so it can be faked
type SendLogMaintainer interface {
	EnsurePartitions(ctx context.Context) error
	Prune(ctx context.Context, before time.Time, archive bool) (int64, error)
}

// The send log's retention edge, shared by the history that points at it - ok is false when retention is off
// Whole months only, the partition unit - anchored to the first of this month so a run is idempotent within a day
func SendHistoryCutoff(ctx context.Context, cfgSvc Settings) (time.Time, bool, error) {
	months, err := cfgSvc.Int(ctx, "signals/analytics/send_log_retention_months", nil)
	if err != nil || months <= 0 {
		return time.Time{}, false, err
	}
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -int(months), 0), true, nil
}

// SendLogRetentionJob is the send-log body: partitions first and unconditionally, then whole months past the window
func SendLogRetentionJob(cfgSvc Settings, m SendLogMaintainer) func(context.Context) error {
	return func(ctx context.Context) error {
		// Unconditional - sends start landing in the DEFAULT partition once the pre-created months run out, whatever retention says
		if err := m.EnsurePartitions(ctx); err != nil {
			return err
		}
		cutoff, ok, err := SendHistoryCutoff(ctx, cfgSvc)
		if err != nil || !ok {
			return err
		}
		archive, err := ArchiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
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
// Each runs whatever the others do - replay is the most sensitive data on the shortest window, and a step failing ahead of it must not keep recordings past theirs
func LensPruneJob(cfgSvc Settings, lensSvc *lens.Service) func(context.Context) error {
	return func(ctx context.Context) error {
		archive, err := ArchiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
		steps := []struct {
			setting string
			prune   func(context.Context, time.Time) (int64, error)
		}{
			{"signals/lens/retention_days", func(ctx context.Context, before time.Time) (int64, error) { return lensSvc.Prune(ctx, before, archive) }},
			{"signals/lens/rollup_retention_days", lensSvc.PruneRollups},
			{"signals/lens/replay_retention_days", lensSvc.PruneReplay},
		}
		var errs []error
		for _, st := range steps {
			days, err := cfgSvc.Int(ctx, st.setting, nil)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", st.setting, err))
				continue
			}
			if days <= 0 {
				continue
			}
			if _, err := st.prune(ctx, time.Now().AddDate(0, 0, -int(days))); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", st.setting, err))
			}
		}
		return errors.Join(errs...)
	}
}

// ArchiveAction reports whether aged data is archived before deletion - the retention_action setting, which every sweep honours
func ArchiveAction(ctx context.Context, cfgSvc Settings) (bool, error) {
	action, err := cfgSvc.String(ctx, "signals/analytics/retention_action", nil)
	if err != nil {
		return false, err
	}
	return action == "archive", nil
}

// archiveSink adapts the BACKUP_* media store to the Recorder's archive seam, verifying every object landed non-empty - the sweep deletes rows on Put's word

type AssistantPruner interface {
	Prune(ctx context.Context, before time.Time) (conversations, turns int64, err error)
}

// AssistantPruneJob deletes whatever retention_action says - transcripts are shoppers' own words, which an anonymised archive
// couldn't keep. They never outlive the event log either, since only shoppers who accepted analytics are kept at all
func AssistantPruneJob(cfgSvc Settings, store AssistantPruner) func(context.Context) (int64, int64, error) {
	return func(ctx context.Context) (int64, int64, error) {
		days, err := cfgSvc.Int(ctx, "signals/assistant/retention_days", nil)
		if err != nil {
			return 0, 0, err
		}
		spine, err := cfgSvc.Int(ctx, "signals/analytics/spine_retention_days", nil)
		if err != nil {
			return 0, 0, err
		}
		if spine > 0 && (days <= 0 || spine < days) {
			days = spine
		}
		if days <= 0 {
			return 0, 0, nil
		}
		conversations, turns, err := store.Prune(ctx, time.Now().AddDate(0, 0, -int(days)))
		if conversations > 0 || turns > 0 {
			slog.Info("assistant conversation retention", "conversations", conversations, "turns", turns)
		}
		return conversations, turns, err
	}
}

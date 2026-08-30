package signalsboot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/skeletg/skelet/internal/analytics"
	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/lens"
	"github.com/skeletg/skelet/internal/media"
)

// The retention bodies live here so cmd/server's scheduler jobs and cmd/signals' ticker loops share one implementation, like TraitComputeJob - hand-kept copies of the same config reads drift

// SpinePruneJob is the analytics-prune body: spine hygiene, then event retention (prune or archive per retention_action), then the send log's whole-month retention
func SpinePruneJob(cfgSvc *config.Service, rec *analytics.Recorder) func(context.Context) error {
	return func(ctx context.Context) error {
		rec.Maintain(ctx) // hygiene first and unconditionally - partition pre-creation and the GDPR erasure sweep must not die with a keep-forever retention setting
		archive, err := archiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
		days, err := cfgSvc.GetInt(ctx, "pulse/analytics/spine_retention_days", config.DefaultScope)
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
		months, err := cfgSvc.GetInt(ctx, "pulse/analytics/email_sends_retention_months", config.DefaultScope)
		if err != nil || months <= 0 {
			return err
		}
		now := time.Now().UTC()
		cutoff := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -int(months), 0)
		n, err := rec.PruneEmailSends(ctx, cutoff, archive)
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
func LensPruneJob(cfgSvc *config.Service, lensSvc *lens.Service) func(context.Context) error {
	return func(ctx context.Context) error {
		archive, err := archiveAction(ctx, cfgSvc)
		if err != nil {
			return err
		}
		if days, err := cfgSvc.GetInt(ctx, "pulse/lens/retention_days", config.DefaultScope); err != nil {
			return err
		} else if days > 0 {
			if _, err := lensSvc.Prune(ctx, time.Now().AddDate(0, 0, -int(days)), archive); err != nil {
				return err
			}
		}
		if days, err := cfgSvc.GetInt(ctx, "pulse/lens/rollup_retention_days", config.DefaultScope); err != nil {
			return err
		} else if days > 0 {
			if _, err := lensSvc.PruneRollups(ctx, time.Now().AddDate(0, 0, -int(days))); err != nil {
				return err
			}
		}
		if days, err := cfgSvc.GetInt(ctx, "pulse/lens/replay_retention_days", config.DefaultScope); err != nil {
			return err
		} else if days > 0 {
			if _, err := lensSvc.PruneReplay(ctx, time.Now().AddDate(0, 0, -int(days))); err != nil {
				return err
			}
		}
		return nil
	}
}

func archiveAction(ctx context.Context, cfgSvc *config.Service) (bool, error) {
	action, err := cfgSvc.GetString(ctx, "pulse/analytics/retention_action", config.DefaultScope)
	if err != nil {
		return false, err
	}
	return action == "archive", nil
}

// archiveSink adapts the BACKUP_* media store to the Recorder's archive seam, verifying every object landed non-empty - the sweep deletes rows on Put's word
type archiveSink struct {
	store  *media.Storage
	prefix string
}

func (a archiveSink) Put(ctx context.Context, key, contentType string, body io.Reader) error {
	full := a.prefix + key
	if err := a.store.UploadStream(ctx, full, contentType, body); err != nil {
		return err
	}
	size, _, exists, err := a.store.Head(ctx, full)
	if err == nil && exists && size > 0 {
		return nil
	}
	_ = a.store.Delete(ctx, full)
	if err != nil {
		return fmt.Errorf("archive object %s failed post-upload verification: %w", full, err)
	}
	return fmt.Errorf("archive object %s missing or empty after upload (size=%d)", full, size)
}

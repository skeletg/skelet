package wiring

import (
	"context"
	"log"
	"time"

	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// The slice of the send log this job drives - narrow so it can be faked, and so the eventual move of the
// table's code out of analytics is a swap of the value passed in rather than a rewrite here
type sendLogMaintainer interface {
	EnsureEmailSendPartitions(ctx context.Context) error
	PruneEmailSends(ctx context.Context, before time.Time, archive bool) (int64, error)
}

// sendLogRetentionJob is core rather than a Signals job: the send log is a commerce table that every transactional
// mail writes, so it has to be maintained with Signals off and on a server that hands its Signals jobs to cmd/signals
func sendLogRetentionJob(cfgSvc *config.Service, m sendLogMaintainer, archiveAction func(context.Context, *config.Service) (bool, error)) func(context.Context) error {
	return func(ctx context.Context) error {
		// Unconditional - sends start landing in the DEFAULT partition once the pre-created months run out, whatever retention says
		if err := m.EnsureEmailSendPartitions(ctx); err != nil {
			return err
		}
		months, err := cfgSvc.GetInt(ctx, "pulse/analytics/email_sends_retention_months", config.DefaultScope)
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
		n, err := m.PruneEmailSends(ctx, cutoff, archive)
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("[sendlog] retention dropped %d rows (archived=%t)", n, archive)
		}
		return nil
	}
}

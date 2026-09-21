package wiring

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/fx"

	"github.com/skeletg/skelet/apps/backend/core/internal/linkgraph"
	"github.com/skeletg/skelet/apps/backend/core/internal/notification"
	"github.com/skeletg/skelet/apps/backend/core/internal/sitecrawl"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

type seoCrawlDeps struct {
	fx.In
	Crawl        *sitecrawl.Service
	Graph        *linkgraph.Service
	Config       *config.Service
	Notification *notification.Service
}

// The nightly crawl is opt-in per channel, and the diff runs off the completion hook rather than the job, because a
// crawl outlives the tick that started it
func wireSEOCrawl(d seoCrawlDeps) {
	d.Crawl.SetOnComplete(func(ctx context.Context, channelID, crawlID uuid.UUID) {
		diff, err := d.Graph.Diff(ctx, channelID, nil, &crawlID)
		if err != nil {
			if !errors.Is(err, linkgraph.ErrNoBaseline) {
				slog.Error("seo crawl diff failed", "channel", channelID, "crawl", crawlID, "err", err)
			}
			return
		}
		scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
		threshold, err := d.Config.GetInt(ctx, "seo/crawl/alert_threshold", scope)
		if err != nil || threshold < 1 {
			threshold = 1
		}
		if int64(diff.Regressions) < threshold {
			return
		}
		d.notify(ctx, channelID, crawlID, diff)
	})
}

func (d seoCrawlDeps) notify(ctx context.Context, channelID, crawlID uuid.UUID, diff linkgraph.Diff) {
	headline := make([]string, 0, 4)
	for _, kind := range []linkgraph.ChangeKind{
		linkgraph.ChangeNowBroken, linkgraph.ChangeGone, linkgraph.ChangeNowOrphan,
		linkgraph.ChangeNowNoindex, linkgraph.ChangeLostInLinks, linkgraph.ChangeNowRedirect,
	} {
		if n := diff.Counts[kind]; n > 0 {
			headline = append(headline, fmt.Sprintf("%d %s", n, strings.ToLower(strings.ReplaceAll(string(kind), "_", " "))))
		}
	}
	err := d.Notification.Create(ctx, notification.Input{
		Category:           "seo",
		Severity:           notification.SeverityWarning,
		Title:              fmt.Sprintf("Nightly crawl found %d regressions", diff.Regressions),
		Body:               strings.Join(headline, ", ") + fmt.Sprintf(" across %d pages compared", diff.Compared),
		RequiredPermission: "urlrewrite.view",
		ChannelID:          &channelID,
		EntityType:         "seo_crawl",
		EntityID:           crawlID.String(),
		ActionURL:          "/seo/link-map",
		DedupeKey:          "seo-crawl-diff:" + crawlID.String(),
	})
	if err != nil {
		slog.Error("seo crawl notification failed", "crawl", crawlID, "err", err)
	}
}

// nightlyCrawl starts a crawl for every channel that opted in, and is a no-op where nobody did
func nightlyCrawl(d seoCrawlDeps) func(context.Context) error {
	return func(ctx context.Context) error {
		channels, err := d.Crawl.OptedInChannels(ctx, func(channelID uuid.UUID) (bool, int) {
			scope := config.Scope{Kind: config.ScopeChannel, ChannelID: &channelID}
			on, err := d.Config.GetBool(ctx, "seo/crawl/nightly_enabled", scope)
			if err != nil || !on {
				return false, 0
			}
			budget, err := d.Config.GetInt(ctx, "seo/crawl/max_pages", scope)
			if err != nil || budget <= 0 {
				budget = 2000
			}
			return true, int(budget)
		})
		if err != nil {
			return err
		}
		if channels > 0 {
			slog.Info("nightly seo crawl started", "channels", channels)
		}
		return nil
	}
}

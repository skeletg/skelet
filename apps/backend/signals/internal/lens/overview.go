package lens

import (
	"context"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// --- read aggregates (dashboard) ---

// One row of the top-pages ranking - Views and Visitors are windowed while Clicks and AvgScroll are cumulative grid totals, since the grid has no per-hit timestamp
// Device is per-pageview, so it only shows in the channel-wide split, not per page where a single value would mislead
type PageStat struct {
	PageKey   string
	Views     int64
	Visitors  int64
	Clicks    int64
	AvgScroll int // 0..100
}

type LabelCount struct {
	Label string
	Views int64
}

type DayViews struct {
	Date  string
	Views int64
}

type Overview struct {
	Views        int64
	Visitors     int64
	Daily        []DayViews
	TopPages     []PageStat
	Devices      []LabelCount
	Referrers    []LabelCount
	Interactions []LabelCount
}

// Reads the daily rollup and visitor seen-table, never raw spine events - per-page unique visitors aren't in the rollup yet, so PageStat.Visitors is 0
func (s *Service) Overview(ctx context.Context, channelID uuid.UUID, since time.Time) (Overview, error) {
	views, err := s.read().LensTotalViews(ctx, signalsdb.LensTotalViewsParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Views: views}
	v, err := s.read().LensVisitorCount(ctx, signalsdb.LensVisitorCountParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Overview{}, err
	}
	out.Visitors = v
	rows, err := s.read().LensDailyViews(ctx, signalsdb.LensDailyViewsParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Overview{}, err
	}
	for _, r := range rows {
		out.Daily = append(out.Daily, DayViews{Date: r.Day, Views: r.Views})
	}
	pages, err := s.read().LensTopPages(ctx, signalsdb.LensTopPagesParams{ChannelID: channelID, Since: since, Lim: 50})
	if err != nil {
		return Overview{}, err
	}
	// Per-page engagement - cumulative clicks and avg scroll from the grid
	type eng struct {
		clicks    int64
		avgScroll int
	}
	engByPage := map[string]eng{}
	grids, err := s.rec.LensPagesGridFromEvents(ctx, channelID, s.retentionEdge(ctx))
	if err != nil {
		return Overview{}, err
	}
	for page, g := range grids {
		avg := 0
		if g.ScrollTotal > 0 {
			avg = int(g.ScrollWeight / g.ScrollTotal)
		}
		engByPage[page] = eng{clicks: g.Clicks, avgScroll: avg}
	}
	for _, p := range pages {
		e := engByPage[p.PageKey]
		out.TopPages = append(out.TopPages, PageStat{
			PageKey: p.PageKey, Views: p.Views,
			Clicks: e.clicks, AvgScroll: e.avgScroll,
		})
	}
	devs, err := s.read().LensDeviceSplit(ctx, signalsdb.LensDeviceSplitParams{ChannelID: channelID, Since: since})
	if err != nil {
		return Overview{}, err
	}
	for _, d := range devs {
		out.Devices = append(out.Devices, LabelCount{Label: d.Device, Views: d.Views})
	}
	refs, err := s.read().LensTopReferrers(ctx, signalsdb.LensTopReferrersParams{ChannelID: channelID, Since: since, Lim: 10})
	if err != nil {
		return Overview{}, err
	}
	for _, r := range refs {
		out.Referrers = append(out.Referrers, LabelCount{Label: r.Referrer, Views: r.Views})
	}
	ix, err := s.read().LensTopInteractions(ctx, signalsdb.LensTopInteractionsParams{ChannelID: channelID, Since: since, Lim: 15})
	if err != nil {
		return Overview{}, err
	}
	for _, r := range ix {
		out.Interactions = append(out.Interactions, LabelCount{Label: r.Name, Views: r.Count})
	}
	return out, nil
}

type BotAgent struct {
	Name     string
	Category string
	Hits     int64
	Pages    int64
	LastSeen time.Time
}

type BotPage struct {
	PageKey string
	Hits    int64
	Bots    int64
}

type BotDayPoint struct {
	Day        string
	BotHits    int64
	HumanViews int64
}

// The crawler-analytics rollup for a channel and window - bot-vs-human totals, the per-agent breakdown, the most-crawled pages (optionally for one bot), and a daily series
type BotTraffic struct {
	BotHits    int64
	HumanViews int64
	BotCount   int64
	PageCount  int64
	Bots       []BotAgent
	TopPages   []BotPage
	Daily      []BotDayPoint
}

// `bot` filters TopPages to one agent - empty means all bots
func (s *Service) BotTraffic(ctx context.Context, channelID uuid.UUID, since time.Time, bot string) (BotTraffic, error) {
	sum, err := s.read().BotTrafficSummary(ctx, signalsdb.BotTrafficSummaryParams{ChannelID: channelID, Since: since})
	if err != nil {
		return BotTraffic{}, err
	}
	out := BotTraffic{BotHits: sum.BotHits, HumanViews: sum.HumanViews, BotCount: sum.BotCount, PageCount: sum.PageCount}
	agents, err := s.read().BotByAgent(ctx, signalsdb.BotByAgentParams{ChannelID: channelID, Since: since})
	if err != nil {
		return BotTraffic{}, err
	}
	for _, r := range agents {
		ls, _ := r.LastSeen.(time.Time)
		out.Bots = append(out.Bots, BotAgent{
			Name: r.BotName, Category: r.BotCategory,
			Hits: r.Hits, Pages: r.Pages, LastSeen: ls,
		})
	}
	pages, err := s.read().BotTopPages(ctx, signalsdb.BotTopPagesParams{ChannelID: channelID, Since: since, BotName: bot, Lim: 25})
	if err != nil {
		return BotTraffic{}, err
	}
	for _, r := range pages {
		out.TopPages = append(out.TopPages, BotPage{PageKey: r.PageKey, Hits: r.Hits, Bots: r.Bots})
	}
	daily, err := s.read().BotDailySeries(ctx, signalsdb.BotDailySeriesParams{ChannelID: channelID, Since: since})
	if err != nil {
		return BotTraffic{}, err
	}
	for _, r := range daily {
		out.Daily = append(out.Daily, BotDayPoint{Day: r.Day, BotHits: r.BotHits, HumanViews: r.HumanViews})
	}
	return out, nil
}

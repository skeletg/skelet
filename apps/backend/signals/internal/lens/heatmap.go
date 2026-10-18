package lens

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// --- heatmap reads (Stage B) ---

// One grid bucket - cell coordinates, multiply by cellSize for px, plus a hit count - For scroll, X is always 0 and Y is the depth percentage
type Cell struct {
	X, Y  int
	Count int64
}

type ElementClick struct {
	Selector string
	Text     string
	Count    int64
}

// A trafficked page for the viewer's page picker - same source as the overview's Top Pages, so the two lists stay consistent
type PageEntry struct {
	PageKey string
	Views   int64
}

// Everything the dashboard needs for one page and viewport - the snapshot backdrop as JSON for rrweb rebuild, its dimensions, the click/move/scroll density grids, and the clicked-element ranking
type Heatmap struct {
	SnapshotJSON string
	Width        int
	Height       int
	Click        []Cell
	Move         []Cell
	Scroll       []Cell
	Elements     []ElementClick
}

func (s *Service) ListPages(ctx context.Context, channelID uuid.UUID) ([]PageEntry, error) {
	rows, err := s.rec.LensListPages(ctx, channelID, s.retentionEdge(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]PageEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, PageEntry{PageKey: r.Value, Views: r.Count})
	}
	return out, nil
}

func (s *Service) Viewports(ctx context.Context, channelID uuid.UUID, pageKey string) ([]string, error) {
	return s.read().LensViewports(ctx, signalsdb.LensViewportsParams{ChannelID: channelID, PageKey: pageKey})
}

func (s *Service) PageDevices(ctx context.Context, channelID uuid.UUID, pageKey string) ([]LabelCount, error) {
	rows, err := s.rec.LensPageDeviceSplit(ctx, channelID, pageKey, s.retentionEdge(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]LabelCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, LabelCount{Label: r.Value, Views: r.Count})
	}
	return out, nil
}

type TimePoint struct {
	Date  string
	Views int64
}

// The rich per-page bundle for the heatmap detail view - view totals, daily timeline and referrer/client/device splits from the spine, plus cumulative click and scroll engagement from the grid
type PageStatsResult struct {
	Views     int64
	Visitors  int64
	Clicks    int64
	AvgScroll int
	Timeline  []TimePoint
	Referrers []LabelCount
	Clients   []LabelCount
	Devices   []LabelCount
}

func (s *Service) PageStats(ctx context.Context, channelID uuid.UUID, pageKey string) (PageStatsResult, error) {
	var res PageStatsResult
	views, visitors, err := s.rec.LensPageTotals(ctx, channelID, pageKey, s.retentionEdge(ctx))
	if err != nil {
		return PageStatsResult{}, err
	}
	res.Views, res.Visitors = views, visitors
	g, err := s.rec.LensPageGridFromEvents(ctx, channelID, pageKey, s.retentionEdge(ctx))
	if err != nil {
		return PageStatsResult{}, err
	}
	res.Clicks = g.Clicks
	if g.ScrollTotal > 0 {
		res.AvgScroll = int(g.ScrollWeight / g.ScrollTotal)
	}
	timeline, err := s.rec.LensPageTimeline(ctx, channelID, pageKey, time.Now().AddDate(0, 0, -30))
	if err != nil {
		return PageStatsResult{}, err
	}
	for _, r := range timeline {
		res.Timeline = append(res.Timeline, TimePoint{Date: r.Date, Views: r.Views})
	}
	referrers, err := s.rec.LensPageReferrers(ctx, channelID, pageKey, s.retentionEdge(ctx))
	if err != nil {
		return PageStatsResult{}, err
	}
	for _, r := range referrers {
		res.Referrers = append(res.Referrers, LabelCount{Label: r.Value, Views: r.Count})
	}
	clients, err := s.rec.LensPageClients(ctx, channelID, pageKey, s.retentionEdge(ctx))
	if err != nil {
		return PageStatsResult{}, err
	}
	for _, r := range clients {
		res.Clients = append(res.Clients, LabelCount{Label: r.Value, Views: r.Count})
	}
	devs, err := s.PageDevices(ctx, channelID, pageKey)
	if err != nil {
		return PageStatsResult{}, err
	}
	res.Devices = devs
	return res, nil
}

// Captured UI states for a page and viewport, 'default' first
func (s *Service) States(ctx context.Context, channelID uuid.UUID, pageKey, viewport string) ([]string, error) {
	return s.read().LensStates(ctx, signalsdb.LensStatesParams{ChannelID: channelID, PageKey: pageKey, ViewportBucket: viewport})
}

// Returns ok=false when that state has no snapshot yet
func (s *Service) Heatmap(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state string) (Heatmap, bool, error) {
	state = stateOr(state)
	snap, err := s.read().LensLatestSnapshot(ctx, signalsdb.LensLatestSnapshotParams{ChannelID: channelID, PageKey: pageKey, ViewportBucket: viewport, StateKey: state})
	if errors.Is(err, sql.ErrNoRows) {
		return Heatmap{}, false, nil // no snapshot yet - not an error to the caller
	}
	if err != nil {
		return Heatmap{}, false, err
	}
	if s.blobs == nil {
		return Heatmap{}, false, nil
	}
	htmlGz, err := s.blobs.GetBytes(ctx, snap.BlobKey)
	if err != nil {
		return Heatmap{}, false, nil // the object is gone, so neither is the heatmap
	}
	hm := Heatmap{SnapshotJSON: string(gunzipBytes(htmlGz)), Width: int(snap.Width), Height: int(snap.Height)}
	for kind, dst := range map[string]*[]Cell{"click": &hm.Click, "move": &hm.Move, "scroll": &hm.Scroll} {
		if *dst, err = s.cells(ctx, channelID, pageKey, viewport, state, kind); err != nil {
			return Heatmap{}, false, err
		}
	}
	els, err := s.rec.LensElementsFromEvents(ctx, channelID, pageKey, viewport, 25, s.retentionEdge(ctx))
	if err != nil {
		return Heatmap{}, false, err
	}
	for _, e := range els {
		hm.Elements = append(hm.Elements, ElementClick{Selector: e.Selector, Text: e.Text, Count: e.Count})
	}
	return hm, true, nil
}

func (s *Service) cells(ctx context.Context, channelID uuid.UUID, pageKey, viewport, state, kind string) ([]Cell, error) {
	rows, err := s.rec.LensCellsFromEvents(ctx, channelID, pageKey, viewport, state, kind, s.retentionEdge(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]Cell, 0, len(rows))
	for _, r := range rows {
		out = append(out, Cell{X: int(r.X), Y: int(r.Y), Count: r.Count})
	}
	return out, nil
}

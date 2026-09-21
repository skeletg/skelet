package signalsgrpc

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
	"github.com/skeletg/skelet/apps/backend/signals/internal/lens"
)

type lensServer struct {
	signalsv1.UnimplementedLensServiceServer
	svc *lens.Service
}

func (s *lensServer) GetOverview(ctx context.Context, req *signalsv1.GetOverviewRequest) (*signalsv1.GetOverviewResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	ov, err := s.svc.Overview(ctx, cid, since(req.GetSinceDays(), 30))
	if err != nil {
		return nil, err
	}
	daily := make([]*signalsv1.LensTimePoint, 0, len(ov.Daily))
	for _, d := range ov.Daily {
		daily = append(daily, &signalsv1.LensTimePoint{Date: d.Date, Views: int32(d.Views)})
	}
	top := make([]*signalsv1.LensPageStat, 0, len(ov.TopPages))
	for _, p := range ov.TopPages {
		top = append(top, &signalsv1.LensPageStat{
			PageKey: p.PageKey, Views: int32(p.Views), Visitors: int32(p.Visitors),
			Clicks: int32(p.Clicks), AvgScroll: int32(p.AvgScroll),
		})
	}
	return &signalsv1.GetOverviewResponse{
		Views: int32(ov.Views), Visitors: int32(ov.Visitors), Daily: daily, TopPages: top,
		Devices: labelCounts(ov.Devices), Referrers: labelCounts(ov.Referrers), Interactions: labelCounts(ov.Interactions),
	}, nil
}

func (s *lensServer) GetBotTraffic(ctx context.Context, req *signalsv1.GetBotTrafficRequest) (*signalsv1.GetBotTrafficResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	bt, err := s.svc.BotTraffic(ctx, cid, since(req.GetSinceDays(), 30), req.GetBot())
	if err != nil {
		return nil, err
	}
	bots := make([]*signalsv1.BotAgent, 0, len(bt.Bots))
	for _, b := range bt.Bots {
		bots = append(bots, &signalsv1.BotAgent{Name: b.Name, Category: b.Category, Hits: int32(b.Hits), Pages: int32(b.Pages), LastSeen: ts(b.LastSeen)})
	}
	pages := make([]*signalsv1.BotPage, 0, len(bt.TopPages))
	for _, p := range bt.TopPages {
		pages = append(pages, &signalsv1.BotPage{PageKey: p.PageKey, Hits: int32(p.Hits), Bots: int32(p.Bots)})
	}
	daily := make([]*signalsv1.BotDayPoint, 0, len(bt.Daily))
	for _, d := range bt.Daily {
		daily = append(daily, &signalsv1.BotDayPoint{Day: d.Day, BotHits: int32(d.BotHits), HumanViews: int32(d.HumanViews)})
	}
	return &signalsv1.GetBotTrafficResponse{
		BotHits: int32(bt.BotHits), HumanViews: int32(bt.HumanViews),
		BotCount: int32(bt.BotCount), PageCount: int32(bt.PageCount),
		Bots: bots, TopPages: pages, Daily: daily,
	}, nil
}

func (s *lensServer) ListPages(ctx context.Context, req *signalsv1.ListPagesRequest) (*signalsv1.ListPagesResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.ListPages(ctx, cid)
	if err != nil {
		return nil, err
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 100)
	if err != nil {
		return nil, err
	}
	window := slice(len(rows), offset, size)
	out := make([]*signalsv1.LensPage, 0, len(window))
	for _, i := range window {
		out = append(out, &signalsv1.LensPage{PageKey: rows[i].PageKey, Views: int32(rows[i].Views)})
	}
	return &signalsv1.ListPagesResponse{Pages: out, NextPageToken: nextToken(offset, size, int32(len(out)), int32(len(rows)))}, nil
}

func (s *lensServer) ListStates(ctx context.Context, req *signalsv1.ListStatesRequest) (*signalsv1.ListStatesResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	states, err := s.svc.States(ctx, cid, req.GetPageKey(), req.GetViewport())
	if err != nil {
		return nil, err
	}
	return &signalsv1.ListStatesResponse{States: states}, nil
}

func (s *lensServer) GetPageStats(ctx context.Context, req *signalsv1.GetPageStatsRequest) (*signalsv1.GetPageStatsResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	ps, err := s.svc.PageStats(ctx, cid, req.GetPageKey())
	if err != nil {
		return nil, err
	}
	timeline := make([]*signalsv1.LensTimePoint, 0, len(ps.Timeline))
	for _, t := range ps.Timeline {
		timeline = append(timeline, &signalsv1.LensTimePoint{Date: t.Date, Views: int32(t.Views)})
	}
	return &signalsv1.GetPageStatsResponse{
		Views: int32(ps.Views), Visitors: int32(ps.Visitors), Clicks: int32(ps.Clicks), AvgScroll: int32(ps.AvgScroll),
		Timeline: timeline, Referrers: labelCounts(ps.Referrers), Clients: labelCounts(ps.Clients), Devices: labelCounts(ps.Devices),
	}, nil
}

func (s *lensServer) GetHeatmap(ctx context.Context, req *signalsv1.GetHeatmapRequest) (*signalsv1.GetHeatmapResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	hm, found, err := s.svc.Heatmap(ctx, cid, req.GetPageKey(), req.GetViewport(), req.GetState())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, status.Error(codes.NotFound, "no snapshot captured for that page, viewport and state")
	}
	elements := make([]*signalsv1.LensElementClick, 0, len(hm.Elements))
	for _, e := range hm.Elements {
		elements = append(elements, &signalsv1.LensElementClick{Selector: e.Selector, Text: e.Text, Count: int32(e.Count)})
	}
	return &signalsv1.GetHeatmapResponse{Heatmap: &signalsv1.Heatmap{
		PageKey: req.GetPageKey(), Viewport: req.GetViewport(), State: req.GetState(),
		Width: int32(hm.Width), Height: int32(hm.Height), SnapshotJson: hm.SnapshotJSON,
		Click: cells(hm.Click), Move: cells(hm.Move), Scroll: cells(hm.Scroll), Elements: elements,
	}}, nil
}

func (s *lensServer) ListSessions(ctx context.Context, req *signalsv1.ListSessionsRequest) (*signalsv1.ListSessionsResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 50)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.SessionsForChannel(ctx, cid, offset+size)
	if err != nil {
		return nil, err
	}
	sessions, next := sessionPage(rows, offset, size)
	return &signalsv1.ListSessionsResponse{Sessions: sessions, NextPageToken: next}, nil
}

func (s *lensServer) ListCustomerSessions(ctx context.Context, req *signalsv1.ListCustomerSessionsRequest) (*signalsv1.ListCustomerSessionsResponse, error) {
	if err := requirePermission(ctx, "lens.view"); err != nil {
		return nil, err
	}
	id, err := parseID("customer_id", req.GetCustomerId())
	if err != nil {
		return nil, err
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 50)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.SessionsForCustomer(ctx, id, offset+size)
	if err != nil {
		return nil, err
	}
	sessions, next := sessionPage(rows, offset, size)
	return &signalsv1.ListCustomerSessionsResponse{Sessions: sessions, NextPageToken: next}, nil
}

func (s *lensServer) ListVisitorSessions(ctx context.Context, req *signalsv1.ListVisitorSessionsRequest) (*signalsv1.ListVisitorSessionsResponse, error) {
	if err := requirePermission(ctx, "lens.view"); err != nil {
		return nil, err
	}
	if req.GetAnonId() == "" {
		return nil, invalidArgument("anon_id is required")
	}
	size, offset, err := page(req.GetPageSize(), req.GetPageToken(), 50)
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.SessionsForAnon(ctx, req.GetAnonId(), offset+size)
	if err != nil {
		return nil, err
	}
	sessions, next := sessionPage(rows, offset, size)
	return &signalsv1.ListVisitorSessionsResponse{Sessions: sessions, NextPageToken: next}, nil
}

// Chunked because a recording runs to megabytes - the caller concatenates them back into the JSON
// array the player takes
func (s *lensServer) GetSessionEvents(req *signalsv1.GetSessionEventsRequest, stream signalsv1.LensService_GetSessionEventsServer) error {
	ctx := stream.Context()
	if err := requirePermission(ctx, "lens.view"); err != nil {
		return err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return err
	}
	info, err := s.svc.GetSession(ctx, id)
	if err != nil {
		return status.Error(codes.NotFound, "session not found")
	}
	if err := requireChannel(ctx, "lens.view", info.ChannelID); err != nil {
		return err
	}
	body, err := s.svc.SessionEvents(ctx, id)
	if err != nil {
		return err
	}
	const chunkSize = 256 << 10 // comfortably under the 4MB default message cap, with room for framing
	for start := 0; start < len(body); start += chunkSize {
		end := min(start+chunkSize, len(body))
		if err := stream.Send(&signalsv1.GetSessionEventsResponse{Chunk: body[start:end]}); err != nil {
			return err
		}
	}
	return nil
}

func (s *lensServer) GetSessionSignals(ctx context.Context, req *signalsv1.GetSessionSignalsRequest) (*signalsv1.GetSessionSignalsResponse, error) {
	if err := requirePermission(ctx, "lens.view"); err != nil {
		return nil, err
	}
	id, err := parseID("id", req.GetId())
	if err != nil {
		return nil, err
	}
	info, err := s.svc.GetSession(ctx, id)
	if err != nil {
		return nil, status.Error(codes.NotFound, "session not found")
	}
	if err := requireChannel(ctx, "lens.view", info.ChannelID); err != nil {
		return nil, err
	}
	sig, err := s.svc.SessionSignals(ctx, info.ChannelID, id.String(), info.StartedAt, info.EndedAt)
	if err != nil {
		return nil, err
	}
	errs := make([]*signalsv1.SessionError, 0, len(sig.Errors))
	for _, e := range sig.Errors {
		errs = append(errs, &signalsv1.SessionError{Message: e.Message, At: ts(e.At)})
	}
	vitals := make([]*signalsv1.SessionVital, 0, len(sig.Vitals))
	for _, v := range sig.Vitals {
		vitals = append(vitals, &signalsv1.SessionVital{Metric: v.Metric, Value: v.Value})
	}
	return &signalsv1.GetSessionSignalsResponse{Errors: errs, Vitals: vitals}, nil
}

func (s *lensServer) GetFrustration(ctx context.Context, req *signalsv1.GetFrustrationRequest) (*signalsv1.GetFrustrationResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	fr, err := s.svc.Frustration(ctx, cid, since(req.GetSinceDays(), 30), 10)
	if err != nil {
		return nil, err
	}
	elements := make([]*signalsv1.LensRageElement, 0, len(fr.RageElements))
	for _, e := range fr.RageElements {
		elements = append(elements, &signalsv1.LensRageElement{Selector: e.Selector, Text: e.Text, Count: int32(e.Count)})
	}
	return &signalsv1.GetFrustrationResponse{
		RagePages: signalPages(fr.RagePages), DeadPages: signalPages(fr.DeadPages), RageElements: elements,
	}, nil
}

func (s *lensServer) ListErrors(ctx context.Context, req *signalsv1.ListErrorsRequest) (*signalsv1.ListErrorsResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.TopErrors(ctx, cid, since(req.GetSinceDays(), 30), topN(req.GetLimit(), 10, 50))
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.LensErrorStat, 0, len(rows))
	for _, e := range rows {
		out = append(out, &signalsv1.LensErrorStat{Message: e.Message, Count: int32(e.Count), Pages: int32(e.Pages), LastSeen: ts(e.LastSeen)})
	}
	return &signalsv1.ListErrorsResponse{Errors: out}, nil
}

func (s *lensServer) GetPerformance(ctx context.Context, req *signalsv1.GetPerformanceRequest) (*signalsv1.GetPerformanceResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.Performance(ctx, cid, since(req.GetSinceDays(), 30))
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.LensPageVitals, 0, len(rows))
	for _, p := range rows {
		out = append(out, &signalsv1.LensPageVitals{
			PageKey: p.PageKey, Lcp: p.LCP, Cls: p.CLS, Inp: p.INP, Ttfb: p.TTFB, Samples: int32(p.Samples),
		})
	}
	return &signalsv1.GetPerformanceResponse{Pages: out}, nil
}

func (s *lensServer) GetVitalsSummary(ctx context.Context, req *signalsv1.GetVitalsSummaryRequest) (*signalsv1.GetVitalsSummaryResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	rows, err := s.svc.VitalsOverall(ctx, cid, since(req.GetSinceDays(), 30))
	if err != nil {
		return nil, err
	}
	out := make([]*signalsv1.LensVital, 0, len(rows))
	for _, v := range rows {
		out = append(out, &signalsv1.LensVital{Metric: v.Metric, P75: v.P75, Samples: int32(v.Samples)})
	}
	return &signalsv1.GetVitalsSummaryResponse{Vitals: out}, nil
}

func (s *lensServer) GetSearchAnalytics(ctx context.Context, req *signalsv1.GetSearchAnalyticsRequest) (*signalsv1.GetSearchAnalyticsResponse, error) {
	cid, err := channelScope(ctx, "lens.view", req.GetChannelId())
	if err != nil {
		return nil, err
	}
	sa, err := s.svc.SearchAnalytics(ctx, cid, since(req.GetSinceDays(), 30))
	if err != nil {
		return nil, err
	}
	daily := make([]*signalsv1.LensSearchDay, 0, len(sa.Daily))
	for _, d := range sa.Daily {
		daily = append(daily, &signalsv1.LensSearchDay{Date: d.Date, Searches: int32(d.Searches), ZeroResults: int32(d.ZeroResults)})
	}
	return &signalsv1.GetSearchAnalyticsResponse{
		TotalSearches: int32(sa.TotalSearches), DistinctTerms: int32(sa.DistinctTerms),
		ZeroResultSearches: int32(sa.ZeroResultSearches),
		Top:                searchTerms(sa.Top), ZeroResult: searchTerms(sa.ZeroResult), Daily: daily,
	}, nil
}

func labelCounts(rows []lens.LabelCount) []*signalsv1.LabelCount {
	out := make([]*signalsv1.LabelCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, &signalsv1.LabelCount{Label: r.Label, Views: int32(r.Views)})
	}
	return out
}

func cells(rows []lens.Cell) []*signalsv1.LensCell {
	out := make([]*signalsv1.LensCell, 0, len(rows))
	for _, c := range rows {
		out = append(out, &signalsv1.LensCell{X: int32(c.X), Y: int32(c.Y), Count: int32(c.Count)})
	}
	return out
}

func signalPages(rows []lens.SignalPage) []*signalsv1.LensSignalPage {
	out := make([]*signalsv1.LensSignalPage, 0, len(rows))
	for _, p := range rows {
		out = append(out, &signalsv1.LensSignalPage{PageKey: p.PageKey, Count: int32(p.Count)})
	}
	return out
}

func searchTerms(rows []lens.TermCount) []*signalsv1.LensSearchTerm {
	out := make([]*signalsv1.LensSearchTerm, 0, len(rows))
	for _, t := range rows {
		out = append(out, &signalsv1.LensSearchTerm{Term: t.Term, Count: int32(t.Count)})
	}
	return out
}

// The session services take a plain limit, so a page is taken by over-fetching and slicing
func sessionPage(rows []lens.SessionMeta, offset, size int32) ([]*signalsv1.LensSession, string) {
	window := slice(len(rows), offset, size)
	out := make([]*signalsv1.LensSession, 0, len(window))
	for _, i := range window {
		m := rows[i]
		sess := &signalsv1.LensSession{
			Id: m.ID.String(), AnonId: m.AnonID,
			StartedAt: ts(m.StartedAt), EndedAt: ts(m.EndedAt),
			DurationMs: int32(m.DurationMs), EventCount: int32(m.EventCount),
			EntryPage: m.EntryPage, ExitPage: m.ExitPage, Viewport: m.Viewport, UserAgent: m.UserAgent,
			RageCount: int32(m.RageCount), ErrorCount: int32(m.ErrorCount),
		}
		if m.CustomerID != nil {
			id := m.CustomerID.String()
			sess.CustomerId = &id
		}
		out = append(out, sess)
	}
	return out, nextToken(offset, size, int32(len(out)), int32(len(rows)))
}

// A window in days back from now - the server owns "now", which is why the wire carries days
func since(days, fallback int32) time.Time {
	return time.Now().AddDate(0, 0, -int(windowDays(days, fallback)))
}

// A ranked top-N takes a limit rather than a page, and one outside the bounds falls back
func topN(requested, fallback, ceiling int32) int32 {
	if requested < 1 || requested > ceiling {
		return fallback
	}
	return requested
}

// The indexes of one page of a slice already read whole
func slice(length int, offset, size int32) []int {
	start := int(offset)
	if start >= length {
		return nil
	}
	end := min(start+int(size), length)
	out := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, i)
	}
	return out
}

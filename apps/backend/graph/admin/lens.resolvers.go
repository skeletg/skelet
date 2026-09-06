package admin

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
)

func (r *queryResolver) LensOverview(ctx context.Context, channelID string, sinceDays *int32) (*model.LensOverview, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	days := int32(30)
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	ov, err := r.LensSvc.Overview(ctx, cid, time.Now().AddDate(0, 0, -int(days)))
	if err != nil {
		return nil, err
	}
	out := &model.LensOverview{
		Views:        int32(ov.Views),
		Visitors:     int32(ov.Visitors),
		Daily:        []*model.LensTimePoint{},
		TopPages:     []*model.LensPageStat{},
		Devices:      []*model.LensLabelCount{},
		Referrers:    []*model.LensLabelCount{},
		Interactions: []*model.LensLabelCount{},
	}
	for _, d := range ov.Daily {
		out.Daily = append(out.Daily, &model.LensTimePoint{Date: d.Date, Views: int32(d.Views)})
	}
	for _, p := range ov.TopPages {
		out.TopPages = append(out.TopPages, &model.LensPageStat{
			PageKey: p.PageKey, Views: int32(p.Views), Visitors: int32(p.Visitors),
			Clicks: int32(p.Clicks), AvgScroll: int32(p.AvgScroll),
		})
	}
	for _, d := range ov.Devices {
		out.Devices = append(out.Devices, &model.LensLabelCount{Label: d.Label, Views: int32(d.Views)})
	}
	for _, rf := range ov.Referrers {
		out.Referrers = append(out.Referrers, &model.LensLabelCount{Label: rf.Label, Views: int32(rf.Views)})
	}
	for _, ix := range ov.Interactions {
		out.Interactions = append(out.Interactions, &model.LensLabelCount{Label: ix.Label, Views: int32(ix.Views)})
	}
	return out, nil
}

func (r *queryResolver) BotTraffic(ctx context.Context, channelID string, sinceDays *int32, bot *string) (*model.BotTraffic, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	days := int32(30)
	if sinceDays != nil && *sinceDays > 0 {
		days = *sinceDays
	}
	filter := ""
	if bot != nil {
		filter = *bot
	}
	bt, err := r.LensSvc.BotTraffic(ctx, cid, time.Now().AddDate(0, 0, -int(days)), filter)
	if err != nil {
		return nil, err
	}
	out := &model.BotTraffic{
		BotHits:    int32(bt.BotHits),
		HumanViews: int32(bt.HumanViews),
		BotCount:   int32(bt.BotCount),
		PageCount:  int32(bt.PageCount),
		Bots:       []*model.BotAgent{},
		TopPages:   []*model.BotPage{},
		Daily:      []*model.BotDayPoint{},
	}
	for _, b := range bt.Bots {
		out.Bots = append(out.Bots, &model.BotAgent{
			Name: b.Name, Category: b.Category, Hits: int32(b.Hits), Pages: int32(b.Pages), LastSeen: b.LastSeen,
		})
	}
	for _, p := range bt.TopPages {
		out.TopPages = append(out.TopPages, &model.BotPage{PageKey: p.PageKey, Hits: int32(p.Hits), Bots: int32(p.Bots)})
	}
	for _, d := range bt.Daily {
		out.Daily = append(out.Daily, &model.BotDayPoint{Day: d.Day, BotHits: int32(d.BotHits), HumanViews: int32(d.HumanViews)})
	}
	return out, nil
}

func (r *queryResolver) LensPages(ctx context.Context, channelID string) ([]*model.LensPage, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	pages, err := r.LensSvc.ListPages(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.LensPage, 0, len(pages))
	for _, p := range pages {
		out = append(out, &model.LensPage{PageKey: p.PageKey, Views: int32(p.Views)})
	}
	return out, nil
}

func (r *queryResolver) LensStates(ctx context.Context, channelID string, pageKey string, viewport string) ([]string, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	states, err := r.LensSvc.States(ctx, cid, pageKey, viewport)
	if err != nil {
		return nil, err
	}
	return states, nil
}

func (r *queryResolver) LensPageStats(ctx context.Context, channelID string, pageKey string) (*model.LensPageStats, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	st, err := r.LensSvc.PageStats(ctx, cid, pageKey)
	if err != nil {
		return nil, err
	}
	out := &model.LensPageStats{
		Views:     int32(st.Views),
		Visitors:  int32(st.Visitors),
		Clicks:    int32(st.Clicks),
		AvgScroll: int32(st.AvgScroll),
		Timeline:  []*model.LensTimePoint{},
		Referrers: lensLabels(st.Referrers),
		Clients:   lensLabels(st.Clients),
		Devices:   lensLabels(st.Devices),
	}
	for _, p := range st.Timeline {
		out.Timeline = append(out.Timeline, &model.LensTimePoint{Date: p.Date, Views: int32(p.Views)})
	}
	return out, nil
}

func (r *queryResolver) LensHeatmap(ctx context.Context, channelID string, pageKey string, viewport string, state *string) (*model.LensHeatmap, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	st := "default"
	if state != nil && *state != "" {
		st = *state
	}
	hm, ok, err := r.LensSvc.Heatmap(ctx, cid, pageKey, viewport, st)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil // no snapshot captured for this state yet
	}
	out := &model.LensHeatmap{
		PageKey:      pageKey,
		Viewport:     viewport,
		State:        st,
		Width:        int32(hm.Width),
		Height:       int32(hm.Height),
		SnapshotJSON: hm.SnapshotJSON,
		Click:        lensCells(hm.Click),
		Move:         lensCells(hm.Move),
		Scroll:       lensCells(hm.Scroll),
		Elements:     []*model.LensElementClick{},
	}
	for _, e := range hm.Elements {
		out.Elements = append(out.Elements, &model.LensElementClick{Selector: e.Selector, Text: e.Text, Count: int32(e.Count)})
	}
	return out, nil
}

func (r *queryResolver) LensSessions(ctx context.Context, channelID string, limit *int32) ([]*model.LensSession, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "lens.view", cid, nil); err != nil {
		return nil, err
	}
	rows, err := r.LensSvc.SessionsForChannel(ctx, cid, sessionLimit(limit))
	if err != nil {
		return nil, err
	}
	return r.sessionModels(ctx, rows), nil
}

func (r *queryResolver) LensCustomerSessions(ctx context.Context, customerID string, limit *int32) ([]*model.LensSession, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(customerID)
	if err != nil {
		return nil, errInvalidID("customerId", err)
	}
	c, err := r.CustomerSvc.GetByID(ctx, cid)
	if err != nil {
		return nil, errNotFound("customer")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "lens.view", c.ChannelID, nil); err != nil {
		return nil, err
	}
	rows, err := r.LensSvc.SessionsForCustomer(ctx, cid, sessionLimit(limit))
	if err != nil {
		return nil, err
	}
	return r.sessionModels(ctx, rows), nil
}

func (r *queryResolver) LensVisitorSessions(ctx context.Context, anonID string, limit *int32) ([]*model.LensSession, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	rows, err := r.LensSvc.SessionsForAnon(ctx, anonID, sessionLimit(limit))
	if err != nil {
		return nil, err
	}
	return r.sessionModels(ctx, rows), nil
}

func (r *queryResolver) LensSessionEvents(ctx context.Context, id string) (string, error) {
	if err := r.requireSignals(); err != nil {
		return "", err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return "", err
	}
	sid, err := uuid.Parse(id)
	if err != nil {
		return "", errInvalidID("id", err)
	}
	info, err := r.LensSvc.GetSession(ctx, sid)
	if err != nil {
		return "", errNotFound("session")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "lens.view", info.ChannelID, nil); err != nil {
		return "", err
	}
	b, err := r.LensSvc.SessionEvents(ctx, sid)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (r *queryResolver) LensFrustration(ctx context.Context, channelID string, sinceDays *int32) (*model.LensFrustration, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	cid, err := lensViewScope(ctx, r, channelID)
	if err != nil {
		return nil, err
	}
	f, err := r.LensSvc.Frustration(ctx, cid, lensSince(sinceDays), 25)
	if err != nil {
		return nil, err
	}
	out := &model.LensFrustration{
		RagePages:    signalPages(f.RagePages),
		DeadPages:    signalPages(f.DeadPages),
		RageElements: []*model.LensRageElement{},
	}
	for _, e := range f.RageElements {
		out.RageElements = append(out.RageElements, &model.LensRageElement{Selector: e.Selector, Text: e.Text, Count: int32(e.Count)})
	}
	return out, nil
}

func (r *queryResolver) LensErrors(ctx context.Context, channelID string, sinceDays *int32, limit *int32) ([]*model.LensErrorStat, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	cid, err := lensViewScope(ctx, r, channelID)
	if err != nil {
		return nil, err
	}
	lim := int32(50)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	rows, err := r.LensSvc.TopErrors(ctx, cid, lensSince(sinceDays), lim)
	if err != nil {
		return nil, err
	}
	out := make([]*model.LensErrorStat, 0, len(rows))
	for _, e := range rows {
		out = append(out, &model.LensErrorStat{Message: e.Message, Count: int32(e.Count), Pages: int32(e.Pages), LastSeen: e.LastSeen})
	}
	return out, nil
}

func (r *queryResolver) LensPerformance(ctx context.Context, channelID string, sinceDays *int32) ([]*model.LensPageVitals, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	cid, err := lensViewScope(ctx, r, channelID)
	if err != nil {
		return nil, err
	}
	rows, err := r.LensSvc.Performance(ctx, cid, lensSince(sinceDays))
	if err != nil {
		return nil, err
	}
	out := make([]*model.LensPageVitals, 0, len(rows))
	for _, p := range rows {
		out = append(out, &model.LensPageVitals{
			PageKey: p.PageKey, Lcp: p.LCP, Cls: p.CLS, Inp: p.INP, Ttfb: p.TTFB, Samples: int32(p.Samples),
		})
	}
	return out, nil
}

func (r *queryResolver) LensVitalsSummary(ctx context.Context, channelID string, sinceDays *int32) ([]*model.LensVital, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	cid, err := lensViewScope(ctx, r, channelID)
	if err != nil {
		return nil, err
	}
	rows, err := r.LensSvc.VitalsOverall(ctx, cid, lensSince(sinceDays))
	if err != nil {
		return nil, err
	}
	out := make([]*model.LensVital, 0, len(rows))
	for _, v := range rows {
		out = append(out, &model.LensVital{Metric: v.Metric, P75: v.P75, Samples: int32(v.Samples)})
	}
	return out, nil
}

func (r *queryResolver) LensSearchAnalytics(ctx context.Context, channelID string, sinceDays *int32) (*model.LensSearchAnalytics, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	cid, err := lensViewScope(ctx, r, channelID)
	if err != nil {
		return nil, err
	}
	a, err := r.LensSvc.SearchAnalytics(ctx, cid, lensSince(sinceDays))
	if err != nil {
		return nil, err
	}
	out := &model.LensSearchAnalytics{
		TotalSearches:      int32(a.TotalSearches),
		DistinctTerms:      int32(a.DistinctTerms),
		ZeroResultSearches: int32(a.ZeroResultSearches),
		Top:                make([]*model.LensSearchTerm, 0, len(a.Top)),
		ZeroResult:         make([]*model.LensSearchTerm, 0, len(a.ZeroResult)),
		Daily:              make([]*model.LensSearchDay, 0, len(a.Daily)),
	}
	for _, t := range a.Top {
		out.Top = append(out.Top, &model.LensSearchTerm{Term: t.Term, Count: int32(t.Count)})
	}
	for _, t := range a.ZeroResult {
		out.ZeroResult = append(out.ZeroResult, &model.LensSearchTerm{Term: t.Term, Count: int32(t.Count)})
	}
	for _, d := range a.Daily {
		out.Daily = append(out.Daily, &model.LensSearchDay{Date: d.Date, Searches: int32(d.Searches), ZeroResults: int32(d.ZeroResults)})
	}
	return out, nil
}

func (r *queryResolver) LensSessionSignals(ctx context.Context, id string) (*model.LensSessionSignals, error) {
	if err := r.requireSignals(); err != nil {
		return nil, err
	}
	if err := auth.RequirePermission(ctx, r.AdminSvc, "lens.view"); err != nil {
		return nil, err
	}
	sid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	info, err := r.LensSvc.GetSession(ctx, sid)
	if err != nil {
		return nil, errNotFound("session")
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "lens.view", info.ChannelID, nil); err != nil {
		return nil, err
	}
	sig, err := r.LensSvc.SessionSignals(ctx, info.ChannelID, id, info.StartedAt, info.EndedAt)
	if err != nil {
		return nil, err
	}
	out := &model.LensSessionSignals{Errors: []*model.LensSessionError{}, Vitals: []*model.LensSessionVital{}}
	for _, e := range sig.Errors {
		out.Errors = append(out.Errors, &model.LensSessionError{Message: e.Message, At: e.At})
	}
	for _, v := range sig.Vitals {
		out.Vitals = append(out.Vitals, &model.LensSessionVital{Metric: v.Metric, Value: v.Value})
	}
	return out, nil
}

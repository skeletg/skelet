package admin

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func (r *queryResolver) LensOverview(ctx context.Context, channelID string, sinceDays *int32) (*model.LensOverview, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	ov, err := r.SignalsAPI.Lens.GetOverview(authed, &signalsv1.GetOverviewRequest{ChannelId: channelID, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.LensOverview{
		Views: ov.GetViews(), Visitors: ov.GetVisitors(),
		Daily:     lensTimePoints(ov.GetDaily()),
		TopPages:  []*model.LensPageStat{},
		Devices:   lensLabels(ov.GetDevices()),
		Referrers: lensLabels(ov.GetReferrers()),
		// Named UI interactions, which the hub shows beside devices and referrers
		Interactions: lensLabels(ov.GetInteractions()),
	}
	for _, p := range ov.GetTopPages() {
		out.TopPages = append(out.TopPages, &model.LensPageStat{
			PageKey: p.GetPageKey(), Views: p.GetViews(), Visitors: p.GetVisitors(),
			Clicks: p.GetClicks(), AvgScroll: p.GetAvgScroll(),
		})
	}
	return out, nil
}

func (r *queryResolver) BotTraffic(ctx context.Context, channelID string, sinceDays *int32, bot *string) (*model.BotTraffic, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	filter := ""
	if bot != nil {
		filter = *bot
	}
	bt, err := r.SignalsAPI.Lens.GetBotTraffic(authed, &signalsv1.GetBotTrafficRequest{
		ChannelId: channelID, SinceDays: signalsDays(sinceDays), Bot: filter,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.BotTraffic{
		BotHits: bt.GetBotHits(), HumanViews: bt.GetHumanViews(),
		BotCount: bt.GetBotCount(), PageCount: bt.GetPageCount(),
		Bots: []*model.BotAgent{}, TopPages: []*model.BotPage{}, Daily: []*model.BotDayPoint{},
	}
	for _, b := range bt.GetBots() {
		out.Bots = append(out.Bots, &model.BotAgent{
			Name: b.GetName(), Category: b.GetCategory(), Hits: b.GetHits(), Pages: b.GetPages(), LastSeen: b.GetLastSeen().AsTime(),
		})
	}
	for _, p := range bt.GetTopPages() {
		out.TopPages = append(out.TopPages, &model.BotPage{PageKey: p.GetPageKey(), Hits: p.GetHits(), Bots: p.GetBots()})
	}
	for _, d := range bt.GetDaily() {
		out.Daily = append(out.Daily, &model.BotDayPoint{Day: d.GetDay(), BotHits: d.GetBotHits(), HumanViews: d.GetHumanViews()})
	}
	return out, nil
}

func (r *queryResolver) LensPages(ctx context.Context, channelID string) ([]*model.LensPage, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Lens.ListPages(authed, &signalsv1.ListPagesRequest{ChannelId: channelID, PageSize: 500})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.LensPage, 0, len(res.GetPages()))
	for _, p := range res.GetPages() {
		out = append(out, &model.LensPage{PageKey: p.GetPageKey(), Views: p.GetViews()})
	}
	return out, nil
}

func (r *queryResolver) LensStates(ctx context.Context, channelID string, pageKey string, viewport string) ([]string, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Lens.ListStates(authed, &signalsv1.ListStatesRequest{ChannelId: channelID, PageKey: pageKey, Viewport: viewport})
	if err != nil {
		return nil, signalsErr(err)
	}
	return res.GetStates(), nil
}

func (r *queryResolver) LensPageStats(ctx context.Context, channelID string, pageKey string) (*model.LensPageStats, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	st, err := r.SignalsAPI.Lens.GetPageStats(authed, &signalsv1.GetPageStatsRequest{ChannelId: channelID, PageKey: pageKey})
	if err != nil {
		return nil, signalsErr(err)
	}
	return &model.LensPageStats{
		Views: st.GetViews(), Visitors: st.GetVisitors(), Clicks: st.GetClicks(), AvgScroll: st.GetAvgScroll(),
		Timeline:  lensTimePoints(st.GetTimeline()),
		Referrers: lensLabels(st.GetReferrers()),
		Clients:   lensLabels(st.GetClients()),
		Devices:   lensLabels(st.GetDevices()),
	}, nil
}

func (r *queryResolver) LensHeatmap(ctx context.Context, channelID string, pageKey string, viewport string, state *string) (*model.LensHeatmap, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	st := "default"
	if state != nil && *state != "" {
		st = *state
	}
	res, err := r.SignalsAPI.Lens.GetHeatmap(authed, &signalsv1.GetHeatmapRequest{
		ChannelId: channelID, PageKey: pageKey, Viewport: viewport, State: st,
	})
	if err != nil {
		if signalsNotFound(err) {
			return nil, nil // no snapshot captured for this state yet
		}
		return nil, signalsErr(err)
	}
	hm := res.GetHeatmap()
	out := &model.LensHeatmap{
		PageKey: pageKey, Viewport: viewport, State: st,
		Width: hm.GetWidth(), Height: hm.GetHeight(), SnapshotJSON: hm.GetSnapshotJson(),
		Click: lensCells(hm.GetClick()), Move: lensCells(hm.GetMove()), Scroll: lensCells(hm.GetScroll()),
		Elements: []*model.LensElementClick{},
	}
	for _, e := range hm.GetElements() {
		out.Elements = append(out.Elements, &model.LensElementClick{Selector: e.GetSelector(), Text: e.GetText(), Count: e.GetCount()})
	}
	return out, nil
}

func (r *queryResolver) LensSessions(ctx context.Context, channelID string, limit *int32) ([]*model.LensSession, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Lens.ListSessions(authed, &signalsv1.ListSessionsRequest{ChannelId: channelID, PageSize: lensSessionLimit(limit)})
	if err != nil {
		return nil, signalsErr(err)
	}
	return lensSessionModels(res.GetSessions()), nil
}

// The suite resolves the customer's channel off its roster and gates on that, so core passes the id through
func (r *queryResolver) LensCustomerSessions(ctx context.Context, customerID string, limit *int32) ([]*model.LensSession, error) {
	authed, err := r.signalsScoped(ctx, "lens.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(customerID); err != nil {
		return nil, errInvalidID("customerId", err)
	}
	res, err := r.SignalsAPI.Lens.ListCustomerSessions(authed, &signalsv1.ListCustomerSessionsRequest{
		CustomerId: customerID, PageSize: lensSessionLimit(limit),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return lensSessionModels(res.GetSessions()), nil
}

func (r *queryResolver) LensVisitorSessions(ctx context.Context, anonID string, limit *int32) ([]*model.LensSession, error) {
	authed, err := r.signalsScoped(ctx, "lens.view")
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Lens.ListVisitorSessions(authed, &signalsv1.ListVisitorSessionsRequest{
		AnonId: anonID, PageSize: lensSessionLimit(limit),
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	return lensSessionModels(res.GetSessions()), nil
}

// The one streamed read - the replay payload runs to megabytes, so it arrives in chunks and is rejoined here
func (r *queryResolver) LensSessionEvents(ctx context.Context, id string) (string, error) {
	authed, err := r.signalsScoped(ctx, "lens.view")
	if err != nil {
		return "", err
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", errInvalidID("id", err)
	}
	stream, err := r.SignalsAPI.Lens.GetSessionEvents(authed, &signalsv1.GetSessionEventsRequest{Id: id})
	if err != nil {
		return "", signalsErr(err)
	}
	var buf []byte
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", signalsErr(err)
		}
		buf = append(buf, chunk.GetChunk()...)
	}
	return string(buf), nil
}

func (r *queryResolver) LensFrustration(ctx context.Context, channelID string, sinceDays *int32) (*model.LensFrustration, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	f, err := r.SignalsAPI.Lens.GetFrustration(authed, &signalsv1.GetFrustrationRequest{ChannelId: channelID, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.LensFrustration{
		RagePages: lensSignalPages(f.GetRagePages()), DeadPages: lensSignalPages(f.GetDeadPages()),
		RageElements: []*model.LensRageElement{},
	}
	for _, e := range f.GetRageElements() {
		out.RageElements = append(out.RageElements, &model.LensRageElement{Selector: e.GetSelector(), Text: e.GetText(), Count: e.GetCount()})
	}
	return out, nil
}

func (r *queryResolver) LensErrors(ctx context.Context, channelID string, sinceDays *int32, limit *int32) ([]*model.LensErrorStat, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	lim := int32(50)
	if limit != nil && *limit > 0 {
		lim = *limit
	}
	res, err := r.SignalsAPI.Lens.ListErrors(authed, &signalsv1.ListErrorsRequest{
		ChannelId: channelID, SinceDays: signalsDays(sinceDays), Limit: lim,
	})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.LensErrorStat, 0, len(res.GetErrors()))
	for _, e := range res.GetErrors() {
		out = append(out, &model.LensErrorStat{
			Message: e.GetMessage(), Count: e.GetCount(), Pages: e.GetPages(), LastSeen: e.GetLastSeen().AsTime(),
		})
	}
	return out, nil
}

func (r *queryResolver) LensPerformance(ctx context.Context, channelID string, sinceDays *int32) ([]*model.LensPageVitals, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Lens.GetPerformance(authed, &signalsv1.GetPerformanceRequest{ChannelId: channelID, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.LensPageVitals, 0, len(res.GetPages()))
	for _, p := range res.GetPages() {
		out = append(out, &model.LensPageVitals{
			PageKey: p.GetPageKey(), Lcp: p.Lcp, Cls: p.Cls, Inp: p.Inp, Ttfb: p.Ttfb, Samples: p.GetSamples(),
		})
	}
	return out, nil
}

func (r *queryResolver) LensVitalsSummary(ctx context.Context, channelID string, sinceDays *int32) ([]*model.LensVital, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Lens.GetVitalsSummary(authed, &signalsv1.GetVitalsSummaryRequest{ChannelId: channelID, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.LensVital, 0, len(res.GetVitals()))
	for _, v := range res.GetVitals() {
		out = append(out, &model.LensVital{Metric: v.GetMetric(), P75: v.GetP75(), Samples: v.GetSamples()})
	}
	return out, nil
}

func (r *queryResolver) LensSearchAnalytics(ctx context.Context, channelID string, sinceDays *int32) (*model.LensSearchAnalytics, error) {
	authed, err := r.signalsChannel(ctx, "lens.view", channelID)
	if err != nil {
		return nil, err
	}
	a, err := r.SignalsAPI.Lens.GetSearchAnalytics(authed, &signalsv1.GetSearchAnalyticsRequest{ChannelId: channelID, SinceDays: signalsDays(sinceDays)})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.LensSearchAnalytics{
		TotalSearches: a.GetTotalSearches(), DistinctTerms: a.GetDistinctTerms(), ZeroResultSearches: a.GetZeroResultSearches(),
		Top:        make([]*model.LensSearchTerm, 0, len(a.GetTop())),
		ZeroResult: make([]*model.LensSearchTerm, 0, len(a.GetZeroResult())),
		Daily:      make([]*model.LensSearchDay, 0, len(a.GetDaily())),
	}
	for _, t := range a.GetTop() {
		out.Top = append(out.Top, &model.LensSearchTerm{Term: t.GetTerm(), Count: t.GetCount()})
	}
	for _, t := range a.GetZeroResult() {
		out.ZeroResult = append(out.ZeroResult, &model.LensSearchTerm{Term: t.GetTerm(), Count: t.GetCount()})
	}
	for _, d := range a.GetDaily() {
		out.Daily = append(out.Daily, &model.LensSearchDay{Date: d.GetDate(), Searches: d.GetSearches(), ZeroResults: d.GetZeroResults()})
	}
	return out, nil
}

func (r *queryResolver) LensSessionSignals(ctx context.Context, id string) (*model.LensSessionSignals, error) {
	authed, err := r.signalsScoped(ctx, "lens.view")
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, errInvalidID("id", err)
	}
	sig, err := r.SignalsAPI.Lens.GetSessionSignals(authed, &signalsv1.GetSessionSignalsRequest{Id: id})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := &model.LensSessionSignals{Errors: []*model.LensSessionError{}, Vitals: []*model.LensSessionVital{}}
	for _, e := range sig.GetErrors() {
		out.Errors = append(out.Errors, &model.LensSessionError{Message: e.GetMessage(), At: e.GetAt().AsTime()})
	}
	for _, v := range sig.GetVitals() {
		out.Vitals = append(out.Vitals, &model.LensSessionVital{Metric: v.GetMetric(), Value: v.GetValue()})
	}
	return out, nil
}

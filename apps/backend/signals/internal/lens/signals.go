package lens

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	signalsdb "github.com/skeletg/skelet/apps/backend/signals/internal/db/sqlc"
)

// --- UX signals: frustration / errors / web vitals (read aggregates) ---

type SignalPage struct {
	PageKey string
	Count   int64
}
type RageElement struct {
	Selector, Text string
	Count          int64
}
type Frustration struct {
	RagePages    []SignalPage
	DeadPages    []SignalPage
	RageElements []RageElement
}
type ErrorStat struct {
	Message      string
	Count, Pages int64
	LastSeen     time.Time
}
type PageVitals struct {
	PageKey             string
	LCP, CLS, INP, TTFB *float64 // p75; nil when the page has no sample for that metric
	Samples             int64
}

// Top pages by rage and dead clicks from the rollup, plus the most rage-clicked elements from the raw spine
func (s *Service) Frustration(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) (Frustration, error) {
	var f Frustration
	for _, d := range []struct {
		dim string
		dst *[]SignalPage
	}{{"rage", &f.RagePages}, {"dead", &f.DeadPages}} {
		rows, err := s.read().LensTopSignalPages(ctx, signalsdb.LensTopSignalPagesParams{ChannelID: channelID, Since: since, Dim: d.dim, Lim: limit})
		if err != nil {
			return f, err
		}
		for _, r := range rows {
			*d.dst = append(*d.dst, SignalPage{PageKey: r.PageKey, Count: r.Count})
		}
	}
	els, err := s.rec.LensTopRageElements(ctx, channelID, since, limit)
	if err != nil {
		return f, err
	}
	for _, r := range els {
		f.RageElements = append(f.RageElements, RageElement{Selector: r.Selector, Text: r.Text, Count: r.Count})
	}
	return f, nil
}

func (s *Service) TopErrors(ctx context.Context, channelID uuid.UUID, since time.Time, limit int32) ([]ErrorStat, error) {
	rows, err := s.rec.LensTopErrors(ctx, channelID, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ErrorStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, ErrorStat{Message: r.Message, Count: r.Count, Pages: r.Pages, LastSeen: r.LastSeen})
	}
	return out, nil
}

// Pivots the per-(page, metric) p75 rows into one row per page with LCP/CLS/INP/TTFB columns
func (s *Service) Performance(ctx context.Context, channelID uuid.UUID, since time.Time) ([]PageVitals, error) {
	rows, err := s.rec.LensVitalsByPage(ctx, channelID, since)
	if err != nil {
		return nil, err
	}
	byPage := map[string]*PageVitals{}
	order := make([]string, 0)
	for _, r := range rows {
		pk := r.PageKey
		pv := byPage[pk]
		if pv == nil {
			pv = &PageVitals{PageKey: pk}
			byPage[pk] = pv
			order = append(order, pk)
		}
		v := r.P75
		switch asString(r.Metric) {
		case "LCP":
			pv.LCP = &v
		case "CLS":
			pv.CLS = &v
		case "INP":
			pv.INP = &v
		case "TTFB":
			pv.TTFB = &v
		}
		pv.Samples += r.Samples
	}
	out := make([]PageVitals, 0, len(order))
	for _, pk := range order {
		out = append(out, *byPage[pk])
	}
	return out, nil
}

// A site-wide p75 for one Core Web Vital, powering the Lighthouse-style gauges on the Lens home
type SiteVital struct {
	Metric  string
	P75     float64
	Samples int64
}

func (s *Service) VitalsOverall(ctx context.Context, channelID uuid.UUID, since time.Time) ([]SiteVital, error) {
	rows, err := s.rec.LensVitalsOverall(ctx, channelID, since)
	if err != nil {
		return nil, err
	}
	out := make([]SiteVital, 0, len(rows))
	for _, r := range rows {
		out = append(out, SiteVital{Metric: r.Metric, P75: r.P75, Samples: r.Samples})
	}
	return out, nil
}

type TermCount struct {
	Term  string
	Count int64
}

type SearchDay struct {
	Date        string
	Searches    int64
	ZeroResults int64
}

// The on-site-search report - headline totals, top terms, zero-result terms as merchandising gaps, and a daily trend
type SearchAnalytics struct {
	TotalSearches      int64
	DistinctTerms      int64
	ZeroResultSearches int64
	Top                []TermCount
	ZeroResult         []TermCount
	Daily              []SearchDay
}

// All rollup-backed, from dim='search' and 'search_zero'
func (s *Service) SearchAnalytics(ctx context.Context, channelID uuid.UUID, since time.Time) (SearchAnalytics, error) {
	var out SearchAnalytics
	totals, err := s.read().LensSearchTotals(ctx, signalsdb.LensSearchTotalsParams{ChannelID: channelID, Since: since})
	if err != nil {
		return out, err
	}
	out.TotalSearches = totals.TotalSearches
	out.DistinctTerms = totals.DistinctTerms
	out.ZeroResultSearches = totals.ZeroResultSearches
	top, err := s.read().LensTopSearches(ctx, signalsdb.LensTopSearchesParams{ChannelID: channelID, Since: since, Lim: 50})
	if err != nil {
		return out, err
	}
	for _, r := range top {
		out.Top = append(out.Top, TermCount{Term: r.Term, Count: r.Count})
	}
	zero, err := s.read().LensZeroResultSearches(ctx, signalsdb.LensZeroResultSearchesParams{ChannelID: channelID, Since: since, Lim: 50})
	if err != nil {
		return out, err
	}
	for _, r := range zero {
		out.ZeroResult = append(out.ZeroResult, TermCount{Term: r.Term, Count: r.Count})
	}
	daily, err := s.read().LensSearchDaily(ctx, signalsdb.LensSearchDailyParams{ChannelID: channelID, Since: since})
	if err != nil {
		return out, err
	}
	for _, r := range daily {
		out.Daily = append(out.Daily, SearchDay{Date: r.Day, Searches: r.Searches, ZeroResults: r.ZeroResults})
	}
	return out, nil
}

// SessionError / SessionVital / SessionSignals are this recording's JS errors and web vitals, for the replay page
type SessionError struct {
	Message string
	At      time.Time
}
type SessionVital struct {
	Metric string
	Value  float64
}
type SessionSignals struct {
	Errors []SessionError
	Vitals []SessionVital
}

// Matched by sessionId and bounded to the session's channel and time window - [from, to] should be the session's started_at and last_event_at
func (s *Service) SessionSignals(ctx context.Context, channelID uuid.UUID, sessionID string, from, to time.Time) (SessionSignals, error) {
	rows, err := s.rec.LensSessionSignals(ctx, channelID, sessionID, from.Add(-2*time.Second), to.Add(2*time.Second))
	if err != nil {
		return SessionSignals{}, err
	}
	var sig SessionSignals
	vital := map[string]float64{} // last value wins per metric
	order := make([]string, 0, 4)
	for _, r := range rows {
		switch r.Name {
		case sigJSError:
			sig.Errors = append(sig.Errors, SessionError{Message: r.Message, At: r.OccurredAt})
		case sigWebVitals:
			m := asString(r.Metric)
			if m == "" {
				continue
			}
			if _, ok := vital[m]; !ok {
				order = append(order, m)
			}
			vital[m], _ = strconv.ParseFloat(asString(r.Value), 64)
		}
	}
	for _, m := range order {
		sig.Vitals = append(sig.Vitals, SessionVital{Metric: m, Value: vital[m]})
	}
	return sig, nil
}

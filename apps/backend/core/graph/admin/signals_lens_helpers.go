package admin

// Converters between the Lens contract and the admin models

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func lensLabels(items []*signalsv1.LabelCount) []*model.LensLabelCount {
	out := make([]*model.LensLabelCount, 0, len(items))
	for _, i := range items {
		out = append(out, &model.LensLabelCount{Label: i.GetLabel(), Views: i.GetViews()})
	}
	return out
}

func lensCells(cells []*signalsv1.LensCell) []*model.LensCell {
	out := make([]*model.LensCell, 0, len(cells))
	for _, c := range cells {
		out = append(out, &model.LensCell{X: c.GetX(), Y: c.GetY(), Count: c.GetCount()})
	}
	return out
}

func lensSignalPages(pages []*signalsv1.LensSignalPage) []*model.LensSignalPage {
	out := make([]*model.LensSignalPage, 0, len(pages))
	for _, p := range pages {
		out = append(out, &model.LensSignalPage{PageKey: p.GetPageKey(), Count: p.GetCount()})
	}
	return out
}

func lensTimePoints(pts []*signalsv1.LensTimePoint) []*model.LensTimePoint {
	out := make([]*model.LensTimePoint, 0, len(pts))
	for _, p := range pts {
		out = append(out, &model.LensTimePoint{Date: p.GetDate(), Views: p.GetViews()})
	}
	return out
}

// The suite knows a customer by id alone - the dashboard labels them from core
func lensSessionModels(rows []*signalsv1.LensSession) []*model.LensSession {
	out := make([]*model.LensSession, 0, len(rows))
	for _, s := range rows {
		m := &model.LensSession{
			ID: s.GetId(), AnonID: s.GetAnonId(),
			StartedAt: s.GetStartedAt().AsTime(), EndedAt: s.GetEndedAt().AsTime(),
			DurationMs: s.GetDurationMs(), EventCount: s.GetEventCount(),
			EntryPage: s.GetEntryPage(), ExitPage: s.GetExitPage(),
			Viewport: s.GetViewport(), UserAgent: s.GetUserAgent(),
			RageCount: s.GetRageCount(), ErrorCount: s.GetErrorCount(),
		}
		if s.CustomerId != nil {
			id := s.GetCustomerId()
			m.CustomerID = &id
		}
		out = append(out, m)
	}
	return out
}

func lensSessionLimit(limit *int32) int32 {
	n := int32(50)
	if limit != nil && *limit > 0 {
		n = *limit
	}
	if n > 200 {
		n = 200
	}
	return n
}

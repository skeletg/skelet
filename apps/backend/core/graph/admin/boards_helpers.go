package admin

import (
	"strings"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A definition's unset fields cross as zero values, and come back to the dashboard as null
func boardReportsIn(in []*model.BoardReportInput) []*signalsv1.BoardReport {
	out := make([]*signalsv1.BoardReport, 0, len(in))
	for _, r := range in {
		d := r.Definition
		filters := make([]*signalsv1.FunnelFilter, 0, len(d.Filters))
		for _, f := range d.Filters {
			filters = append(filters, &signalsv1.FunnelFilter{Step: f.Step, Key: f.Key, Value: f.Value})
		}
		out = append(out, &signalsv1.BoardReport{Title: r.Title, Size: strings.ToLower(string(ptrOr(r.Size, ""))), Definition: &signalsv1.BoardReportDefinition{
			Kind: strings.ToLower(string(d.Kind)), SinceDays: ptrOr(d.SinceDays, 0), Event: ptrOr(d.Event, ""),
			Measure: ptrOr(d.Measure, ""), Unit: ptrOr(d.Unit, ""), Breakdown: ptrOr(d.Breakdown, ""), Steps: d.Steps,
			Filters: filters, WindowHours: ptrOr(d.WindowHours, 0), StartEvent: ptrOr(d.StartEvent, ""),
			ReturnEvent: ptrOr(d.ReturnEvent, ""), Periods: ptrOr(d.Periods, 0), Depth: ptrOr(d.Depth, 0),
			WindowDays: ptrOr(d.WindowDays, 0),
		}})
	}
	return out
}

func boardSummariesOut(in []*signalsv1.BoardSummary) []*model.BoardSummary {
	out := make([]*model.BoardSummary, 0, len(in))
	for _, b := range in {
		out = append(out, &model.BoardSummary{ID: b.GetId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(), ReportCount: b.GetReportCount()})
	}
	return out
}

func boardOut(b *signalsv1.Board) *model.Board {
	out := &model.Board{
		ID: b.GetId(), ChannelID: b.GetChannelId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(), Reports: []*model.BoardReport{},
	}
	for _, r := range b.GetReports() {
		d := r.GetDefinition()
		var filters []*model.BoardReportFilter
		for _, f := range d.GetFilters() {
			filters = append(filters, &model.BoardReportFilter{Step: f.GetStep(), Key: f.GetKey(), Value: f.GetValue()})
		}
		out.Reports = append(out.Reports, &model.BoardReport{Title: r.GetTitle(), Size: nilIfZero(model.BoardReportSize(strings.ToUpper(r.GetSize()))), Definition: &model.BoardReportDefinition{
			Kind: model.BoardReportKind(strings.ToUpper(d.GetKind())), SinceDays: nilIfZero(d.GetSinceDays()), Event: nilIfZero(d.GetEvent()),
			Measure: nilIfZero(d.GetMeasure()), Unit: nilIfZero(d.GetUnit()), Breakdown: nilIfZero(d.GetBreakdown()), Steps: d.GetSteps(),
			Filters: filters, WindowHours: nilIfZero(d.GetWindowHours()), StartEvent: nilIfZero(d.GetStartEvent()),
			ReturnEvent: nilIfZero(d.GetReturnEvent()), Periods: nilIfZero(d.GetPeriods()), Depth: nilIfZero(d.GetDepth()),
			WindowDays: nilIfZero(d.GetWindowDays()),
		}})
	}
	return out
}

func nilIfZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}

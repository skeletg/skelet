package admin

import (
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A report definition crosses as JSON text - Signals reads it strictly and answers what it refuses
func boardReportsIn(in []*model.BoardReportInput) ([]*signalsv1.BoardReport, error) {
	out := make([]*signalsv1.BoardReport, 0, len(in))
	for _, r := range in {
		raw, err := json.Marshal(r.Definition)
		if err != nil {
			return nil, fmt.Errorf("report %q: definition: %w", r.Title, err)
		}
		out = append(out, &signalsv1.BoardReport{Title: r.Title, DefinitionJson: string(raw)})
	}
	return out, nil
}

func boardsOut(in []*signalsv1.Board) ([]*model.Board, error) {
	out := make([]*model.Board, 0, len(in))
	for _, b := range in {
		m, err := boardOut(b)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func boardOut(b *signalsv1.Board) (*model.Board, error) {
	out := &model.Board{
		ID: b.GetId(), ChannelID: b.GetChannelId(), Name: b.GetName(), UpdatedAt: b.GetUpdatedAt().AsTime(),
		ReportCount: b.GetReportCount(), Reports: []*model.BoardReport{},
	}
	for _, r := range b.GetReports() {
		var def any
		if err := json.Unmarshal([]byte(r.GetDefinitionJson()), &def); err != nil {
			return nil, fmt.Errorf("report %q: definition: %w", r.GetTitle(), err)
		}
		out.Reports = append(out.Reports, &model.BoardReport{Title: r.GetTitle(), Definition: def})
	}
	return out, nil
}

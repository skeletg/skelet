package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// The flow composer's document travels as JSON text - the dashboard authors it, the suite validates it

func flowInput(in model.SaveFlowInput) (*signalsv1.FlowInput, error) {
	graph, err := jsonText(in.Graph)
	if err != nil {
		return nil, errInvalidInput("flow graph", err)
	}
	tc, err := jsonText(in.TriggerConfig)
	if err != nil {
		return nil, errInvalidInput("triggerConfig", err)
	}
	return &signalsv1.FlowInput{ChannelId: in.ChannelID, Name: in.Name, TriggerType: in.TriggerType, TriggerConfigJson: tc, GraphJson: graph}, nil
}

func flowModel(f *signalsv1.Flow) *model.Flow {
	return &model.Flow{
		ID:            f.GetId(),
		ChannelID:     f.GetChannelId(),
		Name:          f.GetName(),
		TriggerType:   f.GetTriggerType(),
		TriggerConfig: jsonValue(f.GetTriggerConfigJson(), "{}"),
		Graph:         jsonValue(f.GetGraphJson(), `{"nodes":[],"edges":[]}`),
		Status:        f.GetStatus(),
		DeletedAt:     protoTimePtr(f.DeletedAt),
		CreatedAt:     protoTime(f.GetCreatedAt()),
		UpdatedAt:     protoTime(f.GetUpdatedAt()),
	}
}

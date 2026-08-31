package admin

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/flow"
)

func flowInput(in model.SaveFlowInput) (flow.Input, error) {
	cid, err := uuid.Parse(in.ChannelID)
	if err != nil {
		return flow.Input{}, errInvalidID("channelId", err)
	}
	g, err := graphFromJSON(in.Graph)
	if err != nil {
		return flow.Input{}, err
	}
	tc, err := rawFromJSON(in.TriggerConfig)
	if err != nil {
		return flow.Input{}, err
	}
	return flow.Input{ChannelID: cid, Name: in.Name, TriggerType: in.TriggerType, TriggerConfig: tc, Graph: g}, nil
}

// graphFromJSON decodes the @xyflow JSON scalar into the typed graph.
func graphFromJSON(v any) (flow.Graph, error) {
	var g flow.Graph
	if v == nil {
		return g, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return g, err
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return g, errInvalidInput("flow graph", err)
	}
	return g, nil
}

func rawFromJSON(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func toFlowModel(f flow.Flow) *model.Flow {
	if f.Graph.Nodes == nil {
		f.Graph.Nodes = []flow.Node{}
	}
	if f.Graph.Edges == nil {
		f.Graph.Edges = []flow.Edge{}
	}
	var tc any = json.RawMessage("{}")
	if len(f.TriggerConfig) > 0 {
		tc = f.TriggerConfig
	}
	return &model.Flow{
		ID:            f.ID.String(),
		ChannelID:     f.ChannelID.String(),
		Name:          f.Name,
		TriggerType:   f.TriggerType,
		TriggerConfig: tc,
		Graph:         f.Graph,
		Status:        f.Status,
		DeletedAt:     f.DeletedAt,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     f.UpdatedAt,
	}
}

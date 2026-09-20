package signalsgrpc

// The settings half of the contract. Core administers these - the admin gate runs there, before any
// of this is reached - so they authorize on core's system token like the rest of its own work

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

type settingsServer struct {
	signalsv1.UnimplementedSettingsServiceServer
	repo config.Repository
}

// Only what this side declares - core's own registry is core's, and merging them is its job
func (s *settingsServer) ListDefinitions(ctx context.Context, _ *signalsv1.ListDefinitionsRequest) (*signalsv1.ListDefinitionsResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	out := []*signalsv1.SettingDefinition{}
	for _, d := range config.AllDefinitions() {
		if d.Store != config.StoreSignals {
			continue
		}
		def := &signalsv1.SettingDefinition{
			Path: d.Path, Type: string(d.Type), Default: d.Default, Label: d.Label,
			Description: d.Description, Public: d.Public, Scoping: string(d.Scoping),
		}
		for _, o := range d.Options {
			def.Options = append(def.Options, &signalsv1.SettingOption{Value: o.Value, Label: o.Label})
		}
		out = append(out, def)
	}
	return &signalsv1.ListDefinitionsResponse{Definitions: out}, nil
}

func (s *settingsServer) GetCandidates(ctx context.Context, req *signalsv1.GetCandidatesRequest) (*signalsv1.GetCandidatesResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	scope, err := settingScope(req.GetScope())
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.GetCandidates(ctx, scope, req.GetPath())
	if err != nil {
		return nil, err
	}
	return &signalsv1.GetCandidatesResponse{Values: settingValues(rows)}, nil
}

func (s *settingsServer) ListInScopes(ctx context.Context, req *signalsv1.ListInScopesRequest) (*signalsv1.ListInScopesResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	scope, err := settingScope(req.GetScope())
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListInScopes(ctx, scope)
	if err != nil {
		return nil, err
	}
	return &signalsv1.ListInScopesResponse{Values: settingValues(rows)}, nil
}

func (s *settingsServer) ListAll(ctx context.Context, _ *signalsv1.ListAllRequest) (*signalsv1.ListAllResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return &signalsv1.ListAllResponse{Values: settingValues(rows)}, nil
}

func (s *settingsServer) UpsertValue(ctx context.Context, req *signalsv1.UpsertValueRequest) (*signalsv1.UpsertValueResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	scope, err := settingScope(req.GetScope())
	if err != nil {
		return nil, err
	}
	row, err := s.repo.Upsert(ctx, scope, req.GetPath(), req.GetValue())
	if err != nil {
		return nil, err
	}
	return &signalsv1.UpsertValueResponse{Value: settingValue(row)}, nil
}

func (s *settingsServer) DeleteValue(ctx context.Context, req *signalsv1.DeleteValueRequest) (*signalsv1.DeleteValueResponse, error) {
	if err := requireSystem(ctx); err != nil {
		return nil, err
	}
	scope, err := settingScope(req.GetScope())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(ctx, scope, req.GetPath()); err != nil {
		return nil, err
	}
	return &signalsv1.DeleteValueResponse{}, nil
}

func settingScope(s *signalsv1.SettingScope) (config.Scope, error) {
	out := config.Scope{Kind: s.GetKind()}
	if raw := s.GetChannelId(); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return config.Scope{}, invalidArgument("channel_id is not a uuid")
		}
		out.ChannelID = &id
	}
	return out, nil
}

func settingValue(v config.StoredValue) *signalsv1.SettingValue {
	out := &signalsv1.SettingValue{
		Scope: &signalsv1.SettingScope{Kind: v.Scope.Kind},
		Path:  v.Path, Value: v.Value, UpdatedAt: timestamppb.New(v.UpdatedAt),
	}
	if v.Scope.ChannelID != nil {
		id := v.Scope.ChannelID.String()
		out.Scope.ChannelId = &id
	}
	return out
}

func settingValues(rows []config.StoredValue) []*signalsv1.SettingValue {
	out := make([]*signalsv1.SettingValue, 0, len(rows))
	for _, r := range rows {
		out = append(out, settingValue(r))
	}
	return out
}

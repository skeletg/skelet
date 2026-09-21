package signalsclient

import (
	"context"

	"github.com/google/uuid"

	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// SettingScope is where a value sits - default, or one channel
type SettingScope struct {
	Kind      string
	ChannelID *uuid.UUID
}

// SettingValue is one stored row, as the suite holds it
type SettingValue struct {
	Scope SettingScope
	Path  string
	Value string
	// Zero when the suite did not carry one, which only the orphan sweep would notice
	UpdatedAt int64
}

// SettingDefinition is what the suite declares about a setting, for core's settings page to render
type SettingDefinition struct {
	Path        string
	Type        string
	Default     string
	Label       string
	Description string
	Public      bool
	Options     []SettingOption
	Scoping     string
}

type SettingOption struct{ Value, Label string }

func (c *Client) SettingDefinitions(ctx context.Context) ([]SettingDefinition, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Settings.ListDefinitions(sys, &signalsv1.ListDefinitionsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]SettingDefinition, 0, len(res.GetDefinitions()))
	for _, d := range res.GetDefinitions() {
		def := SettingDefinition{
			Path: d.GetPath(), Type: d.GetType(), Default: d.GetDefault(), Label: d.GetLabel(),
			Description: d.GetDescription(), Public: d.GetPublic(), Scoping: d.GetScoping(),
		}
		for _, o := range d.GetOptions() {
			def.Options = append(def.Options, SettingOption{Value: o.GetValue(), Label: o.GetLabel()})
		}
		out = append(out, def)
	}
	return out, nil
}

func (c *Client) SettingCandidates(ctx context.Context, scope SettingScope, path string) ([]SettingValue, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Settings.GetCandidates(sys, &signalsv1.GetCandidatesRequest{Path: path, Scope: wireScope(scope)})
	if err != nil {
		return nil, err
	}
	return settingValues(res.GetValues()), nil
}

func (c *Client) SettingsInScope(ctx context.Context, scope SettingScope) ([]SettingValue, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Settings.ListInScopes(sys, &signalsv1.ListInScopesRequest{Scope: wireScope(scope)})
	if err != nil {
		return nil, err
	}
	return settingValues(res.GetValues()), nil
}

func (c *Client) AllSettings(ctx context.Context) ([]SettingValue, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.Settings.ListAll(sys, &signalsv1.ListAllRequest{})
	if err != nil {
		return nil, err
	}
	return settingValues(res.GetValues()), nil
}

func (c *Client) UpsertSetting(ctx context.Context, scope SettingScope, path, value string) (SettingValue, error) {
	sys, err := c.System(ctx)
	if err != nil {
		return SettingValue{}, err
	}
	res, err := c.Settings.UpsertValue(sys, &signalsv1.UpsertValueRequest{Scope: wireScope(scope), Path: path, Value: value})
	if err != nil {
		return SettingValue{}, err
	}
	return settingValue(res.GetValue()), nil
}

func (c *Client) DeleteSetting(ctx context.Context, scope SettingScope, path string) error {
	sys, err := c.System(ctx)
	if err != nil {
		return err
	}
	_, err = c.Settings.DeleteValue(sys, &signalsv1.DeleteValueRequest{Scope: wireScope(scope), Path: path})
	return err
}

func wireScope(s SettingScope) *signalsv1.SettingScope {
	out := &signalsv1.SettingScope{Kind: s.Kind}
	if s.ChannelID != nil {
		id := s.ChannelID.String()
		out.ChannelId = &id
	}
	return out
}

func settingValue(v *signalsv1.SettingValue) SettingValue {
	out := SettingValue{Path: v.GetPath(), Value: v.GetValue(), Scope: SettingScope{Kind: v.GetScope().GetKind()}}
	if raw := v.GetScope().GetChannelId(); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			out.Scope.ChannelID = &id
		}
	}
	if ts := v.GetUpdatedAt(); ts != nil {
		out.UpdatedAt = ts.AsTime().Unix()
	}
	return out
}

func settingValues(in []*signalsv1.SettingValue) []SettingValue {
	out := make([]SettingValue, 0, len(in))
	for _, v := range in {
		out = append(out, settingValue(v))
	}
	return out
}

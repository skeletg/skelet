package front

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/channel"
	"github.com/skeletg/skelet/internal/config"
)

// Config reads on the front API resolve against the channel the request carries, so a channel's own
// store name, legal URLs, robots.txt or payment instructions reach its storefront instead of the
// platform defaults - never a locale, since settings are not a per-locale dimension
func requestConfigScope(ctx context.Context) config.Scope {
	sv, ok := channel.FromContext(ctx)
	if !ok || sv == nil {
		return config.DefaultScope
	}
	id := sv.ID
	return config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
}

// For a record that names its own channel - an order's instructions belong to the channel it was
// placed on, not to whichever storefront is rendering it
func channelConfigScope(id string) (config.Scope, bool) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return config.DefaultScope, false
	}
	return config.Scope{Kind: config.ScopeChannel, ChannelID: &parsed}, true
}

func scopingModel(sc config.Scoping) model.ConfigScoping {
	switch sc {
	case config.ScopingChannel:
		return model.ConfigScopingChannel
	default:
		return model.ConfigScopingGlobal
	}
}

func toConfigValueModel(ctx context.Context, e config.Entry) *model.ConfigValue {
	section, group, field, _ := config.ParsePath(e.Definition.Path)
	// Masked defensively so marking a secret Public could never leak its plaintext to anonymous callers - today no secret is Public, but that is not enforced here (mirrors the admin config helper)
	value := e.Value
	if e.Definition.Type == config.TypeSecret {
		value = ""
	}
	m := &model.ConfigValue{
		Path:      e.Definition.Path,
		Section:   section,
		Group:     group,
		Field:     field,
		Type:      toConfigTypeModel(e.Definition.Type),
		Value:     value,
		Label:     e.Definition.Label,
		Public:    e.Definition.Public,
		IsDefault: e.IsDefault,
		Scope:     toConfigScopeModel(e.Scope),
		// Non-null in the schema, so it has to be filled even though the storefront ignores it
		Scoping: scopingModel(config.ScopingFor(e.Definition.Path)),
	}

	opts, err := e.Definition.GetOptions(ctx)
	if err == nil && len(opts) > 0 {
		m.Options = make([]*model.ConfigOption, len(opts))
		for i, o := range opts {
			m.Options[i] = &model.ConfigOption{Value: o.Value, Label: o.Label}
		}
	}

	if !e.IsDefault && !e.UpdatedAt.IsZero() {
		m.UpdatedAt = &e.UpdatedAt
	}
	return m
}

// Preserves the registry declaration order of Sections and Groups - shared by configSections (admin) and publicConfigSections
func buildConfigSections(ctx context.Context, entries []config.Entry) []*model.ConfigSection {
	byGroup := make(map[string]map[string][]*model.ConfigValue)
	for _, e := range entries {
		section, group, _, ok := config.ParsePath(e.Definition.Path)
		if !ok {
			continue
		}
		if byGroup[section] == nil {
			byGroup[section] = make(map[string][]*model.ConfigValue)
		}
		byGroup[section][group] = append(byGroup[section][group], toConfigValueModel(ctx, e))
	}

	allSections := config.AllSections()
	allGroups := config.AllGroups()
	sections := make([]*model.ConfigSection, 0, len(allSections))
	for _, s := range allSections {
		groupsInSection := byGroup[s.Slug]
		if len(groupsInSection) == 0 {
			continue
		}
		groups := make([]*model.ConfigGroup, 0)
		for _, g := range allGroups {
			if g.Section != s.Slug {
				continue
			}
			fields := groupsInSection[g.Slug]
			if len(fields) == 0 {
				continue
			}
			groups = append(groups, &model.ConfigGroup{
				Slug:   g.Slug,
				Label:  g.Label,
				Fields: fields,
			})
		}
		if len(groups) == 0 {
			continue
		}
		sections = append(sections, &model.ConfigSection{
			Slug:   s.Slug,
			Label:  s.Label,
			Groups: groups,
		})
	}
	return sections
}

func toConfigTypeModel(t config.Type) model.ConfigType {
	switch t {
	case config.TypeString:
		return model.ConfigTypeString
	case config.TypeTextarea:
		return model.ConfigTypeTextarea
	case config.TypeInt:
		return model.ConfigTypeInt
	case config.TypeBool:
		return model.ConfigTypeBool
	case config.TypePhone:
		return model.ConfigTypePhone
	case config.TypeSelect:
		return model.ConfigTypeSelect
	case config.TypeMultiselect:
		return model.ConfigTypeMultiselect
	case config.TypeWorkingHours:
		return model.ConfigTypeWorkingHours
	case config.TypeJSON:
		return model.ConfigTypeJSON
	case config.TypeImage:
		return model.ConfigTypeImage
	case config.TypeColor:
		return model.ConfigTypeColor
	}
	return model.ConfigTypeString
}

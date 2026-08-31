package admin

import (
	"context"
	"github.com/skeletg/skelet/apps/backend/internal/auth"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/config"
)

// Editing the default scope rewrites the baseline every channel inherits, which is why it needs its own slug rather than config.manage
// Secret-typed paths take config.manage_secrets ON TOP of the ordinary write grant - gateway keys and
// signing secrets move money and trust, a date format does not, and one grant covering both meant every
// settings operator could repoint Stripe. Unknown paths pass through so Set's own validation answers
func requireSecretWritePermission(ctx context.Context, checker auth.ScopedPermissionChecker, path string, scope config.Scope) error {
	def, ok := config.Find(path)
	if !ok || def.Type != config.TypeSecret {
		return nil
	}
	return requireConfigScopePermission(ctx, checker, "config.manage_secrets", scope)
}

func configWriteSlug(scope config.Scope) string {
	if scope.Kind == config.ScopeDefault {
		return "config.manage_defaults"
	}
	return "config.manage"
}

// The registry resolves a field's scoping from its own declaration, then its group's, then its
// section's - the UI needs the answer, not the ladder
func toConfigScopingModel(sc config.Scoping) model.ConfigScoping {
	switch sc {
	case config.ScopingChannel:
		return model.ConfigScopingChannel
	default:
		return model.ConfigScopingGlobal
	}
}

func toConfigValueModel(ctx context.Context, e config.Entry) *model.ConfigValue {
	section, group, field, _ := config.ParsePath(e.Definition.Path)

	// Secrets are write-only - depending on the read path e.Value is either decrypted plaintext or stored ciphertext, and neither may leave the server, so the UI gets isSet instead
	value := e.Value
	isSet := false
	if e.Definition.Type == config.TypeSecret {
		isSet = !e.IsDefault && e.Value != ""
		value = ""
	}

	m := &model.ConfigValue{
		Path:        e.Definition.Path,
		Section:     section,
		Group:       group,
		Field:       field,
		Type:        toConfigTypeModel(e.Definition.Type),
		Value:       value,
		Label:       e.Definition.Label,
		Description: e.Definition.Description,
		Public:      e.Definition.Public,
		IsDefault:   e.IsDefault,
		IsSet:       isSet,
		Scope:       toConfigScopeModel(e.Scope),
		Scoping:     toConfigScopingModel(config.ScopingFor(e.Definition.Path)),
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

// Section and group order comes from the registry's declaration order, not from the entries
func buildConfigSections(ctx context.Context, entries []config.Entry) []*model.ConfigSection {
	byGroup := make(map[string]map[string][]*model.ConfigValue) // (section, group) slug -> values
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
	case config.TypeSearchAttributes:
		return model.ConfigTypeSearchAttributes
	case config.TypeSortOptions:
		return model.ConfigTypeSortOptions
	case config.TypeImage:
		return model.ConfigTypeImage
	case config.TypeColor:
		return model.ConfigTypeColor
	case config.TypeSecret:
		return model.ConfigTypeSecret
	}
	return model.ConfigTypeString
}

// Mapping only, no validation - the service does that, and an omitted input means DefaultScope
func scopeFromInput(in *model.ConfigScopeInput) config.Scope {
	if in == nil {
		return config.DefaultScope
	}
	scope := config.Scope{Kind: config.ScopeDefault}
	if in.Kind == model.ConfigScopeKindChannel {
		scope.Kind = config.ScopeChannel
	}
	scope.ChannelID = parseOptionalUUID(in.ChannelID)
	scope.Locale = in.Locale
	return scope
}

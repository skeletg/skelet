package admin

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/action"
	"github.com/skeletg/skelet/apps/backend/internal/admin"
	"github.com/skeletg/skelet/apps/backend/internal/auth"
	db "github.com/skeletg/skelet/apps/backend/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/internal/permission"
)

// toAdminActionModel maps a registered action.Definition to its GraphQL representation. Handler is intentionally not
// exposed - the UI doesn't need to know what runs, only what it can invoke
func toAdminActionModel(d action.Definition) *model.AdminAction {
	args := make([]*model.ActionArgument, len(d.Arguments))
	for i, a := range d.Arguments {
		args[i] = toActionArgumentModel(a)
	}
	return &model.AdminAction{
		Slug:               d.Slug,
		Name:               d.Name,
		Description:        d.Description,
		Group:              d.Group,
		PermissionRequired: d.Permission,
		Arguments:          args,
	}
}

func toActionArgumentModel(a action.Argument) *model.ActionArgument {
	opts := make([]*model.ActionArgumentOption, len(a.Options))
	for i, o := range a.Options {
		opts[i] = &model.ActionArgumentOption{Value: o.Value, Label: o.Label}
	}
	var def *string
	if a.Default != "" {
		d := a.Default
		def = &d
	}
	return &model.ActionArgument{
		Name:        a.Name,
		Label:       a.Label,
		Description: a.Description,
		Type:        actionArgumentTypeGQL(a.Type),
		Required:    a.Required,
		Default:     def,
		Options:     opts,
	}
}

func actionArgumentTypeGQL(t action.ArgumentType) model.ActionArgumentType {
	switch t {
	case action.ArgString:
		return model.ActionArgumentTypeString
	case action.ArgInt:
		return model.ActionArgumentTypeInt
	case action.ArgBool:
		return model.ActionArgumentTypeBool
	case action.ArgSelect:
		return model.ActionArgumentTypeSelect
	}
	return model.ActionArgumentType(strings.ToUpper(string(t)))
}

func toAdminUserModel(u admin.AdminUser) *model.AdminUser {
	perms := make([]*model.Permission, len(u.Permissions))
	for i, p := range u.Permissions {
		perms[i] = toPermissionModel(p)
	}
	roles := make([]*model.Role, len(u.Roles))
	for i, role := range u.Roles {
		roles[i] = toRoleModel(role)
	}
	return &model.AdminUser{
		ID:          u.ID.String(),
		Email:       u.Email,
		FirstName:   u.FirstName,
		LastName:    u.LastName,
		IsActive:    u.IsActive,
		IsSuperuser: u.IsSuperuser,
		TotpEnabled: u.TotpEnabled,
		Roles:       roles,
		Permissions: perms,
		Sessions:    []*model.AdminSession{},
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}
}

// clientMeta extracts the client IP and User-Agent from the HTTP request
// stored in the context by the auth middleware.
func clientMeta(ctx context.Context) (ip, userAgent string) {
	r, ok := auth.RequestFromContext(ctx)
	if !ok {
		return "", ""
	}
	return auth.ClientIP(r), r.Header.Get("User-Agent")
}

func toAdminSessionModel(s db.AdminSession, currentSessionID uuid.UUID) *model.AdminSession {
	m := &model.AdminSession{
		ID:           s.ID.String(),
		CreatedAt:    s.CreatedAt,
		LastActiveAt: s.LastActiveAt,
		IsCurrent:    s.ID == currentSessionID,
	}
	if s.IpAddress.Valid {
		ip := s.IpAddress.IPNet.IP.String()
		m.IPAddress = &ip
	}
	if s.UserAgent.Valid {
		m.UserAgent = &s.UserAgent.String
	}
	return m
}

// Description comes from the code registry rather than the row - permissions Sync into the DB
// without one, and an orphaned row whose code was removed correctly resolves to empty
func toPermissionModel(p admin.Permission) *model.Permission {
	return &model.Permission{
		ID:          p.ID.String(),
		Name:        p.Name,
		Slug:        p.Slug,
		Description: permissionDescription(p.Slug),
	}
}

func permissionDescription(slug string) string {
	if def, ok := permission.Find(slug); ok {
		return def.Description
	}
	return ""
}

func toRoleModel(r admin.Role) *model.Role {
	scopes := make([]*model.RolePermissionScope, len(r.PermissionScopes))
	for i, scope := range r.PermissionScopes {
		s := &model.RolePermissionScope{
			Permission: toPermissionModel(scope.Permission),
		}
		if scope.ChannelID != nil {
			id := scope.ChannelID.String()
			s.ChannelID = &id
		}
		if scope.Locale != nil {
			l := *scope.Locale
			s.Locale = &l
		}
		scopes[i] = s
	}
	return &model.Role{
		ID:          r.ID.String(),
		Name:        r.Name,
		Slug:        r.Slug,
		Description: r.Description,
		Scopes:      scopes,
		MemberCount: int32(r.MemberCount),
		CreatedAt:   r.CreatedAt,
	}
}

// rolePermissionScopeInputsFromGQL maps the GraphQL input rows to the domain type, validating channel-id parse and
// tolerating nil channel/locale (= "any")
func rolePermissionScopeInputsFromGQL(inputs []*model.RolePermissionScopeInput) ([]admin.RolePermissionScopeInput, error) {
	scopes := make([]admin.RolePermissionScopeInput, 0, len(inputs))
	for _, in := range inputs {
		if in == nil {
			continue
		}
		out := admin.RolePermissionScopeInput{PermissionSlug: in.PermissionSlug}
		if in.ChannelID != nil && *in.ChannelID != "" {
			id, err := uuid.Parse(*in.ChannelID)
			if err != nil {
				return nil, errInvalidID("channel id", nil)
			}
			out.ChannelID = &id
		}
		if in.Locale != nil && *in.Locale != "" {
			l := *in.Locale
			out.Locale = &l
		}
		scopes = append(scopes, out)
	}
	return scopes, nil
}

func toPermissionGroupModel(g admin.PermissionGroup) *model.PermissionGroup {
	perms := make([]*model.Permission, len(g.Permissions))
	for i, p := range g.Permissions {
		perms[i] = toPermissionModel(p)
	}
	return &model.PermissionGroup{
		ID:          g.ID.String(),
		Name:        g.Name,
		Slug:        g.Slug,
		Permissions: perms,
	}
}

package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/sso"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) AdminSsoStart(ctx context.Context, redirectURI string) (*model.SsoStart, error) {
	if err := r.checkAuthRate(ctx, "admin_sso"); err != nil {
		return nil, err
	}
	st, err := r.SsoSvc.StartAdmin(ctx, redirectURI)
	if err != nil {
		return nil, err
	}
	return &model.SsoStart{AuthorizeURL: st.AuthorizeURL, State: st.State, Binding: st.Binding}, nil
}

func (r *mutationResolver) AdminSsoComplete(ctx context.Context, state string, code string, binding string) (*model.AdminSsoResult, error) {
	if err := r.checkAuthRate(ctx, "admin_sso"); err != nil {
		return nil, err
	}
	in, err := r.SsoSvc.CompleteAdmin(ctx, state, code, binding)
	if err != nil {
		return nil, err
	}
	if in.TotpTicket != "" {
		return &model.AdminSsoResult{TotpTicket: &in.TotpTicket}, nil
	}
	session, err := r.adminSession(ctx, in.User)
	if err != nil {
		return nil, err
	}
	return &model.AdminSsoResult{Session: session}, nil
}

func (r *mutationResolver) AdminSsoTotp(ctx context.Context, ticket string, code string) (*model.AdminAuthPayload, error) {
	if err := r.checkAuthRate(ctx, "admin_sso"); err != nil {
		return nil, err
	}
	user, err := r.SsoSvc.CompleteAdminTotp(ctx, ticket, code)
	if err != nil {
		return nil, err
	}
	return r.adminSession(ctx, user)
}

func (r *mutationResolver) AdminSsoLinkStart(ctx context.Context, redirectURI string) (*model.SsoStart, error) {
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	st, err := r.SsoSvc.StartAdminLink(ctx, claims.AdminID, redirectURI)
	if err != nil {
		return nil, err
	}
	return &model.SsoStart{AuthorizeURL: st.AuthorizeURL, State: st.State, Binding: st.Binding}, nil
}

func (r *mutationResolver) AdminSsoLinkComplete(ctx context.Context, state string, code string, binding string) (*model.AdminSsoLink, error) {
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	link, err := r.SsoSvc.CompleteAdminLink(ctx, claims.AdminID, state, code, binding)
	if err != nil {
		return nil, err
	}
	return toAdminSsoLinkModel(link), nil
}

func (r *mutationResolver) AdminSsoUnlink(ctx context.Context, id string) (bool, error) {
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return false, err
	}
	linkID, err := uuid.Parse(id)
	if err != nil {
		return false, sso.ErrLinkAbsent
	}
	return true, r.SsoSvc.UnlinkAdmin(ctx, claims.AdminID, linkID)
}

func (r *queryResolver) AdminSsoOption(ctx context.Context) (*model.SsoOption, error) {
	if label := r.SsoSvc.Label(ctx, sso.Admins, uuid.Nil); label != "" {
		return &model.SsoOption{Label: label}, nil
	}
	return nil, nil
}

func (r *queryResolver) AdminSsoLinks(ctx context.Context) ([]*model.AdminSsoLink, error) {
	claims, err := auth.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	links, err := r.SsoSvc.AdminLinks(ctx, claims.AdminID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.AdminSsoLink, len(links))
	for i, l := range links {
		out[i] = toAdminSsoLinkModel(l)
	}
	return out, nil
}

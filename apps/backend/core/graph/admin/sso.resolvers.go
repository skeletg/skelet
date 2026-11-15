package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/sso"
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

func (r *mutationResolver) AdminSsoComplete(ctx context.Context, state string, code string, binding string) (*model.AdminAuthPayload, error) {
	if err := r.checkAuthRate(ctx, "admin_sso"); err != nil {
		return nil, err
	}
	user, err := r.SsoSvc.CompleteAdmin(ctx, state, code, binding)
	if err != nil {
		return nil, err
	}
	return r.adminSession(ctx, user)
}

func (r *queryResolver) AdminSsoOption(ctx context.Context) (*model.SsoOption, error) {
	if label := r.SsoSvc.Label(ctx, sso.Admins, uuid.Nil); label != "" {
		return &model.SsoOption{Label: label}, nil
	}
	return nil, nil
}

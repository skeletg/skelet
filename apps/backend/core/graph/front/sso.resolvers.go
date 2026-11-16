package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/sso"
)

func (r *mutationResolver) CustomerSsoStart(ctx context.Context, redirectURI string) (*model.SsoStart, error) {
	if err := r.checkAuthRate(ctx, "sso"); err != nil {
		return nil, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok || store == nil {
		return nil, storeRequired()
	}
	st, err := r.SsoSvc.StartCustomer(ctx, *store, redirectURI)
	if err != nil {
		return nil, err
	}
	return &model.SsoStart{AuthorizeURL: st.AuthorizeURL, State: st.State, Binding: st.Binding}, nil
}

func (r *mutationResolver) CustomerSsoComplete(ctx context.Context, state string, code string, binding string) (*model.CustomerAuthPayload, error) {
	if err := r.checkAuthRate(ctx, "sso"); err != nil {
		return nil, err
	}
	store, ok := channel.FromContext(ctx)
	if !ok || store == nil {
		return nil, storeRequired()
	}
	c, err := r.SsoSvc.CompleteCustomer(ctx, store.ID, state, code, binding)
	if err != nil {
		return nil, err
	}
	return r.customerAuthPayload(ctx, c)
}

func (r *queryResolver) CustomerSsoOption(ctx context.Context) (*model.SsoOption, error) {
	store, ok := channel.FromContext(ctx)
	if !ok || store == nil {
		return nil, nil
	}
	if label := r.SsoSvc.Label(ctx, sso.Customers, store.ID); label != "" {
		return &model.SsoOption{Label: label}, nil
	}
	return nil, nil
}

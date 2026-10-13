package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/remoteprovider"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateRemoteProvider(ctx context.Context, input model.CreateRemoteProviderInput) (*model.CreateRemoteProviderPayload, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.manage"); err != nil {
		return nil, err
	}
	domainInput := remoteprovider.CreateInput{
		Kind:      remoteProviderKindFromGQL(input.Kind),
		Slug:      input.Slug,
		Name:      input.Name,
		TargetURL: input.TargetURL,
	}
	config, err := remoteProviderConfigFromGQL(input.Config)
	if err != nil {
		return nil, err
	}
	domainInput.Config = config
	if input.TimeoutMs != nil {
		domainInput.TimeoutMs = *input.TimeoutMs
	}
	if input.CacheTTLS != nil {
		domainInput.CacheTTLs = *input.CacheTTLS
	}
	if input.FailurePolicy != nil {
		domainInput.FailurePolicy = remoteProviderPolicyFromGQL(*input.FailurePolicy)
	}
	if input.ChannelID != nil {
		cid, err := parseID(*input.ChannelID, "channel")
		if err != nil {
			return nil, err
		}
		domainInput.ChannelID = &cid
	}
	if err := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "remote_providers.manage", domainInput.ChannelID); err != nil {
		return nil, err
	}
	result, err := r.RemoteProviderSvc.Create(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return &model.CreateRemoteProviderPayload{
		RawSecret: result.RawSecret,
		Provider:  toRemoteProviderModel(result.Provider),
	}, nil
}

func (r *mutationResolver) UpdateRemoteProvider(ctx context.Context, input model.UpdateRemoteProviderInput) (*model.RemoteProvider, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.manage"); err != nil {
		return nil, err
	}
	providerID, err := parseID(input.ID, "remote provider")
	if err != nil {
		return nil, err
	}
	if _, err := r.requireRemoteProviderScope(ctx, providerID, "remote_providers.manage"); err != nil {
		return nil, err
	}
	updateInput := remoteprovider.UpdateInput{
		ID:        providerID,
		Name:      input.Name,
		TargetURL: input.TargetURL,
		TimeoutMs: input.TimeoutMs,
		CacheTTLs: input.CacheTTLS,
	}
	if input.Config != nil {
		config, err := remoteProviderConfigFromGQL(input.Config)
		if err != nil {
			return nil, err
		}
		if len(config) == 0 {
			config = []byte("{}")
		}
		updateInput.Config = config
	}
	if input.FailurePolicy != nil {
		p := remoteProviderPolicyFromGQL(*input.FailurePolicy)
		updateInput.FailurePolicy = &p
	}
	p, err := r.RemoteProviderSvc.Update(ctx, updateInput)
	if err != nil {
		return nil, err
	}
	return toRemoteProviderModel(p), nil
}

func (r *mutationResolver) SetRemoteProviderActive(ctx context.Context, id string, active bool) (*model.RemoteProvider, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.manage"); err != nil {
		return nil, err
	}
	providerID, err := parseID(id, "remote provider")
	if err != nil {
		return nil, err
	}
	if _, err := r.requireRemoteProviderScope(ctx, providerID, "remote_providers.manage"); err != nil {
		return nil, err
	}
	p, err := r.RemoteProviderSvc.SetActive(ctx, providerID, active)
	if err != nil {
		return nil, err
	}
	return toRemoteProviderModel(p), nil
}

func (r *mutationResolver) RotateRemoteProviderSecret(ctx context.Context, id string) (*model.RotateRemoteProviderSecretPayload, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.manage"); err != nil {
		return nil, err
	}
	providerID, err := parseID(id, "remote provider")
	if err != nil {
		return nil, err
	}
	if _, err := r.requireRemoteProviderScope(ctx, providerID, "remote_providers.manage"); err != nil {
		return nil, err
	}
	result, err := r.RemoteProviderSvc.RotateSecret(ctx, providerID)
	if err != nil {
		return nil, err
	}
	return &model.RotateRemoteProviderSecretPayload{
		RawSecret: result.RawSecret,
		Provider:  toRemoteProviderModel(result.Provider),
	}, nil
}

func (r *mutationResolver) DeleteRemoteProvider(ctx context.Context, id string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.manage"); err != nil {
		return false, err
	}
	providerID, err := parseID(id, "remote provider")
	if err != nil {
		return false, err
	}
	if _, err := r.requireRemoteProviderScope(ctx, providerID, "remote_providers.manage"); err != nil {
		return false, err
	}
	if err := r.RemoteProviderSvc.Delete(ctx, providerID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) RemoteProviders(ctx context.Context, kind *model.RemoteProviderKind) ([]*model.RemoteProvider, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.view"); err != nil {
		return nil, err
	}
	providers, err := r.RemoteProviderSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	permitted, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "remote_providers.view")
	if err != nil {
		return nil, err
	}
	managed, err := auth.LoadPermittedChannels(ctx, r.AdminSvc, "remote_providers.manage")
	if err != nil {
		return nil, err
	}
	out := make([]*model.RemoteProvider, 0, len(providers))
	for _, p := range providers {
		if kind != nil && p.Kind != remoteProviderKindFromGQL(*kind) {
			continue
		}
		if !permitted.AllowsOptional(p.ChannelID) {
			continue
		}
		out = append(out, redactedUnlessManaged(toRemoteProviderModel(p), managed.AllowsOptional(p.ChannelID)))
	}
	return out, nil
}

func (r *queryResolver) RemoteProvider(ctx context.Context, id string) (*model.RemoteProvider, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "remote_providers.view"); err != nil {
		return nil, err
	}
	providerID, err := parseID(id, "remote provider")
	if err != nil {
		return nil, err
	}
	p, err := r.requireRemoteProviderScope(ctx, providerID, "remote_providers.view")
	if err != nil {
		return nil, err
	}
	manages := auth.RequireGlobalOrChannelPermission(ctx, r.AdminSvc, "remote_providers.manage", p.ChannelID) == nil
	return redactedUnlessManaged(toRemoteProviderModel(p), manages), nil
}

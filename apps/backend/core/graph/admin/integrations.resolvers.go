package admin

import (
	"context"
	"database/sql"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/integration"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (r *mutationResolver) CreateIntegration(ctx context.Context, input model.CreateIntegrationInput) (*model.CreateIntegrationPayload, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	description := ""
	if input.Description != nil {
		description = *input.Description
	}
	domainInput := integration.CreateIntegrationInput{
		Name:        input.Name,
		Description: description,
		Permissions: input.PermissionSlugs,
	}
	if input.ExpiresAt != nil {
		domainInput.ExpiresAt = sql.NullTime{Time: *input.ExpiresAt, Valid: true}
	}
	if input.ChannelID != nil {
		channelID, err := parseID(*input.ChannelID, "channel")
		if err != nil {
			return nil, err
		}
		domainInput.ChannelID = &channelID
	}
	if err := r.requireGrantable(ctx, integrationGrants(input.PermissionSlugs, domainInput.ChannelID)); err != nil {
		return nil, err
	}
	result, err := r.IntegrationSvc.Create(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return &model.CreateIntegrationPayload{
		RawToken:    result.RawToken, // handed back exactly once - never readable again
		Integration: toIntegrationModel(result.Integration),
	}, nil
}

// Nil fields mean "no change" - ClearExpiresAt forces expiresAt to NULL and wins over ExpiresAt
func (r *mutationResolver) UpdateIntegration(ctx context.Context, input model.UpdateIntegrationInput) (*model.Integration, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	id, err := parseID(input.ID, "integration")
	if err != nil {
		return nil, err
	}
	existing, err := r.requireIntegrationWithinOwn(ctx, id)
	if err != nil {
		return nil, err
	}
	domainInput := integration.UpdateIntegrationInput{
		ID:              id,
		Name:            input.Name,
		Description:     input.Description,
		PermissionSlugs: input.PermissionSlugs,
		ExpiresAt:       input.ExpiresAt,
	}
	if input.ClearExpiresAt != nil {
		domainInput.ClearExpiresAt = *input.ClearExpiresAt
	}
	// clearChannel unpins; a channelId repins; both absent leaves the pin alone
	switch {
	case input.ClearChannel != nil && *input.ClearChannel:
		domainInput.SetChannel = true
	case input.ChannelID != nil:
		channelID, err := parseID(*input.ChannelID, "channel")
		if err != nil {
			return nil, err
		}
		domainInput.ChannelID = &channelID
		domainInput.SetChannel = true
	}
	after := existingIntegrationGrants(existing)
	if input.PermissionSlugs != nil || domainInput.SetChannel {
		slugs, pin := input.PermissionSlugs, existing.ChannelID
		if slugs == nil {
			for _, g := range after {
				slugs = append(slugs, g.Slug)
			}
		}
		if domainInput.SetChannel {
			pin = domainInput.ChannelID
		}
		after = integrationGrants(slugs, pin)
	}
	if err := r.requireGrantable(ctx, after); err != nil {
		return nil, err
	}
	i, err := r.IntegrationSvc.Update(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toIntegrationModel(i), nil
}

func (r *mutationResolver) SetIntegrationActive(ctx context.Context, id string, active bool) (*model.Integration, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	integrationID, err := parseID(id, "integration")
	if err != nil {
		return nil, err
	}
	if _, err := r.requireIntegrationWithinOwn(ctx, integrationID); err != nil {
		return nil, err
	}
	i, err := r.IntegrationSvc.SetActive(ctx, integrationID, active) // disabling bites on the next token validation, nothing caches it
	if err != nil {
		return nil, err
	}
	return toIntegrationModel(i), nil
}

func (r *mutationResolver) RegenerateIntegrationToken(ctx context.Context, id string) (*model.RegenerateIntegrationTokenPayload, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	integrationID, err := parseID(id, "integration")
	if err != nil {
		return nil, err
	}
	if _, err := r.requireIntegrationWithinOwn(ctx, integrationID); err != nil {
		return nil, err
	}
	result, err := r.IntegrationSvc.RegenerateToken(ctx, integrationID)
	if err != nil {
		return nil, err
	}
	return &model.RegenerateIntegrationTokenPayload{
		RawToken:    result.RawToken, // the old token dies immediately and this one is shown exactly once
		Integration: toIntegrationModel(result.Integration),
	}, nil
}

func (r *mutationResolver) DeleteIntegration(ctx context.Context, id string) (bool, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return false, err
	}
	integrationID, err := parseID(id, "integration")
	if err != nil {
		return false, err
	}
	if _, err := r.requireIntegrationWithinOwn(ctx, integrationID); err != nil {
		return false, err
	}
	if err := r.IntegrationSvc.Delete(ctx, integrationID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) Integrations(ctx context.Context) ([]*model.Integration, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "admins.manage"); err != nil {
		return nil, err
	}
	list, err := r.IntegrationSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Integration, len(list))
	for idx, i := range list {
		result[idx] = toIntegrationModel(i)
	}
	return result, nil
}

package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/channel"
)

// Gated on inventory.view rather than channels.view - it exposes inventory data, and inventory.manage holders pass through the view/manage rule
func (r *channelResolver) InventoryLocations(ctx context.Context, obj *model.Channel) ([]*model.ChannelInventoryLocation, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "inventory.view"); err != nil {
		return nil, err
	}
	channelID, err := parseID(obj.ID, "channel")
	if err != nil {
		return nil, err
	}
	locations, err := r.InventorySvc.ListChannelLocations(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ChannelInventoryLocation, len(locations))
	for i, loc := range locations {
		out[i] = &model.ChannelInventoryLocation{
			Location: toInventoryLocationModel(loc),
			Priority: loc.Priority,
		}
	}
	return out, nil
}

func (r *mutationResolver) CreateChannel(ctx context.Context, input model.CreateChannelInput) (*model.Channel, error) {
	if err := auth.RequireGlobalPermission(ctx, r.AdminSvc, "channels.manage"); err != nil {
		return nil, err
	}
	domainInput := channel.CreateChannelInput{
		Slug:          input.Slug,
		Name:          input.Name,
		DefaultLocale: stringOr(input.DefaultLocale, "en"),
		Locales:       input.Locales,
		CurrencyCode:  stringOr(input.CurrencyCode, ""),
		PublicURL:     stringOr(input.PublicURL, ""),
		Metadata:      channel.Metadata(metadataInputToStringMap(input.Metadata)),
	}
	sv, err := r.ChannelSvc.Create(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toChannelModel(sv), nil
}

func (r *mutationResolver) UpdateChannel(ctx context.Context, input model.UpdateChannelInput) (*model.Channel, error) {
	id, err := parseID(input.ID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "channels.manage", id, nil); err != nil {
		return nil, err
	}
	domainInput := channel.UpdateChannelInput{
		ID:            id,
		Name:          input.Name,
		DefaultLocale: input.DefaultLocale,
		Locales:       input.Locales,
		IsActive:      input.IsActive,
		CurrencyCode:  input.CurrencyCode,
		PublicURL:     input.PublicURL,
	}
	if input.Metadata != nil {
		m := channel.Metadata(metadataInputToStringMap(input.Metadata))
		domainInput.Metadata = &m
	}
	sv, err := r.ChannelSvc.Update(ctx, domainInput)
	if err != nil {
		return nil, err
	}
	return toChannelModel(sv), nil
}

func (r *mutationResolver) DeleteChannel(ctx context.Context, id string) (bool, error) {
	svID, err := parseID(id, "channel")
	if err != nil {
		return false, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "channels.manage", svID, nil); err != nil {
		return false, err
	}
	if err := r.ChannelSvc.Delete(ctx, svID); err != nil {
		return false, err
	}
	return true, nil
}

// The locale codes are FK-validated in the repository, so a bad one surfaces as a write error rather than a validation one
func (r *mutationResolver) SetChannelLocales(ctx context.Context, channelID string, codes []string) (*model.Channel, error) {
	id, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "channels.manage", id, nil); err != nil {
		return nil, err
	}
	sv, err := r.ChannelSvc.SetLocales(ctx, id, codes)
	if err != nil {
		return nil, err
	}
	return toChannelModel(sv), nil
}

// A unique partial index on is_default=TRUE means setting the new default and clearing the old one have to share a transaction - the service does that
func (r *mutationResolver) SetChannelAsDefault(ctx context.Context, channelID string) (*model.Channel, error) {
	id, err := parseID(channelID, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "channels.manage", id, nil); err != nil {
		return nil, err
	}
	sv, err := r.ChannelSvc.SetDefault(ctx, id)
	if err != nil {
		return nil, err
	}
	return toChannelModel(sv), nil
}

func (r *queryResolver) Channels(ctx context.Context) ([]*model.Channel, error) {
	global, err := hasGlobalPermission(ctx, r.AdminSvc, "channels.view")
	if err != nil {
		return nil, err
	}
	svs, err := r.ChannelSvc.List(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[uuid.UUID]bool{}
	if !global {
		claims, err := auth.RequireAuth(ctx)
		if err != nil {
			return nil, err
		}
		ids, err := r.AdminSvc.ListPermittedChannelIDs(ctx, claims.AdminID, "channels.view")
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			allowed[id] = true
		}
		// channels.view gates ADMINISTERING channels, but this list also feeds the header scope picker - a store manager scoped to one channel usually holds no channels.view and would face an empty picker
		// So fall back to the union of channels any of their grants reference, where a nil-channel grant on any permission means all channels
		if len(allowed) == 0 {
			user, err := r.AdminSvc.GetByIDWithRoles(ctx, claims.AdminID)
			if err != nil {
				return nil, errNotFound("admin")
			}
			for _, role := range user.Roles {
				for _, sc := range role.PermissionScopes {
					if sc.ChannelID == nil {
						global = true
						break
					}
					allowed[*sc.ChannelID] = true
				}
				if global {
					break
				}
			}
		}
	}
	out := make([]*model.Channel, 0, len(svs))
	for _, sv := range svs {
		if global || allowed[sv.ID] {
			out = append(out, toChannelModel(sv))
		}
	}
	return out, nil
}

func (r *queryResolver) Channel(ctx context.Context, id string) (*model.Channel, error) {
	svID, err := parseID(id, "channel")
	if err != nil {
		return nil, err
	}
	if err := auth.RequireChannelPermission(ctx, r.AdminSvc, "channels.view", svID, nil); err != nil {
		return nil, err
	}
	sv, err := r.ChannelSvc.GetByID(ctx, svID)
	if err != nil {
		return nil, err
	}
	return toChannelModel(sv), nil
}

func (r *Resolver) Channel() ChannelResolver { return &channelResolver{r} }

type channelResolver struct{ *Resolver }

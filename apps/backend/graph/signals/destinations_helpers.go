package signals

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/graph/signals/model"
	"github.com/skeletg/skelet/apps/backend/internal/destination"
	"github.com/skeletg/skelet/apps/backend/internal/ptr"
)

func toEventDestinationModel(d destination.Destination) *model.EventDestination {
	return &model.EventDestination{
		ID:              d.ID.String(),
		ChannelID:       d.ChannelID.String(),
		Name:            d.Name,
		Kind:            d.Kind,
		TargetURL:       d.TargetURL,
		EventSources:    d.EventSources,
		EventNames:      d.EventNames,
		IsActive:        d.IsActive,
		LastError:       d.LastError,
		LastForwardedAt: d.LastForwardedAt,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	}
}

func destinationInputFromModel(input model.EventDestinationInput, id *uuid.UUID) (destination.UpsertInput, error) {
	chID, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return destination.UpsertInput{}, errInvalidID("channelId", err)
	}
	return destination.UpsertInput{
		ID:           id,
		ChannelID:    chID,
		Name:         input.Name,
		Kind:         input.Kind,
		TargetURL:    input.TargetURL,
		Secret:       ptr.Deref(input.Secret),
		EventSources: input.EventSources,
		EventNames:   input.EventNames,
		IsActive:     boolDeref(input.IsActive, true),
	}, nil
}

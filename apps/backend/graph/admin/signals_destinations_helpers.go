package admin

// Converters between the Signals contract and the admin models, plus the gate the destination
// surface shares

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/internal/signalspb/skelet/signals/v1"
)

func destinationInputProto(in model.EventDestinationInput) *signalsv1.DestinationInput {
	out := &signalsv1.DestinationInput{
		ChannelId: in.ChannelID, Name: in.Name, Kind: in.Kind, TargetUrl: in.TargetURL,
		EventSources: in.EventSources, EventNames: in.EventNames,
		Secret: in.Secret, IsActive: in.IsActive, // an absent secret leaves the stored one alone
	}
	return out
}

func eventDestinationModel(d *signalsv1.EventDestination) *model.EventDestination {
	out := &model.EventDestination{
		ID: d.GetId(), ChannelID: d.GetChannelId(), Name: d.GetName(), Kind: d.GetKind(),
		TargetURL: d.GetTargetUrl(), EventSources: d.GetEventSources(), EventNames: d.GetEventNames(),
		IsActive: d.GetIsActive(), LastError: d.GetLastError(),
		CreatedAt: d.GetCreatedAt().AsTime(), UpdatedAt: d.GetUpdatedAt().AsTime(),
	}
	if d.GetLastForwardedAt() != nil {
		t := d.GetLastForwardedAt().AsTime()
		out.LastForwardedAt = &t
	}
	return out
}

// Destinations carry delivery credentials, so the whole surface is settings.manage rather than a view grant
func (r *Resolver) signalsWrite(ctx context.Context) (context.Context, error) {
	return r.signalsScoped(ctx, "settings.manage")
}

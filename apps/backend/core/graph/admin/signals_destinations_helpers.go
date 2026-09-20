package admin

// Converters between the Signals contract and the admin models, plus the gate the destination
// surface shares

import (
	"context"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
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

// The suite answers who is in a segment, since membership is its table
// The caller's own token, not a system one - reading a membership is a permission an admin holds
func (r *Resolver) segmentMembers(ctx context.Context, _ uuid.UUID, segmentID uuid.UUID) ([]uuid.UUID, error) {
	authed, err := r.SignalsAPI.Authed(ctx)
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for token := ""; ; {
		res, err := r.SignalsAPI.Segments.ListMembers(authed, &signalsv1.ListMembersRequest{
			SegmentId: segmentID.String(), PageSize: 1000, PageToken: token,
		})
		if err != nil {
			return nil, signalsErr(err)
		}
		for _, m := range res.GetMembers() {
			if id, perr := uuid.Parse(m.GetCustomerId()); perr == nil {
				out = append(out, id)
			}
		}
		if token = res.GetNextPageToken(); token == "" {
			return out, nil
		}
	}
}

func (r *Resolver) customerEmails(ctx context.Context, ids []uuid.UUID) ([]string, error) {
	return r.Queries.CustomerEmailsByIDs(ctx, ids)
}

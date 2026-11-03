package signalsclient

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

type TrackedAttributes interface {
	ListSynapseTrackedAttributes(ctx context.Context) ([]db.ListSynapseTrackedAttributesRow, error)
}

// Core names the attributes and the suite answers with what it has seen of each - a failed read sends none, which the suite treats as no breakdown
func AffinityAttributes(ctx context.Context, src TrackedAttributes) []*signalsv1.AffinityAttribute {
	rows, err := src.ListSynapseTrackedAttributes(ctx)
	if err != nil {
		slog.WarnContext(ctx, "signals: tracked attributes read failed", "err", err)
		return nil
	}
	out := make([]*signalsv1.AffinityAttribute, 0, len(rows))
	for _, a := range rows {
		out = append(out, &signalsv1.AffinityAttribute{Slug: a.Slug, Name: a.Name})
	}
	return out
}

type ChannelReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

// Suite money is minor units in its channel's currency, named on the wire by channel id - "" when unreadable, so it labels and never decides
func ChannelCurrency(ctx context.Context, channels ChannelReader, channelID string) string {
	id, err := uuid.Parse(channelID)
	if err != nil {
		return ""
	}
	ch, err := channels.GetByID(ctx, id)
	if err != nil {
		return ""
	}
	return ch.CurrencyCode
}

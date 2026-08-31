package front

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
)

// Inlined on Channel so the storefront doesn't make a second roundtrip for the symbol + symbolPosition it needs to render prices
func (r *channelResolver) Currency(ctx context.Context, obj *model.Channel) (*model.Currency, error) {
	c, err := r.getCurrency(ctx, obj.CurrencyCode) // a misconfigured store with a code that isn't in currencies errors here, which is where it should be visible
	if err != nil {
		return nil, err
	}
	return toCurrencyModel(c), nil
}

func (r *queryResolver) CurrentChannel(ctx context.Context) (*model.Channel, error) {
	sv, ok := channel.FromContext(ctx) // resolved from the X-Channel-Id header or a matching Host - nil mostly means a dev/probe request, since real storefront queries all need a store
	if !ok {
		return nil, nil
	}
	return toFrontChannelModel(*sv), nil
}

func (r *Resolver) Channel() ChannelResolver { return &channelResolver{r} }

type channelResolver struct{ *Resolver }

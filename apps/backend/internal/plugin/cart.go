package plugin

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/auth"
	"github.com/skeletg/skelet/apps/backend/internal/cart"
	"github.com/skeletg/skelet/apps/backend/internal/channel"
	"github.com/skeletg/skelet/apps/backend/sdk"
)

// Backs sdk.ReadCart. A narrow contract rather than *cart.Service, so the host advertises exactly the
// one read it grants and nothing beside it
type CartReader interface {
	GetCart(ctx context.Context, id uuid.UUID, customerID *uuid.UUID) (cart.Cart, error)
}

var cartReader atomic.Pointer[CartReader]

// Wires cart reads, once, after the cart service exists
func SetCartReader(r CartReader) {
	if installedHost.Load() == nil {
		panic("plugin.SetCartReader called before plugin.Install")
	}
	if r == nil {
		panic("plugin.SetCartReader called with a nil reader")
	}
	if !cartReader.CompareAndSwap(nil, &r) {
		panic("plugin.SetCartReader called twice")
	}
}

// The store fence, applied here rather than left to each plugin: a cart belonging to another channel
// reads as ErrCartNotOnStore, which is what core's own cart resolvers answer. Enforcing it host-side
// means a plugin cannot forget to
func (h *Host) ReadCart(ctx context.Context, cartID uuid.UUID) (sdk.Cart, error) {
	rp := cartReader.Load()
	if rp == nil {
		return sdk.Cart{}, sdk.ErrCartsUnavailable
	}
	c, err := (*rp).GetCart(ctx, cartID, auth.OptionalCustomerID(ctx))
	if err != nil {
		return sdk.Cart{}, sdk.ErrCartNotOnStore // indistinguishable from a foreign cart, deliberately
	}
	if ch, ok := channel.FromContext(ctx); !ok || ch == nil || c.ChannelID != ch.ID {
		return sdk.Cart{}, sdk.ErrCartNotOnStore
	}

	out := sdk.Cart{
		ID:         c.ID,
		ChannelID:  c.ChannelID,
		CustomerID: c.CustomerID,
		Lines:      make([]sdk.CartLine, 0, len(c.Lines)),
	}
	for _, l := range c.Lines {
		out.Lines = append(out.Lines, sdk.CartLine{
			SKU:            l.ProductSku,
			Name:           l.ProductName,
			Quantity:       l.Quantity,
			UnitPriceCents: l.UnitPrice,
			OptionsSummary: optionsSummary(l.Options),
		})
	}
	return out, nil
}

// Flattens option picks to "Title: value; Title: value" so the option model stays out of the SDK -
// a file upload reads as "uploaded file", since the URL is a signed link that would be stale by the
// time anyone read the quote it landed in
func optionsSummary(opts []cart.CartLineOption) string {
	picks := make([]string, 0, len(opts))
	for _, o := range opts {
		v := o.ValueLabel
		if v == "" {
			v = o.TextValue
		}
		if v == "" && o.FileURL != "" {
			v = "uploaded file"
		}
		if v != "" {
			picks = append(picks, o.OptionTitle+": "+v)
		}
	}
	return strings.Join(picks, "; ")
}

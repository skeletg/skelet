package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
)

func (s *Service) AddConfiguredToCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, input AddConfiguredToCartInput) (Cart, error) { // options and price resolve server-side and the deltas fold into the unit price - re-adding the same SKU REPLACES the config, like AddBundleToCart
	if s.products == nil {
		return Cart{}, errNoCatalog
	}
	if err := validQuantity(input.Quantity); err != nil {
		return Cart{}, err
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	return s.addConfiguredLine(ctx, cart, input)
}

func (s *Service) AdminAddConfiguredToCart(ctx context.Context, cartID uuid.UUID, input AddConfiguredToCartInput) (Cart, error) { // ownership-bypassing twin of AddConfiguredToCart - phone orders need option picks too, and without this any required-option SKU is un-orderable through the console
	if s.products == nil {
		return Cart{}, errNoCatalog
	}
	if err := validQuantity(input.Quantity); err != nil {
		return Cart{}, err
	}
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	return s.addConfiguredLine(ctx, cart, input)
}

func (s *Service) addConfiguredLine(ctx context.Context, cart Cart, input AddConfiguredToCartInput) (Cart, error) { // shared by the customer and admin configured adds - callers have already authorized the cart
	res, err := s.products.ResolveProductOptions(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID)
	if err != nil {
		return Cart{}, fmt.Errorf("resolve product options for %q: %w", input.ProductSku, err)
	}
	fileAssets, err := s.resolveOptionFileAssets(ctx, res.ProductID, input.Options)
	if err != nil {
		return Cart{}, err
	}
	options, optionDelta, forcedQty, err := buildConfiguredLineOptions(res, input.Options, fileAssets)
	if err != nil {
		return Cart{}, err
	}
	quantity := input.Quantity // a picked value may force this ("Set of 4" -> 4), and it replaces the request BEFORE tier pricing so breaks apply to what's actually charged
	if forcedQty != nil {
		if err := validQuantity(*forcedQty); err != nil {
			return Cart{}, err
		}
		quantity = *forcedQty
	}
	base := res.UnitPrice // quantity breaks apply to the BASE price, and option surcharges ride on top
	if sp, perr := s.resolvePricing(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID); perr == nil {
		if err := validateQtyBounds(sp, quantity); err != nil { // this upsert REPLACES the line rather than summing, so the requested quantity is the whole prospective total
			return Cart{}, err
		}
		base = pricing.ApplyTiers(base, sp.Tiers, quantity)
	}
	unitPrice := max(base+optionDelta, 0) // a mistyped negative delta must not produce a negative line - free is the floor
	name := res.ProductName
	if name == "" {
		name = input.ProductSku
	}
	if s.bus != nil { // pre-commit hook, same contract as AddToCart - the deltas are already folded into unitPrice
		adding := &LineAdding{
			CartID:      cart.ID,
			ChannelID:   cart.ChannelID,
			CustomerID:  cart.CustomerID,
			ProductSku:  input.ProductSku,
			ProductName: name,
			UnitPrice:   &unitPrice,
			Quantity:    &quantity,
			StockParts:  OptionStockParts(options),
		}
		if err := s.bus.PublishSync(ctx, adding); err != nil {
			return Cart{}, fmt.Errorf("add to cart rejected: %w", err)
		}
		unitPrice, quantity = valueOr(adding.UnitPrice, unitPrice), valueOr(adding.Quantity, quantity) // read off the event, since the SDK setters and a webhook response swap the pointer rather than write through it
	}
	var line CartLine
	if err := s.inTx(ctx, func(r Repository) error {
		var err error
		line, err = r.UpsertConfiguredCartLine(ctx, cart.ID, AddConfiguredLineInput{
			ProductName: name,
			ProductSku:  input.ProductSku,
			UnitPrice:   unitPrice,
			Quantity:    quantity,
			Options:     options,
		})
		return err
	}); err != nil {
		return Cart{}, fmt.Errorf("add configured to cart: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &LineAdded{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
			LineID:     line.ID,
			ProductSku: input.ProductSku,
			UnitPrice:  unitPrice,
			Quantity:   quantity,
		})
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) resolveOptionFileAssets(ctx context.Context, productID uuid.UUID, inputs []CartLineOptionInput) (map[uuid.UUID]FileAsset, error) { // resolved up front so the pure validation and pricing step has the real filename, size and URL - nil when there are no files
	var out map[uuid.UUID]FileAsset
	for _, in := range inputs {
		if in.FileAssetID == nil {
			continue
		}
		if s.files == nil {
			return nil, fmt.Errorf("%w: file uploads are not configured", ErrProductOptionInvalid)
		}
		asset, err := s.files.ResolveFileAsset(ctx, productID, *in.FileAssetID)
		if err != nil {
			return nil, fmt.Errorf("%w: file upload %s: %w", ErrProductOptionInvalid, *in.FileAssetID, err)
		}
		if out == nil {
			out = make(map[uuid.UUID]FileAsset)
		}
		out[*in.FileAssetID] = asset
	}
	return out, nil
}

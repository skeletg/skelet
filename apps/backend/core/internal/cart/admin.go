package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Admin "Order Manager" console - these act on ANY active cart in a permitted channel WITHOUT the ownership check (the admin resolver enforces channel permission instead) and skip the Line* hooks on purpose, since an admin edit shouldn't fire a shopper's stock guards or recovery events; pricing and the add gates are the customer path's

func (s *Service) adminActiveCart(ctx context.Context, cartID uuid.UUID) (Cart, error) { // must exist and be active - no owner check
	cart, err := s.repo.GetCartByID(ctx, cartID)
	if err != nil {
		if IsSQLNoRows(err) {
			return Cart{}, ErrCartNotFound
		}
		return Cart{}, err
	}
	if cart.Status != "active" {
		return Cart{}, ErrCartNotActive
	}
	return cart, nil
}

func (s *Service) AdminCreateCart(ctx context.Context, channelID uuid.UUID, customerID *uuid.UUID) (Cart, error) { // a fresh guest cart when customerID is nil, else the customer's one active cart, since the unique index forbids a second
	if customerID == nil {
		return s.CreateGuestCart(ctx, channelID)
	}
	return GetOrCreateCustomerCart(ctx, s.repo, *customerID, channelID)
}

func (s *Service) AdminAddToCart(ctx context.Context, cartID uuid.UUID, sku string, quantity float64) (Cart, error) {
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	input, err := s.prepareSimpleAdd(ctx, cart, sku, quantity)
	if err != nil {
		return Cart{}, err
	}
	if _, err := s.repo.UpsertCartLineBySku(ctx, cart.ID, input); err != nil {
		return Cart{}, fmt.Errorf("add cart line: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) AdminUpdateCartLine(ctx context.Context, cartID, lineID uuid.UUID, quantity float64) (Cart, error) {
	if err := validQuantity(quantity); err != nil {
		return Cart{}, err
	}
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	line, err := s.findHydratedLine(ctx, cart.ID, lineID)
	if err != nil {
		return Cart{}, err
	}
	repricedUnit, err := s.validateQuantityEdit(ctx, cart, line, quantity) // the console must not break the forced-qty invariant or the ordering bounds either
	if err != nil {
		return Cart{}, err
	}
	if err := s.writeLineQuantity(ctx, line, quantity, repricedUnit); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) AdminRemoveFromCart(ctx context.Context, cartID, lineID uuid.UUID) (Cart, error) {
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if err := verifyLineInCart(ctx, s.repo, lineID, cart.ID); err != nil {
		return Cart{}, err
	}
	if err := s.repo.RemoveCartLine(ctx, lineID); err != nil {
		return Cart{}, fmt.Errorf("remove cart line: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) AdminClearCart(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if err := s.repo.ClearCartLines(ctx, cart.ID); err != nil {
		return Cart{}, fmt.Errorf("clear cart: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// The clone lands on the SOURCE's channel - a fresh guest cart when targetCustomerID is nil, else the customer's active cart, where lines already there win a collision the same way a guest merge does
// Lines are cloned verbatim rather than re-resolved through the add path, since a duplicate should preserve exactly what the shopper had - checkout reprices them anyway
func (s *Service) AdminDuplicateCart(ctx context.Context, sourceCartID uuid.UUID, targetCustomerID *uuid.UUID) (Cart, error) {
	src, err := s.repo.GetCartByID(ctx, sourceCartID)
	if err != nil {
		if IsSQLNoRows(err) {
			return Cart{}, ErrCartNotFound
		}
		return Cart{}, err
	}
	if src.IsOffer() {
		return Cart{}, ErrOfferLocked
	}
	lines, err := s.repo.ListCartLinesByCart(ctx, sourceCartID)
	if err != nil {
		return Cart{}, fmt.Errorf("load source lines: %w", err)
	}
	var destID uuid.UUID
	if err := s.inTx(ctx, func(r Repository) error {
		var dest Cart
		var err error
		if targetCustomerID == nil {
			dest, err = r.CreateCart(ctx, nil, src.ChannelID)
		} else {
			dest, err = GetOrCreateCustomerCart(ctx, r, *targetCustomerID, src.ChannelID)
		}
		if err != nil {
			return err
		}
		if dest.ID == src.ID { // the target customer's active cart IS the source - copying it onto itself would double every plain line
			return ErrDuplicateOntoSource
		}
		destID = dest.ID
		return copyLines(ctx, r, dest.ID, lines)
	}); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, destID)
}

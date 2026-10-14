package cart

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// Save-for-later - cart-scoped and guest-capable, simple lines only in v1, since a bundle or configured snapshot can't round-trip through the simple add path

func (s *Service) ListSavedItems(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID) ([]SavedItem, error) {
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListSavedItems(ctx, cart.ID)
}

func (s *Service) SaveForLater(ctx context.Context, cartID, lineID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	line, err := s.findHydratedLine(ctx, cart.ID, lineID)
	if err != nil {
		return Cart{}, err
	}
	if len(line.Components) > 0 || len(line.Options) > 0 {
		return Cart{}, ErrLineNotSaveable
	}
	if err := s.inTx(ctx, func(r Repository) error {
		if _, err := r.SaveItem(ctx, cart.ID, line.ProductSku, line.ProductName, line.Quantity); err != nil {
			return err
		}
		return r.RemoveCartLine(ctx, lineID)
	}); err != nil {
		return Cart{}, fmt.Errorf("save for later: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// Goes back through AddToCart so it re-prices and hits the stock guard - the saved row is claimed by deleting it first, so a double-click moves it once, and put back if the add refuses
func (s *Service) MoveSavedToCart(ctx context.Context, cartID, savedID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	saved, err := s.repo.GetSavedItem(ctx, savedID)
	if err != nil {
		if IsSQLNoRows(err) {
			return Cart{}, ErrSavedItemNotFound
		}
		return Cart{}, err
	}
	if saved.CartID != cart.ID {
		return Cart{}, ErrSavedItemNotFound
	}
	claimed, err := s.repo.DeleteSavedItem(ctx, savedID, cart.ID)
	if err != nil {
		return Cart{}, err
	}
	if !claimed { // a concurrent move already took it
		return loadCartWithLines(ctx, s.repo, cart.ID)
	}
	if _, err := s.AddToCart(ctx, cart.ID, callerID, saved.ProductSku, saved.Quantity); err != nil {
		if _, rerr := s.repo.SaveItem(ctx, cart.ID, saved.ProductSku, saved.ProductName, saved.Quantity); rerr != nil {
			slog.WarnContext(ctx, "cart: restore saved item after a refused move", "cart", cart.ID, "sku", saved.ProductSku, "err", rerr)
		}
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) RemoveSavedItem(ctx context.Context, cartID, savedID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if _, err := s.repo.DeleteSavedItem(ctx, savedID, cart.ID); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

package cart

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

var ErrOfferOtherAccount = errors.New("this offer was sent to a different account - sign in with the account it was sent to")

func (s *Service) SetOfferLinks(l *OfferLinks) { s.links = l }

// OfferLinks signs the links offers are sent with - nil until wired
func (s *Service) OfferLinks() *OfferLinks { return s.links }

// OfferOpening is what a followed offer link leads to - the cart, where the offer stands, and whether the recipient has to sign in first
type OfferOpening struct {
	CartID        uuid.UUID
	ChannelID     uuid.UUID
	State         OfferState
	RequiresLogin bool // a customer offer opened without its customer signed in
}

// OpenOffer follows an offer link. A link to an offer that has finished still answers, with its state, so the storefront can say so - only a link that was never valid, or was replaced, is refused. The first open by the right person is stamped for the console
func (s *Service) OpenOffer(ctx context.Context, token string, callerID *uuid.UUID) (OfferOpening, error) {
	if s.links == nil {
		return OfferOpening{}, ErrOfferLinkInvalid
	}
	grant, err := s.links.Verify(token)
	if err != nil {
		return OfferOpening{}, err
	}
	cart, err := s.repo.GetCartByID(ctx, grant.CartID)
	if err != nil {
		if IsSQLNoRows(err) {
			return OfferOpening{}, ErrOfferLinkInvalid
		}
		return OfferOpening{}, fmt.Errorf("load offer: %w", err)
	}
	if !cart.IsOffer() || cart.Offer.LinkVersion != grant.LinkVersion {
		return OfferOpening{}, ErrOfferLinkInvalid
	}
	out := OfferOpening{CartID: cart.ID, ChannelID: cart.ChannelID, State: cart.OfferState(time.Now())}
	if cart.CustomerID != nil {
		if callerID == nil {
			out.RequiresLogin = true
			return out, nil
		}
		if *callerID != *cart.CustomerID {
			return OfferOpening{}, ErrOfferOtherAccount
		}
	}
	s.RecordOfferOpened(ctx, &cart)
	out.State = cart.OfferState(time.Now())
	return out, nil
}

// RecordOfferOpened stamps an authorized storefront read, whether reached by link or
// from the account. Call only after checking ownership, offer access and channel.
func (s *Service) RecordOfferOpened(ctx context.Context, c *Cart) {
	if !c.IsOffer() || c.Offer.OpenedAt != nil {
		return
	}
	switch c.OfferState(time.Now()) {
	case OfferStateDraft, OfferStateSent:
		if err := s.repo.MarkOfferOpened(ctx, c.ID); err != nil {
			slog.WarnContext(ctx, "cart: stamp offer opened", "cart", c.ID, "err", err)
			return
		}
		now := time.Now()
		c.Offer.OpenedAt = &now
		if s.bus != nil {
			s.bus.Publish(ctx, &OfferOpened{CartID: c.ID, ChannelID: c.ChannelID, CustomerID: c.CustomerID})
		}
	}
}

// CustomerOffers lists the offers a customer may open from their account - the ones the admin chose to show there, still open
func (s *Service) CustomerOffers(ctx context.Context, customerID, channelID uuid.UUID) ([]Cart, error) {
	carts, err := s.repo.ListCustomerOffers(ctx, customerID, channelID)
	if err != nil {
		return nil, err
	}
	for i := range carts {
		if carts[i].Lines, err = s.repo.ListCartLinesByCart(ctx, carts[i].ID); err != nil {
			return nil, fmt.Errorf("load offer lines: %w", err)
		}
	}
	return carts, nil
}

// AnnounceExpiredOffers publishes OfferExpired once for each offer that ran out unfinished - access is refused at the expiry itself, this only tells subscribers. Returns how many it announced
func (s *Service) AnnounceExpiredOffers(ctx context.Context, limit int32) (int, error) {
	carts, err := s.repo.ListExpiredOffers(ctx, limit)
	if err != nil || len(carts) == 0 {
		return 0, err
	}
	ids := make([]uuid.UUID, len(carts))
	for i, c := range carts {
		ids[i] = c.ID
	}
	if err := s.repo.MarkOffersExpiredNotified(ctx, ids); err != nil { // stamped first - a crash after drops an announcement rather than repeating it
		return 0, fmt.Errorf("mark offers expired: %w", err)
	}
	if s.bus != nil {
		for _, c := range carts {
			s.bus.Publish(ctx, &OfferExpired{CartID: c.ID, ChannelID: c.ChannelID, CustomerID: c.CustomerID, Email: c.Email, ExpiresAt: c.Offer.ExpiresAt})
		}
	}
	return len(carts), nil
}

// PublishOfferSent tells subscribers an admin sent an offer
func (s *Service) PublishOfferSent(ctx context.Context, c Cart, to string) {
	if s.bus != nil && c.IsOffer() {
		s.bus.Publish(ctx, &OfferSent{CartID: c.ID, ChannelID: c.ChannelID, CustomerID: c.CustomerID, Email: to, ExpiresAt: c.Offer.ExpiresAt})
	}
}

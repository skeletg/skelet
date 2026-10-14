package cart

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

// The console's side of offers - like the rest of admin.go these skip the ownership check and the locks (the admin is the one setting them), and the admin resolver enforces channel permission instead

const (
	maxOfferMessageRunes  = 2000
	maxCustomLineName     = 255
	maxCustomLineTaxClass = 100
)

var ErrOfferAccountNeedsCustomer = errors.New("only an offer for a customer can show in their account")

// adminOffer loads an offer the console may still change - an offer that was completed, revoked or merged is history
func (s *Service) adminOffer(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if !cart.IsOffer() {
		return Cart{}, ErrNotOffer
	}
	if cart.Offer.RevokedAt != nil {
		return Cart{}, ErrOfferRevoked
	}
	return cart, nil
}

// AdminCreateOffer starts a new offer - always a fresh cart, never the customer's active one, so a customer can hold any number of offers beside their own cart
func (s *Service) AdminCreateOffer(ctx context.Context, in CreateOfferInput) (Cart, error) {
	if in.CustomerID == nil {
		if strings.TrimSpace(in.Email) == "" {
			return Cart{}, ErrOfferRecipientRequired
		}
	}
	if in.Email != "" {
		email, err := normalizeCartEmail(in.Email)
		if err != nil {
			return Cart{}, err
		}
		in.Email = email
	}
	if !in.ExpiresAt.After(time.Now()) {
		return Cart{}, ErrOfferExpiryInvalid
	}
	return s.repo.CreateOfferCart(ctx, in)
}

// AdminSetOffer saves an offer's settings. ships says whether the cart has anything to deliver - without it, shipping and delivery have nothing to lock
func (s *Service) AdminSetOffer(ctx context.Context, cartID uuid.UUID, in OfferSettings, ships bool) (Cart, error) {
	cart, err := s.adminOffer(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if !in.ExpiresAt.After(time.Now()) {
		return Cart{}, ErrOfferExpiryInvalid
	}
	in.Message = strings.TrimSpace(in.Message)
	if utf8.RuneCountInString(in.Message) > maxOfferMessageRunes {
		return Cart{}, fmt.Errorf("%w: at most %d characters", ErrOfferMessageTooLong, maxOfferMessageRunes)
	}
	if in.ShowInAccount && cart.CustomerID == nil {
		return Cart{}, ErrOfferAccountNeedsCustomer
	}
	if in.Locks, err = NormalizeOfferLocks(in.Locks, ships); err != nil {
		return Cart{}, err
	}
	in.PaymentMethods = dedupe(in.PaymentMethods)
	if err := s.repo.SetOffer(ctx, cart.ID, in); err != nil {
		return Cart{}, fmt.Errorf("save offer: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// AdminSetLinePrice sets a line's price on an offer, which then holds through repricing - nil puts a catalog line back on today's catalog price
func (s *Service) AdminSetLinePrice(ctx context.Context, cartID, lineID uuid.UUID, unitPrice *int64) (Cart, error) {
	cart, err := s.adminOffer(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	line, err := s.findHydratedLine(ctx, cart.ID, lineID)
	if err != nil {
		return Cart{}, err
	}
	if unitPrice != nil {
		if *unitPrice < 0 {
			return Cart{}, ErrOfferLinePriceInvalid
		}
		if _, err := s.repo.SetLinePrice(ctx, line.ID, *unitPrice, true); err != nil {
			return Cart{}, err
		}
		return loadCartWithLines(ctx, s.repo, cart.ID)
	}
	if line.IsCustom || len(line.Components) > 0 { // nothing in the catalog to go back to - a bundle keeps its snapshot like everywhere else
		return Cart{}, fmt.Errorf("%w: this line has no catalog price to go back to", ErrOfferLinePriceInvalid)
	}
	if s.products == nil {
		return Cart{}, errNoCatalog
	}
	p, err := s.resolvePricing(ctx, cart.ChannelID, line.ProductSku, cart.CustomerID)
	if err != nil {
		return Cart{}, fmt.Errorf("resolve pricing for %q: %w", line.ProductSku, err)
	}
	unit := pricing.ApplyTiers(p.UnitPrice, p.Tiers, line.Quantity)
	for _, o := range line.Options {
		unit += o.PriceDelta
	}
	if _, err := s.repo.SetLinePrice(ctx, line.ID, unit, false); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// AdminAddCustomLine adds a free-form charge to an offer - an extra payment, a service, anything with no catalog product. It never ships and reserves nothing, and is taxed by the class the admin gives it
func (s *Service) AdminAddCustomLine(ctx context.Context, cartID uuid.UUID, in CustomLineInput) (Cart, error) {
	cart, err := s.adminOffer(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.TaxClass = strings.TrimSpace(in.TaxClass)
	switch {
	case in.Name == "":
		return Cart{}, ErrCustomLineNameRequired
	case utf8.RuneCountInString(in.Name) > maxCustomLineName:
		return Cart{}, fmt.Errorf("%w: the name is at most %d characters", ErrCustomLineNameRequired, maxCustomLineName)
	case len(in.TaxClass) > maxCustomLineTaxClass:
		return Cart{}, fmt.Errorf("tax class is at most %d characters", maxCustomLineTaxClass)
	case in.UnitPrice < 0:
		return Cart{}, ErrOfferLinePriceInvalid
	}
	if err := validQuantity(in.Quantity); err != nil {
		return Cart{}, err
	}
	if _, err := s.repo.InsertCustomLine(ctx, cart.ID, in); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// AdminOfferForSend loads an offer that can still be sent - not completed, revoked or expired
func (s *Service) AdminOfferForSend(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	cart, err := s.adminOffer(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if cart.OfferState(time.Now()) == OfferStateExpired {
		return Cart{}, ErrOfferExpired
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) MarkOfferSent(ctx context.Context, cartID uuid.UUID) error {
	return s.repo.MarkOfferSent(ctx, cartID)
}

// AdminRevokeOffer withdraws an offer - every link to it stops working, and it can't be revived
func (s *Service) AdminRevokeOffer(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	cart, err := s.adminOffer(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if err := s.repo.RevokeOffer(ctx, cart.ID); err != nil {
		return Cart{}, fmt.Errorf("revoke offer: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// AdminRegenerateOfferLink kills every link handed out so far - the next one is signed for the new version
func (s *Service) AdminRegenerateOfferLink(ctx context.Context, cartID uuid.UUID) (Cart, error) {
	cart, err := s.adminOffer(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if _, err := s.repo.BumpOfferLink(ctx, cart.ID); err != nil {
		return Cart{}, fmt.Errorf("regenerate offer link: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// dedupe keeps the first of each non-blank value, in order
func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

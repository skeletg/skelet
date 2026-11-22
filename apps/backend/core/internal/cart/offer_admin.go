package cart

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"

	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
)

// The console's side of offers - like the rest of admin.go these skip the ownership check and the locks (the admin is the one setting them), and the admin resolver enforces channel permission instead

const (
	maxOfferMessageRunes  = 2000
	maxCustomLineName     = 255
	maxCustomLineTaxClass = 100
	// The most a line's price may be set to - the API's amounts are 32-bit, and nothing sells for more
	MaxLinePriceCents = math.MaxInt32
)

// SetLinePriceInput is an admin's price for a line - a nil UnitPrice puts a catalog line back on its catalog price
type SetLinePriceInput struct {
	UnitPrice *int64
	Reason    string     // why, for whoever reads the order later - required on a shopper's own cart
	By        *uuid.UUID // the admin setting it
	// On a shopper's own cart only: the most that may come off the catalog price, in percent (nil for any price),
	// and when the price stops holding (nil for as long as the cart lives)
	MaxCutPercent *int
	HoldsUntil    *time.Time
}

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
	if err := s.AdminUpdateOffer(ctx, cartID, in, ships, func(*Cart) error { return nil }); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cartID)
}

// AdminUpdateOffer prepares checkout outside the lock and commits its settings and checkout together.
func (s *Service) AdminUpdateOffer(ctx context.Context, cartID uuid.UUID, in OfferSettings, ships bool, apply func(*Cart) error) error {
	authorize := func(c Cart) error {
		if c.Status != "active" {
			return ErrCartNotActive
		}
		if !c.IsOffer() {
			return ErrNotOffer
		}
		if c.Offer.RevokedAt != nil {
			return ErrOfferRevoked
		}
		return normalizeOfferSettings(c, &in, ships)
	}
	_, _, err := s.updateCheckout(ctx, cartID, authorize, false, apply, &in)
	return err
}

func normalizeOfferSettings(c Cart, in *OfferSettings, ships bool) error {
	if !in.ExpiresAt.After(time.Now()) {
		return ErrOfferExpiryInvalid
	}
	in.Message = strings.TrimSpace(in.Message)
	if utf8.RuneCountInString(in.Message) > maxOfferMessageRunes {
		return fmt.Errorf("%w: at most %d characters", ErrOfferMessageTooLong, maxOfferMessageRunes)
	}
	if in.ShowInAccount && c.CustomerID == nil {
		return ErrOfferAccountNeedsCustomer
	}
	locks, err := NormalizeOfferLocks(in.Locks, ships)
	if err != nil {
		return err
	}
	in.Locks = locks
	in.PaymentMethods = dedupe(in.PaymentMethods)
	return nil
}

// AdminSetLinePrice sets a line's price on an offer, which then holds through repricing - nil puts a catalog line back on today's catalog price
// The line keeps who set it, why, and the catalog price it replaced, and its order line does after it
// On a shopper's own cart the price is for the quantity the line holds, needs a reason and stays within the cut the
// store allows - see shopperPrice
func (s *Service) AdminSetLinePrice(ctx context.Context, cartID, lineID uuid.UUID, in SetLinePriceInput) (Cart, error) {
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if cart.IsOffer() && cart.Offer.RevokedAt != nil {
		return Cart{}, ErrOfferRevoked
	}
	line, err := s.findHydratedLine(ctx, cart.ID, lineID)
	if err != nil {
		return Cart{}, err
	}
	if in.UnitPrice != nil {
		reason := strings.TrimSpace(in.Reason)
		if err := validLinePrice(*in.UnitPrice); err != nil {
			return Cart{}, err
		}
		if utf8.RuneCountInString(reason) > MaxPriceReason {
			return Cart{}, fmt.Errorf("%w: the reason is at most %d characters", ErrOfferLinePriceInvalid, MaxPriceReason)
		}
		price := LinePrice{UnitPrice: *in.UnitPrice, Source: PriceAdmin, SetBy: in.By, Reason: reason, ListUnitPrice: s.catalogUnit(ctx, cart, line)}
		if !cart.IsOffer() {
			if price, err = shopperPrice(price, line, in); err != nil {
				return Cart{}, err
			}
		}
		if _, err := s.repo.SetLinePrice(ctx, line.ID, price); err != nil {
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
	if _, err := s.repo.SetLinePrice(ctx, line.ID, LinePrice{UnitPrice: unit, Source: PriceCatalog}); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func validLinePrice(cents int64) error {
	switch {
	case cents < 0:
		return fmt.Errorf("%w: it can't be negative", ErrOfferLinePriceInvalid)
	case cents > MaxLinePriceCents:
		return fmt.Errorf("%w: it's more than a line can cost", ErrOfferLinePriceInvalid)
	}
	return nil
}

// What the catalog asks for a line as it stands, the price a set price replaces - today's figure where it resolves,
// else the last one known: the price the line was added at, or what an earlier set price replaced. Nil for a custom
// line, which has no catalog price
func (s *Service) catalogUnit(ctx context.Context, c Cart, l CartLine) *int64 {
	if l.IsCustom {
		return nil
	}
	known := l.ListUnitPrice
	if !l.PriceHeld() {
		added := l.UnitPrice
		known = &added
	}
	if s.products == nil {
		return known
	}
	if len(l.Components) > 0 {
		if unit, _, ok := s.liveBundleLine(ctx, c, l); ok {
			return &unit
		}
		return known
	}
	p, err := s.resolvePricing(ctx, c.ChannelID, l.ProductSku, c.CustomerID)
	if err != nil {
		return known
	}
	unit := pricing.ApplyTiers(p.UnitPrice, p.Tiers, l.Quantity)
	for _, o := range l.Options {
		unit += o.PriceDelta
	}
	return &unit
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
	}
	if err := validLinePrice(in.UnitPrice); err != nil {
		return Cart{}, err
	}
	if err := validCustomQuantity(in.Quantity); err != nil {
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

func (s *Service) MarkOfferSent(ctx context.Context, cartID uuid.UUID, to string) error {
	return s.offerEventTransaction(ctx, func(r Repository, enqueue func(events.Event) error) error {
		c, err := r.GetCartByID(ctx, cartID)
		if err != nil {
			return err
		}
		if !c.IsOffer() {
			return ErrNotOffer
		}
		if err := r.MarkOfferSent(ctx, cartID); err != nil {
			return err
		}
		return enqueue(&OfferSent{CartID: c.ID, ChannelID: c.ChannelID, CustomerID: c.CustomerID, Email: to, ExpiresAt: c.Offer.ExpiresAt})
	})
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

// AdminRegenerateOfferLink kills every link handed out so far - rotates the opaque token and invalidates grants with a new version
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

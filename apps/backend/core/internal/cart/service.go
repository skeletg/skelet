package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	coredb "github.com/skeletg/skelet/apps/backend/core/internal/db"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Orders live in the order package, which orchestrates both repos in one transaction at placement - the bus is wired late via SetEventBus and events no-op when it's nil, so cart works without one and tests stay cheap
type Service struct {
	repo          Repository
	tx            *txrunner.Manager
	products      ProductResolver
	files         FileAssetResolver
	bus           *events.Bus
	remotePricing RemotePricingSource // overrides the catalog price for B2B or dynamic pricing, consulted through resolvePricing - nil means catalog price
	links         *OfferLinks         // resolves offer links - nil means no offer link opens
	outbox        OutboxEnqueuer
}

const (
	minLineQuantity = 0.0001 // the smallest step numeric(14,4) keeps - anything finer rounds to a zero the quantity check refuses
	maxLineQuantity = 1e9    // well inside numeric(14,4), and a line total at any sane price still fits int64 cents
	maxNoteRunes    = 2000
	maxEmailBytes   = 254
)

var errNoCatalog = errors.New("cart: product resolver is not configured") // nothing to price from, and storing the caller's own price is exactly what the catalog lookup exists to prevent

func NewService(repo Repository, txMgr *txrunner.Manager, products ProductResolver) *Service { // products looks up a kind at add time, so non-SIMPLE products are rejected before they reach placement
	return &Service{repo: repo, tx: txMgr, products: products}
}

func (s *Service) Repo() Repository { return s.repo } // exposed so the order package can thread cart ops through a placement tx

func (s *Service) SetEventBus(b *events.Bus) { s.bus = b } // optional - without it the service emits no events

func (s *Service) SetFileResolver(f FileAssetResolver) { s.files = f } // optional - without it FILE option picks carrying an asset id are rejected, since the cart can't verify the upload

func (s *Service) SetRemotePricing(src RemotePricingSource) { s.remotePricing = src }

// Runs fn on a transaction-bound repo, or on the plain one when no manager is wired (unit tests)
func (s *Service) inTx(ctx context.Context, fn func(r Repository) error) error {
	if s.tx == nil {
		return fn(s.repo)
	}
	return s.tx.InTransaction(ctx, func(tx *sql.Tx) error { return fn(s.repo.WithTx(tx)) })
}

func (s *Service) GetOrCreateCart(ctx context.Context, customerID, channelID uuid.UUID) (Cart, error) {
	return GetOrCreateCustomerCart(ctx, s.repo, customerID, channelID)
}

func GetOrCreateCustomerCart(ctx context.Context, repo Repository, customerID, channelID uuid.UUID) (Cart, error) { // repo-parameterised so the same logic runs in or out of a transaction - the order package calls it holding a tx-bound repo
	cart, err := repo.GetCartByCustomerAndChannel(ctx, customerID, channelID)
	if err != nil {
		if !IsSQLNoRows(err) {
			return Cart{}, fmt.Errorf("get cart: %w", err)
		}
		cart, err = repo.CreateCart(ctx, &customerID, channelID)
		if coredb.IsUniqueViolation(err) { // a second tab created it between our read and insert - the one active cart per store is theirs too
			cart, err = repo.GetCartByCustomerAndChannel(ctx, customerID, channelID)
		}
		if err != nil {
			return Cart{}, fmt.Errorf("create cart: %w", err)
		}
	}
	lines, err := repo.ListCartLinesByCart(ctx, cart.ID)
	if err != nil {
		return Cart{}, fmt.Errorf("load cart lines: %w", err)
	}
	cart.Lines = lines
	return cart, nil
}

func (s *Service) CreateGuestCart(ctx context.Context, channelID uuid.UUID) (Cart, error) { // ownerless - the UUID is the capability, and whoever holds it can operate the cart until it's merged or converted
	cart, err := s.repo.CreateCart(ctx, nil, channelID)
	if err != nil {
		return Cart{}, fmt.Errorf("create guest cart: %w", err)
	}
	return cart, nil
}

func (s *Service) GetCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID) (Cart, error) { // guest carts to any caller, customer carts only to the owner
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	lines, err := s.repo.ListCartLinesByCart(ctx, cart.ID)
	if err != nil {
		return Cart{}, fmt.Errorf("load cart lines: %w", err)
	}
	cart.Lines = lines
	return cart, nil
}

func (s *Service) AuthorizeCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID) (Cart, error) { // GetCart's gate without the lines, for a caller that only needs to know the cart is theirs and where it lives
	return authorizeCart(ctx, s.repo, cartID, callerID)
}

func (s *Service) SetCartEmail(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, email string) error { // a checkout-entry email makes an abandoned cart recoverable before any account or order exists
	email, err := normalizeCartEmail(email)
	if err != nil {
		return err
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return err
	}
	if cart.IsOffer() { // the offer was made out to this address
		if email == cart.Email {
			return nil
		}
		return lockedErr("email")
	}
	return s.repo.SetEmail(ctx, cart.ID, email)
}

func (s *Service) SetCartNote(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, note string) error { // note-to-merchant, carried to the order at checkout - an empty string clears it
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxNoteRunes {
		return fmt.Errorf("%w: at most %d characters", ErrCartNoteTooLong, maxNoteRunes)
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return err
	}
	return s.repo.SetNote(ctx, cart.ID, note)
}

// How many times a step is recomputed when the checkout moves under it before the shopper is told to reload
const checkoutSaveAttempts = 3

// errCheckoutMoved is one attempt losing the race - UpdateCheckout recomputes the step from the new row
var errCheckoutMoved = errors.New("checkout moved")

// UpdateCheckout saves one checkout step. The step edits c.Checkout, and may set c.Email - the details step saves contact and addresses as one write. The resolvers validate the step against shipping, payment and gift cards inside apply - the cart only stores what it's given
// apply runs on the cart as loaded, with its lines, and outside any transaction: its quotes, payment eligibility and address checks may wait on a third party and read through the shared pool, so holding the row lock and a pooled connection across them would let a burst of saves exhaust the pool. The commit is a short transaction that re-checks access, writes only when checkout_version hasn't moved since the load, and otherwise recomputes the step
// On an offer, a step that moves a locked part - or the recipient's email - is refused whole
func (s *Service) UpdateCheckout(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, apply func(c *Cart) error) error {
	return s.updateCheckout(ctx, cartID, func(c Cart) error { return CheckCartAccess(ctx, c, callerID) }, true, apply, nil)
}

// AdminUpdateCheckout is UpdateCheckout for the console - it fills in an offer's checkout up front, so it neither asks who owns the cart nor stops at the locks the admin is setting up
func (s *Service) AdminUpdateCheckout(ctx context.Context, cartID uuid.UUID, apply func(c *Cart) error) error {
	return s.updateCheckout(ctx, cartID, func(c Cart) error {
		if c.Status != "active" {
			return ErrCartNotActive
		}
		return nil
	}, false, apply, nil)
}

func (s *Service) updateCheckout(ctx context.Context, cartID uuid.UUID, authorize func(Cart) error, enforceLocks bool, apply func(c *Cart) error, offer *OfferSettings) error {
	for range checkoutSaveAttempts {
		err := s.updateCheckoutOnce(ctx, cartID, authorize, enforceLocks, apply, offer)
		if !errors.Is(err, errCheckoutMoved) {
			return err
		}
	}
	return ErrCheckoutConflict
}

func (s *Service) updateCheckoutOnce(ctx context.Context, cartID uuid.UUID, authorize func(Cart) error, enforceLocks bool, apply func(c *Cart) error, offer *OfferSettings) error {
	// Prepare - no transaction and no lock
	cart, err := s.repo.GetCartByID(ctx, cartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCartForbidden
		}
		return fmt.Errorf("load cart: %w", err)
	}
	if err := authorize(cart); err != nil {
		return err
	}
	if cart.Lines, err = s.repo.ListCartLinesByCart(ctx, cart.ID); err != nil {
		return fmt.Errorf("load cart lines: %w", err)
	}
	next := cart
	next.Checkout.GiftCardCodes = slices.Clone(cart.Checkout.GiftCardCodes)
	next.Checkout.BusinessIDs = maps.Clone(cart.Checkout.BusinessIDs)
	next.Checkout.Deliveries = maps.Clone(cart.Checkout.Deliveries) // a step writes the map in place, and the lock diff needs the before
	if err := apply(&next); err != nil {
		return err
	}
	emailChanged := strings.TrimSpace(next.Email) != cart.Email
	if emailChanged {
		if next.Email, err = normalizeCartEmail(next.Email); err != nil {
			return err
		}
	}

	// Commit - re-read under the lock and write, nothing slower
	return s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		locked, err := r.LockCart(ctx, cartID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrCartForbidden
			}
			return fmt.Errorf("lock cart: %w", err)
		}
		if err := authorize(locked); err != nil {
			return err
		}
		if locked.CheckoutVersion != cart.CheckoutVersion {
			return errCheckoutMoved
		}
		// Check locks against the locked row even when the prepared checkout version still matches.
		if enforceLocks && locked.IsOffer() {
			if err := checkOfferLocks(locked, next); err != nil {
				return err
			}
			if emailChanged && locked.Email != "" && !strings.EqualFold(next.Email, locked.Email) {
				return lockedErr("email")
			}
		}
		if emailChanged {
			if err := r.SetEmail(ctx, cart.ID, next.Email); err != nil {
				return err
			}
		}
		if offer != nil {
			if err := r.SetOffer(ctx, cart.ID, *offer); err != nil {
				return fmt.Errorf("save offer: %w", err)
			}
		}
		return r.SetCheckout(ctx, cart.ID, next.Checkout)
	})
}

func (s *Service) PruneStale(ctx context.Context, olderThan time.Duration) (int64, error) { // retention reaper - a converted cart's durable record lives on its order, so dropping the stale row loses nothing
	if olderThan <= 0 {
		return 0, nil
	}
	return s.repo.PruneStale(ctx, time.Now().Add(-olderThan))
}

// The price checkout would charge each line of an already-authorized cart, for flagging drift - a simple line's base plus best tier, a bundle's re-summed components. Configured lines are snapshot-locked and omitted, and a pricing miss just leaves the line out
func (s *Service) CurrentLinePrices(ctx context.Context, c Cart) map[uuid.UUID]int64 {
	out := make(map[uuid.UUID]int64, len(c.Lines))
	if s.products == nil {
		return out
	}
	for _, l := range c.Lines {
		if l.PriceOverride || l.IsCustom {
			continue
		}
		if len(l.Components) > 0 {
			if unit, _, ok := s.liveBundleLine(ctx, c, l); ok {
				out[l.ID] = unit
			}
			continue
		}
		if len(l.Options) > 0 {
			continue
		}
		p, err := s.resolvePricing(ctx, c.ChannelID, l.ProductSku, c.CustomerID)
		if err != nil {
			continue
		}
		unit := pricing.ApplyTiers(p.UnitPrice, p.Tiers, l.Quantity)
		out[l.ID] = unit
	}
	return out
}

func (s *Service) GetCartByIDReadOnly(ctx context.Context, cartID uuid.UUID) (Cart, error) { // bypasses the ownership check - trusted server-side callers only (the REST API gates it behind orders.view), never wire it to a storefront resolver
	return loadCartWithLines(ctx, s.repo, cartID)
}

// Sums quantity on a same-SKU conflict where the incoming unit_price wins - the direct-line-kinds admission check lives in the catalog adapter's ResolveSimplePricing, so the cart stays decoupled and placement inherits the invariant
func (s *Service) AddToCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, sku string, quantity float64) (Cart, error) {
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if err := checkProtectedOfferLine(cart, uuid.Nil, sku); err != nil {
		return Cart{}, err
	}
	input, err := s.prepareSimpleAdd(ctx, cart, sku, quantity)
	if err != nil {
		return Cart{}, err
	}
	if s.bus != nil { // a LineAdding hook may reject the add or move price and quantity - intentional for dynamic-pricing plugins running with operator trust
		adding := &LineAdding{
			CartID:      cart.ID,
			ChannelID:   cart.ChannelID,
			CustomerID:  cart.CustomerID,
			ProductSku:  input.ProductSku,
			ProductName: input.ProductName,
			UnitPrice:   &input.UnitPrice,
			Quantity:    &input.Quantity,
		}
		if err := s.bus.PublishSync(ctx, adding); err != nil {
			return Cart{}, fmt.Errorf("add to cart rejected: %w", err)
		}
		input.UnitPrice, input.Quantity = valueOr(adding.UnitPrice, input.UnitPrice), valueOr(adding.Quantity, input.Quantity) // read off the event, since the SDK setters and a webhook response swap the pointer rather than write through it
	}
	line, err := s.repo.UpsertCartLineBySku(ctx, cart.ID, input)
	if err != nil {
		return Cart{}, fmt.Errorf("add to cart: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &LineAdded{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
			LineID:     line.ID,
			ProductSku: input.ProductSku,
			UnitPrice:  input.UnitPrice,
			Quantity:   input.Quantity,
		})
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

// Everything a plain add checks before its upsert, shared by the storefront and the admin console - catalog name and price whatever the caller sent, so a hostile caller can't buy a $25 shirt for a cent
// A REQUIRED-options product must go through the configured add, or an under-priced line reaches checkout - only options visible with no picks count, since one hidden behind an unsatisfied visible_if is how a customer legitimately buys without it
// Bounds and the tier break apply to the PROSPECTIVE total, since the upsert sums into an existing same-SKU line
func (s *Service) prepareSimpleAdd(ctx context.Context, c Cart, sku string, quantity float64) (AddCartLineInput, error) {
	if s.products == nil {
		return AddCartLineInput{}, errNoCatalog
	}
	if err := validQuantity(quantity); err != nil {
		return AddCartLineInput{}, err
	}
	p, err := s.resolvePricing(ctx, c.ChannelID, sku, c.CustomerID)
	if err != nil {
		return AddCartLineInput{}, fmt.Errorf("resolve pricing for %q: %w", sku, err)
	}
	res, oerr := s.products.ResolveProductOptions(ctx, c.ChannelID, sku, c.CustomerID)
	if oerr != nil && !errors.Is(oerr, ErrOptionsUnsupportedKind) {
		return AddCartLineInput{}, fmt.Errorf("resolve product options for %q: %w", sku, oerr)
	}
	if oerr == nil && HasVisiblyRequiredOptions(res) {
		return AddCartLineInput{}, ErrProductOptionsRequired
	}
	lines, err := s.repo.ListCartLinesByCart(ctx, c.ID)
	if err != nil {
		return AddCartLineInput{}, fmt.Errorf("load cart lines: %w", err)
	}
	total := quantity
	for _, l := range lines {
		if l.ProductSku != sku {
			continue
		}
		if len(l.Options) > 0 || len(l.Components) > 0 { // the plain upsert would sum quantity but reset the price with the snapshot rows still attached
			return AddCartLineInput{}, ErrLineConfigured
		}
		total += l.Quantity
		break
	}
	if err := validateQtyBounds(p, total); err != nil {
		return AddCartLineInput{}, err
	}
	// the upsert's incoming-price-wins rule re-prices the whole line, so the tier reads the line's prospective total
	return AddCartLineInput{ProductSku: sku, ProductName: p.ProductName, UnitPrice: pricing.ApplyTiers(p.UnitPrice, p.Tiers, total), Quantity: quantity}, nil
}

// Shared by customer and admin quantity edits - enforces the forced-qty lock (fail-closed, the lock is an invariant not a hint) and the min/max/step bounds, returning the tier-repriced unit with the line's snapshot option deltas back on top (nil = price unchanged)
// Bundle lines keep their snapshot price, since a bundle SKU has no simple pricing to re-resolve
// A custom line has no catalog product to check against, and an admin-priced line keeps its price at any quantity - it still sits inside its product's bounds
func (s *Service) validateQuantityEdit(ctx context.Context, cart Cart, line CartLine, quantity float64) (*int64, error) {
	if line.IsCustom {
		return nil, validCustomQuantity(quantity)
	}
	if s.products == nil || len(line.Components) > 0 {
		return nil, nil
	}
	if len(line.Options) > 0 {
		res, rerr := s.products.ResolveProductOptions(ctx, cart.ChannelID, line.ProductSku, cart.CustomerID)
		if rerr != nil {
			return nil, fmt.Errorf("resolve product options for %q: %w", line.ProductSku, rerr)
		}
		if forced := forcedQtyForSnapshot(res, line.Options); forced != nil && *forced != quantity {
			return nil, fmt.Errorf("%w: quantity is fixed at %v", ErrProductOptionQtyForced, *forced)
		}
	}
	p, perr := s.resolvePricing(ctx, cart.ChannelID, line.ProductSku, cart.CustomerID)
	if perr != nil {
		return nil, nil // pricing hiccup keeps the snapshot price rather than blocking the edit
	}
	if err := validateQtyBounds(p, quantity); err != nil {
		return nil, err
	}
	if line.PriceOverride {
		return nil, nil
	}
	unit := pricing.ApplyTiers(p.UnitPrice, p.Tiers, quantity) // crossing a tier boundary in EITHER direction moves the base price, and the snapshot option surcharges are re-added below so a quantity edit never strips personalization pricing
	for _, o := range line.Options {
		unit += o.PriceDelta
	}
	if unit != line.UnitPrice {
		return &unit, nil
	}
	return nil, nil
}

// A bundle line's components scale with it, since reservation reads their totals as-is
func (s *Service) writeLineQuantity(ctx context.Context, line CartLine, quantity float64, repricedUnit *int64) error {
	var err error
	switch {
	case len(line.Components) > 0:
		err = s.repo.SetBundleLineQuantity(ctx, line.ID, quantity)
	case repricedUnit != nil:
		_, err = s.repo.UpdateCartLineQuantityPrice(ctx, line.ID, quantity, *repricedUnit)
	default:
		_, err = s.repo.UpdateCartLineQuantity(ctx, line.ID, quantity)
	}
	if err != nil {
		return fmt.Errorf("update cart line: %w", err)
	}
	return nil
}

func (s *Service) UpdateCartLine(ctx context.Context, cartID, lineID uuid.UUID, callerID *uuid.UUID, quantity float64) (Cart, error) {
	if err := validQuantity(quantity); err != nil {
		return Cart{}, err
	}
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	line, err := s.findHydratedLine(ctx, cart.ID, lineID)
	if err != nil {
		return Cart{}, err
	}
	if err := checkProtectedOfferLine(cart, lineID, ""); err != nil {
		return Cart{}, err
	}
	repricedUnit, err := s.validateQuantityEdit(ctx, cart, line, quantity)
	if err != nil {
		return Cart{}, err
	}
	if s.bus != nil {
		requested := quantity
		updating := &LineUpdating{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			LineID:     lineID,
			ProductSku: line.ProductSku,
			Quantity:   &quantity,
			StockParts: append(OptionStockParts(line.Options), ComponentStockParts(line)...),
		}
		if err := s.bus.PublishSync(ctx, updating); err != nil {
			return Cart{}, fmt.Errorf("update cart line rejected: %w", err)
		}
		quantity = valueOr(updating.Quantity, requested)
		if quantity != requested { // a hook moved it, so the new quantity passes the same lock, bounds and tier as the requested one
			if repricedUnit, err = s.validateQuantityEdit(ctx, cart, line, quantity); err != nil {
				return Cart{}, err
			}
		}
	}
	if err := s.writeLineQuantity(ctx, line, quantity, repricedUnit); err != nil {
		return Cart{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &LineUpdated{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
			LineID:     lineID,
			Quantity:   quantity,
		})
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) RemoveFromCart(ctx context.Context, cartID, lineID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if err := checkProtectedOfferLine(cart, lineID, ""); err != nil {
		return Cart{}, err
	}
	if err := verifyLineInCart(ctx, s.repo, lineID, cart.ID); err != nil {
		return Cart{}, err
	}
	if s.bus != nil {
		removing := &LineRemoving{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			LineID:     lineID,
		}
		if err := s.bus.PublishSync(ctx, removing); err != nil {
			return Cart{}, fmt.Errorf("remove from cart rejected: %w", err)
		}
	}
	if err := s.repo.RemoveCartLine(ctx, lineID); err != nil {
		return Cart{}, fmt.Errorf("remove from cart: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &LineRemoved{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
			LineID:     lineID,
		})
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) ClearCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID) error {
	cart, err := authorizeLineEdit(ctx, s.repo, cartID, callerID)
	if err != nil {
		return err
	}
	if err := checkProtectedOfferLine(cart, uuid.Nil, ""); err != nil {
		return err
	}
	if err := s.repo.ClearCartLines(ctx, cart.ID); err != nil {
		return fmt.Errorf("clear cart: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &Cleared{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
		})
	}
	return nil
}

func (s *Service) findHydratedLine(ctx context.Context, cartID, lineID uuid.UUID) (CartLine, error) { // WITH option and component rows - the bare GetCartLineByID loads neither, only the cart-wide list does; a line of another cart is simply not found here
	lines, err := s.repo.ListCartLinesByCart(ctx, cartID)
	if err != nil {
		return CartLine{}, fmt.Errorf("load cart lines: %w", err)
	}
	for _, l := range lines {
		if l.ID == lineID {
			return l, nil
		}
	}
	return CartLine{}, ErrCartLineNotFound
}

// Placement's last word on the lines - a product, bundle or component that left the channel can't be placed, and every other line must sit inside its product's min/max/step, since a merged guest cart or two concurrent adds can each sum a line past a bound the add path checked
// Only a definite catalog answer refuses, never a pricing hiccup, and it reads the catalog directly - bounds don't depend on a remote price
func (s *Service) ValidateCheckoutLines(ctx context.Context, c Cart) error {
	if s.products == nil {
		return nil
	}
	for _, l := range c.Lines {
		if l.IsGift || l.IsCustom {
			continue
		}
		if len(l.Components) == 0 {
			p, err := s.products.ResolveSimplePricing(ctx, c.ChannelID, l.ProductSku, c.CustomerID)
			if errors.Is(err, ErrProductNotPublished) || errors.Is(err, ErrProductNotPurchasable) {
				return fmt.Errorf("%s: %w", l.ProductSku, err)
			}
			if err != nil {
				continue
			}
			if err := validateQtyBounds(p, l.Quantity); err != nil {
				return fmt.Errorf("%s: %w", l.ProductSku, err)
			}
			continue
		}
		snap, err := s.products.ResolveBundleBySKU(ctx, c.ChannelID, l.ProductSku)
		if errors.Is(err, ErrProductNotPublished) {
			return err
		}
		if err == nil { // the picks were checked against the bundle as it was at add time - an option or selection removed, or a rule tightened since, would place an order the bundle no longer allows
			if _, err := bundlePicks(snap, picksFromComponents(snap, l.Components), extrasFromLine(l)); err != nil {
				return fmt.Errorf("%w: %s: %s", ErrBundleChanged, l.ProductSku, err.Error()) // text only, not wrapped - the inner rule's code would compete with BUNDLE_CHANGED for the response
			}
		}
		for _, comp := range l.Components {
			if _, err := s.products.ResolveSimplePricing(ctx, c.ChannelID, comp.ProductSku, c.CustomerID); errors.Is(err, ErrProductNotPublished) || errors.Is(err, ErrProductNotPurchasable) {
				return fmt.Errorf("bundle component %s: %w", comp.ProductSku, err)
			}
		}
	}
	return nil
}

// Re-resolves each line's base price from today's catalog and persists any drift - carts snapshot prices at add time, so without this a checkout after a price change would charge the stale snapshot, and the math mirrors UpdateCartLine's reprice: tier break on the base, the line's snapshot option deltas on top
// Bundle lines re-sum today's components (repriceBundleLine) - fail-open per line, so a pricing hiccup or persist failure keeps the snapshot rather than blocking checkout, and the returned cart always matches what was persisted
func (s *Service) RepriceCartLines(ctx context.Context, c Cart) Cart {
	if s.products == nil {
		return c
	}
	for i := range c.Lines {
		l := &c.Lines[i]
		if l.PriceOverride || l.IsCustom { // an admin's offer price is the price
			continue
		}
		if len(l.Components) > 0 {
			s.repriceBundleLine(ctx, c, l)
			continue
		}
		p, err := s.resolvePricing(ctx, c.ChannelID, l.ProductSku, c.CustomerID)
		if err != nil {
			continue
		}
		unit := pricing.ApplyTiers(p.UnitPrice, p.Tiers, l.Quantity)
		for _, o := range l.Options {
			unit += o.PriceDelta
		}
		if unit == l.UnitPrice {
			continue
		}
		if _, err := s.repo.UpdateCartLineQuantityPrice(ctx, l.ID, l.Quantity, unit); err != nil {
			slog.WarnContext(ctx, "cart: reprice line", "cart", c.ID, "sku", l.ProductSku, "err", err)
			continue
		}
		l.UnitPrice = unit
	}
	return c
}

// Rewrites a bundle line from today's components when its price, or a component's price or quantity per bundle, has moved - same fail-open contract as simple lines, and a line that can't be re-resolved is left for ValidateCheckoutLines to refuse
func (s *Service) repriceBundleLine(ctx context.Context, c Cart, l *CartLine) {
	unit, components, ok := s.liveBundleLine(ctx, c, *l)
	if !ok || !bundleLineDrifted(*l, unit, components) {
		return
	}
	var updated CartLine
	if err := s.inTx(ctx, func(r Repository) error {
		var err error
		updated, err = r.UpsertBundleCartLine(ctx, c.ID, AddBundleLineInput{
			ProductName: l.ProductName,
			ProductSku:  l.ProductSku,
			UnitPrice:   unit,
			Quantity:    l.Quantity,
			Components:  components,
			HookPriced:  l.BundleHookPriced,
		})
		return err
	}); err != nil {
		slog.WarnContext(ctx, "cart: reprice bundle line", "cart", c.ID, "sku", l.ProductSku, "err", err)
		return
	}
	l.UnitPrice, l.Components = unit, updated.Components
}

func (s *Service) resolvePricing(ctx context.Context, channelID uuid.UUID, sku string, customerID *uuid.UUID) (SimplePricing, error) { // the single funnel for cart pricing, catalog price then the remote override when wired, so every call site applies it uniformly
	p, err := s.products.ResolveSimplePricing(ctx, channelID, sku, customerID)
	if err != nil {
		return p, err
	}
	if s.remotePricing != nil {
		cents, ok, rerr := s.remotePricing.Price(ctx, channelID, sku, customerID, p.UnitPrice)
		if rerr != nil {
			return SimplePricing{}, rerr
		}
		if ok {
			p.UnitPrice = cents
		}
	}
	return p, nil
}

func (s *Service) ListAbandoned(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]AbandonedCart, error) { // feeds the abandoned-cart scanner - one channel's active non-empty carts last touched between maxAge and threshold, not yet notified
	return s.repo.ListAbandoned(ctx, channelID, threshold, maxAge, limit)
}

func (s *Service) MarkAbandonedNotified(ctx context.Context, ids []uuid.UUID) error { // stamped so each abandonment episode enrolls once
	return s.repo.MarkAbandonedNotified(ctx, ids)
}

// --- helpers ---

// The form customer emails take, so recovery and the guest-order claim match it - SetCartEmail and the details step share it
func normalizeCartEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > maxEmailBytes {
		return "", ErrCartEmailInvalid
	}
	return email, nil
}

// A nil callerID is the guest path, allowed only for guest carts - not-found and not-authorized both return "forbidden" to avoid UUID enumeration
func authorizeCart(ctx context.Context, repo Repository, cartID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := repo.GetCartByID(ctx, cartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Cart{}, ErrCartForbidden
		}
		return Cart{}, fmt.Errorf("load cart: %w", err) // a DB failure is NOT a permission answer - masking it as forbidden told the shopper their own cart was gone
	}
	if err := CheckCartAccess(ctx, cart, callerID); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

// authorizeCart for a change to the lines - an offer with its lines locked refuses it
func authorizeLineEdit(ctx context.Context, repo Repository, cartID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := authorizeCart(ctx, repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if cart.Locked(LockLines) {
		return Cart{}, lockedErr(LockLines)
	}
	if cart.IsOffer() {
		if cart.Lines, err = repo.ListCartLinesByCart(ctx, cart.ID); err != nil {
			return Cart{}, fmt.Errorf("load offer lines: %w", err)
		}
	}
	return cart, nil
}

// CheckCartAccess validates a storefront caller against the current cart row, including offer grants and lifetime.
func CheckCartAccess(ctx context.Context, cart Cart, callerID *uuid.UUID) error {
	if cart.Status != "active" {
		return ErrCartNotActive
	}
	if cart.Kind == KindOffer {
		return checkOfferAccess(ctx, cart, callerID)
	}
	if cart.CustomerID == nil {
		return nil
	}
	if callerID == nil || *callerID != *cart.CustomerID {
		return ErrCartForbidden
	}
	return nil
}

func valueOr[T any](p *T, fallback T) T { // a hook that nils an override field means "keep the input"
	if p == nil {
		return fallback
	}
	return *p
}

func validQuantity(q float64) error { // NaN fails every comparison, so it's refused explicitly rather than slipping past the bounds
	if math.IsNaN(q) || q < minLineQuantity || q > maxLineQuantity {
		return fmt.Errorf("%w, got %v", ErrQuantityInvalid, q)
	}
	return nil
}

func validateQtyBounds(p SimplePricing, qty float64) error {
	if p.MinQty != nil && qty < *p.MinQty {
		return fmt.Errorf("%w: minimum order quantity is %g", ErrQuantityOutOfBounds, *p.MinQty)
	}
	if p.MaxQty != nil && qty > *p.MaxQty {
		return fmt.Errorf("%w: maximum order quantity is %g", ErrQuantityOutOfBounds, *p.MaxQty)
	}
	if p.QtyStep > 0 {
		steps := qty / p.QtyStep
		if math.Abs(steps-math.Round(steps)) > 1e-9 {
			return fmt.Errorf("%w: quantity must be a multiple of %g", ErrQuantityOutOfBounds, p.QtyStep)
		}
	}
	return nil
}

func verifyLineInCart(ctx context.Context, repo Repository, lineID, cartID uuid.UUID) error {
	line, err := repo.GetCartLineByID(ctx, lineID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrCartLineNotFound
		}
		return fmt.Errorf("load cart line: %w", err)
	}
	if line.CartID != cartID {
		return ErrCartForbidden // a line from another cart is the same enumeration risk
	}
	return nil
}

func loadCartWithLines(ctx context.Context, repo Repository, cartID uuid.UUID) (Cart, error) { // the post-mutation response payload
	cart, err := repo.GetCartByID(ctx, cartID)
	if err != nil {
		return Cart{}, fmt.Errorf("load cart: %w", err)
	}
	lines, err := repo.ListCartLinesByCart(ctx, cartID)
	if err != nil {
		return Cart{}, fmt.Errorf("load cart lines: %w", err)
	}
	cart.Lines = lines
	return cart, nil
}

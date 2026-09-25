package cart

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/money"
	"github.com/skeletg/skelet/apps/backend/core/internal/pricing"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
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
	remotePricing RemotePricingSource // overrides the catalog price for B2B or dynamic pricing, consulted through resolvePricing - nil means catalog price (remote_pricing.go)
}

func NewService(repo Repository, txMgr *txrunner.Manager, products ProductResolver) *Service { // products looks up a kind at add time, so non-SIMPLE products are rejected before they reach placement
	return &Service{repo: repo, tx: txMgr, products: products}
}

func (s *Service) Repo() Repository { return s.repo } // exposed so the order package can thread cart ops through a placement tx

func (s *Service) SetEventBus(b *events.Bus) { s.bus = b } // optional - without it the service emits no events

func (s *Service) SetFileResolver(f FileAssetResolver) { s.files = f } // optional - without it FILE option picks carrying an asset id are rejected, since the cart can't verify the upload

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

func (s *Service) SetCartEmail(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, email string) error { // a checkout-entry email makes an abandoned cart recoverable before any account or order exists
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return apperr.New("CART_EMAIL_INVALID", apperr.ClassValidation, "a valid email is required")
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return err
	}
	return s.repo.SetEmail(ctx, cart.ID, email)
}

func (s *Service) SetCartNote(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, note string) error { // note-to-merchant, carried to the order at checkout - an empty string clears it
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return err
	}
	return s.repo.SetNote(ctx, cart.ID, strings.TrimSpace(note))
}

func (s *Service) PruneStale(ctx context.Context, olderThan time.Duration) (int64, error) { // retention reaper - a converted cart's durable record lives on its order, so dropping the stale row loses nothing
	if olderThan <= 0 {
		return 0, nil
	}
	return s.repo.PruneStale(ctx, time.Now().Add(-olderThan))
}

func (s *Service) ListSavedItems(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID) ([]SavedItem, error) {
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListSavedItems(ctx, cart.ID)
}

func (s *Service) SaveForLater(ctx context.Context, cartID, lineID uuid.UUID, callerID *uuid.UUID) (Cart, error) { // bundle and configured lines aren't saveable in v1, since their snapshot can't round-trip through the simple add path
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	line, err := s.findHydratedLine(ctx, cart.ID, lineID)
	if err != nil {
		return Cart{}, apperr.New("CART_LINE_NOT_FOUND", apperr.ClassNotFound, "cart line not found")
	}
	if len(line.Components) > 0 || len(line.Options) > 0 {
		return Cart{}, apperr.New("CART_LINE_NOT_SAVEABLE", apperr.ClassValidation,
			"bundle and configured items can't be saved for later yet")
	}
	if _, err := s.repo.SaveItem(ctx, cart.ID, line.ProductSku, line.ProductName, line.Quantity); err != nil {
		return Cart{}, err
	}
	if err := s.repo.RemoveCartLine(ctx, lineID); err != nil {
		return Cart{}, fmt.Errorf("remove saved line: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) MoveSavedToCart(ctx context.Context, cartID, savedID uuid.UUID, callerID *uuid.UUID) (Cart, error) { // goes back through AddToCart so it re-prices and hits the stock guard
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	saved, err := s.repo.GetSavedItem(ctx, savedID)
	if err != nil || saved.CartID != cart.ID {
		return Cart{}, apperr.New("SAVED_ITEM_NOT_FOUND", apperr.ClassNotFound, "saved item not found")
	}
	if _, err := s.AddToCart(ctx, cart.ID, callerID, AddCartLineInput{
		ProductSku:  saved.ProductSku,
		ProductName: saved.ProductName,
		Quantity:    saved.Quantity,
	}); err != nil {
		return Cart{}, err
	}
	if err := s.repo.DeleteSavedItem(ctx, savedID, cart.ID); err != nil {
		return Cart{}, fmt.Errorf("delete saved item: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) RemoveSavedItem(ctx context.Context, cartID, savedID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if err := s.repo.DeleteSavedItem(ctx, savedID, cart.ID); err != nil {
		return Cart{}, err
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) CurrentSimpleLinePrices(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID) (map[uuid.UUID]int64, error) { // effective price (base plus best tier) for flagging drift - bundle and configured lines are snapshot-locked and omitted
	if s.products == nil {
		return map[uuid.UUID]int64{}, nil
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return nil, err
	}
	lines, err := s.repo.ListCartLinesByCart(ctx, cart.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]int64, len(lines))
	for _, l := range lines {
		if len(l.Components) > 0 || len(l.Options) > 0 {
			continue
		}
		p, err := s.resolvePricing(ctx, cart.ChannelID, l.ProductSku, cart.CustomerID)
		if err != nil {
			continue
		}
		unit := p.UnitPrice
		if best := pricing.BestTier(p.Tiers, l.Quantity, nil, ""); best != nil && *best < unit {
			unit = *best
		}
		out[l.ID] = unit
	}
	return out, nil
}

func (s *Service) GetCartByIDReadOnly(ctx context.Context, cartID uuid.UUID) (Cart, error) { // bypasses the ownership check - trusted server-side callers only (the REST API gates it behind orders.view), never wire it to a storefront resolver
	return loadCartWithLines(ctx, s.repo, cartID)
}

// Sums quantity on a same-SKU conflict where the incoming unit_price wins - the direct-line-kinds admission check lives in the catalog adapter's ResolveSimplePricing, so the cart stays decoupled and placement inherits the invariant
func (s *Service) AddToCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, input AddCartLineInput) (Cart, error) {
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if s.products != nil {
		// Catalog price wins - the mutation still accepts productName and unitPrice for wire compatibility but overwrites both, so a hostile caller can't buy a $25 shirt for a cent; a LineAdding hook may still move it, which is intentional for dynamic-pricing plugins running with operator trust
		p, err := s.resolvePricing(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID)
		if err != nil {
			return Cart{}, fmt.Errorf("resolve pricing for %q: %w", input.ProductSku, err)
		}
		input.ProductName = p.ProductName
		input.UnitPrice = p.UnitPrice
		// Required-options gate - a plain add validates no options and folds no surcharges, so a REQUIRED-options product must go through AddConfiguredToCart or un-fulfillable under-priced lines reach checkout; fail closed on a resolve error, and count only options visible with NO picks since one hidden behind an unsatisfied visible_if is how a customer legitimately buys without it
		res, oerr := s.products.ResolveProductOptions(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID)
		if oerr != nil && !errors.Is(oerr, ErrOptionsUnsupportedKind) {
			return Cart{}, fmt.Errorf("resolve product options for %q: %w", input.ProductSku, oerr)
		}
		if oerr == nil && HasVisiblyRequiredOptions(res) {
			return Cart{}, ErrProductOptionsRequired
		}
		total := input.Quantity // bounds-check the PROSPECTIVE total - the upsert sums into an existing same-SKU line, so adding one more of an at-max product must fail too
		if lines, lerr := s.repo.ListCartLinesByCart(ctx, cart.ID); lerr == nil {
			for _, l := range lines {
				if l.ProductSku == input.ProductSku {
					if len(l.Options) > 0 { // the plain upsert would sum quantity but reset the price to base while leaving option rows attached - reject rather than corrupt the line
						return Cart{}, ErrLineConfigured
					}
					total += l.Quantity
					break
				}
			}
		}
		if err := validateQtyBounds(p, total); err != nil {
			return Cart{}, err
		}
		if best := pricing.BestTier(p.Tiers, total, nil, ""); best != nil && *best < input.UnitPrice { // quantity break on the SUMMED line quantity, tiers pre-filtered to the caller's group and channel by the adapter - the upsert's incoming-price-wins rule then re-prices the whole line
			input.UnitPrice = *best
		}
	}
	unitPrice := input.UnitPrice // the pre-commit hook below may reject the add or mutate these through the pointer fields, and the mutated values flow into the repo upsert
	quantity := input.Quantity
	if s.bus != nil {
		adding := &LineAdding{
			CartID:      cart.ID,
			ChannelID:   cart.ChannelID,
			CustomerID:  cart.CustomerID,
			ProductSku:  input.ProductSku,
			ProductName: input.ProductName,
			UnitPrice:   &unitPrice,
			Quantity:    &quantity,
		}
		if err := s.bus.PublishSync(ctx, adding); err != nil {
			return Cart{}, fmt.Errorf("add to cart rejected: %w", err)
		}
		input.UnitPrice = unitPrice
		input.Quantity = quantity
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

type AddBundleSelectionInput struct { // SelectionIDs is at most one for radio and dropdown, zero-or-more for checkbox and multiselect
	OptionID     uuid.UUID
	SelectionIDs []uuid.UUID
}

type AddBundleToCartInput struct { // the bundle counterpart to AddCartLineInput, one Selections entry per option the customer picked for
	ProductSku string
	Quantity   float64
	Selections []AddBundleSelectionInput
}

func (s *Service) AddBundleToCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, input AddBundleToCartInput) (Cart, error) { // prices the line from the picked components - re-adding the same SKU REPLACES the prior selection set
	if s.products == nil {
		return Cart{}, fmt.Errorf("bundle resolver is not configured")
	}
	if input.Quantity <= 0 {
		return Cart{}, fmt.Errorf("quantity must be positive")
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	snap, err := s.products.ResolveBundleBySKU(ctx, cart.ChannelID, input.ProductSku)
	if errors.Is(err, ErrProductNotPublished) {
		return Cart{}, err
	}
	if err != nil {
		return Cart{}, fmt.Errorf("%w: %w", ErrProductNotBundle, err)
	}

	optByID := make(map[uuid.UUID]BundleOptionSnapshot, len(snap.Options)) // indexed for O(1) lookups while walking the selections
	for _, o := range snap.Options {
		optByID[o.ID] = o
	}
	selByID := func(opt BundleOptionSnapshot, id uuid.UUID) (BundleSelectionSnapshot, bool) {
		for _, sel := range opt.Selections {
			if sel.ID == id {
				return sel, true
			}
		}
		return BundleSelectionSnapshot{}, false
	}

	suppliedByOption := make(map[uuid.UUID][]uuid.UUID, len(input.Selections))
	for _, e := range input.Selections {
		opt, ok := optByID[e.OptionID]
		if !ok {
			return Cart{}, fmt.Errorf("%w: %s", ErrBundleOptionUnknown, e.OptionID)
		}
		if isSingleSelect(opt.InputType) && len(e.SelectionIDs) > 1 {
			return Cart{}, fmt.Errorf("%w: option %s expected at most 1 selection, got %d", ErrBundleSelectionCardinality, opt.ID, len(e.SelectionIDs))
		}
		suppliedByOption[e.OptionID] = append(suppliedByOption[e.OptionID], e.SelectionIDs...)
	}
	for _, opt := range snap.Options { // auto-apply defaults for untouched options - a supplied option always wins, even an explicit empty list, so a customer can deselect an optional default
		if _, supplied := suppliedByOption[opt.ID]; supplied {
			continue
		}
		var defaults []uuid.UUID
		for _, sel := range opt.Selections {
			if !sel.IsDefault {
				continue
			}
			defaults = append(defaults, sel.ID)
			if isSingleSelect(opt.InputType) {
				break
			}
		}
		if len(defaults) > 0 {
			suppliedByOption[opt.ID] = defaults
		}
	}
	for _, opt := range snap.Options { // bounds walk the bundle's own options so missing ones are caught - the effective minimum is min_selections, raised to 1 when required
		count := int32(len(suppliedByOption[opt.ID]))
		effMin := opt.MinSelections
		if opt.IsRequired && effMin < 1 {
			effMin = 1
		}
		if count < effMin {
			if effMin == 1 {
				return Cart{}, fmt.Errorf("%w: option %s", ErrBundleSelectionMissing, opt.ID)
			}
			return Cart{}, fmt.Errorf("%w: option %s needs at least %d selections, got %d", ErrBundleSelectionCardinality, opt.ID, effMin, count)
		}
		if opt.MaxSelections != nil && count > *opt.MaxSelections {
			return Cart{}, fmt.Errorf("%w: option %s allows at most %d selections, got %d", ErrBundleSelectionCardinality, opt.ID, *opt.MaxSelections, count)
		}
	}

	var components []AddCartLineComponentInput // materialized rows, summed into the line's unit_price below
	var unitPrice int64
	sortOrder := int32(0)
	for _, opt := range snap.Options {
		for _, selID := range suppliedByOption[opt.ID] {
			sel, ok := selByID(opt, selID)
			if !ok {
				return Cart{}, fmt.Errorf("%w: selection %s on option %s", ErrBundleSelectionInvalid, selID, opt.ID)
			}
			lineQty := sel.QuantityPerBundle * input.Quantity
			compUnit := sel.UnitPrice // the snapshot's catalog default misses listing overrides, specials and remote pricing, so a channel-priced deployment would charge the wrong bundle total - fail-open back to it below so a pricing hiccup never blocks the add
			sp, perr := s.resolvePricing(ctx, cart.ChannelID, sel.ProductSKU, cart.CustomerID)
			switch {
			case perr == nil:
				compUnit = sp.UnitPrice
			case errors.Is(perr, ErrProductNotPublished), errors.Is(perr, ErrProductNotPurchasable):
				return Cart{}, fmt.Errorf("bundle component %s: %w", sel.ProductSKU, perr)
			}
			components = append(components, AddCartLineComponentInput{
				BundleOptionID:    opt.ID,
				BundleSelectionID: sel.ID,
				ProductID:         sel.ProductID,
				ProductSku:        sel.ProductSKU,
				ProductName:       sel.ProductName,
				UnitPrice:         compUnit,
				Quantity:          lineQty,
				SortOrder:         sortOrder,
			})
			unitPrice += money.LineTotal(compUnit, sel.QuantityPerBundle)
			sortOrder++
		}
	}

	bundleName := snap.ProductName
	if bundleName == "" {
		bundleName = input.ProductSku
	}
	bundleUnitPrice := unitPrice // the pre-commit hook below may reject the bundle or override this summed price
	bundleQty := input.Quantity
	if s.bus != nil {
		adding := &BundleAdding{
			CartID:      cart.ID,
			ChannelID:   cart.ChannelID,
			CustomerID:  cart.CustomerID,
			ProductSku:  input.ProductSku,
			ProductName: bundleName,
			UnitPrice:   &bundleUnitPrice,
			Quantity:    &bundleQty,
		}
		if err := s.bus.PublishSync(ctx, adding); err != nil {
			return Cart{}, fmt.Errorf("add bundle to cart rejected: %w", err)
		}
	}
	bundleLine, err := s.repo.UpsertBundleCartLine(ctx, cart.ID, AddBundleLineInput{
		ProductName: bundleName,
		ProductSku:  input.ProductSku,
		UnitPrice:   bundleUnitPrice,
		Quantity:    bundleQty,
		Components:  components,
	})
	if err != nil {
		return Cart{}, fmt.Errorf("add bundle to cart: %w", err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &BundleAdded{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			AnonID:     auth.AnonID(ctx),
			LineID:     bundleLine.ID,
			ProductSku: input.ProductSku,
			UnitPrice:  bundleUnitPrice,
			Quantity:   bundleQty,
		})
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) AddConfiguredToCart(ctx context.Context, cartID uuid.UUID, callerID *uuid.UUID, input AddConfiguredToCartInput) (Cart, error) { // options and price resolve server-side and the deltas fold into the unit price - re-adding the same SKU REPLACES the config, like AddBundleToCart
	if s.products == nil {
		return Cart{}, fmt.Errorf("product resolver is not configured")
	}
	if input.Quantity <= 0 {
		return Cart{}, fmt.Errorf("quantity must be positive")
	}
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	return s.addConfiguredLine(ctx, cart, input)
}

func (s *Service) AdminAddConfiguredToCart(ctx context.Context, cartID uuid.UUID, input AddConfiguredToCartInput) (Cart, error) { // ownership-bypassing twin of AddConfiguredToCart - phone orders need option picks too, and without this any required-option SKU is un-orderable through the console
	if s.products == nil {
		return Cart{}, fmt.Errorf("product resolver is not configured")
	}
	if input.Quantity <= 0 {
		return Cart{}, fmt.Errorf("quantity must be positive")
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
	fileAssets, err := s.resolveOptionFileAssets(ctx, input.Options)
	if err != nil {
		return Cart{}, err
	}
	options, optionDelta, forcedQty, err := buildConfiguredLineOptions(res, input.Options, fileAssets)
	if err != nil {
		return Cart{}, err
	}
	quantity := input.Quantity // a picked value may force this ("Set of 4" -> 4), and it replaces the request BEFORE tier pricing so breaks apply to what's actually charged
	if forcedQty != nil {
		quantity = *forcedQty
	}
	base := res.UnitPrice // quantity breaks apply to the BASE price, and option surcharges ride on top
	if sp, perr := s.resolvePricing(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID); perr == nil {
		if err := validateQtyBounds(sp, quantity); err != nil { // same ordering rules as the plain add, so the configured path isn't a min/max/step bypass - this upsert REPLACES the line rather than summing, so the requested quantity is the whole prospective total
			return Cart{}, err
		}
		if best := pricing.BestTier(sp.Tiers, quantity, nil, ""); best != nil && *best < base {
			base = *best
		}
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
		}
		if err := s.bus.PublishSync(ctx, adding); err != nil {
			return Cart{}, fmt.Errorf("add to cart rejected: %w", err)
		}
	}
	line, err := s.repo.UpsertConfiguredCartLine(ctx, cart.ID, AddConfiguredLineInput{
		ProductName: name,
		ProductSku:  input.ProductSku,
		UnitPrice:   unitPrice,
		Quantity:    quantity,
		Options:     options,
	})
	if err != nil {
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

func (s *Service) resolveOptionFileAssets(ctx context.Context, inputs []CartLineOptionInput) (map[uuid.UUID]FileAsset, error) { // resolved up front so the pure validation and pricing step has the real filename, size and URL - nil when there are no files
	var out map[uuid.UUID]FileAsset
	for _, in := range inputs {
		if in.FileAssetID == nil {
			continue
		}
		if s.files == nil {
			return nil, fmt.Errorf("%w: file uploads are not configured", ErrProductOptionInvalid)
		}
		asset, err := s.files.ResolveFileAsset(ctx, *in.FileAssetID)
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

func isSingleSelect(inputType string) bool {
	return inputType == "radio" || inputType == "dropdown"
}

func (s *Service) validateQuantityEdit(ctx context.Context, cart Cart, lineID uuid.UUID, quantity float64) (*int64, string, error) { // shared by customer and admin quantity edits - enforces the forced-qty lock (fail-closed on a resolve error, the lock is an invariant not a hint) and the min/max/step bounds, returning the tier-repriced unit with the line's snapshot option deltas back on top (nil = price unchanged)
	if s.products == nil {
		return nil, "", nil
	}
	line, lerr := s.findHydratedLine(ctx, cart.ID, lineID)
	if lerr != nil {
		return nil, "", nil // hydration hiccup - the line was already verified to exist, proceed as a plain quantity update
	}
	if len(line.Options) > 0 {
		res, rerr := s.products.ResolveProductOptions(ctx, cart.ChannelID, line.ProductSku, cart.CustomerID)
		if rerr != nil {
			return nil, "", fmt.Errorf("resolve product options for %q: %w", line.ProductSku, rerr)
		}
		if forced := forcedQtyForSnapshot(res, line.Options); forced != nil && *forced != quantity {
			return nil, "", fmt.Errorf("%w: quantity is fixed at %v", ErrProductOptionQtyForced, *forced)
		}
	}
	p, perr := s.resolvePricing(ctx, cart.ChannelID, line.ProductSku, cart.CustomerID)
	if perr != nil {
		return nil, line.ProductSku, nil // pricing hiccup keeps the snapshot price rather than blocking the edit
	}
	if err := validateQtyBounds(p, quantity); err != nil {
		return nil, line.ProductSku, err
	}
	unit := p.UnitPrice // crossing a tier boundary in EITHER direction moves the base price, and the snapshot option surcharges are re-added below so a quantity edit never strips personalization pricing
	if best := pricing.BestTier(p.Tiers, quantity, nil, ""); best != nil && *best < unit {
		unit = *best
	}
	for _, o := range line.Options {
		unit += o.PriceDelta
	}
	if unit != line.UnitPrice {
		return &unit, line.ProductSku, nil
	}
	return nil, line.ProductSku, nil
}

func (s *Service) UpdateCartLine(ctx context.Context, cartID, lineID uuid.UUID, callerID *uuid.UUID, quantity float64) (Cart, error) {
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return Cart{}, err
	}
	if err := verifyLineInCart(ctx, s.repo, lineID, cart.ID); err != nil {
		return Cart{}, err
	}
	repricedUnit, lineSku, err := s.validateQuantityEdit(ctx, cart, lineID, quantity)
	if err != nil {
		return Cart{}, err
	}
	if s.bus != nil {
		updating := &LineUpdating{
			CartID:     cart.ID,
			ChannelID:  cart.ChannelID,
			CustomerID: cart.CustomerID,
			LineID:     lineID,
			ProductSku: lineSku,
			Quantity:   &quantity,
		}
		if err := s.bus.PublishSync(ctx, updating); err != nil {
			return Cart{}, fmt.Errorf("update cart line rejected: %w", err)
		}
	}
	if repricedUnit != nil {
		if _, err := s.repo.UpdateCartLineQuantityPrice(ctx, lineID, quantity, *repricedUnit); err != nil {
			return Cart{}, fmt.Errorf("update cart line: %w", err)
		}
	} else if _, err := s.repo.UpdateCartLineQuantity(ctx, lineID, quantity); err != nil {
		return Cart{}, fmt.Errorf("update cart line: %w", err)
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
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
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
	cart, err := authorizeCart(ctx, s.repo, cartID, callerID)
	if err != nil {
		return err
	}
	if err := s.repo.ClearCartLines(ctx, cart.ID); err != nil {
		return err
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

// Admin "Order Manager" console - these act on ANY active cart in a permitted channel WITHOUT the ownership check (the admin resolver enforces channel permission instead) and skip the Line* hooks on purpose, since an admin edit shouldn't fire a shopper's stock guards or recovery events; pricing is still resolved server-side, and v1 line editing is simple products only

func (s *Service) adminActiveCart(ctx context.Context, cartID uuid.UUID) (Cart, error) { // must exist and be active - no owner check
	cart, err := s.repo.GetCartByID(ctx, cartID)
	if err != nil {
		return Cart{}, fmt.Errorf("cart not found")
	}
	if cart.Status != "active" {
		return Cart{}, fmt.Errorf("cart is not active")
	}
	return cart, nil
}

func (s *Service) AdminCreateCart(ctx context.Context, channelID uuid.UUID, customerID *uuid.UUID) (Cart, error) { // a fresh guest cart when customerID is nil, else the customer's one active cart, since the unique index forbids a second
	if customerID == nil {
		return s.CreateGuestCart(ctx, channelID)
	}
	return GetOrCreateCustomerCart(ctx, s.repo, *customerID, channelID)
}

func (s *Service) findHydratedLine(ctx context.Context, cartID, lineID uuid.UUID) (CartLine, error) { // WITH option rows - the bare GetCartLineByID doesn't load options, only the cart-wide list does
	lines, err := s.repo.ListCartLinesByCart(ctx, cartID)
	if err != nil {
		return CartLine{}, err
	}
	for _, l := range lines {
		if l.ID == lineID {
			return l, nil
		}
	}
	return CartLine{}, fmt.Errorf("cart line not found")
}

func (s *Service) AdminAddToCart(ctx context.Context, cartID uuid.UUID, input AddCartLineInput) (Cart, error) { // price and name still come from the catalog
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if s.products != nil {
		p, perr := s.resolvePricing(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID)
		if perr != nil {
			return Cart{}, fmt.Errorf("resolve pricing for %q: %w", input.ProductSku, perr)
		}
		input.ProductName = p.ProductName
		input.UnitPrice = p.UnitPrice
		if best := pricing.BestTier(p.Tiers, input.Quantity, nil, ""); best != nil && *best < input.UnitPrice {
			input.UnitPrice = *best
		}
		res, oerr := s.products.ResolveProductOptions(ctx, cart.ChannelID, input.ProductSku, cart.CustomerID) // same gates as the customer path - no plain add for required-options SKUs, no upsert onto a configured line, fail closed on a resolve error, and skip requireds hidden behind an unsatisfied visible_if
		if oerr != nil && !errors.Is(oerr, ErrOptionsUnsupportedKind) {
			return Cart{}, fmt.Errorf("resolve product options for %q: %w", input.ProductSku, oerr)
		}
		if oerr == nil && HasVisiblyRequiredOptions(res) {
			return Cart{}, ErrProductOptionsRequired
		}
		if lines, lerr := s.repo.ListCartLinesByCart(ctx, cart.ID); lerr == nil {
			for _, l := range lines {
				if l.ProductSku == input.ProductSku && len(l.Options) > 0 {
					return Cart{}, ErrLineConfigured
				}
			}
		}
	}
	if _, err := s.repo.UpsertCartLineBySku(ctx, cart.ID, input); err != nil {
		return Cart{}, fmt.Errorf("add cart line: %w", err)
	}
	return loadCartWithLines(ctx, s.repo, cart.ID)
}

func (s *Service) AdminUpdateCartLine(ctx context.Context, cartID, lineID uuid.UUID, quantity float64) (Cart, error) {
	cart, err := s.adminActiveCart(ctx, cartID)
	if err != nil {
		return Cart{}, err
	}
	if err := verifyLineInCart(ctx, s.repo, lineID, cart.ID); err != nil {
		return Cart{}, err
	}
	repricedUnit, _, err := s.validateQuantityEdit(ctx, cart, lineID, quantity) // same guard as the customer path - the admin console must not break the forced-qty invariant or the ordering bounds
	if err != nil {
		return Cart{}, err
	}
	if repricedUnit != nil {
		if _, err := s.repo.UpdateCartLineQuantityPrice(ctx, lineID, quantity, *repricedUnit); err != nil {
			return Cart{}, fmt.Errorf("update cart line: %w", err)
		}
	} else if _, err := s.repo.UpdateCartLineQuantity(ctx, lineID, quantity); err != nil {
		return Cart{}, fmt.Errorf("update cart line: %w", err)
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

func (s *Service) AdminDuplicateCart(ctx context.Context, sourceCartID uuid.UUID, targetCustomerID *uuid.UUID) (Cart, error) { // the clone lands on the SOURCE's channel - guest when targetCustomerID is nil, else the customer's active cart
	src, err := s.repo.GetCartByID(ctx, sourceCartID)
	if err != nil {
		return Cart{}, fmt.Errorf("source cart not found")
	}
	lines, err := s.repo.ListCartLinesByCart(ctx, sourceCartID)
	if err != nil {
		return Cart{}, fmt.Errorf("load source lines: %w", err)
	}
	dest, err := s.AdminCreateCart(ctx, src.ChannelID, targetCustomerID)
	if err != nil {
		return Cart{}, err
	}
	for _, l := range lines {
		switch {
		case len(l.Components) > 0:
			comps := make([]AddCartLineComponentInput, len(l.Components)) // cloned verbatim rather than re-resolved through the add path - a duplicate should preserve exactly what the shopper had
			for i, c := range l.Components {
				comps[i] = AddCartLineComponentInput{
					BundleOptionID:    c.BundleOptionID,
					BundleSelectionID: c.BundleSelectionID,
					ProductID:         c.ProductID,
					ProductSku:        c.ProductSku,
					ProductName:       c.ProductName,
					UnitPrice:         c.UnitPrice,
					Quantity:          c.Quantity,
					SortOrder:         c.SortOrder,
				}
			}
			if _, err := s.repo.UpsertBundleCartLine(ctx, dest.ID, AddBundleLineInput{
				ProductName: l.ProductName,
				ProductSku:  l.ProductSku,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
				Components:  comps,
			}); err != nil {
				return Cart{}, fmt.Errorf("copy bundle line %q: %w", l.ProductSku, err)
			}
		case len(l.Options) > 0:
			if _, err := s.repo.UpsertConfiguredCartLine(ctx, dest.ID, AddConfiguredLineInput{ // option snapshot rows cloned verbatim - UnitPrice already includes their deltas
				ProductName: l.ProductName,
				ProductSku:  l.ProductSku,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
				Options:     l.Options,
			}); err != nil {
				return Cart{}, fmt.Errorf("copy configured line %q: %w", l.ProductSku, err)
			}
		default:
			if _, err := s.repo.UpsertCartLineBySku(ctx, dest.ID, AddCartLineInput{
				ProductSku:  l.ProductSku,
				ProductName: l.ProductName,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
			}); err != nil {
				return Cart{}, fmt.Errorf("copy line %q: %w", l.ProductSku, err)
			}
		}
	}
	return loadCartWithLines(ctx, s.repo, dest.ID)
}

// Sums by SKU with the guest's unit_price winning, and with no active cart the guest cart is adopted outright - idempotent, so a guest cart already non-active just returns the current active cart
func (s *Service) MergeGuestCart(ctx context.Context, customerID, guestCartID uuid.UUID) (Cart, error) {
	var resultID uuid.UUID
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)

		guest, err := r.GetCartByID(ctx, guestCartID)
		if err != nil {
			return fmt.Errorf("guest cart not found")
		}
		if guest.CustomerID != nil {
			return fmt.Errorf("not a guest cart")
		}

		if guest.Status != "active" { // already consumed, so hand back the customer's active cart, creating one if needed
			cc, err := GetOrCreateCustomerCart(ctx, r, customerID, guest.ChannelID)
			if err != nil {
				return err
			}
			resultID = cc.ID
			return nil
		}

		cc, err := r.GetCartByCustomerAndChannel(ctx, customerID, guest.ChannelID) // no active cart on this store takes the adoption path below, repurposing the guest cart - cross-store merges are rejected
		if err != nil {
			if !IsSQLNoRows(err) {
				return err
			}
			adopted, err := r.ReassignCart(ctx, guest.ID, customerID)
			if err != nil {
				return err
			}
			resultID = adopted.ID
			return nil
		}
		if cc.ChannelID != guest.ChannelID {
			return ErrCartStoreMismatch
		}

		if s.bus != nil { // sync pre-commit hook, so a plugin can reject the merge - the bus inside the tx is fine, since subscribers run on the caller's goroutine with no detached fan-out
			if err := s.bus.PublishSync(ctx, &Merging{
				GuestCartID:    guest.ID,
				CustomerCartID: cc.ID,
				CustomerID:     customerID,
				ChannelID:      cc.ChannelID,
			}); err != nil {
				return fmt.Errorf("merge rejected: %w", err)
			}
		}

		if err := r.SetCartMerged(ctx, guest.ID); err != nil { // flipping status FIRST is the serialization point against a concurrent second merge - losing the race skips the upserts and returns the cart as-is
			if errors.Is(err, ErrCartAlreadyConverted) {
				resultID = cc.ID
				return nil
			}
			return err
		}
		guestLines, err := r.ListCartLinesByCart(ctx, guest.ID)
		if err != nil {
			return err
		}
		existing, lerr := r.ListCartLinesByCart(ctx, cc.ID) // collision map: a plain guest upsert onto a configured line would sum quantity and reset unit_price with the option rows still attached (the corruption AddToCart rejects with ErrLineConfigured), and a configured guest upsert would silently replace the customer's picks - the customer's line wins either way
		if lerr != nil {
			return lerr
		}
		existingPlain := make(map[string]bool)
		existingConfigured := make(map[string]bool)
		for _, el := range existing {
			if len(el.Options) > 0 || len(el.Components) > 0 {
				existingConfigured[el.ProductSku] = true
			} else {
				existingPlain[el.ProductSku] = true
			}
		}
		for _, l := range guestLines {
			if len(l.Components) > 0 {
				if existingPlain[l.ProductSku] || existingConfigured[l.ProductSku] {
					continue
				}
				comps := make([]AddCartLineComponentInput, len(l.Components)) // the bundle upsert replaces selections wholesale, overwriting quantity and price
				for i, c := range l.Components {
					comps[i] = AddCartLineComponentInput{
						BundleOptionID:    c.BundleOptionID,
						BundleSelectionID: c.BundleSelectionID,
						ProductID:         c.ProductID,
						ProductSku:        c.ProductSku,
						ProductName:       c.ProductName,
						UnitPrice:         c.UnitPrice,
						Quantity:          c.Quantity,
						SortOrder:         c.SortOrder,
					}
				}
				if _, err := r.UpsertBundleCartLine(ctx, cc.ID, AddBundleLineInput{
					ProductName: l.ProductName,
					ProductSku:  l.ProductSku,
					UnitPrice:   l.UnitPrice,
					Quantity:    l.Quantity,
					Components:  comps,
				}); err != nil {
					return err
				}
				continue
			}
			if len(l.Options) > 0 { // same fidelity rule as AdminDuplicateCart - the plain SKU upsert would drop the option snapshots while keeping their price deltas
				if existingPlain[l.ProductSku] || existingConfigured[l.ProductSku] {
					continue
				}
				if _, err := r.UpsertConfiguredCartLine(ctx, cc.ID, AddConfiguredLineInput{
					ProductName: l.ProductName,
					ProductSku:  l.ProductSku,
					UnitPrice:   l.UnitPrice,
					Quantity:    l.Quantity,
					Options:     l.Options,
				}); err != nil {
					return err
				}
				continue
			}
			if existingConfigured[l.ProductSku] {
				continue
			}
			if _, err := r.UpsertCartLineBySku(ctx, cc.ID, AddCartLineInput{
				ProductName: l.ProductName,
				ProductSku:  l.ProductSku,
				UnitPrice:   l.UnitPrice,
				Quantity:    l.Quantity,
			}); err != nil {
				return err
			}
		}
		if saved, serr := r.ListSavedItems(ctx, guest.ID); serr == nil { // saved-for-later comes along too - the adoption path keeps them via the same cart_id, but merging into an existing cart would drop them when the guest cart is tombstoned; the upsert dedupes by (cart_id, sku)
			for _, si := range saved {
				if _, err := r.SaveItem(ctx, cc.ID, si.ProductSku, si.ProductName, si.Quantity); err != nil {
					return err
				}
			}
		}
		resultID = cc.ID
		return nil
	})
	if err != nil {
		return Cart{}, err
	}
	if s.bus != nil {
		merged, lookupErr := s.repo.GetCartByID(ctx, resultID) // the tx has committed, and resultID is either the adopted guest cart or the customer's existing one
		if lookupErr == nil {
			s.bus.Publish(ctx, &Merged{
				CustomerCartID: resultID,
				GuestCartID:    guestCartID,
				CustomerID:     customerID,
				ChannelID:      merged.ChannelID,
			})
		}
	}
	return loadCartWithLines(ctx, s.repo, resultID)
}

// A bundle line whose bundle or component left the channel can't be placed - only a not-sold answer refuses, never a pricing hiccup
func (s *Service) ValidateBundleLines(ctx context.Context, c Cart) error {
	if s.products == nil {
		return nil
	}
	for _, l := range c.Lines {
		if len(l.Components) == 0 {
			continue
		}
		if _, err := s.products.ResolveBundleBySKU(ctx, c.ChannelID, l.ProductSku); errors.Is(err, ErrProductNotPublished) {
			return err
		}
		for _, comp := range l.Components {
			if _, err := s.resolvePricing(ctx, c.ChannelID, comp.ProductSku, c.CustomerID); errors.Is(err, ErrProductNotPublished) || errors.Is(err, ErrProductNotPurchasable) {
				return fmt.Errorf("bundle component %s: %w", comp.ProductSku, err)
			}
		}
	}
	return nil
}

// Re-resolves each line's base price from today's catalog and persists any drift - carts snapshot prices at add time, so without this a checkout after a price change would charge the stale snapshot, and the math mirrors UpdateCartLine's reprice: tier break on the base, the line's snapshot option deltas on top
// Bundle lines keep their snapshot, since component pricing has no per-SKU re-resolution path - fail-open per line, so a pricing hiccup or persist failure keeps the snapshot rather than blocking checkout, and the returned cart always matches what was persisted
func (s *Service) RepriceCartLines(ctx context.Context, c Cart) Cart {
	if s.products == nil {
		return c
	}
	for i := range c.Lines {
		l := &c.Lines[i]
		if len(l.Components) > 0 {
			continue
		}
		p, err := s.resolvePricing(ctx, c.ChannelID, l.ProductSku, c.CustomerID)
		if err != nil {
			continue
		}
		unit := p.UnitPrice
		if best := pricing.BestTier(p.Tiers, l.Quantity, nil, ""); best != nil && *best < unit {
			unit = *best
		}
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

// --- helpers ---

// A nil callerID is the guest path, allowed only for guest carts - not-found and not-authorized both return "forbidden" to avoid UUID enumeration
func authorizeCart(ctx context.Context, repo Repository, cartID uuid.UUID, callerID *uuid.UUID) (Cart, error) {
	cart, err := repo.GetCartByID(ctx, cartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Cart{}, ErrCartForbidden
		}
		return Cart{}, fmt.Errorf("load cart: %w", err) // a DB failure is NOT a permission answer - masking it as forbidden told the shopper their own cart was gone
	}
	if cart.Status != "active" {
		return Cart{}, ErrCartNotActive
	}
	if cart.CustomerID == nil {
		return cart, nil
	}
	if callerID == nil || *callerID != *cart.CustomerID {
		return Cart{}, ErrCartForbidden
	}
	return cart, nil
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

func (s *Service) ListAbandoned(ctx context.Context, channelID uuid.UUID, threshold, maxAge time.Time, limit int32) ([]AbandonedCart, error) { // feeds the abandoned-cart scanner - one channel's active non-empty carts last touched between maxAge and threshold, not yet notified
	return s.repo.ListAbandoned(ctx, channelID, threshold, maxAge, limit)
}

func (s *Service) MarkAbandonedNotified(ctx context.Context, ids []uuid.UUID) error { // stamped so each abandonment episode enrolls once
	return s.repo.MarkAbandonedNotified(ctx, ids)
}

package order

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/skeletg/skelet/apps/backend/core/internal/ptr"
	"log"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/address"
	"github.com/skeletg/skelet/apps/backend/core/internal/cart"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/internal/promotion"
	shippingpkg "github.com/skeletg/skelet/apps/backend/core/internal/shipping"
	"github.com/skeletg/skelet/apps/backend/core/internal/tax"
	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/phone"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

// Returned when an order op runs without a current store - the storefront resolver populates it, surfaced as STORE_REQUIRED
var ErrStoreRequired = errors.New("store required")

var ErrAddressNotFound = errors.New("address not found in the customer's address book")

var ErrAddressInvalid = errors.New("address is missing or incomplete")

// Missing and not-yours read the same, so a caller can't learn which order ids exist
var ErrOrderNotFound = errors.New("order not found")

var ErrReturnNotFound = errors.New("return not found")

var ErrFulfillmentNotFound = errors.New("fulfillment not found")

func init() {
	apperr.RegisterSentinel(ErrOrderNotFound, "ORDER_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrReturnNotFound, "RETURN_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrFulfillmentNotFound, "FULFILLMENT_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrStoreRequired, "STORE_REQUIRED", apperr.ClassValidation)
	apperr.RegisterSentinel(ErrAddressNotFound, "ADDRESS_NOT_FOUND", apperr.ClassNotFound)
	apperr.RegisterSentinel(ErrAddressInvalid, "ADDRESS_INVALID", apperr.ClassValidation)
}

// PlaceOrder atomically converts a cart into an order, so the service holds both repos - bus fires OrderPlacing (sync, in tx) and OrderPlaced (async, post-commit)
type Service struct {
	repo        Repository
	cartRepo    cart.Repository
	customerSvc *customer.Service
	tx          *txrunner.Manager
	bus         *events.Bus
	// Baseline calculators - each runs only when the matching OrderPlacing pointer is still nil after the sync chain, i.e. no plugin took the seam - Nil means no baseline
	tax        *tax.Service
	shipping   *shippingpkg.Service
	promotion  *promotion.Service
	taxClasses ProductTaxClassLookup
	costs      ProductCostLookup
	// Computes order tax via an external endpoint before core (remote_tax.go)
	remoteTax RemoteTaxSource
	// Screens at the pre-tx gate, nil skips (remote_fraud.go)
	remoteFraud RemoteFraudSource
	// Gates return requests through an external policy (remote_returns.go)
	remoteReturns RemoteReturnsPolicy
	countries     CountryValidator
	payments      PaymentValidator
	// Re-resolves cart line prices at placement so checkout charges today's catalog price, not the add-time snapshot - nil keeps snapshots (cart.Service in prod)
	repricer CartRepricer
	// Makes OrderPlaced durable - enqueued in the placement tx and relayed after commit, so a crash can't lose it - Nil falls back to fire-and-forget Publish
	outbox      OutboxEnqueuer
	giftcard    GiftCardTender
	storeCredit StoreCreditTender
	// Authorizes the amount owed after placement - nil leaves the order unpaid
	paymentAuthorizer PaymentAuthorizer
	// Resolves the channel auto-hold score - nil or 0 disables it
	riskHoldThreshold func(ctx context.Context, channelID uuid.UUID) int
	shippingTax       func(ctx context.Context, channelID uuid.UUID) (class string, taxable bool)
	addresses         AddressChecker
	// channels, pricer and inventoryEditor power pre-fulfillment editing - channel config, added-line pricing, in-tx re-reservation - All optional, and nil disables the capability
	channels        ChannelLoader
	pricer          ProductPricer
	inventoryEditor InventoryEditor
	// Gates promotion free gifts on salable stock - nil grants them unconditionally
	giftStock GiftStockChecker
	kits      KitComponentLookup
}

// A kit SKU's per-unit components, nil for anything that isn't a kit
type KitComponentLookup interface {
	KitComponentsBySKU(ctx context.Context, sku string) ([]cart.CartLineStockPart, error)
}

// Loads a channel for the edit recompute (tax-inclusive, currency) so order doesn't import channel directly
type ChannelLoader interface {
	LoadChannel(ctx context.Context, id uuid.UUID) (channel.Channel, error)
}

// An optional ProductPricer capability - admin line entry rejects SKUs with REQUIRED options, since those paths carry no picks and the order would be un-fulfillable and under-priced - Type-assert the pricer
type OptionAwarePricer interface {
	HasRequiredOptions(ctx context.Context, channelID uuid.UUID, sku string) (bool, error)
}

type ProductPricer interface {
	ResolveSimplePricing(ctx context.Context, channelID uuid.UUID, sku string) (SimplePricing, error)
}

type SimplePricing struct {
	UnitPriceCents int64
	ProductName    string
	// The unit COGS (0 = unknown) for the margin snapshot
	CostCents int64
}

// Re-reserves an order's stock for an edited line set inside the edit tx - releases the current holds, reserves the new set, and a shortage aborts - Here so order doesn't import inventory
type InventoryEditor interface {
	ReReserveForEditTx(ctx context.Context, sqlTx *sql.Tx, channelID, orderID uuid.UUID, c cart.Cart) error
}

// publishDurable routes an event through the outbox when one is wired (its own small tx - these sites run outside any) so a crash or failed subscriber can't lose it, falling back to the fire-and-forget bus otherwise - OrderEdited and payment-status changes carry money-drift signals nothing can reconstruct after the fact
func (s *Service) publishDurable(ctx context.Context, e events.Event) {
	if s.outbox != nil && s.tx != nil {
		err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
			return s.outbox.Enqueue(ctx, sqlTx, e)
		})
		if err == nil {
			return
		}
		log.Printf("[order] outbox enqueue %s failed, falling back to bus: %v", e.Type(), err)
	}
	if s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}

// Writes an event into the outbox using the caller's tx, so order doesn't depend on the outbox package
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, tx *sql.Tx, e events.Event) error
}

// Re-resolves cart line prices from today's catalog at placement (cart.Service in prod) - fail-open by contract, always returns a usable cart
type CartRepricer interface {
	RepriceCartLines(ctx context.Context, c cart.Cart) cart.Cart
}

// Enforces the channel's country and region options on addresses, and its phone rules on their phones - nil skips
// both, leaving a phone read in its address's country alone
type CountryValidator interface {
	PhoneRulesForChannel(ctx context.Context, channelID uuid.UUID) phone.Rules
	ValidateAddressForChannel(ctx context.Context, channelID uuid.UUID, countryCode string, region *string) error
}

// Enforces that a chosen provider is enabled for the channel and shipping country, nil skips - here so order to payment stays a one-way interface dep
type PaymentValidator interface {
	ValidatePaymentForChannel(ctx context.Context, channelID uuid.UUID, providerSlug, countryCode string) error
}

// Resolves SKUs to tax classes for per-line tax - nil means the default class everywhere
type ProductTaxClassLookup interface {
	ProductTaxClassesBySKUs(ctx context.Context, skus []string) (map[string]string, error)
}

// Batch-resolves SKUs to unit COGS for the margin snapshot - nil or absent means a NULL cost
type ProductCostLookup interface {
	ProductCostsBySKUs(ctx context.Context, skus []string) (map[string]int64, error)
}

// Whether a promotion free gift can actually be stocked - a gift is a bonus, so an unstockable one is dropped rather than allowed to fail the reservation and take the paid order with it
// Nil grants gifts unconditionally, which is right when inventory isn't authoritative here anyway
type GiftStockChecker interface {
	CanFulfillSKU(ctx context.Context, channelID uuid.UUID, sku string, qty float64) bool
}

// Deps is what the service cannot work without - the three below the datastore line are interfaces rather than concrete services because subscription imports this package, so the dependency can only run one way
type Deps struct {
	Repo     Repository
	CartRepo cart.Repository
	Customer *customer.Service
	Tx       *txrunner.Manager
	Bus      *events.Bus

	Repricer          CartRepricer
	Outbox            OutboxEnqueuer
	PaymentAuthorizer PaymentAuthorizer
}

// A seam satisfied by a value type - PaymentAuthorizerAdapter is one - has no nil to test, and asking reflect
// for one panics, so the kind is checked before the value
func isNil(v any) bool {
	if v == nil {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func NewService(d Deps) (*Service, error) {
	var missing []string
	for name, dep := range map[string]any{
		"Repo": d.Repo, "CartRepo": d.CartRepo, "Customer": d.Customer, "Tx": d.Tx, "Bus": d.Bus,
		"Repricer": d.Repricer, "Outbox": d.Outbox, "PaymentAuthorizer": d.PaymentAuthorizer,
	} {
		if isNil(dep) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("order.NewService: %s not wired", strings.Join(missing, ", "))
	}
	return &Service{
		repo: d.Repo, cartRepo: d.CartRepo, customerSvc: d.Customer, tx: d.Tx, bus: d.Bus,
		repricer: d.Repricer, outbox: d.Outbox, paymentAuthorizer: d.PaymentAuthorizer,
	}, nil
}

// The Set* seams below are all optional wiring called from main.go
func (s *Service) SetTax(t *tax.Service)                            { s.tax = t }
func (s *Service) SetShipping(sh *shippingpkg.Service)              { s.shipping = sh }
func (s *Service) SetPromotion(p *promotion.Service)                { s.promotion = p }
func (s *Service) SetChannelLoader(c ChannelLoader)                 { s.channels = c }
func (s *Service) SetProductPricer(p ProductPricer)                 { s.pricer = p }
func (s *Service) SetInventoryEditor(e InventoryEditor)             { s.inventoryEditor = e }
func (s *Service) SetGiftStockChecker(c GiftStockChecker)           { s.giftStock = c }
func (s *Service) SetProductTaxClassLookup(l ProductTaxClassLookup) { s.taxClasses = l }
func (s *Service) SetProductCostLookup(l ProductCostLookup)         { s.costs = l }
func (s *Service) SetKitComponentLookup(l KitComponentLookup)       { s.kits = l }
func (s *Service) SetCountryValidator(v CountryValidator)           { s.countries = v }
func (s *Service) SetPaymentValidator(v PaymentValidator)           { s.payments = v }
func (s *Service) SetGiftCardTender(g GiftCardTender)               { s.giftcard = g }
func (s *Service) SetStoreCreditTender(t StoreCreditTender)         { s.storeCredit = t }

type AddressChecker interface {
	Validate(ctx context.Context, channelID uuid.UUID, in address.Input) (address.Result, error)
	Strict(ctx context.Context, channelID uuid.UUID) bool
}

func (s *Service) SetAddressChecker(c AddressChecker) { s.addresses = c }

// nil taxes shipping at the default class
func (s *Service) SetShippingTax(f func(ctx context.Context, channelID uuid.UUID) (class string, taxable bool)) {
	s.shippingTax = f
}

func (s *Service) SetRiskHoldThreshold(f func(ctx context.Context, channelID uuid.UUID) int) {
	s.riskHoldThreshold = f
}

func (s *Service) HoldOrder(ctx context.Context, orderID uuid.UUID, reason string) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if !validTransition(o.Status, OrderStatusOnHold) {
		return Order{}, errInvalidTransition(o.Status, OrderStatusOnHold)
	}
	updated, err := s.repo.HoldOrderFrom(ctx, orderID, o.Status) // stamps where it came from, so releasing doesn't confirm a pending order behind the operator's back
	if err != nil {
		return Order{}, err
	}
	msg := "Order placed on hold"
	if reason != "" {
		msg = "Order placed on hold: " + reason
	}
	if _, err := s.repo.AddOrderEvent(ctx, OrderEvent{OrderID: orderID, Type: "hold", Message: msg}); err != nil {
		log.Printf("[order] record hold event for order %s: %v", orderID, err)
	}
	return updated, nil
}

func (s *Service) ReleaseOrder(ctx context.Context, orderID uuid.UUID) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if o.Status != OrderStatusOnHold {
		return Order{}, errInvalidTransition(o.Status, "released")
	}
	updated, err := s.repo.ReleaseOrderToHeldFrom(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if _, err := s.repo.AddOrderEvent(ctx, OrderEvent{
		OrderID: orderID,
		Type:    "hold_released",
		Message: fmt.Sprintf("Hold released - order returned to %s.", updated.Status),
	}); err != nil {
		log.Printf("[order] record hold-release event for order %s: %v", orderID, err)
	}
	return updated, nil
}

// CustomerID is nil for guest checkouts, and Email is always required - it's the receipt destination and the second half of the guest-order bearer credential
// Addresses come from the customer's saved book by ID or inline, which covers guest one-shots and logged-in customers shipping somewhere one-off without polluting their book
type PlaceOrderInput struct {
	CustomerID *uuid.UUID
	Email      string
	CartID     *uuid.UUID
	// One of *AddressID or *Address per slot - the ID form needs CustomerID, since only their own book is consulted
	BillingAddressID  *uuid.UUID
	BillingAddress    *OrderAddress
	ShippingAddressID *uuid.UUID
	ShippingAddress   *OrderAddress
	// The inline BillingAddress only names the payer - a pickup where no billing address was asked for - so it isn't held to a postal address's required fields or the strict validator
	BillingNameOnly bool
	CustomerNote    string
	// The buyer's PO reference, meaningful with the on-account tender but stored regardless
	PONumber string
	// The company identifiers on a business purchase, with the labels the shopper saw - printed on the invoice
	BusinessIDs []BusinessID
	// The chosen carrier and method - both empty falls back to the cheapest quote, and the price is always re-derived server-side
	ShippingCarrierSlug *string
	ShippingMethodCode  *string
	// The chosen method - optional, where nil skips validation and records none, otherwise it's validated as enabled and allowed for PaymentCountry, then persisted
	PaymentProviderSlug *string
	// The country the payment method must be allowed in - empty means the shipping address's. The checkout steps check the destination's, which for a pickup isn't the store's own address
	PaymentCountry string
	PaymentToken   *string
	// Redeemed as tenders against the total, with the remainder owed to the provider
	GiftCardCodes []string
	// A saved method's token, routed to the provider's vaulted-token key
	VaultedPaymentToken string
	UseStoreCredit      bool
	// A split delivery - one entry per package, each re-priced at its own destination, replacing ShippingCarrierSlug and ShippingMethodCode. ShippingAddress is then the primary package's
	Shipments []ShipmentInput
	// none, physical or mixed - a none order takes no shipping address and pays no shipping, and tax follows the billing address
	FulfillmentNeed string
}

// Redeems a gift card as a tender in the placement tx, where applied is the cents covered - local so order doesn't import giftcard, which imports order
type GiftCardTender interface {
	RedeemTx(ctx context.Context, tx *sql.Tx, code string, channelID uuid.UUID, amountDue int64, currency string, orderID uuid.UUID) (applied int64, err error)
}

// Spends the customer's balance as a tender in the placement tx - local so order doesn't import storecredit
type StoreCreditTender interface {
	RedeemTx(ctx context.Context, tx *sql.Tx, customerID uuid.UUID, amountDue int64, currency string, orderID uuid.UUID) (applied int64, err error)
}

// A request to authorize the amount owed - Token is the client-side gateway token, empty for intent-based providers
type PaymentAuthorizeInput struct {
	OrderID      uuid.UUID
	ChannelID    uuid.UUID
	ProviderSlug string
	AmountCents  int64
	CurrencyCode string
	CustomerID   *uuid.UUID
	Email        string
	Token        string
	VaultedToken string
}

// What the gateway did - RequiresAction means it's parked on a client 3DS/SCA step, which the storefront runs before calling finalizeOrderPayment
// Authorized distinguishes a real hold from the offline no-op, which also returns a zero outcome - only a real hold gets an authorize ledger row
type PaymentAuthorizeOutcome struct {
	// ActionKind is "", "redirect" or "challenge" - the storefront branches on it rather than on
	// the provider slug, so a new redirect gateway needs no client change
	ActionKind        string
	ActionValue       string
	Authorized        bool
	ProviderReference string
}

func (o PaymentAuthorizeOutcome) RequiresAction() bool { return o.ActionKind != "" }

// Authorizes an order's payment - a payment adapter wired in main.go, so order doesn't import payment, and the result reaches the order asynchronously through the settlement bridge
type PaymentAuthorizer interface {
	Authorize(ctx context.Context, in PaymentAuthorizeInput) (PaymentAuthorizeOutcome, error)
}

// Converts a cart into a confirmed order, authenticated or guest, where guests pass inline addresses and an explicit cart id
// The whole create, add-lines, emit and convert-cart sequence is one transaction so a failure leaves no partial state - Plugins hook order.placing (sync, pre-commit, can abort or enrich) and order.placed (async, post-commit), both carrying a nil-able CustomerID
func (s *Service) PlaceOrder(ctx context.Context, in PlaceOrderInput) (Order, error) {
	// resolveCheckout owns all the input validation and resolution
	cc, err := s.resolveCheckout(ctx, in, false)
	if err != nil {
		return Order{}, err
	}
	c, billing, shipping := cc.customer, cc.billing, cc.shipping
	store, paymentProvider := cc.store, cc.paymentProvider
	cartObj, subtotal, preallocatedOrderID := cc.cart, cc.subtotal, cc.orderID

	// Promotion engine - evaluate rules matching the cart's codes, or auto-applies - The Ledger carries the discount, free-shipping flag and gift add-ons, and it runs BEFORE the sync chain so subscribers see post-discount totals
	var (
		evalResult promotion.EvalResult
		ranEngine  bool
	)
	if s.promotion != nil {
		// Segment slugs and per-line catalog facts are resolved inside EvaluateCart, so this and the storefront's cart hydration can't drift into pricing the same cart differently
		cartEval, err := s.promotion.EvaluateCart(ctx, promotion.CartEvalInput{
			CartID:            cartObj.ID,
			SubtotalCents:     subtotal,
			Currency:          store.CurrencyCode,
			CountryCode:       shipping.CountryCode,
			ChannelID:         &store.ID,
			ChannelSlug:       store.Slug,
			CustomerID:        in.CustomerID,
			CustomerGroupSlug: customerGroupSlug(c),
			CustomerEmail:     in.Email,
			Lines:             cart.PromotionLineViews(cartObj),
		})
		if err != nil {
			return Order{}, err
		}
		evalResult = cartEval.Result
		ranEngine = true
		// Append the granted gifts as zero-priced cart lines BEFORE anything downstream reads the cart
		// Doing it here means they flow through the whole placement unchanged: fraud and tax see them, the shipping quote carries their weight, inventory reserves them off OrderPlacing, and persistOrderLines writes them - all without a cart_lines row the shopper could edit
		cartObj = s.appendFreeGiftLines(ctx, cartObj, store.ID, evalResult.Ledger.FreeGifts())
		cc.cart = cartObj
	}
	stamped, err := s.stampStockParts(ctx, cartObj.Lines)
	if err != nil {
		return Order{}, err
	}
	cartObj.Lines = stamped
	cc.cart = cartObj

	// Pre-tx gate for validators that don't write in the placement tx - fraud, address, B2B - A veto aborts before any tx opens, and enrichment seeds the order.placing payload below
	validating := &CheckoutValidating{
		OrderID:         preallocatedOrderID,
		CustomerID:      in.CustomerID,
		Email:           in.Email,
		ChannelID:       store.ID,
		Cart:            cartObj,
		Subtotal:        subtotal,
		PaymentProvider: paymentProvider,
		BillingAddress:  billing,
		ShippingAddress: shipping,
	}
	if ranEngine {
		validating.EngineDiscountCents = evalResult.Ledger.SubtotalDiscountCents()
	}
	if s.bus != nil {
		if err := s.bus.PublishSync(ctx, validating); err != nil {
			return Order{}, fmt.Errorf("checkout rejected: %w", err)
		}
	}

	// Remote fraud screen, authoritative and one per channel - pre-tx so a slow screen never holds the placement tx open - A reject aborts, and the score folds into the risk-hold signal by max
	if s.remoteFraud != nil {
		score, reject, reason, err := s.remoteFraud.Screen(ctx, RemoteFraudInput{
			ChannelID:       store.ID,
			OrderID:         preallocatedOrderID,
			CustomerID:      in.CustomerID,
			Email:           in.Email,
			Currency:        store.CurrencyCode,
			SubtotalCents:   subtotal,
			DiscountCents:   validating.DiscountCents,
			PaymentProvider: paymentProvider,
			Billing:         billing,
			Shipping:        shipping,
			Lines:           remoteTaxInputFromCheckout(cc, validating.DiscountCents, 0).Lines,
		})
		if err != nil {
			return Order{}, fmt.Errorf("checkout rejected: %w", err)
		}
		if reject {
			return Order{}, apperr.New("CHECKOUT_REJECTED", apperr.ClassValidation, "checkout rejected: "+reason)
		}
		if score > validating.RiskScore {
			validating.RiskScore = score
		}
	}

	// In-tx hook where subscribers may reject or enrich - the chain fires inside the placement tx, so DB writers join via placing.Tx and roll back with the order
	placing := &OrderPlacing{
		OrderID:       preallocatedOrderID,
		CustomerID:    in.CustomerID,
		Email:         in.Email,
		ChannelID:     store.ID,
		Cart:          cartObj,
		Subtotal:      subtotal,
		DiscountCents: validating.DiscountCents,
		RiskScore:     validating.RiskScore,
		Tags:          validating.Tags,
	}
	if ranEngine {
		placing.DiscountCents += evalResult.Ledger.SubtotalDiscountCents()
	}
	var orderID uuid.UUID
	var orderNumber string
	var orderTotal int64
	var orderCurrency string
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		// The sync chain joins the placement tx via placing.Tx - webhooks here hold the tx open across their round-trip, so keep them fast
		placing.Tx = sqlTx
		if s.bus != nil {
			if err := s.bus.PublishSync(ctx, placing); err != nil {
				return fmt.Errorf("order rejected: %w", err)
			}
		}

		// No plugin claimed the seam, so: free-shipping promo, else shipping-override promo, else the chosen method re-priced server-side, else cheapest - priced ahead of tax since shipping is taxed too
		var (
			shipSel         shippingSelection
			shipmentAmounts []int64
		)
		if placing.ShippingCents == nil && (len(in.Shipments) > 0 || in.FulfillmentNeed == FulfillmentNone) {
			sel, amounts, err := s.selectShipments(ctx, in, cc, placing.DiscountCents, ranEngine, evalResult)
			if err != nil {
				return err
			}
			cents := sel.Cents
			placing.ShippingCents = &cents
			shipSel, shipmentAmounts = sel, amounts
		}
		if placing.ShippingCents == nil && s.shipping != nil {
			sel, err := s.resolveShippingCents(ctx, in, cc, placing.DiscountCents, ranEngine, evalResult)
			if err != nil {
				return err
			}
			cents := sel.Cents
			placing.ShippingCents = &cents
			shipSel = sel
		}

		var shippingCents int64
		if placing.ShippingCents != nil {
			shippingCents = *placing.ShippingCents
		}

		// The remote tax provider claims the TaxCents seam ahead of core when no plugin took it - fail_open falls through to core, fail_closed aborts
		if placing.TaxCents == nil && s.remoteTax != nil {
			cents, ok, err := s.remoteTax.Tax(ctx, remoteTaxInputFromCheckout(cc, placing.DiscountCents, shippingCents))
			if err != nil {
				return fmt.Errorf("order rejected: %w", err)
			}
			if ok {
				placing.TaxCents = &cents
			}
		}

		// No plugin claimed the seam, so call core PER LINE - product tax classes apply, the discount is distributed proportionally, and rounding happens per line then sums - Tax-inclusive stores split out the tax PORTION rather than adding on top
		var shippingTax int64
		if placing.TaxCents == nil && s.tax != nil {
			t, err := s.computeLineTax(ctx, cc, placing.DiscountCents, shippingCents)
			if err != nil {
				return err
			}
			placing.TaxCents = &t
			shippingTax = cc.shippingTax
		}

		var taxCents int64
		if placing.TaxCents != nil {
			taxCents = *placing.TaxCents
		}
		// Tax-inclusive stores don't add tax on top - taxCents is recorded for the receipt breakdown but is informational
		total := computeOrderTotal(subtotal, placing.DiscountCents, taxCents, shippingCents, store.TaxInclusivePricing, placing.TotalCents)

		orderR := s.repo.WithTx(sqlTx)
		cartR := s.cartRepo.WithTx(sqlTx)

		order, err := orderR.CreateOrder(ctx, Order{
			ID:                  preallocatedOrderID,
			CustomerID:          in.CustomerID,
			Email:               in.Email,
			ChannelID:           store.ID,
			Status:              OrderStatusPending,
			PaymentStatus:       PaymentUnpaid,
			PaymentProvider:     paymentProvider,
			BillingAddress:      billing,
			ShippingAddress:     storedShipping(cc),
			SubtotalAmount:      subtotal,
			DiscountAmount:      placing.DiscountCents,
			TaxAmount:           taxCents,
			ShippingTaxAmount:   shippingTax,
			ShippingAmount:      shippingCents,
			TotalAmount:         total,
			CurrencyCode:        store.CurrencyCode,
			CustomerNote:        in.CustomerNote,
			PONumber:            in.PONumber,
			BusinessIDs:         in.BusinessIDs,
			ShippingMethodID:    shipSel.MethodID,
			ShippingCarrierSlug: ptr.NonZero(shipSel.CarrierSlug),
			ShippingMethodCode:  ptr.NonZero(shipSel.MethodCode),
			AnonID:              auth.AnonID(ctx),
		})
		if err != nil {
			return fmt.Errorf("create order: %w", err)
		}
		if err := s.persistOrderLines(ctx, orderR, order.ID, cartObj.ChannelID, cartObj.Lines); err != nil {
			return err
		}
		if err := s.persistShipments(ctx, orderR, order.ID, in.Shipments, shipmentAmounts); err != nil {
			return err
		}
		if in.FulfillmentNeed != "" {
			if err := orderR.SetFulfillmentNeed(ctx, order.ID, in.FulfillmentNeed); err != nil {
				return err
			}
		}
		// Gift-card tenders redeem codes against the full total, since tax was on it, and the provider takes the remainder - in this tx so a failure rolls the debits back
		giftCardApplied, err := s.applyGiftCardTenders(ctx, sqlTx, orderR, order.ID, store.ID, in, total, store.CurrencyCode)
		if err != nil {
			return err
		}
		// Store credit is an opt-in tender against what gift cards left uncovered, with the same tx semantics
		storeCreditApplied, err := s.applyStoreCreditTender(ctx, sqlTx, orderR, order.ID, in, total, giftCardApplied, store.CurrencyCode)
		if err != nil {
			return err
		}
		// The tender seam - a plugin reserves in this tx against the balance core's own tenders left
		if remaining := total - giftCardApplied - storeCreditApplied; remaining > 0 && s.bus != nil {
			tendering := &OrderTendering{
				Tx:             sqlTx,
				OrderID:        order.ID,
				OrderNumber:    order.Number,
				ChannelID:      store.ID,
				CustomerID:     in.CustomerID,
				Email:          in.Email,
				Provider:       paymentProvider,
				CurrencyCode:   store.CurrencyCode,
				TotalCents:     total,
				RemainingCents: remaining,
			}
			if err := s.bus.PublishSync(ctx, tendering); err != nil {
				return err
			}
		}
		if total-giftCardApplied-storeCreditApplied > 0 && paymentProvider == "" {
			return apperr.New("PAYMENT_REQUIRED", apperr.ClassValidation, "a payment method is required: the order has an unpaid balance")
		}
		if _, err := orderR.AddOrderEvent(ctx, OrderEvent{
			OrderID: order.ID,
			Type:    "order_placed",
			Message: "Order placed by customer",
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		// In the placement tx, so the breakdown can't disagree with tax_amount
		for _, tl := range cc.taxLines {
			if err := orderR.InsertTaxLine(ctx, order.ID, tl); err != nil {
				return err
			}
		}
		// Promotion redemption - bump usage counters, write the audit row and fire the hooks, all in this tx so a failed order rolls the debit back
		if ranEngine && s.promotion != nil {
			if err := s.redeemPromotions(ctx, sqlTx, cc, in, evalResult); err != nil {
				return err
			}
		}
		if cartObj.IsOffer() {
			locked, err := cartR.LockCart(ctx, cartObj.ID)
			if err != nil {
				return fmt.Errorf("lock checkout offer: %w", err)
			}
			if err := cart.CheckCartAccess(ctx, locked, in.CustomerID); err != nil {
				return err
			}
			if locked.Offer.LinkVersion != cartObj.Offer.LinkVersion {
				return cart.ErrOfferLinkInvalid
			}
		}
		// ConvertCart is the serialization point - a concurrent placement that already converted returns ErrCartAlreadyConverted, rolling back everything above
		if err := cartR.ConvertCart(ctx, cartObj.ID); err != nil {
			return err
		}
		if cartObj.IsOffer() { // the console links a completed offer to what it became
			if err := cartR.SetCartOrder(ctx, cartObj.ID, order.ID); err != nil {
				return fmt.Errorf("link offer to order: %w", err)
			}
		}
		// Durable dispatch - OrderPlaced commits with the order and is relayed after commit, so a crash can't lose it
		if s.outbox != nil {
			if err := s.outbox.Enqueue(ctx, sqlTx, &OrderPlaced{
				OrderID:     order.ID,
				OrderNumber: order.Number,
				CustomerID:  in.CustomerID,
				Email:       in.Email,
				AnonID:      auth.AnonID(ctx),
				ChannelID:   store.ID,
				TotalCents:  order.TotalAmount,
				Currency:    order.CurrencyCode,
				Tags:        placing.Tags,
			}); err != nil {
				return fmt.Errorf("enqueue order.placed: %w", err)
			}
		}
		orderID = order.ID
		orderNumber = order.Number
		orderTotal = order.TotalAmount
		orderCurrency = order.CurrencyCode
		return nil
	})
	if err != nil {
		return Order{}, err
	}

	// Fire-and-forget fallback when there's no outbox
	if s.outbox == nil && s.bus != nil {
		s.bus.Publish(ctx, &OrderPlaced{
			OrderID:     orderID,
			OrderNumber: orderNumber,
			CustomerID:  in.CustomerID,
			Email:       in.Email,
			AnonID:      auth.AnonID(ctx),
			ChannelID:   store.ID,
			TotalCents:  orderTotal,
			Currency:    orderCurrency,
			Tags:        placing.Tags,
		})
	}

	placed, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}

	// Risk hold - a score at or above the threshold parks the order on_hold before fulfillment - Authorization still proceeds, so funds are held during review
	if s.riskHoldThreshold != nil && placing.RiskScore > 0 {
		if threshold := s.riskHoldThreshold(ctx, placed.ChannelID); threshold > 0 && placing.RiskScore >= threshold {
			if updated, herr := s.repo.UpdateOrderStatusFrom(ctx, placed.ID, placed.Status, OrderStatusOnHold); herr == nil {
				placed.Status = updated.Status
				if _, evErr := s.repo.AddOrderEvent(ctx, OrderEvent{
					OrderID: placed.ID,
					Type:    "risk_hold",
					Message: fmt.Sprintf("Order held for review: risk score %d >= threshold %d", placing.RiskScore, threshold),
				}); evErr != nil {
					log.Printf("[order] record risk-hold event for order %s: %v", placed.ID, evErr)
				}
			} else {
				log.Printf("[order] risk-hold transition for order %s: %v", placed.ID, herr)
			}
		}
	}

	// Authorize the amount owed, best-effort post-commit since a gateway failure can't roll the order back - so it logs and emits events rather than returning fatally
	// A 3DS step hands back the client secret, carried on the placement response only
	outcome := s.authorizePostPlacement(ctx, in, placed)
	placed.PaymentActionKind = outcome.ActionKind
	placed.PaymentActionSecret = outcome.ActionValue

	return placed, nil
}

// Assembles the total from priced parts, pure and table-tested - tax-inclusive stores don't re-add tax, a non-nil override wins, and the result floors at zero
func computeOrderTotal(subtotal, discount, tax, shipping int64, taxInclusive bool, override *int64) int64 {
	var total int64
	if taxInclusive {
		total = subtotal - discount + shipping
	} else {
		total = subtotal - discount + tax + shipping
	}
	if override != nil {
		total = *override
	}
	if total < 0 {
		total = 0
	}
	return total
}

func (s *Service) GetOrderForCustomer(ctx context.Context, customerID, orderID uuid.UUID) (Order, error) {
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if order.CustomerID == nil || *order.CustomerID != customerID {
		return Order{}, ErrOrderNotFound
	}
	return order, nil
}

// Guest orders only, for unauthenticated confirmation pages where the (id, email) pair is the bearer credential
func (s *Service) GetGuestOrder(ctx context.Context, orderID uuid.UUID, email string) (Order, error) {
	if email == "" {
		return Order{}, apperr.New("EMAIL_REQUIRED", apperr.ClassValidation, "email required")
	}
	return s.repo.GetGuestOrder(ctx, orderID, email)
}

// Attaches a channel's guest orders under an email to a new customer and returns the count - subscribed to customer.email_verified so history shows up once the address is proven
func (s *Service) ClaimGuestOrdersByEmail(ctx context.Context, customerID uuid.UUID, email string, channelID uuid.UUID) (int64, error) {
	if email == "" {
		return 0, nil
	}
	return s.repo.ClaimGuestOrdersByEmail(ctx, customerID, email, channelID)
}

// ListOrdersPage clamps like every other pager - the statuses arrive lowercase, matching the column
// The newest limit orders in the given channels, nil for every channel - no page clamp and no count, for the unpaged admin list
func (s *Service) ListRecentOrders(ctx context.Context, channelIDs []uuid.UUID, limit int32) ([]Order, error) {
	rows, _, err := s.repo.ListOrdersPage(ctx, ListPageFilter{ChannelIDs: channelIDs, Limit: limit, SkipCount: true})
	return rows, err
}

func (s *Service) ListOrdersPage(ctx context.Context, f ListPageFilter) ([]Order, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	f.Search = strings.TrimSpace(f.Search)
	return s.repo.ListOrdersPage(ctx, f)
}

func (s *Service) ListOrdersByCustomer(ctx context.Context, customerID uuid.UUID) ([]Order, error) {
	return s.repo.ListOrdersByCustomer(ctx, customerID)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (Order, error) {
	return s.repo.GetOrderByID(ctx, id)
}

// Only pending and confirmed orders can be cancelled
func (s *Service) CancelOrder(ctx context.Context, customerID, orderID uuid.UUID) (Order, error) {
	order, err := s.GetOrderForCustomer(ctx, customerID, orderID)
	if err != nil {
		return Order{}, err
	}
	if order.Status != OrderStatusPending && order.Status != OrderStatusConfirmed {
		return Order{}, errInvalidTransition(order.Status, OrderStatusCancelled)
	}
	return s.updateStatusWithEvent(ctx, order, OrderStatusCancelled, "order_cancelled", "Order cancelled by customer", "", "")
}

func (s *Service) UpdateStatus(ctx context.Context, orderID uuid.UUID, newStatus string) (Order, error) {
	return s.UpdateStatusWithTracking(ctx, orderID, newStatus, "", "")
}

// UpdateStatus with optional carrier and tracking, only meaningful for "fulfilled" - OrderFulfilling fires seeded with these, and a subscriber's writes override them
func (s *Service) UpdateStatusWithTracking(ctx context.Context, orderID uuid.UUID, newStatus, carrier, trackingNumber string) (Order, error) {
	order, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if !validTransition(order.Status, newStatus) {
		return Order{}, errInvalidTransition(order.Status, newStatus)
	}
	return s.updateStatusWithEvent(
		ctx, order, newStatus, "order_status_changed",
		fmt.Sprintf("Status changed to %s", newStatus),
		carrier, trackingNumber,
	)
}

// --- helpers ---

// prior is the caller's own read - re-reading it here hydrated the whole order a second time for nothing
func (s *Service) updateStatusWithEvent(ctx context.Context, prior Order, status, eventType, message, carrier, trackingNumber string) (Order, error) {
	orderID := prior.ID
	// Sync pre-commit hooks for the gate-able transitions - refund-on-cancel, dispatch-on-fulfill - which can reject or enrich the audit message
	if s.bus != nil {
		switch status {
		case OrderStatusCancelled:
			ing := &OrderCancelling{
				OrderID:    orderID,
				ChannelID:  prior.ChannelID,
				FromStatus: prior.Status,
			}
			if err := s.bus.PublishSync(ctx, ing); err != nil {
				return Order{}, err
			}
			if ing.Note != "" {
				message = message + " " + ing.Note
			}
		case OrderStatusFulfilled:
			// Seed admin tracking as defaults - a subscriber can override, so read back after the chain
			ing := &OrderFulfilling{
				OrderID:        orderID,
				ChannelID:      prior.ChannelID,
				FromStatus:     prior.Status,
				Carrier:        carrier,
				TrackingNumber: trackingNumber,
			}
			if err := s.bus.PublishSync(ctx, ing); err != nil {
				return Order{}, err
			}
			carrier = ing.Carrier
			trackingNumber = ing.TrackingNumber
			if trackingNumber != "" || carrier != "" {
				message = strings.TrimSpace(message + " " + carrier + " " + trackingNumber)
			}
		}
	}
	refundable := 1.0
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		if _, err := r.UpdateOrderStatusFrom(ctx, orderID, prior.Status, status); err != nil {
			return err
		}
		// Read the shipped split with the order row held - read before, a shipment landing on a partially fulfilled order (whose status doesn't change) slipped in and the reversals refunded tenders for goods that had just left
		if status == OrderStatusCancelled {
			refundable = unshippedFraction(ctx, r, prior)
		}
		// Stamp tracking only on the fulfilled transition
		if status == OrderStatusFulfilled && (carrier != "" || trackingNumber != "") {
			if _, err := r.UpdateOrderTracking(ctx, orderID, carrier, trackingNumber); err != nil {
				return err
			}
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{
			OrderID: orderID,
			Type:    eventType,
			Message: message,
		}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		// Durable dispatch, enqueued in the status-update tx so inventory's compensation can't be lost - at-least-once, and the handler is idempotent
		if s.outbox != nil {
			if err := s.outbox.Enqueue(ctx, sqlTx, &OrderStatusChanged{
				OrderID:    orderID,
				ChannelID:  prior.ChannelID,
				FromStatus: prior.Status,
				ToStatus:   status,
			}); err != nil {
				return fmt.Errorf("enqueue order.status_changed: %w", err)
			}
			// Cancellation reverses every tender, so it has to commit with the status change - published post-commit it was one
			// killed pod away from leaving the customer's money spent with nothing to retry
			if status == OrderStatusCancelled {
				if err := s.outbox.Enqueue(ctx, sqlTx, &OrderCancelled{
					OrderID:            orderID,
					ChannelID:          prior.ChannelID,
					FromStatus:         prior.Status,
					RefundableFraction: refundable,
				}); err != nil {
					return fmt.Errorf("enqueue order.cancelled: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return Order{}, err
	}
	if s.bus != nil {
		// Fire-and-forget fallback without the outbox - with one, publishing here would double-deliver
		if s.outbox == nil {
			s.bus.Publish(ctx, &OrderStatusChanged{
				OrderID:    orderID,
				ChannelID:  prior.ChannelID,
				FromStatus: prior.Status,
				ToStatus:   status,
			})
		}
		// Specific transition events, so subscribers needn't filter on toStatus
		switch status {
		case OrderStatusCancelled:
			if s.outbox == nil {
				s.bus.Publish(ctx, &OrderCancelled{
					OrderID:            orderID,
					ChannelID:          prior.ChannelID,
					FromStatus:         prior.Status,
					RefundableFraction: refundable,
				})
			}
		case OrderStatusFulfilled:
			s.bus.Publish(ctx, &OrderFulfilled{
				OrderID:    orderID,
				ChannelID:  prior.ChannelID,
				FromStatus: prior.Status,
			})
		}
	}
	return s.repo.GetOrderByID(ctx, orderID)
}

func findAddress(c customer.Customer, id uuid.UUID) (customer.Address, error) {
	for _, a := range c.Addresses {
		if a.ID == id {
			return a, nil
		}
	}
	return customer.Address{}, ErrAddressNotFound
}

// storedShipping is the address the order keeps - none for a digital-only order, whose shipping only mirrored billing for pricing
func storedShipping(cc *checkoutContext) OrderAddress {
	if cc.noShipping {
		return OrderAddress{}
	}
	return cc.shipping
}

// Picks an OrderAddress for one slot from an inline payload (guests, one-off addresses) or an id in the customer's book (logged-in only) - inline wins when both are set, so a caller can override a stale book pick
// A quote prices a checkout still being filled in, so it accepts a destination-only address - country, region and postcode are all shipping and tax read
func resolveAddress(rules phone.Rules, inline *OrderAddress, id *uuid.UUID, hasCustomer bool, c customer.Customer, slot string, quote bool) (OrderAddress, error) {
	if inline != nil {
		if quote && inline.CountryCode != "" {
			return *inline, nil
		}
		if inline.Street1 == "" || inline.City == "" || inline.PostalCode == "" || inline.CountryCode == "" {
			return OrderAddress{}, fmt.Errorf("%w: %s address is missing required fields", ErrAddressInvalid, slot)
		}
		return normalizeOrderAddress(rules, *inline, slot)
	}
	if id != nil {
		if !hasCustomer {
			return OrderAddress{}, fmt.Errorf("%w: %s address by id requires a signed-in customer", ErrAddressInvalid, slot)
		}
		addr, err := findAddress(c, *id)
		if err != nil {
			return OrderAddress{}, fmt.Errorf("%s address: %w", slot, err)
		}
		return normalizeOrderAddress(rules, toOrderAddress(addr), slot)
	}
	return OrderAddress{}, fmt.Errorf("%w: %s address is required", ErrAddressInvalid, slot)
}

func customerGroupSlug(c customer.Customer) string {
	if c.Group == nil {
		return ""
	}
	return c.Group.Slug
}

// The lowest-priced option across an aggregated quote, plus whether any existed - ties keep the first
// cheapestRatedQuote picks among methods that deliver to the shopper's address - the fallback has no point or pickup to send the order to, so a $0 store "collect" must never win it
func cheapestRatedQuote(quotes []shippingpkg.RatedQuote) (shippingpkg.RatedQuote, bool) {
	var best shippingpkg.RatedQuote
	ok := false
	for _, q := range quotes {
		if q.Method.Kind != "" && q.Method.Kind != shippingpkg.DefaultKind {
			continue
		}
		if !ok || q.Method.PriceCents < best.Method.PriceCents {
			best, ok = q, true
		}
	}
	return best, ok
}

func toOrderAddress(a customer.Address) OrderAddress {
	return OrderAddress{
		FirstName:   a.FirstName,
		LastName:    a.LastName,
		Company:     a.Company,
		Street1:     a.Street1,
		Street2:     a.Street2,
		City:        a.City,
		State:       a.State,
		PostalCode:  a.PostalCode,
		CountryCode: a.CountryCode,
		Phone:       a.Phone,
	}
}

const (
	// An admin order not yet placed - priced and editable, holds no stock, fires no events - Becomes Pending via PlaceDraftOrder, and never arrives from checkout
	OrderStatusDraft              = "draft"
	OrderStatusPending            = "pending"
	OrderStatusConfirmed          = "confirmed"
	OrderStatusProcessing         = "processing"
	OrderStatusPartiallyFulfilled = "partially_fulfilled"
	OrderStatusFulfilled          = "fulfilled"
	OrderStatusCancelled          = "cancelled"
	// Parks a pending or confirmed order before fulfillment, either automatically when the risk score crosses the threshold or manually - authorization proceeds, and the hold gates fulfillment only
	OrderStatusOnHold = "on_hold"
)

// The fulfillment state machine, a package var so it isn't rebuilt per call
var orderStatusTransitions = map[string][]string{
	OrderStatusDraft:              {OrderStatusPending, OrderStatusCancelled},
	OrderStatusPending:            {OrderStatusConfirmed, OrderStatusOnHold, OrderStatusCancelled},
	OrderStatusConfirmed:          {OrderStatusProcessing, OrderStatusOnHold, OrderStatusCancelled},
	OrderStatusOnHold:             {OrderStatusConfirmed, OrderStatusCancelled},
	OrderStatusProcessing:         {OrderStatusPartiallyFulfilled, OrderStatusFulfilled, OrderStatusCancelled},
	OrderStatusPartiallyFulfilled: {OrderStatusPartiallyFulfilled, OrderStatusFulfilled, OrderStatusCancelled},
	OrderStatusFulfilled:          {},
	OrderStatusCancelled:          {},
}

func IsStatus(s string) bool {
	_, ok := orderStatusTransitions[s]
	return ok
}

func validTransition(from, to string) bool {
	return slices.Contains(orderStatusTransitions[from], to)
}

// The status-guarded UPDATEs match no row when someone else moved the order between the caller's read and the write - a conflict the caller can retry against the new status, not a server fault
func errInvalidTransition(from, to string) error {
	return apperr.New("ORDER_INVALID_TRANSITION", apperr.ClassConflict,
		fmt.Sprintf("an order in status %q cannot be %s", from, to))
}

func errStatusRaced(from, to string) error {
	return apperr.New("ORDER_STATUS_RACED", apperr.ClassConflict,
		fmt.Sprintf("the order left %q before it could move to %q - reload it and try again", from, to))
}

// Resolve once per operation, before taking any order lock.
func (s *Service) phoneRules(ctx context.Context, channelID uuid.UUID) phone.Rules {
	if s.countries != nil {
		return s.countries.PhoneRulesForChannel(ctx, channelID)
	}
	return phone.Rules{}
}

func normalizeOrderAddress(rules phone.Rules, a OrderAddress, slot string) (OrderAddress, error) {
	var err error
	if a.Phone, err = rules.NormalizePtr(a.Phone, a.CountryCode); err != nil {
		return OrderAddress{}, fmt.Errorf("%s address: %w", slot, err)
	}
	return a, nil
}
